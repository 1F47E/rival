package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

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
	commandGeminiCmd.Flags().Bool("no-queue", false, "bypass the review queue")
	commandCmd.AddCommand(commandGeminiCmd)
}

func commandGeminiAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")
	noQueue, _ := cmd.Flags().GetBool("no-queue")

	if stat, statErr := os.Stdin.Stat(); statErr == nil && (stat.Mode()&os.ModeCharDevice) != 0 {
		_, _ = fmt.Fprintln(os.Stdout, geminiUsage)
		return nil
	}

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

	// Auto-detect git scope for reviews without explicit scope.
	if parsed.IsReview && parsed.AutoScope {
		resolveGitScope(parsed, workdir)
	}

	if err := executor.GeminiPreflight(); err != nil {
		return err
	}

	mode := "raw"
	if parsed.IsReview {
		mode = "review"
	}

	sess, err := session.NewQueued("gemini", mode, config.GeminiModel, parsed.Effort, workdir, parsed.Prompt, parsed.ReviewScope, "")
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	defer func() {
		if sess.Status == "running" || sess.Status == "queued" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("effort", parsed.Effort).Str("mode", mode).Msg("starting gemini (command mode)")

	// Cancel the queue wait / child process on SIGINT/SIGTERM so the deferred Fail runs.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	release, err := waitForQueueSlot(ctx, noQueue, []*session.Session{sess}, mode, workdir)
	if err != nil {
		return err
	}
	defer release()

	result, err := executor.RunGemini(ctx, sess, parsed.Prompt, parsed.Effort, workdir, nil)
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
	if _, err := os.Stdout.Write(logData); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}

	if result.ExitCode != 0 {
		return &ExitCodeError{Code: result.ExitCode, Err: fmt.Errorf("gemini exited with code %d", result.ExitCode)}
	}

	return nil
}
