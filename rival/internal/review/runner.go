package review

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/executor"
	"github.com/1F47E/rival/internal/queue"
	"github.com/1F47E/rival/internal/session"
	"github.com/rs/zerolog/log"
)

// SkippedCLI records a reviewer that was unavailable/failed during megareview.
// Model is the concrete model (set for runtime failures) so opencode's several
// models are distinguishable in the skipped list rather than repeated "opencode".
type SkippedCLI struct {
	CLI    string
	Model  string
	Reason string
}

// Label returns the display label for a skipped reviewer, distinguishing
// opencode models by their short model name.
func (s SkippedCLI) Label() string {
	return config.EngineLabel(s.CLI, s.Model)
}

// RunResult holds the outcome of the full mega review pipeline.
type RunResult struct {
	Output    *ConsiliumOutput
	Inputs    []ReviewInput
	Threshold int
	JudgeCLI  string
	Skipped   []SkippedCLI
}

// cliResult holds the outcome of a single CLI reviewer.
type cliResult struct {
	CLI       string
	Model     string
	Role      Role
	RawOutput string
	ExitCode  int
	Err       error
}

// reviewerPlan pairs a pre-created session with the CLI it will run. model and
// role are carried explicitly so that a single cli ("opencode") can back several
// reviewers, one per opencode model — the cli string stays "opencode" (one
// dispatch case, one display branch) while model/role distinguish them.
type reviewerPlan struct {
	cli   string
	model string
	role  Role
	sess  *session.Session
}

