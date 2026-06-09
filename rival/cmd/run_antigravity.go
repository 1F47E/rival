package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/executor"
	"github.com/1F47E/rival/internal/session"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var runAntigravityCmd = &cobra.Command{
	Use:   "antigravity",
	Short: "Run Antigravity CLI",
	RunE:  runAntigravityAction,
}

func init() {
	runAntigravityCmd.Flags().String("effort", config.DefaultEffort, "reasoning effort (accepted but ignored by agy)")
	runAntigravityCmd.Flags().String("workdir", ".", "working directory")
	runAntigravityCmd.Flags().Bool("prompt-stdin", false, "read prompt from stdin")
	runAntigravityCmd.Flags().String("review", "", "review scope (enables review mode)")
	runCmd.AddCommand(runAntigravityCmd)
}

func runAntigravityAction(cmd *cobra.Command, args []string) error {
	effort, _ := cmd.Flags().GetString("effort")
	workdir, _ := cmd.Flags().GetString("workdir")
	promptStdin, _ := cmd.Flags().GetBool("prompt-stdin")
	reviewScope, _ := cmd.Flags().GetString("review")

	if !config.IsValidEffort(effort) {
		return fmt.Errorf("invalid effort level %q, must be one of: %v", effort, config.ValidEfforts)
	}

	if err := executor.AntigravityPreflight(); err != nil {
		return err
	}

	var prompt string
	mode := "raw"

	if cmd.Flags().Changed("review") {
		mode = "review"
		scope := reviewScope
		if scope == "" {
			scope = "the entire project"
		}
		prompt = strings.ReplaceAll(config.ReviewPrompt, "{SCOPE}", scope)
	} else if promptStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		prompt = string(data)
	} else {
		return fmt.Errorf("provide --prompt-stdin or --review")
	}

	if prompt == "" {
		return fmt.Errorf("empty prompt")
	}

	sess, err := session.New("antigravity", mode, config.AntigravityModel, effort, workdir, prompt, reviewScope, "")
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	defer func() {
		if sess.Status == "running" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("effort", effort).Msg("starting antigravity")

	result, err := executor.RunAntigravity(context.Background(), sess, prompt, effort, workdir, os.Stdout)
	if err != nil {
		if saveErr := sess.Fail(1, err.Error()); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		return err
	}

	if result.ExitCode != 0 {
		if saveErr := sess.Fail(result.ExitCode, fmt.Sprintf("antigravity exited with code %d", result.ExitCode)); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		return &ExitCodeError{Code: result.ExitCode, Err: fmt.Errorf("antigravity exited with code %d", result.ExitCode)}
	}

	// agy exits 0 on a 429 with empty stdout, writing the quota error only to
	// its log/stderr. Read the session log (captures stdout+stderr) and fail
	// loudly so a quota-blocked run is not mistaken for a clean, empty review.
	if logData, readErr := os.ReadFile(sess.LogFile); readErr == nil && executor.IsQuotaExhausted(string(logData)) {
		msg := "antigravity hit provider quota/rate limit (429): authenticate to a quota-bearing account (agy login) or wait for quota reset"
		if saveErr := sess.Fail(1, msg); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		log.Error().Str("session", sess.ID).Msg(msg)
		return &ExitCodeError{Code: 1, Err: fmt.Errorf("%s", msg)}
	}

	if saveErr := sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines); saveErr != nil {
		log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session completion")
	}
	return nil
}
