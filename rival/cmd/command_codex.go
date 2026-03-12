package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/executor"
	"github.com/1F47E/rival/internal/parser"
	"github.com/1F47E/rival/internal/session"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const codexUsage = `Usage:
  /rival-codex 'explain the auth flow' — run any prompt via codex
  /rival-codex -re xhigh 'find bugs in src/main.go' — run with xhigh reasoning effort
  /rival-codex review — ruthless code review of the entire project
  /rival-codex review src/api/ — review specific scope
  /rival-codex -re xhigh review src/api/ — review with xhigh reasoning
  /rival-codex — show this usage info

Reasoning effort (-re): low, medium (default), high, xhigh`

var commandCodexCmd = &cobra.Command{
	Use:   "codex",
	Short: "Skill-facing codex executor",
	RunE:  commandCodexAction,
}

func init() {
	commandCodexCmd.Flags().String("workdir", ".", "working directory")
	commandCmd.AddCommand(commandCodexCmd)
}

func commandCodexAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")

	// Read raw args from stdin.
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	parsed, err := parser.ParseCodexArgs(string(raw))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	if parsed.IsEmpty {
		_, _ = fmt.Fprintln(os.Stdout, codexUsage)
		return nil
	}

	if err := executor.CodexPreflight(); err != nil {
		return err
	}

	mode := "raw"
	if parsed.IsReview {
		mode = "review"
	}

	sess, err := session.New("codex", mode, config.CodexModel, parsed.Effort, workdir, parsed.Prompt, parsed.ReviewScope, "")
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	log.Info().Str("session", sess.ID).Str("effort", parsed.Effort).Str("mode", mode).Msg("starting codex (command mode)")

	// No stdout mirror in command mode — skill reads final output.
	result, err := executor.RunCodex(context.Background(), sess, parsed.Prompt, parsed.Effort, workdir, nil)
	if err != nil {
		if saveErr := sess.Fail(1, err.Error()); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		return err
	}

	if result.ExitCode != 0 {
		if saveErr := sess.Fail(result.ExitCode, fmt.Sprintf("codex exited with code %d", result.ExitCode)); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
	} else {
		if saveErr := sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session completion")
		}
	}

	// Print log file contents to stdout for the skill to capture.
	logData, err := os.ReadFile(sess.LogFile)
	if err != nil {
		return fmt.Errorf("read log file: %w", err)
	}
	_, _ = fmt.Fprint(os.Stdout, string(logData))

	if result.ExitCode != 0 {
		return &ExitCodeError{Code: result.ExitCode, Err: fmt.Errorf("codex exited with code %d", result.ExitCode)}
	}

	return nil
}
