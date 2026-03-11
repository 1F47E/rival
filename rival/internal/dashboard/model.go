package dashboard

import (
	"context"

	"github.com/1F47E/rival/internal/session"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	paneList   = 0
	paneDetail = 1
)

// Model is the bubbletea model for the TUI dashboard.
type Model struct {
	sessions     []*session.Session
	selected     int
	activePane   int
	logScroll    int
	width        int
	height       int
	events       chan SessionEvent
	ctx          context.Context
	cancel       context.CancelFunc
	errText      string
	quitting     bool
}

// New creates a new dashboard model.
func New() Model {
	events := make(chan SessionEvent, 10)
	ctx, cancel := context.WithCancel(context.Background())
	return Model{
		events: events,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Init starts the file watcher and waits for events.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		if err := WatchSessions(m.ctx, m.events); err != nil {
			return errMsg{err}
		}
		// WatchSessions sends initial state before returning, so first event is ready.
		return <-m.events
	}
}

type errMsg struct{ error }

func waitForEvent(events chan SessionEvent) tea.Cmd {
	return func() tea.Msg {
		return <-events
	}
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "j", "down":
			if m.activePane == paneList && m.selected < len(m.sessions)-1 {
				m.selected++
				m.logScroll = 0
			}
			if m.activePane == paneDetail {
				m.logScroll = max(0, m.logScroll-1)
			}
		case "k", "up":
			if m.activePane == paneList && m.selected > 0 {
				m.selected--
				m.logScroll = 0
			}
			if m.activePane == paneDetail {
				m.logScroll++
			}
		case "tab", "l", "right":
			m.activePane = (m.activePane + 1) % 2
		case "shift+tab", "h", "left":
			m.activePane = (m.activePane + 1) % 2
		case "enter":
			m.activePane = paneDetail
		case "g":
			if m.activePane == paneList {
				m.selected = 0
			}
		case "G":
			if m.activePane == paneList && len(m.sessions) > 0 {
				m.selected = len(m.sessions) - 1
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case SessionEvent:
		m.sessions = msg.Sessions
		if m.selected >= len(m.sessions) {
			m.selected = max(0, len(m.sessions)-1)
		}
		return m, waitForEvent(m.events)

	case errMsg:
		m.errText = msg.Error()
		return m, nil
	}

	return m, nil
}

// View renders the UI.
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if m.errText != "" {
		return "Error: " + m.errText
	}

	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	listWidth := m.width * 2 / 5
	detailWidth := m.width - listWidth - 4 // borders
	contentHeight := m.height - 3           // help bar

	// Left pane: session list.
	listContent := renderSessionList(m.sessions, m.selected, listWidth-2, contentHeight-2)
	var leftPane string
	if m.activePane == paneList {
		leftPane = activePaneStyle.Width(listWidth - 2).Height(contentHeight - 2).Render(listContent)
	} else {
		leftPane = paneStyle.Width(listWidth - 2).Height(contentHeight - 2).Render(listContent)
	}

	// Right pane: detail view.
	var sel *session.Session
	if m.selected < len(m.sessions) {
		sel = m.sessions[m.selected]
	}
	detailContent := renderDetailView(sel, detailWidth-2, contentHeight-2, m.logScroll)
	var rightPane string
	if m.activePane == paneDetail {
		rightPane = activePaneStyle.Width(detailWidth - 2).Height(contentHeight - 2).Render(detailContent)
	} else {
		rightPane = paneStyle.Width(detailWidth - 2).Height(contentHeight - 2).Render(detailContent)
	}

	main := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
	help := helpStyle.Render("  j/k: navigate  tab: switch pane  enter: detail  q: quit")

	return lipgloss.JoinVertical(lipgloss.Left, main, help)
}
