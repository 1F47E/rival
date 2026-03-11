package cmd

import (
	"github.com/spf13/cobra"
)

var commandCmd = &cobra.Command{
	Use:   "command",
	Short: "Skill-facing command (reads raw args from stdin, parses, executes)",
	Long:  "Used by Claude Code skills. Reads raw slash-command arguments from stdin, parses them, executes the appropriate CLI, and prints the final output.",
}

func init() {
	rootCmd.AddCommand(commandCmd)
}
