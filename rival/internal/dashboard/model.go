package dashboard

import (
	"context"
	"strings"

	"github.com/1F47E/rival/internal/session"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	viewList   = 0
	viewDetail = 1
)

// Model is the bubbletea model for the TUI dashboard.
type Model struct {
	sessions  []*session.Session
	selected  int
	viewMode  int
	logScroll int // scroll offset from top of log (0 = first line visible)
	width     int
	height    int
	events    chan SessionEvent
	ctx       context.Context
	cancel    context.CancelFunc
	errText   string
	quitting  bool
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
		return <-m.events
	}
}

type errMsg struct{ error }

func waitForEvent(events chan SessionEvent) tea.Cmd {
	return func() tea.Msg {
		return <-events
	}
}

// scrollBottom returns the max valid scroll offset for the current session's log.
// It reads the log to compute total lines, so it's used sparingly (on navigation events).
func (m Model) scrollBottom() int {
	if m.selected >= len(m.sessions) {
		return 0
	}
	s := m.sessions[m.selected]

	// Approximate content height for log area (mirrors renderDetailView's metaLines calc).
	contentHeight := m.height - 1
	metaLines := 10 // title + blank + 7 base fields + blank + "Log" header + newline
	if s.Duration != "" {
		metaLines++
	}
	if s.ExitCode != nil {
		metaLines++
	}
	if s.OutputBytes > 0 {
		metaLines++
	}
	if s.ReviewScope != "" {
		metaLines++
	}
	if s.PromptPreview != "" {
		metaLines++
	}
	if s.ErrorMsg != "" {
		metaLines++
	}
	logHeight := contentHeight - metaLines
	if logHeight < 3 {
		logHeight = 3
	}

	lines := wrapLogLines(s.LogFile, m.width)
	ms := len(lines) - logHeight
	if ms < 0 {
		return 0
	}
	return ms
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
			if m.viewMode == viewList {
				if m.selected < len(m.sessions)-1 {
					m.selected++
					m.logScroll = m.scrollBottom()
				}
			} else {
				maxS := m.scrollBottom()
				if m.logScroll < maxS {
					m.logScroll++
				}
			}

		case "k", "up":
			if m.viewMode == viewList {
				if m.selected > 0 {
					m.selected--
					m.logScroll = m.scrollBottom()
				}
			} else {
				if m.logScroll > 0 {
					m.logScroll--
				}
			}

		case "enter":
			if m.viewMode == viewList && len(m.sessions) > 0 {
				m.viewMode = viewDetail
				m.logScroll = m.scrollBottom() // start at tail
			}

		case "esc", "backspace":
			if m.viewMode == viewDetail {
				m.viewMode = viewList
			}

		case "g":
			if m.viewMode == viewList {
				m.selected = 0
			} else {
				m.logScroll = 0
			}

		case "G":
			if m.viewMode == viewList && len(m.sessions) > 0 {
				m.selected = len(m.sessions) - 1
			} else if m.viewMode == viewDetail {
				m.logScroll = m.scrollBottom()
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case SessionEvent:
		oldSelected := m.selected
		m.sessions = msg.Sessions
		if m.selected >= len(m.sessions) {
			m.selected = max(0, len(m.sessions)-1)
		}
		if m.selected != oldSelected {
			m.logScroll = m.scrollBottom()
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

	contentHeight := m.height - 1 // reserve 1 line for help bar

	var content string
	var help string

	switch m.viewMode {
	case viewList:
		content = renderSessionList(m.sessions, m.selected, m.width, contentHeight)
		help = helpStyle.Render("  j/k: navigate  enter: open  g/G: top/bottom  q: quit")

	case viewDetail:
		var sel *session.Session
		if m.selected < len(m.sessions) {
			sel = m.sessions[m.selected]
		}
		content = clipLines(renderDetailView(sel, m.width, contentHeight, m.logScroll), contentHeight)
		help = helpStyle.Render("  j/k: scroll  g/G: top/bottom  esc: back  q: quit")
	}

	return lipgloss.JoinVertical(lipgloss.Left, content, help)
}

// clipLines hard-truncates content to at most maxLines lines.
func clipLines(s string, maxLines int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	return strings.Join(lines[:maxLines], "\n")
}
