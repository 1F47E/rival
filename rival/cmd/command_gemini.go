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

const geminiUsage = `Usage:
  /rival-gemini 'explain the auth flow' — run any prompt via gemini
  /rival-gemini -re high 'analyze this complex algorithm' — use high thinking budget
  /rival-gemini review — ruthless code review of the entire project
  /rival-gemini review src/api/ — review specific scope
  /rival-gemini -re xhigh review src/api/ — review with xhigh reasoning
  /rival-gemini — show this usage info

Reasoning effort (-re): low, medium (default), high, xhigh`

var commandGeminiCmd = &cobra.Command{
	Use:   "gemini",
	Short: "Skill-facing gemini executor",
	RunE:  commandGeminiAction,
}

func init() {
	commandGeminiCmd.Flags().String("workdir", ".", "working directory")
	commandCmd.AddCommand(commandGeminiCmd)
}

func commandGeminiAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	parsed, err := parser.ParseGeminiArgs(string(raw))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	if parsed.IsEmpty {
		_, _ = fmt.Fprintln(os.Stdout, geminiUsage)
		return nil
	}

	if err := executor.GeminiPreflight(); err != nil {
		return err
	}

	mode := "raw"
	if parsed.IsReview {
		mode = "review"
	}

	sess, err := session.New("gemini", mode, config.GeminiModel, parsed.Effort, workdir, parsed.Prompt, parsed.ReviewScope, "")
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	log.Info().Str("session", sess.ID).Str("effort", parsed.Effort).Str("mode", mode).Msg("starting gemini (command mode)")

	result, err := executor.RunGemini(context.Background(), sess, parsed.Prompt, parsed.Effort, workdir, nil)
	if err != nil {
		if saveErr := sess.Fail(1, err.Error()); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		return err
	}

	if result.ExitCode != 0 {
		if saveErr := sess.Fail(result.ExitCode, fmt.Sprintf("gemini exited with code %d", result.ExitCode)); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
	} else {
		if saveErr := sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session completion")
		}
	}

	logData, err := os.ReadFile(sess.LogFile)
	if err != nil {
		return fmt.Errorf("read log file: %w", err)
	}
	_, _ = fmt.Fprint(os.Stdout, string(logData))

	if result.ExitCode != 0 {
		return &ExitCodeError{Code: result.ExitCode, Err: fmt.Errorf("gemini exited with code %d", result.ExitCode)}
	}

	return nil
}
