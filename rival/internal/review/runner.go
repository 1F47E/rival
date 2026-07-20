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
	Output     *ConsiliumOutput
	Inputs     []ReviewInput
	Threshold  int
	JudgeCLI   string
	JudgeModel string
	Skipped    []SkippedCLI
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

// RunMegaReview runs the curated default roster. It is kept as the stable
// entry point for callers that do not need per-run model selection.
func RunMegaReview(ctx context.Context, scope, effort, workdir, groupID string, noQueue bool) (*RunResult, error) {
	return RunMegaReviewWithModels(ctx, scope, effort, workdir, groupID, noQueue, nil)
}

// RunMegaReviewWithModels runs the full pipeline: resolve roster → preflight →
// enqueue → spawn reviewers → parse → consilium → filter. An empty model list
// uses the curated default roster; a non-empty list is the exact roster for
// this run. One queue ticket covers both reviewers and the consilium phase.
func RunMegaReviewWithModels(ctx context.Context, scope, effort, workdir, groupID string, noQueue bool, modelSelectors []string) (*RunResult, error) {
	threshold := DefaultConfidenceThreshold

	targets, err := config.ResolveReviewTargets(modelSelectors)
	if err != nil {
		return nil, err
	}

	// Preflight only the selected targets so an explicit one-model review never
	// checks or launches an unselected reviewer.
	var skipped []SkippedCLI
	available := make([]config.ReviewTarget, 0, len(targets))
	for _, target := range targets {
		var preflightErr error
		switch target.CLI {
		case "codex":
			preflightErr = executor.CodexPreflight()
		case "opencode":
			preflightErr = executor.OpencodePreflightModel(target.Model, workdir)
		default:
			preflightErr = fmt.Errorf("unsupported reviewer CLI: %s", target.CLI)
		}
		if preflightErr != nil {
			label := config.EngineLabel(target.CLI, target.Model)
			reason := config.PublicRuntimeError(target.CLI, target.Model, preflightErr.Error())
			log.Warn().Str("reviewer", label).Str("reason", reason).Msg("reviewer unavailable")
			skipped = append(skipped, SkippedCLI{CLI: target.CLI, Model: target.Model, Reason: reason})
			continue
		}
		available = append(available, target)
	}
	if len(available) == 0 {
		return nil, fmt.Errorf("no selected reviewers available: %s", formatSkipped(skipped))
	}

	// The first selected model that passes preflight is the initial judge.
	judgeCLI, judgeModel := preferredJudgeForTargets(available)

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
		effectiveEffort, err := config.ResolveEffort(model, effort, config.DefaultReviewEffort)
		if err != nil {
			return err
		}
		sess, err := newReviewerSession(cli, model, role, scope, effectiveEffort, workdir, groupID)
		if err != nil {
			return err
		}
		plans = append(plans, reviewerPlan{cli: cli, model: sess.Model, role: role, sess: sess})
		sessions = append(sessions, sess)
		return nil
	}
	for _, target := range available {
		if err := addReviewer(target.CLI, target.Model, Role(target.Role)); err != nil {
			return nil, err
		}
	}

	// Pre-create the consilium judge session too, so the queue ticket covers
	// the WHOLE pipeline. If rival is SIGKILL'd during consilium and the judge
	// child survives, the ticket's liveness check sees that running session and
	// keeps the slot held instead of letting another process double-promote.
	consiliumSess, err := newConsiliumSession(judgeCLI, judgeModel, scope, plans[0].sess.Effort, workdir, groupID)
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
	type indexedCLIResult struct {
		index  int
		result cliResult
	}
	var wg sync.WaitGroup
	results := make(chan indexedCLIResult, len(plans))

	for i, p := range plans {
		wg.Add(1)
		go func(index int, pl reviewerPlan) {
			defer wg.Done()
			results <- indexedCLIResult{index: index, result: runReviewer(ctx, pl.sess, pl.cli, pl.model, pl.role, scope, workdir)}
		}(i, p)
	}

	wg.Wait()
	close(results)

	// Restore requested order after concurrent completion so prompts, console
	// attribution, and skipped-model reporting stay deterministic.
	orderedResults := make([]cliResult, len(plans))
	for result := range results {
		orderedResults[result.index] = result.result
	}

	// Collect and parse reviewer outputs.
	var inputs []ReviewInput
	for _, r := range orderedResults {
		label := config.EngineLabel(r.CLI, r.Model)
		if r.Err != nil {
			reason := config.PublicRuntimeError(r.CLI, r.Model, r.Err.Error())
			log.Error().Str("reviewer", label).Str("reason", reason).Msg("reviewer failed")
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Model: r.Model, Reason: reason})
			continue
		}
		if r.ExitCode != 0 {
			log.Error().Str("reviewer", label).Int("exit_code", r.ExitCode).Msg("reviewer exited with error")
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Model: r.Model, Reason: fmt.Sprintf("exited with code %d", r.ExitCode)})
			continue
		}
		// Detect quota exhaustion from the captured log so a quota-blocked
		// reviewer is reported as skipped, not counted as a successful empty
		// review that silently degrades the consilium.
		if executor.IsQuotaExhausted(r.RawOutput) {
			log.Error().Str("reviewer", label).Msg("reviewer hit provider quota/rate limit (429) — skipping")
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Model: r.Model, Reason: "quota/rate limit reached (429) — not authenticated to a quota-bearing account or quota exhausted"})
			continue
		}
		// A provider can exit 0 having produced nothing at all. An empty review is
		// not successful: count it as skipped so the consilium isn't fed a no-op
		// input and the TUI shows why instead of a blank "(empty log)".
		if strings.TrimSpace(r.RawOutput) == "" {
			log.Error().Str("reviewer", label).Msg("reviewer produced empty output — skipping")
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Model: r.Model, Reason: "produced no output (empty result) — the provider CLI exited without writing a review; likely an auth/session failure"})
			continue
		}

		parsed, parseErr := ParseReviewerOutput(r.RawOutput)
		if parseErr != nil {
			log.Warn().Str("reviewer", label).Err(parseErr).Msg("failed to parse structured output, using raw")
		}

		inputs = append(inputs, ReviewInput{
			CLI:       r.CLI,
			Model:     r.Model,
			Role:      string(r.Role),
			RawOutput: config.PublicRuntimeLog(r.CLI, r.Model, r.RawOutput),
			Parsed:    parsed,
		})
	}

	if len(inputs) == 0 {
		return nil, fmt.Errorf("all reviewers failed or hit quota limits (see skipped reasons): %s", formatSkipped(skipped))
	}

	// Correlated-failure signal: if we planned OpenCode-backed reviewers but
	// none produced a review, log it separately from losing one reviewer.
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
		log.Warn().Int("planned", opencodePlanned).Msg("all selected opencode reviewers failed; review degraded to the other selected reviewers")
	}

	// Re-select the judge from the models that ACTUALLY produced a review. The
	// preflight-time choice above can be stale: a reviewer that preflighted OK
	// can still 429 or fail at runtime, and judging with a quota-dead CLI would
	// fail the whole review even though another reviewer succeeded. The
	// invocation's exact requested order controls the fallback preference.
	if pickedCLI, pickedModel := pickJudge(inputs, available); pickedCLI != "" {
		if pickedCLI != judgeCLI {
			log.Info().
				Str("from", config.EngineLabel(judgeCLI, judgeModel)).
				Str("to", config.EngineLabel(pickedCLI, pickedModel)).
				Msg("re-selecting consilium judge to a reviewer that produced a review")
			judgeCLI = pickedCLI
			consiliumSess.CLI = pickedCLI
		}
		// Always align the judge's model to one that actually produced a review —
		// The pre-created judge follows the requested priority, but if that
		// reviewer fails, use the highest-priority reviewer that succeeded.
		if pickedModel != "" {
			judgeModel = pickedModel
			consiliumSess.Model = pickedModel
			for _, plan := range plans {
				if plan.cli == pickedCLI && plan.model == pickedModel {
					consiliumSess.Effort = plan.sess.Effort
					break
				}
			}
		}
	}

	log.Info().Int("successful", len(inputs)).Str("judge", config.EngineLabel(judgeCLI, judgeModel)).Msg("reviewers complete, running consilium")

	// Phase 2: Run consilium judge (under the same held slot).
	consiliumOutput, err := runConsilium(ctx, consiliumSess, judgeCLI, inputs, scope, workdir, threshold)
	if err != nil {
		return nil, fmt.Errorf("consilium: %w", err)
	}

	// Phase 3: Filter and sort.
	consiliumOutput.Findings = FilterByConfidence(consiliumOutput.Findings, threshold)
	SortFindings(consiliumOutput.Findings)
	consiliumOutput.ReviewerCount = len(inputs)

	return &RunResult{
		Output:     consiliumOutput,
		Inputs:     inputs,
		Threshold:  threshold,
		JudgeCLI:   judgeCLI,
		JudgeModel: judgeModel,
		Skipped:    skipped,
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
		return nil, fmt.Errorf("create %s session: %w", config.EngineLabel(cli, model), err)
	}
	return sess, nil
}

