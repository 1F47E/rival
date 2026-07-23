package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/executor"
	"github.com/1F47E/rival/internal/session"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var runFableCmd = &cobra.Command{
	Use:   config.FableLabel,
	Short: "Run Fable",
	RunE:  runFableAction,
}

func init() {
	configureRunFableFlags(runFableCmd)
	runCmd.AddCommand(runFableCmd)
}

func configureRunFableFlags(cmd *cobra.Command) {
	cmd.Flags().String("effort", "", "reasoning effort override (low, medium, high, xhigh)")
	cmd.Flags().String("workdir", ".", "working directory")
	cmd.Flags().Bool("prompt-stdin", false, "read prompt from stdin")
	cmd.Flags().String("review", "", "review scope (enables review mode)")
	cmd.Flags().Bool("no-queue", false, "bypass the review queue")
}

func runFableAction(cmd *cobra.Command, args []string) error {
	effort, _ := cmd.Flags().GetString("effort")
	workdir, _ := cmd.Flags().GetString("workdir")
	promptStdin, _ := cmd.Flags().GetBool("prompt-stdin")
	reviewScope, _ := cmd.Flags().GetString("review")
	noQueue, _ := cmd.Flags().GetBool("no-queue")

	if effort != "" && !config.IsValidEffort(effort) {
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
	effort, err := config.ResolveEffort(config.FableModel, effort, "")
	if err != nil {
		return err
	}

	sess, err := session.NewQueued("claude", mode, config.FableModel, effort, workdir, prompt, reviewScope, "")
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	sess.Account = config.ClaudeSubscription()

	defer func() {
		if sess.Status == "running" || sess.Status == "queued" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("effort", effort).Msg("starting fable")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	release, err := waitForQueueSlot(ctx, noQueue, []*session.Session{sess}, mode, workdir)
	if err != nil {
		return err
	}
	defer release()

	// Bound the run so a hung provider CLI cannot wait forever.
	runCtx, cancelRun := config.WithRunTimeout(ctx, 1)
	defer cancelRun()

	result, err := executor.RunFable(runCtx, sess, prompt, effort, workdir, os.Stdout)
	if err != nil {
		if saveErr := sess.Fail(1, runTimeoutFailMsg(runCtx, err.Error())); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		return err
	}

	if result.ExitCode != 0 {
		if saveErr := sess.Fail(result.ExitCode, runTimeoutFailMsg(runCtx, fmt.Sprintf("fable exited with code %d", result.ExitCode))); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		if hint := executor.ClaudeAuthHint(sess.LogFile); hint != "" {
			_, _ = fmt.Fprintln(os.Stderr, hint)
		}
		return &ExitCodeError{Code: result.ExitCode, Err: fmt.Errorf("fable exited with code %d", result.ExitCode)}
	}

	if saveErr := sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines); saveErr != nil {
		log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session completion")
	}
	return nil
}
