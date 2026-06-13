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

var runGeminiCmd = &cobra.Command{
	Use:   "gemini",
	Short: "Run Gemini CLI",
	RunE:  runGeminiAction,
}

func init() {
	runGeminiCmd.Flags().String("effort", config.DefaultEffort, "reasoning effort (low, medium, high, xhigh)")
	runGeminiCmd.Flags().String("workdir", ".", "working directory")
	runGeminiCmd.Flags().Bool("prompt-stdin", false, "read prompt from stdin")
	runGeminiCmd.Flags().String("review", "", "review scope (enables review mode)")
	runGeminiCmd.Flags().Bool("no-queue", false, "bypass the review queue")
	runCmd.AddCommand(runGeminiCmd)
}

func runGeminiAction(cmd *cobra.Command, args []string) error {
	effort, _ := cmd.Flags().GetString("effort")
	workdir, _ := cmd.Flags().GetString("workdir")
	promptStdin, _ := cmd.Flags().GetBool("prompt-stdin")
	reviewScope, _ := cmd.Flags().GetString("review")
	noQueue, _ := cmd.Flags().GetBool("no-queue")

	if !config.IsValidEffort(effort) {
		return fmt.Errorf("invalid effort level %q, must be one of: %v", effort, config.ValidEfforts)
	}

	if err := executor.GeminiPreflight(); err != nil {
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

	sess, err := session.NewQueued("gemini", mode, config.GeminiModel, effort, workdir, prompt, reviewScope, "")
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	defer func() {
		if sess.Status == "running" || sess.Status == "queued" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("effort", effort).Msg("starting gemini")

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

	result, err := executor.RunGemini(runCtx, sess, prompt, effort, workdir, os.Stdout)
	if err != nil {
		if saveErr := sess.Fail(1, runTimeoutFailMsg(runCtx, err.Error())); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		return err
	}

	if result.ExitCode != 0 {
		if saveErr := sess.Fail(result.ExitCode, runTimeoutFailMsg(runCtx, fmt.Sprintf("gemini exited with code %d", result.ExitCode))); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		return &ExitCodeError{Code: result.ExitCode, Err: fmt.Errorf("gemini exited with code %d", result.ExitCode)}
	}

	if saveErr := sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines); saveErr != nil {
		log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session completion")
	}
	return nil
}
