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
	"github.com/1F47E/rival/internal/parser"
	"github.com/1F47E/rival/internal/review"
	"github.com/1F47E/rival/internal/session"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const antislopUsage = `Usage:
  /rival-antislop — quality-only review of the changed files (git auto-detect)
  /rival-antislop src/api/ — review a specific scope
  /rival-antislop -m fable -re high src/ — pick model and reasoning effort
  /rival-antislop-plan path/to/plan.md — cut list for a plan/spec document
  rival command antislop --help — show native command options

Antislop hunts slop and over-engineering — reuse/DRY, simplification,
efficiency, altitude, backward-compat hoarding, library reinvention, comment
and wrapper slop — and returns a leanness rating (1-10) plus a cut list. It
never reports bugs; use the code review commands for that.

Input starts plan mode with the token "plan" followed by the document path
(everything after the token is the path). Anything else is a code-review
scope. "--" ends option parsing AND takes the rest verbatim as a code scope,
so "-- plan handling in the parser" reviews code; "./plan" reviews a
directory named plan. Default model is sol; -m accepts sol and fable
(comma-separated). Default reasoning effort is xhigh; override with
-re/--effort or per model in ~/.rival/config.yaml.`

var defaultAntislopModels = []string{config.SolLabel}

var commandAntislopCmd = &cobra.Command{
	Use:   "antislop",
	Short: "Quality-only slop & over-engineering review (code or plan)",
	RunE:  commandAntislopAction,
}

func init() {
	commandAntislopCmd.Flags().String("workdir", ".", "working directory")
	commandAntislopCmd.Flags().Bool("no-queue", false, "bypass the review queue")
	commandAntislopCmd.Flags().StringSliceP("model", "m", defaultAntislopModels, "antislop model(s): sol, fable (comma-separated)")
	commandAntislopCmd.Flags().String("effort", "", "override reasoning effort for every selected model: low, medium, high, ultra")
	commandCmd.AddCommand(commandAntislopCmd)
}

// antislopMode splits the parsed scope into plan mode (leading "plan" token,
// everything after it is the document path — spaces preserved) or code mode.
// A bare "plan" with no path is a usage error rather than an empty-path stat
// failure. autoScope means no scope tokens were given at all, which is always
// code mode. escaped means the scope came after "--", which takes it verbatim
// as a code scope — so "-- plan handling in the parser" is reviewable.
func antislopMode(scope string, autoScope, escaped bool) (planMode bool, planPath string, err error) {
	if autoScope || escaped {
		return false, "", nil
	}
	token, rest := popPlanToken(scope)
	if token != "plan" {
		return false, "", nil
	}
	if strings.TrimSpace(rest) == "" {
		return false, "", fmt.Errorf("plan mode requires a document path: antislop plan <path>; to review a directory named \"plan\", pass ./plan")
	}
	return true, strings.TrimSpace(rest), nil
}

// buildAntislopCodePrompt assembles the code-mode prompt and the scope strings.
// target is recorded as the sessions' review scope; display goes on the output
// Scope line.
func buildAntislopCodePrompt(scope string, autoScope bool, workdir string) (prompt, target, display string) {
	if !autoScope {
		return strings.ReplaceAll(config.AntislopCodePrompt, "{SCOPE}", scope), scope, scope
	}
	preamble, files := buildDiffPreamble(workdir)
	if files == "" {
		log.Debug().Msg("git scope: no changes detected, falling back to full project")
		const full = "the entire project"
		return strings.ReplaceAll(config.AntislopCodePrompt, "{SCOPE}", full), full, full
	}
	log.Info().Str("files", files).Msg("git scope: auto-detected changed files")
	prompt = preamble + strings.ReplaceAll(config.AntislopCodePrompt, "{SCOPE}", "the changed files listed above")
	return prompt, files, "changed files (git auto-detect)"
}

func commandAntislopAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")
	noQueue, _ := cmd.Flags().GetBool("no-queue")
	rawModels, _ := cmd.Flags().GetStringSlice("model")
	flagEffort, _ := cmd.Flags().GetString("effort")
	effortSet := cmd.Flags().Changed("effort")

	if effortSet && !config.IsValidEffort(flagEffort) {
		err := fmt.Errorf("invalid effort %q, must be one of: %v", flagEffort, config.ValidEfforts)
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	// If stdin is a terminal, show usage instead of hanging. Guard against a nil
	// stat (stdin closed/invalid) so we don't panic dereferencing it.
	if stat, statErr := os.Stdin.Stat(); statErr == nil && (stat.Mode()&os.ModeCharDevice) != 0 {
		_, _ = fmt.Fprintln(os.Stdout, antislopUsage)
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
		_, _ = fmt.Fprintln(os.Stdout, antislopUsage)
		return nil
	}

	if cmd.Flags().Changed("model") && len(parsed.Models) > 0 {
		err := fmt.Errorf("model selection was provided both as --model command flags and in arguments; use one form")
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}
	modelSelectors := rawModels
	if len(parsed.Models) > 0 {
		modelSelectors = parsed.Models
	}
	clis, err := parsePlanModels(modelSelectors)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	effort, err := mergePlanEffort(flagEffort, effortSet, parsed.Effort)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	planMode, planPath, err := antislopMode(parsed.ReviewScope, parsed.AutoScope, parsed.Escaped)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	var prompt, target, display string
	if planMode {
		absPath, err := resolvePlanPath(planPath, workdir)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stdout, err.Error())
			return &ExitCodeError{Code: 1, Err: err}
		}
		prompt = strings.ReplaceAll(config.AntislopPlanPrompt, "{FILE}", absPath)
		target, display = absPath, absPath
	} else {
		prompt, target, display = buildAntislopCodePrompt(parsed.ReviewScope, parsed.AutoScope, workdir)
	}

	// Cancel the queue wait / child processes on SIGINT/SIGTERM.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	groupID := uuid.New().String()

	result, err := review.RunDocReview(ctx, session.ModeAntislop, prompt, target, effort, config.DefaultAntislopEffort, workdir, groupID, noQueue, clis)
	if err != nil {
		return err
	}

	out := review.FormatAntislopResult(result, display, planMode)
	if _, err := io.WriteString(os.Stdout, out); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	return nil
}
