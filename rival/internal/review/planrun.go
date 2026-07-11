package review

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/executor"
	"github.com/1F47E/rival/internal/session"
	"github.com/rs/zerolog/log"
)

// buildPlanPrompt renders the plan-review prompt for a plan file at absPath.
func buildPlanPrompt(absPath string) string {
	return strings.ReplaceAll(config.PlanReviewPrompt, "{FILE}", absPath)
}

// runTimeoutReason turns an executor error into a session-failure reason. When
// the run's context hit the RIVAL_RUN_TIMEOUT deadline, it returns a clear
// "run timeout" message so a hung provider CLI is distinguishable from a normal
// failure; otherwise it returns the provider's own error text.
func runTimeoutReason(ctx context.Context, model, fallback string) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Sprintf("%s run timeout after %s (RIVAL_RUN_TIMEOUT) — model did not finish", model, config.RunTimeout())
	}
	return fallback
}

// planFailureReason keeps user-facing failures model-specific even though the
// implementation uses local adapter binaries underneath.
func planFailureReason(cli, reason string) string {
	return config.PublicRuntimeError(cli, planModelForCLI(cli), reason)
}

// PlanRunResult holds the outcome of a plan review across one or more CLIs.
// Results carries the successful reviews (Parsed may be nil if a CLI's output
// failed to parse — its Raw is retained); Skipped records CLIs that were
// unavailable, failed, or hit quota so the caller can surface why.
type PlanRunResult struct {
	Results []PlanCLIResult
	Skipped []SkippedCLI
}

// planCLIRun is the raw outcome of running one CLI's plan review, before quota
// detection and JSON parsing. assemblePlanResults turns a batch of these (plus
// the pre-run skipped list) into the final PlanRunResult — keeping that logic
// pure and unit-testable without shelling real CLIs.
type planCLIRun struct {
	CLI      string
	Model    string
	Raw      string
	ExitCode int
	Err      error
	// Reason is a human-readable failure reason set on the error path (e.g. a
	// RIVAL_RUN_TIMEOUT message or the provider's own error), so a skipped CLI
	// reports the real failure mode instead of a bare "exited with code -1".
	Reason string
}

// planExecutor wraps the preflight + run steps behind func fields so tests can
// inject fakes. The zero value is unusable; use defaultPlanExecutor().
type planExecutor struct {
	preflight func(cli string) error
	// run executes one CLI's plan review on sess and returns the raw log output.
	run func(ctx context.Context, sess *session.Session, cli, prompt, effort, workdir string) (raw string, exitCode int, err error)
}

func defaultPlanExecutor() planExecutor {
	return planExecutor{
		preflight: func(cli string) error {
			switch cli {
			case "codex":
				return executor.CodexPreflight()
			case "fable":
				return executor.ClaudePreflight()
			default:
				return fmt.Errorf("unsupported plan cli: %s", cli)
			}
		},
		run: func(ctx context.Context, sess *session.Session, cli, prompt, effort, workdir string) (string, int, error) {
			var result *executor.Result
			var err error
			switch cli {
			case "codex":
				result, err = executor.RunCodexModel(ctx, sess, prompt, effort, workdir, config.GPT56SolModel, nil)
			case "fable":
				result, err = executor.RunFable(ctx, sess, prompt, effort, workdir, nil)
			default:
				return "", 1, fmt.Errorf("unsupported plan cli: %s", cli)
			}
			if err != nil {
				return "", 1, err
			}
			logData, readErr := os.ReadFile(sess.LogFile)
			if readErr != nil {
				return "", result.ExitCode, fmt.Errorf("read log: %w", readErr)
			}
			return string(logData), result.ExitCode, nil
		},
	}
}

// planModelForCLI returns the model id recorded for a plan CLI's session.
func planModelForCLI(cli string) string {
	switch cli {
	case "codex":
		return config.GPT56SolModel
	case "fable":
		return config.FableModel
	default:
		return cli
	}
}

