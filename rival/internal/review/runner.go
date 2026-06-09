package review

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/executor"
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

// RunMegaReview runs the full pipeline: spawn reviewers → parse → consilium → filter.
func RunMegaReview(ctx context.Context, scope, effort, workdir, groupID string) (*RunResult, error) {
	threshold := DefaultConfidenceThreshold

	// Preflight — megareview uses Codex + Antigravity only.
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

	// Phase 1: Spawn reviewers in parallel with role-specific prompts.
	var wg sync.WaitGroup
	results := make(chan cliResult, 2)

	if codexOK {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- runReviewer(ctx, "codex", groupID, scope, effort, workdir)
		}()
	}
	if antigravityOK {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- runReviewer(ctx, "antigravity", groupID, scope, effort, workdir)
		}()
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

	// Phase 2: Run consilium judge.
	consiliumOutput, err := runConsilium(ctx, judgeCLI, inputs, scope, effort, workdir, groupID, threshold)
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

func runReviewer(ctx context.Context, cli, groupID, scope, effort, workdir string) cliResult {
	role := RoleForCLI(cli)
	model := modelForCLI(cli)

	prompt := BuildRolePrompt(role, scope)

	sess, err := session.New(cli, "megareview", model, effort, workdir, prompt, scope, groupID)
	if err != nil {
		return cliResult{CLI: cli, Model: model, Role: role, Err: fmt.Errorf("create session: %w", err)}
	}

	defer func() {
		if sess.Status == "running" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("cli", cli).Str("role", string(role)).Msg("starting reviewer")

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

func runConsilium(ctx context.Context, judgeCLI string, inputs []ReviewInput, scope, effort, workdir, groupID string, threshold int) (*ConsiliumOutput, error) {
	prompt := BuildConsiliumPrompt(inputs, scope, threshold)

	model := modelForCLI(judgeCLI)

	sess, err := session.New(judgeCLI, "consilium", model, effort, workdir, prompt, scope, groupID)
	if err != nil {
		return nil, fmt.Errorf("create consilium session: %w", err)
	}

	defer func() {
		if sess.Status == "running" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("cli", judgeCLI).Msg("starting consilium judge")

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
