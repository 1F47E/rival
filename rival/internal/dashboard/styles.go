package dashboard

import "github.com/charmbracelet/lipgloss"

var (
	// Colors.
	colorPrimary   = lipgloss.Color("#7C3AED") // violet
	colorSecondary = lipgloss.Color("#64748B") // slate
	colorSuccess   = lipgloss.Color("#22C55E") // green
	colorError     = lipgloss.Color("#EF4444") // red
	colorRunning   = lipgloss.Color("#F59E0B") // amber

	// Title.
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	// List header row.
	headerStyle = lipgloss.NewStyle().
			Foreground(colorSecondary).
			Bold(true)

	// Session list item.
	selectedItemStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(colorPrimary)

	normalItemStyle = lipgloss.NewStyle()

	// Status badges.
	runningStyle = lipgloss.NewStyle().
			Foreground(colorRunning).
			Bold(true)

	completedStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	failedStyle = lipgloss.NewStyle().
			Foreground(colorError)

	// Detail view.
	labelStyle = lipgloss.NewStyle().
			Foreground(colorSecondary)

	valueStyle = lipgloss.NewStyle().
			Bold(true)

	// Help.
	helpStyle = lipgloss.NewStyle().
			Foreground(colorSecondary).
			Italic(true)
)

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "running":
		return runningStyle
	case "completed":
		return completedStyle
	case "failed":
		return failedStyle
	default:
		return normalItemStyle
	}
}
