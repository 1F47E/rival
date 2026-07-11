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

const opusUsage = `Usage:
  echo 'explain the auth flow' | rival run opus --prompt-stdin
  echo 'find bugs in src/main.go' | rival run opus --prompt-stdin --effort xhigh
  rival run opus --review 'the entire project'
  rival run opus --review src/api/
  rival run opus --review src/api/ --effort high
  rival command opus --help — show native command options

Reasoning effort (--effort): low, medium, high, xhigh (default)`

var commandOpusCmd = &cobra.Command{
	Use:   config.OpusLabel,
	Short: "Skill-facing Opus executor",
	RunE:  commandClaudeAction,
}

var commandClaudeCmd = &cobra.Command{
	Use:    "claude",
	Hidden: true,
	RunE:   commandClaudeAction,
}

func init() {
	configureCommandOpusFlags(commandOpusCmd)
	configureCommandOpusFlags(commandClaudeCmd)
	mirrorHiddenHelp(commandClaudeCmd, commandOpusCmd)
	commandCmd.AddCommand(commandOpusCmd)
	commandCmd.AddCommand(commandClaudeCmd)
}

func configureCommandOpusFlags(cmd *cobra.Command) {
	cmd.Flags().String("workdir", ".", "working directory")
	cmd.Flags().Bool("no-queue", false, "bypass the review queue")
}

func commandClaudeAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")
	noQueue, _ := cmd.Flags().GetBool("no-queue")

	// If stdin is a terminal, show usage instead of hanging.
	if stat, statErr := os.Stdin.Stat(); statErr == nil && (stat.Mode()&os.ModeCharDevice) != 0 {
		_, _ = fmt.Fprintln(os.Stdout, opusUsage)
		return nil
	}

	// Read raw args from stdin.
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	parsed, err := parser.ParseClaudeArgs(string(raw))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	if parsed.IsEmpty {
		_, _ = fmt.Fprintln(os.Stdout, opusUsage)
		return nil
	}

	// Auto-detect git scope for reviews without explicit scope.
	if parsed.IsReview && parsed.AutoScope {
		resolveGitScope(parsed, workdir)
	}

	if err := executor.ClaudePreflight(); err != nil {
		return err
	}

	mode := "raw"
	if parsed.IsReview {
		mode = "review"
	}

	sess, err := session.NewQueued("claude", mode, config.ClaudeModel, parsed.Effort, workdir, parsed.Prompt, parsed.ReviewScope, "")
	if err == nil {
		sess.Account = config.ClaudeSubscription()
	}
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	defer func() {
		if sess.Status == "running" || sess.Status == "queued" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("effort", parsed.Effort).Str("mode", mode).Msg("starting opus (command mode)")

	// Cancel the queue wait / child process on SIGINT/SIGTERM so the deferred Fail runs.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	release, err := waitForQueueSlot(ctx, noQueue, []*session.Session{sess}, mode, workdir)
	if err != nil {
		return err
	}
	defer release()

	// Bound the run itself: a hung provider CLI must not keep the slot (and the
	// detached rival) alive forever. Clock starts now, after slot promotion.
	runCtx, cancelRun := config.WithRunTimeout(ctx, 1)
	defer cancelRun()

	// No stdout mirror in command mode — skill reads final output.
	result, err := executor.RunClaude(runCtx, sess, parsed.Prompt, parsed.Effort, workdir, nil)
	if err != nil {
		if saveErr := sess.Fail(1, runTimeoutFailMsg(runCtx, err.Error())); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		return err
	}

	if result.ExitCode != 0 {
		if saveErr := sess.Fail(result.ExitCode, runTimeoutFailMsg(runCtx, fmt.Sprintf("opus exited with code %d", result.ExitCode))); saveErr != nil {
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
	if _, err := io.WriteString(os.Stdout, config.PublicRuntimeLog(sess.CLI, sess.Model, string(logData))); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}

	if result.ExitCode != 0 {
		if hint := executor.ClaudeAuthHint(sess.LogFile); hint != "" {
			_, _ = fmt.Fprintln(os.Stdout, "\n"+hint)
		}
		return &ExitCodeError{Code: result.ExitCode, Err: fmt.Errorf("opus exited with code %d", result.ExitCode)}
	}

	return nil
}
