package dashboard

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/1F47E/rival/internal/session"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	viewList   = 0
	viewDetail = 1
)

// displayItem wraps one or more sessions for display in the TUI.
// A megareview group has two sessions; everything else has one.
type displayItem struct {
	Sessions []*session.Session
}

// Primary returns the first session (used for shared metadata).
func (d *displayItem) Primary() *session.Session {
	if len(d.Sessions) == 0 {
		return nil
	}
	return d.Sessions[0]
}

// IsGroup returns true if this item contains multiple sessions.
func (d *displayItem) IsGroup() bool {
	return len(d.Sessions) > 1
}

// groupSessions merges sessions sharing a GroupID into display items.
func groupSessions(sessions []*session.Session) []displayItem {
	// Collect groups by GroupID, preserving order of first appearance.
	groups := make(map[string]*displayItem)
	var order []string

	for _, s := range sessions {
		if s.GroupID != "" {
			if g, ok := groups[s.GroupID]; ok {
				g.Sessions = append(g.Sessions, s)
			} else {
				groups[s.GroupID] = &displayItem{Sessions: []*session.Session{s}}
				order = append(order, s.GroupID)
			}
		} else {
			// Standalone session — use ID as unique key.
			key := "solo:" + s.ID
			groups[key] = &displayItem{Sessions: []*session.Session{s}}
			order = append(order, key)
		}
	}

	items := make([]displayItem, 0, len(order))
	for _, key := range order {
		items = append(items, *groups[key])
	}
	return items
}

// Model is the bubbletea model for the TUI dashboard.
type Model struct {
	items          []displayItem
	selected       int
	viewMode       int
	promptExpanded bool
	width          int
	height         int
	events         chan SessionEvent
	ctx            context.Context
	cancel         context.CancelFunc
	errText        string
	quitting       bool
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

// tickMsg fires periodically to refresh live timers and log tails.
type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func waitForEvent(events chan SessionEvent) tea.Cmd {
	return func() tea.Msg {
		return <-events
	}
}

// hasRunning returns true if any session in the items is still running.
func hasRunning(items []displayItem) bool {
	for _, item := range items {
		for _, s := range item.Sessions {
			if s.Status == "running" {
				return true
			}
		}
	}
	return false
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
				if m.selected < len(m.items)-1 {
					m.selected++
				}
			}

		case "k", "up":
			if m.viewMode == viewList {
				if m.selected > 0 {
					m.selected--
				}
			}

		case "enter":
			if m.viewMode == viewList && len(m.items) > 0 {
				m.viewMode = viewDetail
				m.promptExpanded = false
			}

		case "esc", "backspace":
			if m.viewMode == viewDetail {
				m.viewMode = viewList
				m.promptExpanded = false
			}

		case "g":
			if m.viewMode == viewList {
				m.selected = 0
			}

		case "G":
			if m.viewMode == viewList && len(m.items) > 0 {
				m.selected = len(m.items) - 1
			}

		case "p":
			if m.viewMode == viewDetail {
				m.promptExpanded = !m.promptExpanded
			}

		case "o":
			if m.viewMode == viewDetail && m.selected < len(m.items) {
				item := m.items[m.selected]
				if s := item.Primary(); s != nil && s.LogFile != "" {
					_ = exec.Command("open", s.LogFile).Start()
				}
			}

		case "x":
			if m.viewMode == viewDetail && m.selected < len(m.items) {
				item := m.items[m.selected]
				for _, s := range item.Sessions {
					if s.Status != "running" || s.PID <= 0 {
						continue
					}
					if err := syscall.Kill(s.PID, syscall.SIGTERM); err != nil {
						// Process already dead — mark failed immediately.
						_ = s.Fail(1, "killed (process already dead)")
					} else {
						// Signal sent — mark failed so TUI updates instantly.
						// The subprocess executor will overwrite with its own status.
						_ = s.Fail(137, "killed by user")
					}
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case SessionEvent:
		m.items = groupSessions(msg.Sessions)
		if m.selected >= len(m.items) {
			m.selected = max(0, len(m.items)-1)
		}
		cmds := []tea.Cmd{waitForEvent(m.events)}
		if hasRunning(m.items) {
			cmds = append(cmds, tickCmd())
		}
		return m, tea.Batch(cmds...)

	case tickMsg:
		// Re-render for live timers and log tails. Keep ticking while running.
		if hasRunning(m.items) {
			return m, tickCmd()
		}
		return m, nil

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
		content = renderSessionList(m.items, m.selected, m.width, contentHeight)
		help = helpStyle.Render("  j/k: navigate  enter: open  g/G: top/bottom  q: quit")

	case viewDetail:
		var item *displayItem
		if m.selected < len(m.items) {
			item = &m.items[m.selected]
		}
		content = clipLines(renderDetailView(item, m.width, contentHeight, m.promptExpanded), contentHeight)
		help = helpStyle.Render("  p: prompt  o: open log  x: stop  esc: back  q: quit")
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
