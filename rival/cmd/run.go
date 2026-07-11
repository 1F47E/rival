package cmd

import (
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a CLI executor directly (terminal use)",
	Long:  "Execute a model runner with explicit flags and stream output to stdout.",
}

func init() {
	rootCmd.AddCommand(runCmd)
}
