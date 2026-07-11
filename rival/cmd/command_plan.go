package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/review"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const planUsage = `Usage:
  /rival-plan-sol path/to/plan.md — review with Sol
  /rival-plan-sol -re ultra path/to/plan.md — use ultra reasoning
  /rival-plan-fable path/to/plan.md — review with Fable
  rival command plan --help — show native command options

Input is a single path to a markdown plan/spec file. Reasoning effort defaults to high
(low for Fable alone); use -re/--effort ultra for the deepest review. --model
accepts sol and fable. An unavailable model
is skipped, not fatal.`

var defaultPlanModels = []string{config.SolLabel, config.FableLabel}

var commandPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Review a plan/spec with Sol and/or Fable",
	RunE:  commandPlanAction,
}

func init() {
	commandPlanCmd.Flags().String("workdir", ".", "working directory")
	commandPlanCmd.Flags().Bool("no-queue", false, "bypass the review queue")
	commandPlanCmd.Flags().StringSliceP("model", "m", defaultPlanModels, "plan review model(s): sol, fable (comma-separated)")
	commandPlanCmd.Flags().StringSlice("cli", nil, "legacy plan reviewer selector")
	if err := commandPlanCmd.Flags().MarkHidden("cli"); err != nil {
		panic(err)
	}
	commandCmd.AddCommand(commandPlanCmd)
	commandPlanCmd.Flags().String("effort", config.DefaultPlanEffort, "reasoning effort: low, medium, high, ultra")
}

// parsePlanModels validates model-facing selectors and maps them to the
// internal adapters used by RunPlanReview. It de-duplicates by concrete model
// while preserving the user's order.
func parsePlanModels(raw []string) ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	for _, value := range raw {
		for _, part := range strings.Split(value, ",") {
			model := strings.ToLower(strings.TrimSpace(part))
			var cli string
			switch model {
			case config.SolLabel, config.GPT56SolModel:
				cli = "codex"
			case config.FableLabel, config.FableModel:
				cli = "fable"
			case "":
				return nil, fmt.Errorf("model selector cannot be empty")
			default:
				return nil, fmt.Errorf("unknown plan model %q; use one of: sol, fable", part)
			}
			if !seen[cli] {
				seen[cli] = true
				out = append(out, cli)
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no plan models selected")
	}
	return out, nil
}

// parsePlanCLIs validates and de-duplicates the --cli values, preserving order.
func parsePlanCLIs(raw []string) ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	for _, v := range raw {
		c := strings.ToLower(strings.TrimSpace(v))
		switch c {
		case "codex", "fable":
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		case "":
			continue
		default:
			return nil, fmt.Errorf("unknown legacy plan selector; use --model with sol or fable")
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no plan models selected; use --model with sol or fable")
	}
	return out, nil
}

func commandPlanAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")
	noQueue, _ := cmd.Flags().GetBool("no-queue")
	rawModels, _ := cmd.Flags().GetStringSlice("model")
	rawCLIs, _ := cmd.Flags().GetStringSlice("cli")
	effort, _ := cmd.Flags().GetString("effort")

	if !config.IsValidReviewEffort(effort) {
		err := fmt.Errorf("invalid effort %q, must be one of: %v", effort, config.ReviewEfforts)
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	modelsSet := cmd.Flags().Changed("model")
	legacySet := cmd.Flags().Changed("cli")
	if modelsSet && legacySet {
		err := fmt.Errorf("model selection was provided more than once; use --model")
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	var clis []string
	var err error
	if legacySet {
		clis, err = parsePlanCLIs(rawCLIs)
	} else {
		clis, err = parsePlanModels(rawModels)
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}
	if !cmd.Flags().Changed("effort") {
		effort = defaultPlanEffort(clis)
	}

	// If stdin is a terminal, show usage instead of hanging. Guard against a nil
	// stat (stdin closed/invalid) so we don't panic dereferencing it.
	if stat, statErr := os.Stdin.Stat(); statErr == nil && (stat.Mode()&os.ModeCharDevice) != 0 {
		_, _ = fmt.Fprintln(os.Stdout, planUsage)
		return nil
	}

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	rawPath, inputEffort, err := parsePlanInput(string(raw))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}
	if inputEffort != "" {
		if cmd.Flags().Changed("effort") {
			err := fmt.Errorf("reasoning effort was provided both as --effort command flags and in plan arguments; use one form")
			_, _ = fmt.Fprintln(os.Stdout, err.Error())
			return &ExitCodeError{Code: 1, Err: err}
		}
		effort = inputEffort
	}
	if rawPath == "" {
		_, _ = fmt.Fprintln(os.Stdout, planUsage)
		return nil
	}

	absPath, err := resolvePlanPath(rawPath, workdir)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	// Cancel the queue wait / child processes on SIGINT/SIGTERM.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	groupID := uuid.New().String()

	result, err := review.RunPlanReview(ctx, absPath, effort, workdir, groupID, noQueue, clis)
	if err != nil {
		return err
	}

	out := review.FormatPlanResult(result, absPath)
	if _, err := io.WriteString(os.Stdout, out); err != nil {
		return fmt.Errorf("write stdout: %w", err)
	}
	return nil
}

func defaultPlanEffort(clis []string) string {
	if len(clis) == 1 && clis[0] == "fable" {
		return "low"
	}
	return config.DefaultPlanEffort
}

// parsePlanInput extracts an optional skill-facing -re/--effort prefix while
// leaving the rest of the input intact as the path (including spaces). Native
// callers can use --effort; slash-command skills pass `-re ultra plan.md` on
// stdin. A leading `--` escapes a path beginning with a dash.
func parsePlanInput(raw string) (path, effort string, err error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", nil
	}
	if strings.HasPrefix(s, "-- ") {
		return strings.TrimSpace(strings.TrimPrefix(s, "--")), "", nil
	}

	option, rest := popPlanToken(s)
	name, inline, hasInline := splitPlanOption(option)
	if name != "-re" && name != "--effort" {
		if strings.HasPrefix(option, "-") {
			return "", "", fmt.Errorf("unknown plan option %q; use -re/--effort or -- before a path beginning with '-'", option)
		}
		return s, "", nil
	}

	if hasInline {
		effort = strings.TrimSpace(inline)
	} else {
		if strings.TrimSpace(rest) == "" {
			return "", "", fmt.Errorf("option %s requires a value", name)
		}
		effort, rest = popPlanToken(rest)
	}
	if effort == "" || strings.HasPrefix(effort, "-") {
		return "", "", fmt.Errorf("option %s requires a value", name)
	}
	if !config.IsValidReviewEffort(effort) {
		return "", "", fmt.Errorf("invalid effort %q, must be one of: low, medium, high, ultra", effort)
	}
	path = strings.TrimSpace(rest)
	if path == "" {
		return "", "", fmt.Errorf("plan path is required after %s %s", name, effort)
	}
	return path, effort, nil
}

func popPlanToken(s string) (token, rest string) {
	s = strings.TrimLeft(s, " \t\r\n")
	if i := strings.IndexAny(s, " \t\r\n"); i >= 0 {
		return s[:i], strings.TrimLeft(s[i:], " \t\r\n")
	}
	return s, ""
}

func splitPlanOption(token string) (name, value string, hasValue bool) {
	if i := strings.Index(token, "="); i >= 0 {
		return token[:i], token[i+1:], true
	}
	return token, "", false
}

// resolvePlanPath turns the raw user-supplied path into a validated absolute path
// to an existing regular file. Relative paths are resolved against workdir. The
// .md extension is preferred but not required (lenient).
func resolvePlanPath(rawPath, workdir string) (string, error) {
	p := strings.TrimSpace(rawPath)
	// Expand a leading ~ to the home directory.
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(workdir, p)
	}
	// Reject control characters (e.g. a newline in the filename): the path is
	// interpolated into the model prompt, so a control char could inject prompt
	// text. Real plan files never have these in their path.
	if i := strings.IndexFunc(p, func(r rune) bool { return r < 0x20 || r == 0x7f }); i >= 0 {
		return "", fmt.Errorf("plan path contains a control character at position %d — refusing", i)
	}

	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolve plan path %q: %w", rawPath, err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("plan file not found: %s", abs)
		}
		return "", fmt.Errorf("cannot read plan file %s: %w", abs, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("plan path is a directory, not a file: %s", abs)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("plan path is not a regular file: %s", abs)
	}
	// Confirm the file is actually readable now, so an unreadable file fails here
	// with a clear message rather than later inside a model runner with an opaque error.
	f, err := os.Open(abs)
	if err != nil {
		return "", fmt.Errorf("cannot read plan file %s: %w", abs, err)
	}
	_ = f.Close()
	return abs, nil
}