// RunMegaReview runs the full pipeline: enqueue → spawn reviewers → parse → consilium → filter.
// One queue ticket covers the whole pipeline (both reviewers + consilium run
// under the single held slot). Pass noQueue to bypass the queue.
func RunMegaReview(ctx context.Context, scope, effort, workdir, groupID string, noQueue bool) (*RunResult, error) {
	threshold := DefaultConfidenceThreshold

	// Preflight — megareview uses Codex + Opencode (GLM + DeepSeek). Antigravity
	// was dropped from the roster. Runs BEFORE enqueue so a doomed review never
	// occupies a slot.
	codexOK := true
	opencodeOK := true
	var skipped []SkippedCLI
	if err := executor.CodexPreflight(); err != nil {
		log.Warn().Err(err).Msg("codex unavailable")
		codexOK = false
		skipped = append(skipped, SkippedCLI{CLI: "codex", Reason: err.Error()})
	}
	if err := executor.OpencodePreflight(); err != nil {
		log.Warn().Err(err).Msg("opencode unavailable")
		opencodeOK = false
		skipped = append(skipped, SkippedCLI{CLI: "opencode", Reason: err.Error()})
	}
	if !codexOK && !opencodeOK {
		return nil, fmt.Errorf("no CLI reviewers available")
	}

	// Determine which CLI to use for the consilium judge. Prefer codex, then
	// opencode — whichever is available (at least one is, per the guard above).
	var judgeCLI string
	if codexOK {
		judgeCLI = "codex"
	} else {
		judgeCLI = "opencode"
	}

	// Fail any created session still left "queued" (never started) when we
	// return — covers bail-before-run, the creation error paths below, and the
	// error paths further down. Registered BEFORE session creation and closing
	// over `sessions` (appended as each is made), so a failure part-way through
	// creation still cleans up the sessions already created rather than orphaning
	// them. MarkRunning flips a session to "running" only once it is actually
	// about to execute, so a session stuck "queued" here genuinely never ran.
	var sessions []*session.Session
	defer func() {
		for _, s := range sessions {
			if s.Status == "queued" {
				_ = s.Fail(1, "interrupted")
			}
		}
	}()

	// Create reviewer sessions up front (status "queued") so they appear in
	// the TUI/web while waiting, and so the queue ticket can reference them.
	var plans []reviewerPlan
	addReviewer := func(cli, model string, role Role) error {
		sess, err := newReviewerSession(cli, model, role, scope, effort, workdir, groupID)
		if err != nil {
			return err
		}
		plans = append(plans, reviewerPlan{cli: cli, model: sess.Model, role: role, sess: sess})
		sessions = append(sessions, sess)
		return nil
	}
	if codexOK {
		if err := addReviewer("codex", "", ""); err != nil {
			return nil, err
		}
	}
	// opencode: one reviewer per model in the roster (all share the single
	// opencode CLI + credential), each with its own model + role.
	if opencodeOK {
		for _, r := range config.OpencodeReviewerList() {
			if err := addReviewer("opencode", r.Model, Role(r.Role)); err != nil {
				return nil, err
			}
		}
	}

	// Pre-create the consilium judge session too, so the queue ticket covers
	// the WHOLE pipeline. If rival is SIGKILL'd during consilium and the judge
	// child survives, the ticket's liveness check sees that running session and
	// keeps the slot held instead of letting another process double-promote.
	consiliumSess, err := newConsiliumSession(judgeCLI, scope, effort, workdir, groupID)
	if err != nil {
		return nil, err
	}
	sessions = append(sessions, consiliumSess)

	// Acquire one queue slot covering the whole pipeline. Only the reviewer
	// sessions are marked running on promotion; the consilium session stays
	// queued until its phase, but is already in the ticket for liveness.
	reviewerSessions := sessions[:len(plans)]
	release, err := waitForGroupSlot(ctx, noQueue, sessions, reviewerSessions, workdir, groupID, "megareview")
	if err != nil {
		return nil, err
	}
	defer release()

	// Bound the whole pipeline once a slot is held: a hung reviewer or judge must
	// not keep the slot (and the detached rival) alive forever. 2× the per-run
	// budget covers the two sequential phases (concurrent reviewers, then judge).
	ctx, cancelRun := config.WithRunTimeout(ctx, 2)
	defer cancelRun()

	// Phase 1: Spawn reviewers in parallel with role-specific prompts.
	var wg sync.WaitGroup
	results := make(chan cliResult, len(plans))

	for _, p := range plans {
		wg.Add(1)
		go func(pl reviewerPlan) {
			defer wg.Done()
			results <- runReviewer(ctx, pl.sess, pl.cli, pl.model, pl.role, scope, effort, workdir)
		}(p)
	}

	wg.Wait()
	close(results)

	// Collect and parse reviewer outputs.
	var inputs []ReviewInput
	for r := range results {
		if r.Err != nil {
			log.Error().Str("cli", r.CLI).Str("model", r.Model).Err(r.Err).Msg("reviewer failed")
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Model: r.Model, Reason: r.Err.Error()})
			continue
		}
		if r.ExitCode != 0 {
			log.Error().Str("cli", r.CLI).Str("model", r.Model).Int("exit_code", r.ExitCode).Msg("reviewer exited with error")
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Model: r.Model, Reason: fmt.Sprintf("exited with code %d", r.ExitCode)})
			continue
		}
		// agy exits 0 on a 429; detect quota exhaustion from the captured log so
		// a quota-blocked reviewer is reported as skipped, not counted as a
		// successful (but empty) review that silently degrades the consilium.
		if executor.IsQuotaExhausted(r.RawOutput) {
			log.Error().Str("cli", r.CLI).Str("model", r.Model).Msg("reviewer hit provider quota/rate limit (429) — skipping")
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Model: r.Model, Reason: "quota/rate limit reached (429) — not authenticated to a quota-bearing account or quota exhausted"})
			continue
		}
		// agy can exit 0 having produced nothing at all (empty stdout + empty log,
		// no 429 envelope) — e.g. a silent auth/session failure. An empty review is
		// not a successful one: count it as skipped so the consilium isn't fed a
		// no-op input and the TUI shows why instead of a blank "(empty log)".
		if strings.TrimSpace(r.RawOutput) == "" {
			log.Error().Str("cli", r.CLI).Str("model", r.Model).Msg("reviewer produced empty output — skipping")
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Model: r.Model, Reason: "produced no output (empty result) — the provider CLI exited without writing a review; likely an auth/session failure"})
			continue
		}

		parsed, parseErr := ParseReviewerOutput(r.RawOutput)
		if parseErr != nil {
			log.Warn().Str("cli", r.CLI).Err(parseErr).Msg("failed to parse structured output, using raw")
		}

		inputs = append(inputs, ReviewInput{
			CLI:       r.CLI,
			Model:     r.Model,
			Role:      string(r.Role),
			RawOutput: r.RawOutput,
			Parsed:    parsed,
		})
	}

	if len(inputs) == 0 {
		return nil, fmt.Errorf("all reviewers failed or hit quota limits (see skipped reasons): %s", formatSkipped(skipped))
	}

	// Correlated-failure signal: all opencode models share ONE opencode-go
	// credential/quota bucket, so a 429 tends to take out the whole family at
	// once. If we planned opencode reviewers but none produced a review, log it
	// distinctly — the review silently degrading to codex+antigravity is worth
	// surfacing separately from losing a single reviewer.
	opencodePlanned, opencodeSucceeded := 0, 0
	for _, p := range plans {
		if p.cli == "opencode" {
			opencodePlanned++
		}
	}
	for _, in := range inputs {
		if in.CLI == "opencode" {
			opencodeSucceeded++
		}
	}
	if opencodePlanned > 0 && opencodeSucceeded == 0 {
		log.Warn().Int("planned", opencodePlanned).Msg("all opencode reviewers failed (they share one opencode-go credential/quota — likely a correlated 429); review degraded to the other CLIs")
	}

	// Re-select the judge from the CLIs that ACTUALLY produced a review. The
	// preflight-time choice above can be stale: a reviewer that preflighted OK
	// can still 429 or fail at runtime, and judging with a quota-dead CLI would
	// fail the whole review even though another reviewer succeeded. Prefer
	// codex → antigravity → opencode among the successful inputs; fall back to
	// the preflight choice only if none of the preferred CLIs succeeded.
	if pickedCLI, pickedModel := pickJudge(inputs); pickedCLI != "" {
		if pickedCLI != judgeCLI {
			log.Info().Str("from", judgeCLI).Str("to", pickedCLI).Msg("re-selecting consilium judge to a CLI that produced a review")
			judgeCLI = pickedCLI
			consiliumSess.CLI = pickedCLI
		}
		// Always align the judge's model to one that actually produced a review —
		// e.g. the pre-created opencode judge defaults to glm, but if only
		// deepseek-pro succeeded, judge with deepseek-pro (glm may have 429'd).
		if pickedModel != "" {
			consiliumSess.Model = pickedModel
		}
	}

	log.Info().Int("successful", len(inputs)).Str("judge", judgeCLI).Msg("reviewers complete, running consilium")

	// Phase 2: Run consilium judge (under the same held slot).
	consiliumOutput, err := runConsilium(ctx, consiliumSess, judgeCLI, inputs, scope, effort, workdir, threshold)
	if err != nil {
		return nil, fmt.Errorf("consilium: %w", err)
	}

	// Phase 3: Filter and sort.
	consiliumOutput.Findings = FilterByConfidence(consiliumOutput.Findings, threshold)
	SortFindings(consiliumOutput.Findings)
	consiliumOutput.ReviewerCount = len(inputs)

	return &RunResult{
		Output:    consiliumOutput,
		Inputs:    inputs,
		Threshold: threshold,
		JudgeCLI:  judgeCLI,
		Skipped:   skipped,
	}, nil
}

