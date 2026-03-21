package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/gitscope"
	"github.com/1F47E/rival/internal/review"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var reviewCmd = &cobra.Command{
	Use:   "review [scope]",
	Short: "Run Codex + Gemini code review with consilium judge",
	Long: `Run both Codex and Gemini code reviews in parallel with role-specific prompts,
then merge findings via a consilium judge for a unified verdict.

Without a scope argument, auto-detects changed files via git:
  1. Dirty files (staged + unstaged + untracked) → review those
  2. Last commit (if clean) → review files from HEAD
  3. Full project → fallback if no git changes found

With a scope argument, reviews exactly that scope.`,
	RunE: reviewAction,
}

func init() {
	reviewCmd.Flags().String("effort", config.DefaultEffort, "reasoning effort (low, medium, high, xhigh)")
	reviewCmd.Flags().String("workdir", ".", "working directory")
	rootCmd.AddCommand(reviewCmd)
}

func reviewAction(cmd *cobra.Command, args []string) error {
	effort, _ := cmd.Flags().GetString("effort")
	workdir, _ := cmd.Flags().GetString("workdir")

	if !config.IsValidEffort(effort) {
		return fmt.Errorf("invalid effort level %q, must be one of: %v", effort, config.ValidEfforts)
	}

	// Build scope from args or auto-detect via git.
	scope := strings.Join(args, " ")
	if scope == "" {
		files := gitscope.Resolve(workdir)
		if files != "" {
			log.Info().Str("files", files).Msg("git scope: auto-detected changed files")
			scope = config.DiffReviewPreamble
			scope = strings.ReplaceAll(scope, "{FILES}", files)
			scope += "\nFocus your review on the changed files listed above."
		} else {
			scope = "the entire project"
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	groupID := uuid.New().String()

	result, err := review.RunMegaReview(ctx, scope, effort, workdir, groupID)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprint(os.Stdout, review.FormatConsole(result.Output, result.Inputs, result.Threshold, result.JudgeCLI, result.Skipped))
	return nil
}
