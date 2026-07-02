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

// SkippedCLI records a CLI that was unavailable during megareview.
type SkippedCLI struct {
	CLI    string
	Reason string
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

// reviewerPlan pairs a pre-created session with the CLI it will run.
type reviewerPlan struct {
	cli  string
	sess *session.Session
}

// RunMegaReview runs the full pipeline: enqueue → spawn reviewers → parse → consilium → filter.
// One queue ticket covers the whole pipeline (both reviewers + consilium run
// under the single held slot). Pass noQueue to bypass the queue.
func RunMegaReview(ctx context.Context, scope, effort, workdir, groupID string, noQueue bool) (*RunResult, error) {
	threshold := DefaultConfidenceThreshold

	// Preflight — megareview uses Codex + Antigravity only. Runs BEFORE
	// enqueue so a doomed review never occupies a slot.
	codexOK := true
	antigravityOK := true
	var skipped []SkippedCLI
	if err := executor.CodexPreflight(); err != nil {
		log.Warn().Err(err).Msg("codex unavailable")
		codexOK = false
		skipped = append(skipped, SkippedCLI{CLI: "codex", Reason: err.Error()})
	}
	if err := executor.AntigravityPreflight(); err != nil {
		log.Warn().Err(err).Msg("antigravity unavailable")
		antigravityOK = false
		skipped = append(skipped, SkippedCLI{CLI: "antigravity", Reason: err.Error()})
	}
	if !codexOK && !antigravityOK {
		return nil, fmt.Errorf("no CLI reviewers available")
	}

	// Determine which CLI to use for the consilium judge.
	judgeCLI := "codex"
	if !codexOK {
		judgeCLI = "antigravity"
	}

	// Create reviewer sessions up front (status "queued") so they appear in
	// the TUI/web while waiting, and so the queue ticket can reference them.
	var plans []reviewerPlan
	if codexOK {
		sess, err := newReviewerSession("codex", scope, effort, workdir, groupID)
		if err != nil {
			return nil, err
		}
		plans = append(plans, reviewerPlan{cli: "codex", sess: sess})
	}
	if antigravityOK {
		sess, err := newReviewerSession("antigravity", scope, effort, workdir, groupID)
		if err != nil {
			return nil, err
		}
		plans = append(plans, reviewerPlan{cli: "antigravity", sess: sess})
	}

	// Pre-create the consilium judge session too, so the queue ticket covers
	// the WHOLE pipeline. If rival is SIGKILL'd during consilium and the judge
	// child survives, the ticket's liveness check sees that running session and
	// keeps the slot held instead of letting another process double-promote.
	consiliumSess, err := newConsiliumSession(judgeCLI, scope, effort, workdir, groupID)
	if err != nil {
		return nil, err
	}

	// sessions = the queue ticket's covered sessions (both reviewers + judge).
	sessions := make([]*session.Session, 0, len(plans)+1)
	for _, p := range plans {
		sessions = append(sessions, p.sess)
	}
	sessions = append(sessions, consiliumSess)

	// Fail any session still left "queued" (never started) when we return —
	// covers bail-before-run and the error paths below. MarkRunning flips a
	// session to "running" only once it is actually about to execute, so a
	// session stuck "queued" here genuinely never ran.
	defer func() {
		for _, s := range sessions {
			if s.Status == "queued" {
				_ = s.Fail(1, "interrupted")
			}
		}
	}()

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
			results <- runReviewer(ctx, pl.sess, pl.cli, scope, effort, workdir)
		}(p)
	}

	wg.Wait()
	close(results)

	// Collect and parse reviewer outputs.
	var inputs []ReviewInput
	for r := range results {
		if r.Err != nil {
			log.Error().Str("cli", r.CLI).Err(r.Err).Msg("reviewer failed")
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Reason: r.Err.Error()})
			continue
		}
		if r.ExitCode != 0 {
			log.Error().Str("cli", r.CLI).Int("exit_code", r.ExitCode).Msg("reviewer exited with error")
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Reason: fmt.Sprintf("exited with code %d", r.ExitCode)})
			continue
		}
		// agy exits 0 on a 429; detect quota exhaustion from the captured log so
		// a quota-blocked reviewer is reported as skipped, not counted as a
		// successful (but empty) review that silently degrades the consilium.
		if executor.IsQuotaExhausted(r.RawOutput) {
			log.Error().Str("cli", r.CLI).Msg("reviewer hit provider quota/rate limit (429) — skipping")
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Reason: "quota/rate limit reached (429) — not authenticated to a quota-bearing account or quota exhausted"})
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

// newReviewerSession creates a queued session for one reviewer CLI.
func newReviewerSession(cli, scope, effort, workdir, groupID string) (*session.Session, error) {
	role := RoleForCLI(cli)
	model := modelForCLI(cli)
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

func runReviewer(ctx context.Context, sess *session.Session, cli, scope, effort, workdir string) cliResult {
	role := RoleForCLI(cli)
	model := modelForCLI(cli)
	prompt := BuildRolePrompt(role, scope)

	defer func() {
		if sess.Status == "running" || sess.Status == "queued" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("cli", cli).Str("role", string(role)).Msg("starting reviewer")

	var err error
	var result *executor.Result
	switch cli {
	case "codex":
		result, err = executor.RunCodex(ctx, sess, prompt, effort, workdir, nil)
	case "antigravity":
		result, err = executor.RunAntigravity(ctx, sess, prompt, effort, workdir, nil)
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

	_ = sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines)

	output, err := ParseConsiliumOutput(string(logData))
	if err != nil {
		// Dump raw for debugging.
		log.Error().Str("raw", truncate(string(logData), 500)).Msg("consilium parse failed")
		return nil, fmt.Errorf("parse consilium output: %w", err)
	}

	return output, nil
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