// newReviewerSession creates a queued session for one reviewer. model and role
// may be given explicitly (for opencode, where the cli "opencode" backs several
// models); an empty model/role is derived from the cli via modelForCLI/RoleForCLI.
func newReviewerSession(cli, model string, role Role, scope, effort, workdir, groupID string) (*session.Session, error) {
	if role == "" {
		role = RoleForCLI(cli)
	}
	if model == "" {
		model = modelForCLI(cli)
	}
	prompt := BuildRolePrompt(role, scope)
	sess, err := session.NewQueued(cli, "megareview", model, effort, workdir, prompt, scope, groupID)
	if err != nil {
		return nil, fmt.Errorf("create %s session: %w", cli, err)
	}
	return sess, nil
}

// newConsiliumSession creates a queued session for the consilium judge. The
// prompt is finalized later (it depends on reviewer output), so we store a
// placeholder; runConsilium does not re-create the session.
func newConsiliumSession(judgeCLI, scope, effort, workdir, groupID string) (*session.Session, error) {
	model := modelForCLI(judgeCLI)
	sess, err := session.NewQueued(judgeCLI, "consilium", model, effort, workdir, "", scope, groupID)
	if err != nil {
		return nil, fmt.Errorf("create consilium session: %w", err)
	}
	return sess, nil
}