// newConsiliumSession creates a queued session for the consilium judge. The
// prompt is finalized later (it depends on reviewer output), so we store a
// placeholder; runConsilium does not re-create the session.
func newConsiliumSession(judgeCLI, model, scope, effort, workdir, groupID string) (*session.Session, error) {
	if model == "" {
		model = modelForCLI(judgeCLI)
	}
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
func runReviewer(ctx context.Context, sess *session.Session, cli, model string, role Role, scope, workdir string) cliResult {
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

	log.Info().Str("session", sess.ID).Str("reviewer", config.EngineLabel(cli, model)).Str("role", string(role)).Msg("starting reviewer")

	var err error
	var result *executor.Result
	switch cli {
	case "codex":
		result, err = executor.RunCodexModel(ctx, sess, prompt, sess.Effort, workdir, model, nil)
	case "opencode":
		result, err = executor.RunOpencode(ctx, sess, prompt, sess.Effort, workdir, model, nil)
	default:
		return cliResult{CLI: cli, Model: model, Role: role, Err: fmt.Errorf("unsupported cli: %s", cli)}
	}

	if err != nil {
		reason := config.PublicRuntimeError(cli, model, err.Error())
		_ = sess.Fail(1, reason)
		return cliResult{CLI: cli, Model: model, Role: role, Err: errors.New(reason)}
	}

	// Read the log file to get raw output, including stdout and stderr.
	logData, err := os.ReadFile(sess.LogFile)
	if err != nil {
		if result.ExitCode != 0 {
			_ = sess.Fail(result.ExitCode, fmt.Sprintf("%s exited with code %d", config.EngineLabel(cli, model), result.ExitCode))
		}
		return cliResult{CLI: cli, Model: model, Role: role, Err: fmt.Errorf("read log: %w", err), ExitCode: result.ExitCode}
	}

	raw := string(logData)

	switch {
	case result.ExitCode != 0:
		_ = sess.Fail(result.ExitCode, fmt.Sprintf("%s exited with code %d", config.EngineLabel(cli, model), result.ExitCode))
	case executor.IsQuotaExhausted(raw):
		_ = sess.Fail(1, fmt.Sprintf("%s hit provider quota/rate limit (429)", config.EngineLabel(cli, model)))
	case strings.TrimSpace(raw) == "":
		// Exited 0 but wrote nothing — a silent provider no-op. Record it as
		// failed so the TUI shows the reason instead of a blank "(empty log)",
		// and so it is not counted as a successful review.
		_ = sess.Fail(1, fmt.Sprintf("%s produced no output (empty result) — likely an auth/session failure", config.EngineLabel(cli, model)))
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
		reason := config.PublicRuntimeError(s.CLI, s.Model, s.Reason)
		parts = append(parts, fmt.Sprintf("%s: %s", s.Label(), reason))
	}
	return strings.Join(parts, "; ")
}

// runConsilium runs the judge on a pre-created session (already in the queue
// ticket). The prompt is finalized here since it depends on reviewer output;
// MarkRunning flips the session from queued→running just before execution.
func runConsilium(ctx context.Context, sess *session.Session, judgeCLI string, inputs []ReviewInput, scope, workdir string, threshold int) (*ConsiliumOutput, error) {
	judgeLabel := config.EngineLabel(judgeCLI, sess.Model)
	prompt := BuildConsiliumPrompt(inputs, scope, threshold)
	if err := sess.MarkRunning(); err != nil {
		return nil, fmt.Errorf("start consilium session: %w", err)
	}

	defer func() {
		if sess.Status == "running" || sess.Status == "queued" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("judge", judgeLabel).Msg("starting consilium judge")

	var err error
	var result *executor.Result
	switch judgeCLI {
	case "codex":
		result, err = executor.RunCodexModel(ctx, sess, prompt, sess.Effort, workdir, sess.Model, nil)
	case "opencode":
		// The consilium session carries the concrete opencode model to judge with
		// (set when the judge is selected/re-selected); RunOpencode falls back to
		// the default model if it is somehow empty.
		result, err = executor.RunOpencode(ctx, sess, prompt, sess.Effort, workdir, sess.Model, nil)
	default:
		return nil, fmt.Errorf("unsupported judge CLI: %s", judgeCLI)
	}
	if err != nil {
		reason := config.PublicRuntimeError(judgeCLI, sess.Model, err.Error())
		_ = sess.Fail(1, reason)
		return nil, errors.New(reason)
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
		_ = sess.Fail(1, fmt.Sprintf("consilium judge (%s) hit provider quota/rate limit (429)", judgeLabel))
		return nil, fmt.Errorf("consilium judge (%s) hit provider quota/rate limit (429) — authenticate to a quota-bearing account or wait for reset", judgeLabel)
	}

	// Parse BEFORE marking the session complete: an empty or unparseable judge
	// output is a failure, and marking it "completed" first would leave the
	// dashboard showing a successful session for a run that produced no verdict.
	output, err := ParseConsiliumOutput(string(logData))
	if err != nil {
		// Dump raw for debugging.
		log.Error().Str("raw", truncate(config.PublicRuntimeLog(judgeCLI, sess.Model, string(logData)), 500)).Msg("consilium parse failed")
		_ = sess.Fail(1, fmt.Sprintf("consilium judge (%s) output could not be parsed", judgeLabel))
		return nil, fmt.Errorf("parse consilium output: %w", err)
	}

	_ = sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines)
	return output, nil
}

// preferredJudgeForTargets chooses the first requested reviewer that passed
// preflight as the initial judge.
func preferredJudgeForTargets(targets []config.ReviewTarget) (cli, model string) {
	if len(targets) > 0 {
		return targets[0].CLI, targets[0].Model
	}
	return "", ""
}

// pickJudge chooses a consilium judge that actually produced a successful
// review. Models are considered in exact per-invocation target order, not
// goroutine completion order.
func pickJudge(inputs []ReviewInput, targets []config.ReviewTarget) (cli, model string) {
	successful := func(target config.ReviewTarget) bool {
		for _, input := range inputs {
			if input.CLI == target.CLI && (input.Model == "" || target.Model == "" || input.Model == target.Model) {
				return true
			}
		}
		return false
	}

	for _, target := range targets {
		if successful(target) {
			return target.CLI, target.Model
		}
	}

	// Defensive fallback for a successful input whose target metadata was lost.
	for _, input := range inputs {
		return input.CLI, input.Model
	}
	return "", ""
}

func modelForCLI(cli string) string {
	switch cli {
	case "codex":
		return config.GPT56SolModel
	case "claude":
		return config.ClaudeModel
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
