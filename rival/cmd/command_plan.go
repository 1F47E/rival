package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/executor"
	"github.com/1F47E/rival/internal/review"
	"github.com/1F47E/rival/internal/session"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const planUsage = `Usage:
  /rival-plan path/to/plan.md — review a plan/spec document with codex (rate 1-10, find bugs + gaps)
  /rival-plan — show this usage info

Input is a single path to a markdown plan/spec file. Reasoning effort is fixed at xhigh.`

var commandPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Skill-facing plan/spec reviewer (codex only)",
	RunE:  commandPlanAction,
}

func init() {
	commandPlanCmd.Flags().String("workdir", ".", "working directory")
	commandCmd.AddCommand(commandPlanCmd)
}

func commandPlanAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")

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

	prompt := strings.ReplaceAll(config.PlanReviewPrompt, "{FILE}", absPath)

	if err := executor.CodexPreflight(); err != nil {
		return err
	}

	sess, err := session.New("codex", "plan", config.CodexModel, config.DefaultEffort, workdir, prompt, absPath, "")
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	defer func() {
		if sess.Status == "running" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("file", absPath).Msg("starting plan review (command mode)")

	result, err := executor.RunCodex(context.Background(), sess, prompt, config.DefaultEffort, workdir, nil)
	if err != nil {
		if saveErr := sess.Fail(1, err.Error()); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		return err
	}

	if result.ExitCode != 0 {
		if saveErr := sess.Fail(result.ExitCode, fmt.Sprintf("codex exited with code %d", result.ExitCode)); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
	} else {
		if saveErr := sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session completion")
		}
	}

	logData, err := os.ReadFile(sess.LogFile)
	if err != nil {
		return fmt.Errorf("read log file: %w", err)
	}
	rawLog := string(logData)

	// Parse the structured plan output; on failure, fall back to the raw codex
	// output so nothing the model produced is lost.
	parsed, parseErr := review.ParsePlanOutput(rawLog)
	if parseErr != nil {
		log.Warn().Err(parseErr).Str("session", sess.ID).Msg("failed to parse plan output, returning raw")
		_, _ = fmt.Fprint(os.Stdout, rawLog)
	} else {
		_, _ = fmt.Fprint(os.Stdout, review.FormatPlanConsole(parsed, absPath))
	}

	if result.ExitCode != 0 {
		return &ExitCodeError{Code: result.ExitCode, Err: fmt.Errorf("codex exited with code %d", result.ExitCode)}
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