// RunPlanReview reviews the plan/spec file at absPath with each CLI in clis,
// concurrently, under a single queue ticket. There is no consilium judge — each
// CLI's structured 1-10 rating + findings are returned independently. A CLI that
// is unavailable, fails, or hits quota is recorded in Skipped rather than
// aborting the run; only when every CLI is unusable does this return an error.
func RunPlanReview(ctx context.Context, absPath, effort, workdir, groupID string, noQueue bool, clis []string) (*PlanRunResult, error) {
	return runPlanReview(ctx, defaultPlanExecutor(), absPath, effort, workdir, groupID, noQueue, clis)
}

func runPlanReview(ctx context.Context, ex planExecutor, absPath, effort, workdir, groupID string, noQueue bool, clis []string) (*PlanRunResult, error) {
	if len(clis) == 0 {
		return nil, fmt.Errorf("no plan models requested")
	}

	prompt := buildPlanPrompt(absPath)

	// Preflight each CLI BEFORE enqueuing so a doomed run never occupies a slot.
	// Unavailable CLIs are skipped; sessions are created only for the survivors.
	type plan struct {
		cli  string
		sess *session.Session
	}
	var plans []plan
	var skipped []SkippedCLI
	var sessions []*session.Session

	// Fail any created session still "queued" (never started) when we return —
	// registered BEFORE the creation loop so a NewQueued failure mid-loop still
	// cleans up the sessions already created, not just the ones that started.
	defer func() {
		for _, s := range sessions {
			if s.Status == "queued" {
				_ = s.Fail(1, "interrupted")
			}
		}
	}()

	for _, cli := range clis {
		if err := ex.preflight(cli); err != nil {
			model := planModelForCLI(cli)
			reason := planFailureReason(cli, err.Error())
			log.Warn().Str("reviewer", config.EngineLabel(cli, model)).Str("reason", reason).Msg("plan reviewer unavailable")
			skipped = append(skipped, SkippedCLI{CLI: cli, Model: model, Reason: reason})
			continue
		}
		sess, err := session.NewQueued(cli, "plan", planModelForCLI(cli), effort, workdir, prompt, absPath, groupID)
		if err != nil {
			return nil, fmt.Errorf("create %s plan session: %w", config.EngineLabel(cli, planModelForCLI(cli)), err)
		}
		if cli == "fable" {
			sess.Account = config.ClaudeSubscription()
		}
		plans = append(plans, plan{cli: cli, sess: sess})
		sessions = append(sessions, sess)
	}

	if len(plans) == 0 {
		return nil, fmt.Errorf("no plan models available (see skipped reasons): %s", formatSkipped(skipped))
	}

	// One queue ticket covers all plan sessions; all of them are the run set
	// (there is no deferred consilium phase like megareview has).
	release, err := waitForGroupSlot(ctx, noQueue, sessions, sessions, workdir, groupID, "plan")
	if err != nil {
		return nil, err
	}
	defer release()

	// Bound the run once a slot is held: a hung CLI must not keep the slot (and
	// the detached rival) alive forever. Single phase → mult 1.
	ctx, cancelRun := config.WithRunTimeout(ctx, 1)
	defer cancelRun()

	// Run every CLI concurrently.
	var wg sync.WaitGroup
	runs := make(chan planCLIRun, len(plans))
	for _, p := range plans {
		wg.Add(1)
		go func(pl plan) {
			defer wg.Done()
			runs <- runPlanCLI(ctx, ex, pl.sess, pl.cli, prompt, effort, workdir)
		}(p)
	}
	wg.Wait()
	close(runs)

	batch := make([]planCLIRun, 0, len(plans))
	for r := range runs {
		batch = append(batch, r)
	}

	return assemblePlanResults(batch, skipped)
}

