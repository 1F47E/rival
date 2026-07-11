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

const solUsage = `Usage:
  /rival-sol 'explain the auth flow' — run any prompt with Sol
  /rival-sol -re ultra 'find bugs in src/main.go' — use ultra reasoning
  /rival-sol review — ruthless code review of the entire project
  /rival-sol review src/api/ — review specific scope
  /rival-sol -re ultra review src/api/ — review with ultra reasoning
  /rival-sol — show this usage info

Reasoning effort (-re): low, medium, high (default), ultra`

var commandSolCmd = &cobra.Command{
	Use:   config.SolLabel,
	Short: "Skill-facing Sol executor",
	RunE:  commandGPT56SolAction,
}

// Retained for scripts created before the short model-named command was introduced.
var commandGPT56SolCmd = &cobra.Command{
	Use:    config.GPT56SolModel,
	Hidden: true,
	RunE:   commandGPT56SolAction,
}

var commandCodexCmd = &cobra.Command{
	Use:    "codex",
	Hidden: true,
	RunE:   commandGPT56SolAction,
}

func init() {
	configureCommandGPT56SolFlags(commandSolCmd)
	configureCommandGPT56SolFlags(commandGPT56SolCmd)
	configureCommandGPT56SolFlags(commandCodexCmd)
	mirrorHiddenHelp(commandGPT56SolCmd, commandSolCmd)
	mirrorHiddenHelp(commandCodexCmd, commandSolCmd)
	commandCmd.AddCommand(commandSolCmd)
	commandCmd.AddCommand(commandGPT56SolCmd)
	commandCmd.AddCommand(commandCodexCmd)
}

func configureCommandGPT56SolFlags(cmd *cobra.Command) {
	cmd.Flags().String("workdir", ".", "working directory")
	cmd.Flags().Bool("no-queue", false, "bypass the review queue")
}

func commandGPT56SolAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")
	noQueue, _ := cmd.Flags().GetBool("no-queue")

	// If stdin is a terminal, show usage instead of hanging.
	if stat, statErr := os.Stdin.Stat(); statErr == nil && (stat.Mode()&os.ModeCharDevice) != 0 {
		_, _ = fmt.Fprintln(os.Stdout, solUsage)
		return nil
	}

	// Read raw args from stdin.
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	parsed, err := parser.ParseGPT56SolArgs(string(raw))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	if parsed.IsEmpty {
		_, _ = fmt.Fprintln(os.Stdout, solUsage)
		return nil
	}

	// Auto-detect git scope for reviews without explicit scope.
	if parsed.IsReview && parsed.AutoScope {
		resolveGitScope(parsed, workdir)
	}

	if err := executor.CodexPreflight(); err != nil {
		return err
	}

	mode := "raw"
	if parsed.IsReview {
		mode = "review"
	}

	sess, err := session.NewQueued("codex", mode, config.CodexModel, parsed.Effort, workdir, parsed.Prompt, parsed.ReviewScope, "")
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	defer func() {
		if sess.Status == "running" || sess.Status == "queued" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("effort", parsed.Effort).Str("mode", mode).Msg("starting sol (command mode)")

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

	// No stdout mirror in command mode — skill reads final output.
	result, err := executor.RunCodex(runCtx, sess, parsed.Prompt, parsed.Effort, workdir, nil)
	if err != nil {
		if saveErr := sess.Fail(1, runTimeoutFailMsg(runCtx, err.Error())); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		return err
	}

	if result.ExitCode != 0 {
		if saveErr := sess.Fail(result.ExitCode, runTimeoutFailMsg(runCtx, fmt.Sprintf("%s exited with code %d", config.SolLabel, result.ExitCode))); saveErr != nil {
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
		return &ExitCodeError{Code: result.ExitCode, Err: fmt.Errorf("%s exited with code %d", config.SolLabel, result.ExitCode)}
	}

	return nil
}
