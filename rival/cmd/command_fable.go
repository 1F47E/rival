package cmd

import (
	"github.com/spf13/cobra"
)

const fableUsage = `Usage:
  /rival-fable 'explain the auth flow' — run any prompt with Fable
  /rival-fable -re medium 'find bugs in src/main.go' — run with a lower reasoning effort
  /rival-fable review — ruthless code review of the entire project
  /rival-fable review src/api/ — review specific scope
  /rival-fable -re medium review src/api/ — review with medium reasoning
  /rival-fable — show this usage info

Reasoning effort (-re): low, medium, high, xhigh.
Omitted uses efforts.fable from ~/.rival/config.yaml (built-in default: medium).`

var commandFableCmd = &cobra.Command{
	Use:   "fable",
	Short: "Skill-facing Fable executor",
	RunE:  commandFableAction,
}

func init() {
	commandFableCmd.Flags().String("workdir", ".", "working directory")
	commandFableCmd.Flags().Bool("no-queue", false, "bypass the review queue")
	commandCmd.AddCommand(commandFableCmd)
}

func commandFableAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")
	noQueue, _ := cmd.Flags().GetBool("no-queue")
	return runModelCommand(fableSpec(), workdir, noQueue)
}
