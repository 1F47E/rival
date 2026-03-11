package cmd

import (
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a CLI executor directly (terminal use)",
	Long:  "Execute Codex or Gemini with explicit flags. Streams output to stdout.",
}

func init() {
	rootCmd.AddCommand(runCmd)
}
