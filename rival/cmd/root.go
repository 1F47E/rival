package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set via ldflags.
var Version = "dev"

// ExitCodeError wraps an error with a specific exit code.
type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string { return e.Err.Error() }
func (e *ExitCodeError) Unwrap() error { return e.Err }

var rootCmd = &cobra.Command{
	Use:           "rival",
	Short:         "Dispatch prompts to external AI CLIs (Codex, Gemini)",
	Long:          "rival — run Codex and Gemini from Claude Code skills or the terminal, with session tracking and a TUI dashboard.",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		var exitErr *ExitCodeError
		if errors.As(err, &exitErr) {
			_, _ = fmt.Fprintln(os.Stderr, exitErr.Err)
			os.Exit(exitErr.Code)
		}
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
