package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/1F47E/rival/internal/session"
	"github.com/1F47E/rival/internal/telemetry"
	"github.com/1F47E/rival/internal/update"
	"github.com/spf13/cobra"
)

// Version is set via ldflags.
var Version = "dev"

const banner = `
         _             __
   _____(_)   ______ _/ /
  / ___/ / | / / __ ` + "`" + `/ /
 / /  / /| |/ / /_/ / /
/_/  /_/ |___/\__,_/_/
`

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
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		session.ReapOrphans()
		update.Check(Version)
	},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(banner)
		fmt.Printf("  v%s — Codex & Gemini from your terminal\n\n", Version)
		cmd.SetOut(os.Stdout)
		_ = cmd.Usage()
	},
}

func Execute() {
	defer telemetry.RecoverPanic()
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