// waitForGroupSlot enqueues one ticket (with the given mode label, e.g.
// "megareview" or "plan") covering ticketSessions and blocks until promoted, then
// marks only runSessions running. Any ticketSessions not in runSessions (e.g. a
// megareview's consilium judge) stay queued until their own phase but are already
// in the ticket for liveness. Mirrors cmd.waitForQueueSlot but lives here to
// avoid an import cycle.
func waitForGroupSlot(ctx context.Context, noQueue bool, ticketSessions, runSessions []*session.Session, workdir, groupID, mode string) (release func(), err error) {
	markRunning := func() error {
		for i, s := range runSessions {
			if err := s.MarkRunning(); err != nil {
				// Roll back any session already flipped to running, so a partial
				// failure never strands a session "running" with no process.
				for _, prev := range runSessions[:i] {
					_ = prev.Fail(1, "aborted: failed to start review batch")
				}
				return fmt.Errorf("mark session running: %w", err)
			}
		}
		return nil
	}

	if noQueue || config.QueueDisabled() {
		return func() {}, markRunning()
	}

	ids := make([]string, len(ticketSessions))
	for i, s := range ticketSessions {
		ids[i] = s.ID
	}

	m := queue.New()
	if _, enqErr := m.Enqueue(groupID, ids, mode, workdir); enqErr != nil {
		log.Warn().Err(enqErr).Str("mode", mode).Msg("queue unavailable — running without queueing")
		return func() {}, markRunning()
	}

	start := time.Now()
	waitErr := m.WaitForSlot(ctx, func(pos, total, running int) {
		_, _ = fmt.Fprintf(os.Stderr, "rival queue: position %d/%d (%d running), waiting %s\n",
			pos, total, running, time.Since(start).Round(time.Second))
		for _, s := range ticketSessions {
			_ = s.SetQueuePosition(pos)
		}
	})
	if waitErr != nil {
		m.Release()
		msg := "cancelled while queued"
		if errors.Is(waitErr, queue.ErrQueueTimeout) {
			msg = fmt.Sprintf("queue timeout after %s — queue may be wedged; inspect with 'rival queue', purge with 'rival queue clear'", m.Timeout)
		}
		for _, s := range ticketSessions {
			_ = s.Fail(1, msg)
		}
		return nil, fmt.Errorf("rival queue: %s", msg)
	}

	if err := markRunning(); err != nil {
		m.Release()
		return nil, err
	}
	if waited := time.Since(start); waited >= time.Second {
		_, _ = fmt.Fprintf(os.Stderr, "rival queue: slot acquired after %s\n", waited.Round(time.Second))
	}
	return m.Release, nil
}

