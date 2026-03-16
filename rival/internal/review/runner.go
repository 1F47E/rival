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

// RunResult holds the outcome of the full mega review pipeline.
type RunResult struct {
	Output    *ConsiliumOutput
	Inputs    []ReviewInput
	Threshold int
	JudgeCLI  string
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

	// Preflight.
	codexOK := true
	geminiOK := true
	if err := executor.CodexPreflight(); err != nil {
		log.Warn().Err(err).Msg("codex unavailable")
		codexOK = false
	}
	if err := executor.GeminiPreflight(); err != nil {
		log.Warn().Err(err).Msg("gemini unavailable")
		geminiOK = false
	}
	if !codexOK && !geminiOK {
		return nil, fmt.Errorf("no CLI reviewers available")
	}

	// Determine which CLI to use for the consilium judge (prefer codex, fallback to gemini).
	judgeCLI := "codex"
	if !codexOK {
		judgeCLI = "gemini"
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
	if geminiOK {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- runReviewer(ctx, "gemini", groupID, scope, effort, workdir)
		}()
	}

	wg.Wait()
	close(results)

	// Collect and parse reviewer outputs.
	var inputs []ReviewInput
	for r := range results {
		if r.Err != nil {
			log.Error().Str("cli", r.CLI).Err(r.Err).Msg("reviewer failed")
			continue
		}
		if r.ExitCode != 0 {
			log.Error().Str("cli", r.CLI).Int("exit_code", r.ExitCode).Msg("reviewer exited with error")
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
		return nil, fmt.Errorf("all reviewers failed")
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
	}, nil
}

func runReviewer(ctx context.Context, cli, groupID, scope, effort, workdir string) cliResult {
	role := RoleForCLI(cli)
	model := config.CodexModel
	if cli == "gemini" {
		model = config.GeminiModel
	}

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
	case "gemini":
		result, err = executor.RunGemini(ctx, sess, prompt, effort, workdir, nil)
	default:
		return cliResult{CLI: cli, Model: model, Role: role, Err: fmt.Errorf("unsupported cli: %s", cli)}
	}

	if err != nil {
		_ = sess.Fail(1, err.Error())
		return cliResult{CLI: cli, Model: model, Role: role, Err: err}
	}

	if result.ExitCode != 0 {
		_ = sess.Fail(result.ExitCode, fmt.Sprintf("%s exited with code %d", cli, result.ExitCode))
	} else {
		_ = sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines)
	}

	// Read the log file to get raw output.
	logData, err := os.ReadFile(sess.LogFile)
	if err != nil {
		return cliResult{CLI: cli, Model: model, Role: role, Err: fmt.Errorf("read log: %w", err), ExitCode: result.ExitCode}
	}

	return cliResult{
		CLI:       cli,
		Model:     model,
		Role:      role,
		RawOutput: string(logData),
		ExitCode:  result.ExitCode,
	}
}

func runConsilium(ctx context.Context, judgeCLI string, inputs []ReviewInput, scope, effort, workdir, groupID string, threshold int) (*ConsiliumOutput, error) {
	prompt := BuildConsiliumPrompt(inputs, scope, threshold)

	model := config.CodexModel
	if judgeCLI == "gemini" {
		model = config.GeminiModel
	}

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
	case "gemini":
		result, err = executor.RunGemini(ctx, sess, prompt, effort, workdir, nil)
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

	_ = sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines)

	logData, err := os.ReadFile(sess.LogFile)
	if err != nil {
		return nil, fmt.Errorf("read consilium log: %w", err)
	}

	output, err := ParseConsiliumOutput(string(logData))
	if err != nil {
		// Dump raw for debugging.
		log.Error().Str("raw", truncate(string(logData), 500)).Msg("consilium parse failed")
		return nil, fmt.Errorf("parse consilium output: %w", err)
	}

	return output, nil
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
