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
  /rival-plan path/to/plan.md — review a plan/spec document with codex + claude-fable (rate 1-10, find bugs + gaps)
  /rival-plan — show this usage info

Input is a single path to a markdown plan/spec file. Reasoning effort is fixed at xhigh.
The --cli flag (codex,fable — default both) selects the review engine(s); the codex-only
and fable-only skills pass a single value. An engine that is unavailable is skipped, not fatal.`

// defaultPlanCLIs is the engine set used when --cli is not narrowed (the dual
// /rival-plan skill): both codex and fable.
var defaultPlanCLIs = []string{"codex", "fable"}

var commandPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Skill-facing plan/spec reviewer (codex + claude-fable)",
	RunE:  commandPlanAction,
}

func init() {
	commandPlanCmd.Flags().String("workdir", ".", "working directory")
	commandPlanCmd.Flags().Bool("no-queue", false, "bypass the review queue")
	commandPlanCmd.Flags().StringSlice("cli", defaultPlanCLIs, "plan review engine(s): codex, fable (comma-separated)")
	commandCmd.AddCommand(commandPlanCmd)
	commandPlanCmd.Flags().String("effort", config.DefaultEffort, "reasoning effort: low, medium, high, xhigh")
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
			return nil, fmt.Errorf("unknown --cli value %q (valid: codex, fable)", c)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid --cli values (valid: codex, fable)")
	}
	return out, nil
}

func commandPlanAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")
	noQueue, _ := cmd.Flags().GetBool("no-queue")
	rawCLIs, _ := cmd.Flags().GetStringSlice("cli")
	effort, _ := cmd.Flags().GetString("effort")

	if !config.IsValidEffort(effort) {
		err := fmt.Errorf("invalid effort %q, must be one of: %v", effort, config.ValidEfforts)
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	clis, err := parsePlanCLIs(rawCLIs)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
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

	rawPath := strings.TrimSpace(string(raw))
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
	// interpolated into the codex prompt, so a control char could inject prompt
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
	// with a clear message rather than later inside codex with an opaque error.
	f, err := os.Open(abs)
	if err != nil {
		return "", fmt.Errorf("cannot read plan file %s: %w", abs, err)
	}
	_ = f.Close()
	return abs, nil
}
