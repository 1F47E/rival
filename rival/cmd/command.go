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
	// --detach re-execs into a new process session (setsid) so the launching
	// shell returns immediately and shell/process-group teardown cannot kill
	// the review. Skills rely on this; see cmd/detach.go.
	commandCmd.PersistentFlags().Bool("detach", false, "run detached in a new process session; prints 'rival: detached pid=N' and exits")
	rootCmd.AddCommand(commandCmd)
}