// runPlanCLI executes one CLI's plan review and finalizes its session, returning
// the raw outcome for assemblePlanResults to interpret.
func runPlanCLI(ctx context.Context, ex planExecutor, sess *session.Session, cli, prompt, effort, workdir string) planCLIRun {
	model := planModelForCLI(cli)

	defer func() {
		if sess.Status == "running" || sess.Status == "queued" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("reviewer", config.EngineLabel(cli, model)).Msg("starting plan reviewer")

	raw, exitCode, err := ex.run(ctx, sess, cli, prompt, effort, workdir)

	// The fable path runs through the Claude executor, which overwrites sess.Mode
	// with the transport ("native"/"docker"). Restore the task mode so the TUI/web
	// classify this as a plan session (they key off Mode == "plan"). Done before
	// Complete/Fail, which persist the session.
	sess.Mode = "plan"

	if err != nil {
		reason := runTimeoutReason(ctx, config.EngineLabel(cli, model), planFailureReason(cli, err.Error()))
		_ = sess.Fail(1, reason)
		return planCLIRun{CLI: cli, Model: model, Err: err, Reason: reason, ExitCode: -1}
	}

	switch {
	case exitCode != 0:
		_ = sess.Fail(exitCode, fmt.Sprintf("%s exited with code %d", config.EngineLabel(cli, model), exitCode))
	case executor.IsQuotaExhausted(raw):
		_ = sess.Fail(1, fmt.Sprintf("%s hit provider quota/rate limit (429)", config.EngineLabel(cli, model)))
	case strings.TrimSpace(raw) == "":
		// Exited 0 but wrote nothing — a silent no-op (e.g. an auth/session
		// failure). Fail it so it is reported as skipped, not a "successful" plan
		// review that formats to an empty string while the command exits 0.
		_ = sess.Fail(1, fmt.Sprintf("%s produced no output (empty result) — likely an auth/session failure", config.EngineLabel(cli, model)))
	default:
		_ = sess.Complete(exitCode, int64(len(raw)), 0)
	}

	return planCLIRun{CLI: cli, Model: model, Raw: raw, ExitCode: exitCode}
}

// assemblePlanResults turns raw CLI runs (plus the pre-run skipped list) into the
// final PlanRunResult. It is pure: given the same inputs it produces the same
// output with no I/O, so the full success/failure matrix is unit-testable without
// real CLIs. A CLI that errored, exited non-zero, or hit quota is moved to
// Skipped; a successful CLI whose output does not parse keeps its Raw with a nil
// Parsed so nothing is lost. If no CLI succeeds, it returns an error listing the
// skipped reasons.
func assemblePlanResults(batch []planCLIRun, skipped []SkippedCLI) (*PlanRunResult, error) {
	var results []PlanCLIResult
	for _, r := range batch {
		model := r.Model
		if model == "" {
			model = planModelForCLI(r.CLI)
		}
		switch {
		case r.Err != nil:
			reason := r.Reason
			if reason == "" {
				reason = planFailureReason(r.CLI, r.Err.Error())
			}
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Model: model, Reason: reason})
			continue
		case r.ExitCode != 0:
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Model: model, Reason: fmt.Sprintf("exited with code %d", r.ExitCode)})
			continue
		case executor.IsQuotaExhausted(r.Raw):
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Model: model, Reason: "quota/rate limit reached (429) — not authenticated to a quota-bearing account or quota exhausted"})
			continue
		case strings.TrimSpace(r.Raw) == "":
			// Defensive: an exit-0 run that wrote nothing is not a review. Skip it
			// so a single-CLI run never formats to an empty string with exit 0.
			skipped = append(skipped, SkippedCLI{CLI: r.CLI, Model: model, Reason: "produced no output (empty result) — the model exited without writing a review; likely an auth/session failure"})
			continue
		}

		res := PlanCLIResult{CLI: r.CLI, Model: model, Raw: r.Raw}
		parsed, parseErr := ParsePlanOutput(r.Raw)
		if parseErr != nil {
			log.Warn().Str("reviewer", config.EngineLabel(r.CLI, model)).Err(parseErr).Msg("failed to parse plan output, keeping raw")
		} else {
			res.Parsed = parsed
		}
		results = append(results, res)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("all plan reviewers failed or hit quota limits (see skipped reasons): %s", formatSkipped(skipped))
	}

	return &PlanRunResult{Results: results, Skipped: skipped}, nil
}
