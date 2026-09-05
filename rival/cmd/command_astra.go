package cmd

import (
	"github.com/1F47E/rival/internal/config"
	"github.com/spf13/cobra"
)

const astraUsage = `Usage:
  /rival-astra 'explain the auth flow' — run any prompt with Astra
  /rival-astra -re high 'find bugs in src/main.go' — run with a lower reasoning effort
  /rival-astra review — ruthless code review of the entire project
  /rival-astra review src/api/ — review specific scope
  /rival-astra -re high review src/api/ — review with high reasoning
  /rival-astra — show this usage info

Reasoning effort (-re): low, medium, high, xhigh, ultra.
Omitted uses efforts.astra from ~/.rival/config.yaml (built-in: xhigh).`

var commandAstraCmd = &cobra.Command{
	Use:   config.AstraLabel,
	Short: "Skill-facing Astra executor",
	RunE:  commandAstraAction,
}

func init() {
	configureCommandAstraFlags(commandAstraCmd)
	commandCmd.AddCommand(commandAstraCmd)
}

func configureCommandAstraFlags(cmd *cobra.Command) {
	cmd.Flags().String("workdir", ".", "working directory")
	cmd.Flags().Bool("no-queue", false, "bypass the review queue")
}

func commandAstraAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")
	noQueue, _ := cmd.Flags().GetBool("no-queue")
	return runModelCommand(astraSpec(), workdir, noQueue)
}
