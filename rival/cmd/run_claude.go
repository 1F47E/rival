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

var runClaudeCmd = &cobra.Command{
	Use:   "claude",
	Short: "Run Claude CLI",
	RunE:  runClaudeAction,
}

func init() {
	runClaudeCmd.Flags().String("effort", config.DefaultEffort, "reasoning effort (low, medium, high, xhigh)")
	runClaudeCmd.Flags().String("workdir", ".", "working directory")
	runClaudeCmd.Flags().Bool("prompt-stdin", false, "read prompt from stdin")
	runClaudeCmd.Flags().String("review", "", "review scope (enables review mode)")
	runCmd.AddCommand(runClaudeCmd)
}

func runClaudeAction(cmd *cobra.Command, args []string) error {
	effort, _ := cmd.Flags().GetString("effort")
	workdir, _ := cmd.Flags().GetString("workdir")
	promptStdin, _ := cmd.Flags().GetBool("prompt-stdin")
	reviewScope, _ := cmd.Flags().GetString("review")

	if !config.IsValidEffort(effort) {
		return fmt.Errorf("invalid effort level %q, must be one of: %v", effort, config.ValidEfforts)
	}

	if err := executor.ClaudePreflight(); err != nil {
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

	sess, err := session.New("claude", mode, config.ClaudeModel, effort, workdir, prompt, reviewScope, "")
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	defer func() {
		if sess.Status == "running" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("effort", effort).Msg("starting claude")

	result, err := executor.RunClaude(context.Background(), sess, prompt, effort, workdir, os.Stdout)
	if err != nil {
		if saveErr := sess.Fail(1, err.Error()); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		return err
	}

	if result.ExitCode != 0 {
		if saveErr := sess.Fail(result.ExitCode, fmt.Sprintf("claude exited with code %d", result.ExitCode)); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		return &ExitCodeError{Code: result.ExitCode, Err: fmt.Errorf("claude exited with code %d", result.ExitCode)}
	}

	if saveErr := sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines); saveErr != nil {
		log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session completion")
	}
	return nil
}
