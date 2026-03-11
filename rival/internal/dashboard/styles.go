package dashboard

import "github.com/charmbracelet/lipgloss"

var (
	// Colors.
	colorPrimary   = lipgloss.Color("#7C3AED") // violet
	colorSecondary = lipgloss.Color("#64748B") // slate
	colorSuccess   = lipgloss.Color("#22C55E") // green
	colorError     = lipgloss.Color("#EF4444") // red
	colorRunning   = lipgloss.Color("#F59E0B") // amber
	colorBorder    = lipgloss.Color("#334155")

	// Pane styles.
	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	activePaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(0, 1)

	// Title.
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

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
