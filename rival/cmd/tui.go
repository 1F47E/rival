package cmd

import (
	"fmt"

	"github.com/1F47E/rival/internal/dashboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the TUI dashboard",
	RunE: func(cmd *cobra.Command, args []string) error {
		m := dashboard.New()
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("tui: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
