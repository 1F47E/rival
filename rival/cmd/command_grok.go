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

const grokUsage = `Usage:
  /rival-grok 'explain the auth flow' — run any prompt with Grok
  /rival-grok -re high 'find bugs in src/main.go' — pick the reasoning level
  /rival-grok review — ruthless code review of the entire project
  /rival-grok review src/api/ — review specific scope
  /rival-grok -re high review src/api/ — review with high reasoning
  /rival-grok — show this usage info

Reasoning effort (-re): low, medium, high (levels above high clamp to high).
Omitted uses efforts.grok from ~/.rival/config.yaml (built-in: high).
Review mode runs read-only sandboxed; raw prompts can edit files in the workdir.`

var commandGrokCmd = &cobra.Command{
	Use:   config.GrokLabel,
	Short: "Skill-facing Grok executor",
	RunE:  commandGrokAction,
}

func init() {
	commandGrokCmd.Flags().String("workdir", ".", "working directory")
	commandGrokCmd.Flags().Bool("no-queue", false, "bypass the review queue")
	commandCmd.AddCommand(commandGrokCmd)
}

// sessionMode names the session mode for a parsed invocation. The review
// sandbox is selected from this same value, so the two can never disagree.
func sessionMode(isReview bool) string {
	if isReview {
		return "review"
	}
	return "raw"
}

func commandGrokAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")
	noQueue, _ := cmd.Flags().GetBool("no-queue")

	// If stdin is a terminal, show usage instead of hanging.
	if stat, statErr := os.Stdin.Stat(); statErr == nil && (stat.Mode()&os.ModeCharDevice) != 0 {
		_, _ = fmt.Fprintln(os.Stdout, grokUsage)
		return nil
	}

	// Read raw args from stdin.
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	parsed, err := parser.ParseGrokArgs(string(raw))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	if parsed.IsEmpty {
		_, _ = fmt.Fprintln(os.Stdout, grokUsage)
		return nil
	}

	// Auto-detect git scope for reviews without explicit scope.
	if parsed.IsReview && parsed.AutoScope {
		resolveGitScope(parsed, workdir)
	}

	effort, err := config.ResolveEffort(config.GrokModel, parsed.Effort, config.DefaultReviewEffort)
	if err != nil {
		return err
	}

	if err := executor.GrokPreflight(); err != nil {
		return err
	}

	mode := sessionMode(parsed.IsReview)

	sess, err := session.NewQueued(config.GrokLabel, mode, config.GrokModel, effort, workdir, parsed.Prompt, parsed.ReviewScope, "")
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	defer func() {
		if sess.Status == "running" || sess.Status == "queued" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("effort", effort).Str("mode", mode).Msg("starting grok (command mode)")

	// Cancel the queue wait / child process on SIGINT/SIGTERM so the deferred Fail runs.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	release, err := waitForQueueSlot(ctx, noQueue, []*session.Session{sess}, mode, workdir)
	if err != nil {
		return err
	}
	defer release()

	// Bound the run itself: a hung model runtime must not keep the slot (and the
	// detached rival) alive forever. Clock starts now, after slot promotion.
	runCtx, cancelRun := config.WithRunTimeout(ctx, 1)
	defer cancelRun()

	// The sandbox flag follows the session mode: a review must never run with
	// write access. No stdout mirror in command mode — skill reads final output.
	result, err := executor.RunGrok(runCtx, sess, parsed.Prompt, effort, workdir, mode == "review", nil)
	if err != nil {
		if saveErr := sess.Fail(1, runTimeoutFailMsg(runCtx, err.Error())); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		return err
	}

	if result.ExitCode != 0 {
		if saveErr := sess.Fail(result.ExitCode, runTimeoutFailMsg(runCtx, fmt.Sprintf("%s exited with code %d", config.GrokLabel, result.ExitCode))); saveErr != nil {
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
		return &ExitCodeError{Code: result.ExitCode, Err: fmt.Errorf("%s exited with code %d", config.GrokLabel, result.ExitCode)}
	}

	return nil
}
