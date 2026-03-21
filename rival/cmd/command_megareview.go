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
	"github.com/1F47E/rival/internal/gitscope"
	"github.com/1F47E/rival/internal/parser"
	"github.com/1F47E/rival/internal/review"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const megareviewUsage = `Usage:
  /rival-megareview — review the entire project with both Codex and Gemini in parallel
  /rival-megareview src/api/ — review specific scope
  /rival-megareview -re xhigh src/api/ — review with xhigh reasoning effort
  /rival-megareview — show this usage info

Reasoning effort (-re): low, medium (default), high, xhigh`

var commandMegareviewCmd = &cobra.Command{
	Use:   "megareview",
	Short: "Run Codex and Gemini reviews with consilium judge",
	RunE:  commandMegareviewAction,
}

func init() {
	commandMegareviewCmd.Flags().String("workdir", ".", "working directory")
	commandCmd.AddCommand(commandMegareviewCmd)
}

func commandMegareviewAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")

	if stat, _ := os.Stdin.Stat(); (stat.Mode() & os.ModeCharDevice) != 0 {
		_, _ = fmt.Fprintln(os.Stdout, megareviewUsage)
		return nil
	}

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	parsed, err := parser.ParseReviewArgs(string(raw))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	if parsed.IsEmpty {
		_, _ = fmt.Fprintln(os.Stdout, megareviewUsage)
		return nil
	}

	// Build scope.
	scope := parsed.ReviewScope
	if parsed.AutoScope {
		files := gitscope.Resolve(workdir)
		if files != "" {
			log.Info().Str("files", files).Msg("git scope: auto-detected changed files")
			scope = strings.ReplaceAll(config.DiffReviewPreamble, "{FILES}", files)
			scope += "\nFocus your review on the changed files listed above."
		} else {
			scope = "the entire project"
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	groupID := uuid.New().String()

	result, err := review.RunMegaReview(ctx, scope, parsed.Effort, workdir, groupID)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprint(os.Stdout, review.FormatConsole(result.Output, result.Inputs, result.Threshold, result.JudgeCLI, result.Skipped))
	return nil
}
