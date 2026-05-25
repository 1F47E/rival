package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/executor"
	"github.com/1F47E/rival/internal/parser"
	"github.com/1F47E/rival/internal/session"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const antigravityUsage = `Usage:
  /rival-antigravity 'explain the auth flow' — run any prompt via antigravity
  /rival-antigravity review — ruthless code review of the entire project
  /rival-antigravity review src/api/ — review specific scope
  /rival-antigravity — show this usage info

Note: agy uses Gemini 3.5 Flash with fixed reasoning — the -re flag is accepted but ignored.`

var commandAntigravityCmd = &cobra.Command{
	Use:   "antigravity",
	Short: "Skill-facing antigravity executor",
	RunE:  commandAntigravityAction,
}

func init() {
	commandAntigravityCmd.Flags().String("workdir", ".", "working directory")
	commandCmd.AddCommand(commandAntigravityCmd)
}

func commandAntigravityAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")

	if stat, _ := os.Stdin.Stat(); (stat.Mode() & os.ModeCharDevice) != 0 {
		_, _ = fmt.Fprintln(os.Stdout, antigravityUsage)
		return nil
	}

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	parsed, err := parser.ParseAntigravityArgs(string(raw))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	if parsed.IsEmpty {
		_, _ = fmt.Fprintln(os.Stdout, antigravityUsage)
		return nil
	}

	if parsed.IsReview && parsed.AutoScope {
		resolveGitScope(parsed, workdir)
	}

	if err := executor.AntigravityPreflight(); err != nil {
		return err
	}

	mode := "raw"
	if parsed.IsReview {
		mode = "review"
	}

	sess, err := session.New("antigravity", mode, config.AntigravityModel, parsed.Effort, workdir, parsed.Prompt, parsed.ReviewScope, "")
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	defer func() {
		if sess.Status == "running" {
			_ = sess.Fail(1, "interrupted")
		}
	}()

	log.Info().Str("session", sess.ID).Str("effort", parsed.Effort).Str("mode", mode).Msg("starting antigravity (command mode)")

	result, err := executor.RunAntigravity(context.Background(), sess, parsed.Prompt, parsed.Effort, workdir, nil)
	if err != nil {
		if saveErr := sess.Fail(1, err.Error()); saveErr != nil {
			log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session failure")
		}
		return err
	}

	if result.ExitCode != 0 {
		if saveErr := sess.Fail(result.ExitCode, fmt.Sprintf("antigravity exited with code %d", result.ExitCode)); saveErr != nil {
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
	_, _ = fmt.Fprint(os.Stdout, string(logData))

	if result.ExitCode != 0 {
		return &ExitCodeError{Code: result.ExitCode, Err: fmt.Errorf("antigravity exited with code %d", result.ExitCode)}
	}

	return nil
}