// runReviewer runs one reviewer. model and role are passed explicitly so a single
// cli ("opencode") can back several models; an empty model/role falls back to the
// cli-derived default.
func runReviewer(ctx context.Context, sess *session.Session, cli, model string, role Role, scope, effort, workdir string) cliResult {
	if role == "" {
		role = RoleForCLI(cli)
	}
	if model == "" {
		model = modelForCLI(cli)
	}
	prompt := BuildRolePrompt(role, scope)

	defer func() {
		if sess.Status == "running" || sess.Status == "queued" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("cli", cli).Str("model", model).Str("role", string(role)).Msg("starting reviewer")

	var err error
	var result *executor.Result
	switch cli {
	case "codex":
		result, err = executor.RunCodex(ctx, sess, prompt, effort, workdir, nil)
	case "antigravity":
		result, err = executor.RunAntigravity(ctx, sess, prompt, effort, workdir, nil)
	case "opencode":
		result, err = executor.RunOpencode(ctx, sess, prompt, effort, workdir, model, nil)
	default:
		return cliResult{CLI: cli, Model: model, Role: role, Err: fmt.Errorf("unsupported cli: %s", cli)}
	}

	if err != nil {
		_ = sess.Fail(1, err.Error())
		return cliResult{CLI: cli, Model: model, Role: role, Err: err}
	}

	// Read the log file to get raw output (includes stdout + stderr, so the
	// agy 429 envelope is captured here even though it exits 0).
	logData, err := os.ReadFile(sess.LogFile)
	if err != nil {
		if result.ExitCode != 0 {
			_ = sess.Fail(result.ExitCode, fmt.Sprintf("%s exited with code %d", cli, result.ExitCode))
		}
		return cliResult{CLI: cli, Model: model, Role: role, Err: fmt.Errorf("read log: %w", err), ExitCode: result.ExitCode}
	}

	raw := string(logData)

	switch {
	case result.ExitCode != 0:
		_ = sess.Fail(result.ExitCode, fmt.Sprintf("%s exited with code %d", cli, result.ExitCode))
	case executor.IsQuotaExhausted(raw):
		_ = sess.Fail(1, fmt.Sprintf("%s hit provider quota/rate limit (429)", cli))
	case strings.TrimSpace(raw) == "":
		// Exited 0 but wrote nothing — a silent no-op (e.g. agy auth/session
		// failure). Record it as failed so the TUI shows the reason instead of a
		// blank "(empty log)", and so it is not counted as a successful review.
		_ = sess.Fail(1, fmt.Sprintf("%s produced no output (empty result) — likely an auth/session failure", cli))
	default:
		_ = sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines)
	}

	return cliResult{
		CLI:       cli,
		Model:     model,
		Role:      role,
		RawOutput: raw,
		ExitCode:  result.ExitCode,
	}
}

// formatSkipped renders skipped reviewers as "cli: reason" pairs for error messages.
func formatSkipped(skipped []SkippedCLI) string {
	if len(skipped) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(skipped))
	for _, s := range skipped {
		parts = append(parts, fmt.Sprintf("%s: %s", s.CLI, s.Reason))
	}
	return strings.Join(parts, "; ")
}

