package cmd

import (
	"github.com/spf13/cobra"
)

var runK3Cmd = &cobra.Command{
	Use:   "k3",
	Short: "Run Kimi K3 (via opencode)",
	RunE:  runK3Action,
}

func init() {
	runK3Cmd.Flags().String("workdir", ".", "working directory")
	runK3Cmd.Flags().Bool("prompt-stdin", false, "read prompt from stdin")
	runK3Cmd.Flags().String("review", "", "review scope (enables review mode)")
	runK3Cmd.Flags().Bool("no-queue", false, "bypass the review queue")
	runCmd.AddCommand(runK3Cmd)
}

func runK3Action(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")
	promptStdin, _ := cmd.Flags().GetBool("prompt-stdin")
	reviewScope, _ := cmd.Flags().GetString("review")
	noQueue, _ := cmd.Flags().GetBool("no-queue")

	return runModelRun(k3Spec(), runOptions{
		workdir:     workdir,
		noQueue:     noQueue,
		reviewScope: reviewScope,
		isReview:    cmd.Flags().Changed("review"),
		promptStdin: promptStdin,
	})
}
