package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/executor"
	"github.com/1F47E/rival/internal/gitscope"
	"github.com/1F47E/rival/internal/parser"
	"github.com/1F47E/rival/internal/session"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var reviewCmd = &cobra.Command{
	Use:   "review [scope]",
	Short: "Run Codex + Gemini code review in parallel",
	Long: `Run both Codex and Gemini code reviews in parallel (megareview).

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
	reviewCmd.Flags().String("cli", "", "run only one CLI: codex or gemini (default: both)")
	rootCmd.AddCommand(reviewCmd)
}

func reviewAction(cmd *cobra.Command, args []string) error {
	effort, _ := cmd.Flags().GetString("effort")
	workdir, _ := cmd.Flags().GetString("workdir")
	cliOnly, _ := cmd.Flags().GetString("cli")

	if !config.IsValidEffort(effort) {
		return fmt.Errorf("invalid effort level %q, must be one of: %v", effort, config.ValidEfforts)
	}

	// Build scope from args or auto-detect via git.
	scope := strings.Join(args, " ")
	parsed := &parser.ParseResult{
		Effort:   effort,
		IsReview: true,
	}

	if scope == "" {
		// Auto-detect via git.
		files := gitscope.Resolve(workdir)
		if files != "" {
			log.Info().Str("files", files).Msg("git scope: auto-detected changed files")
			parsed.ReviewScope = files
			preamble := strings.ReplaceAll(config.DiffReviewPreamble, "{FILES}", files)
			review := strings.ReplaceAll(config.ReviewPrompt, "{SCOPE}", "the changed files listed above")
			parsed.Prompt = preamble + review
		} else {
			parsed.ReviewScope = "the entire project"
			parsed.Prompt = strings.ReplaceAll(config.ReviewPrompt, "{SCOPE}", "the entire project")
		}
	} else {
		parsed.ReviewScope = scope
		parsed.Prompt = strings.ReplaceAll(config.ReviewPrompt, "{SCOPE}", scope)
	}

	// Validate --cli flag.
	switch cliOnly {
	case "", "codex", "gemini":
	default:
		return fmt.Errorf("invalid --cli value %q, must be codex or gemini", cliOnly)
	}

	// Preflight.
	codexOK := cliOnly != "gemini"
	geminiOK := cliOnly != "codex"

	if codexOK {
		if err := executor.CodexPreflight(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: codex unavailable: %v\n", err)
			codexOK = false
		}
	}
	if geminiOK {
		if err := executor.GeminiPreflight(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warning: gemini unavailable: %v\n", err)
			geminiOK = false
		}
	}
	if !codexOK && !geminiOK {
		return fmt.Errorf("no CLIs available")
	}

	groupID := uuid.New().String()

	var wg sync.WaitGroup
	results := make(chan reviewResult, 2)

	if codexOK {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- runReviewCLI("codex", groupID, parsed, workdir)
		}()
	}
	if geminiOK {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- runReviewCLI("gemini", groupID, parsed, workdir)
		}()
	}

	wg.Wait()
	close(results)

	anySuccess := false
	for r := range results {
		_, _ = fmt.Fprintf(os.Stdout, "\n=== %s REVIEW ===\n", strings.ToUpper(r.cli))
		if r.err != nil {
			_, _ = fmt.Fprintf(os.Stdout, "ERROR: %v\n", r.err)
		} else {
			_, _ = os.Stdout.Write(r.log)
			if r.exCode == 0 {
				anySuccess = true
			}
		}
	}

	if !anySuccess {
		return &ExitCodeError{Code: 1, Err: fmt.Errorf("all reviews failed")}
	}
	return nil
}

func runReviewCLI(cli, groupID string, parsed *parser.ParseResult, workdir string) reviewResult {
	model := config.CodexModel
	if cli == "gemini" {
		model = config.GeminiModel
	}

	sess, err := session.New(cli, "review", model, parsed.Effort, workdir, parsed.Prompt, parsed.ReviewScope, groupID)
	if err != nil {
		return reviewResult{cli: cli, err: fmt.Errorf("create session: %w", err)}
	}

	defer func() {
		if sess.Status == "running" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("cli", cli).Msg("starting review")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var result *executor.Result
	switch cli {
	case "codex":
		result, err = executor.RunCodex(ctx, sess, parsed.Prompt, parsed.Effort, workdir, nil)
	case "gemini":
		result, err = executor.RunGemini(ctx, sess, parsed.Prompt, parsed.Effort, workdir, nil)
	default:
		return reviewResult{cli: cli, err: fmt.Errorf("unsupported cli: %s", cli)}
	}

	if err != nil {
		_ = sess.Fail(1, err.Error())
		return reviewResult{cli: cli, err: err}
	}

	if result.ExitCode != 0 {
		_ = sess.Fail(result.ExitCode, fmt.Sprintf("%s exited with code %d", cli, result.ExitCode))
	} else {
		_ = sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines)
	}

	logData, err := os.ReadFile(sess.LogFile)
	if err != nil {
		return reviewResult{cli: cli, err: fmt.Errorf("read log: %w", err), exCode: result.ExitCode}
	}

	return reviewResult{cli: cli, log: logData, exCode: result.ExitCode}
}