// runConsilium runs the judge on a pre-created session (already in the queue
// ticket). The prompt is finalized here since it depends on reviewer output;
// MarkRunning flips the session from queued→running just before execution.
func runConsilium(ctx context.Context, sess *session.Session, judgeCLI string, inputs []ReviewInput, scope, effort, workdir string, threshold int) (*ConsiliumOutput, error) {
	prompt := BuildConsiliumPrompt(inputs, scope, threshold)
	if err := sess.MarkRunning(); err != nil {
		return nil, fmt.Errorf("start consilium session: %w", err)
	}

	defer func() {
		if sess.Status == "running" || sess.Status == "queued" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("cli", judgeCLI).Msg("starting consilium judge")

	var err error
	var result *executor.Result
	switch judgeCLI {
	case "codex":
		result, err = executor.RunCodex(ctx, sess, prompt, effort, workdir, nil)
	case "antigravity":
		result, err = executor.RunAntigravity(ctx, sess, prompt, effort, workdir, nil)
	case "opencode":
		// The consilium session carries the concrete opencode model to judge with
		// (set when the judge is selected/re-selected); RunOpencode falls back to
		// the default model if it is somehow empty.
		result, err = executor.RunOpencode(ctx, sess, prompt, effort, workdir, sess.Model, nil)
	default:
		return nil, fmt.Errorf("unsupported judge CLI: %s", judgeCLI)
	}
	if err != nil {
		_ = sess.Fail(1, err.Error())
		return nil, err
	}

	if result.ExitCode != 0 {
		_ = sess.Fail(result.ExitCode, fmt.Sprintf("consilium exited with code %d", result.ExitCode))
		return nil, fmt.Errorf("consilium exited with code %d", result.ExitCode)
	}

	logData, err := os.ReadFile(sess.LogFile)
	if err != nil {
		_ = sess.Fail(1, "read consilium log failed")
		return nil, fmt.Errorf("read consilium log: %w", err)
	}

	if executor.IsQuotaExhausted(string(logData)) {
		_ = sess.Fail(1, fmt.Sprintf("consilium judge (%s) hit provider quota/rate limit (429)", judgeCLI))
		return nil, fmt.Errorf("consilium judge (%s) hit provider quota/rate limit (429) — authenticate to a quota-bearing account or wait for reset", judgeCLI)
	}

	// Parse BEFORE marking the session complete: an empty or unparseable judge
	// output is a failure, and marking it "completed" first would leave the
	// dashboard showing a successful session for a run that produced no verdict.
	output, err := ParseConsiliumOutput(string(logData))
	if err != nil {
		// Dump raw for debugging.
		log.Error().Str("raw", truncate(string(logData), 500)).Msg("consilium parse failed")
		_ = sess.Fail(1, fmt.Sprintf("consilium judge (%s) output could not be parsed", judgeCLI))
		return nil, fmt.Errorf("parse consilium output: %w", err)
	}

	_ = sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines)
	return output, nil
}

// pickJudge chooses a consilium judge from the CLIs that produced a successful
// review, in preference order codex → antigravity → opencode. It returns the cli
// and the concrete model to judge with. For opencode it picks the successful
// model highest in the roster order (config.OpencodeReviewerList), so the choice
// is DETERMINISTIC — not whichever model's goroutine happened to finish first
// (that let the fastest/weakest model, e.g. flash, judge over a preferred one).
// Returns ("","") if none of the preferred CLIs are among the successful inputs,
// letting the caller keep its earlier (preflight) choice.
func pickJudge(inputs []ReviewInput) (cli, model string) {
	succeeded := make(map[string]bool, len(inputs)) // opencode models that succeeded
	haveCLI := make(map[string]string, len(inputs)) // cli → any successful model
	for _, in := range inputs {
		if in.CLI == "opencode" {
			succeeded[in.Model] = true
		}
		if _, ok := haveCLI[in.CLI]; !ok {
			haveCLI[in.CLI] = in.Model
		}
	}
	for _, c := range []string{"codex", "antigravity", "opencode"} {
		if _, ok := haveCLI[c]; !ok {
			continue
		}
		if c != "opencode" {
			return c, haveCLI[c]
		}
		// Pick the highest-priority opencode model (roster order) that succeeded.
		for _, r := range config.OpencodeReviewerList() {
			if succeeded[r.Model] {
				return "opencode", r.Model
			}
		}
		// A successful opencode model not in the current roster (e.g. roster
		// changed mid-run) — fall back to any successful one.
		return "opencode", haveCLI["opencode"]
	}
	return "", ""
}

func modelForCLI(cli string) string {
	switch cli {
	case "codex":
		return config.CodexModel
	case "gemini":
		return config.GeminiModel
	case "claude":
		return config.ClaudeModel
	case "antigravity":
		return config.AntigravityModel
	case "opencode":
		return config.OpencodeModel
	default:
		return cli
	}
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
