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

var runGrokCmd = &cobra.Command{
	Use:   config.GrokLabel,
	Short: "Run Grok",
	RunE:  runGrokAction,
}

func init() {
	runGrokCmd.Flags().String("effort", "", "reasoning effort override: low, medium, high (ultra clamps to high)")
	runGrokCmd.Flags().String("workdir", ".", "working directory")
	runGrokCmd.Flags().Bool("prompt-stdin", false, "read prompt from stdin")
	runGrokCmd.Flags().String("review", "", "review scope (enables review mode)")
	runGrokCmd.Flags().Bool("no-queue", false, "bypass the review queue")
	runCmd.AddCommand(runGrokCmd)
}

func runGrokAction(cmd *cobra.Command, args []string) error {
	effort, _ := cmd.Flags().GetString("effort")
	workdir, _ := cmd.Flags().GetString("workdir")
	promptStdin, _ := cmd.Flags().GetBool("prompt-stdin")
	reviewScope, _ := cmd.Flags().GetString("review")
	noQueue, _ := cmd.Flags().GetBool("no-queue")

	if effort != "" && !config.IsValidReviewEffort(effort) {
		return fmt.Errorf("invalid effort level %q, must be one of: %v", effort, config.ReviewEfforts)
	}
	effort, err := config.ResolveEffort(config.GrokModel, effort, config.DefaultReviewEffort)
	if err != nil {
		return err
	}
	// grok clamps rival's richer ladder onto low/medium/high. Clamp here so the
	// session records the effort actually sent, not the one that was asked for.
	effort, err = executor.GrokEffort(effort)
	if err != nil {
		return err
	}

	if err := executor.GrokPreflight(); err != nil {
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

	sess, err := session.NewQueued(config.GrokLabel, mode, config.GrokModel, effort, workdir, prompt, reviewScope, "")
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	defer func() {
		if sess.Status == "running" || sess.Status == "queued" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("effort", effort).Str("mode", mode).Msg("starting grok")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	release, err := waitForQueueSlot(ctx, noQueue, []*session.Session{sess}, mode, workdir)
	if err != nil {
		return err
	}
	defer release()

	// Bound the run so a hung model runtime cannot wait forever.
	runCtx, cancelRun := config.WithRunTimeout(ctx, 1)
	defer cancelRun()

	// The sandbox flag follows the session mode: a review must never run with
	// write access.
	result, err := executor.RunGrok(runCtx, sess, prompt, effort, workdir, mode == "review", os.Stdout)
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
		return &ExitCodeError{Code: result.ExitCode, Err: fmt.Errorf("%s exited with code %d", config.GrokLabel, result.ExitCode)}
	}

	if saveErr := sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines); saveErr != nil {
		log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session completion")
	}
	return nil
}
