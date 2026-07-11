package cmd

import (
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a CLI executor directly (terminal use)",
	Long:  "Execute a model runner (gpt-5.6-sol, gemini-3.5-flash, gemini-3.1-pro-preview, claude-opus-4-8) with explicit flags. Streams output to stdout.",
}

func init() {
	rootCmd.AddCommand(runCmd)
}
