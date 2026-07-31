package dashboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
	"github.com/charmbracelet/x/ansi"
)

const (
	viewList   = 0
	viewDetail = 1
)

// displayItem wraps one or more sessions for display in the TUI.
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

// IsGroup returns true for a logical grouped run, including a degraded run
// where only one requested model passed preflight.
func (d *displayItem) IsGroup() bool {
	return len(d.Sessions) > 1 || (len(d.Sessions) == 1 && d.Sessions[0].GroupID != "")
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
		session.SortGroupMembers(groups[key].Sessions)
		items = append(items, *groups[key])
	}
	return items
}

const pageSize = 100

// Model is the bubbletea model for the TUI dashboard.
type Model struct {
	allItems       []displayItem // all grouped items
	items          []displayItem // visible slice (paginated)
	visibleCount   int           // how many items to show
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
	totalSessions  int            // total session count (before grouping)
	logView        viewport.Model // scrollable log pane for the detail view
}

// Version is set from cmd package before launching the TUI.
var Version = "dev"

// New creates a new dashboard model.
func New() Model {
	events := make(chan SessionEvent, 10)
	ctx, cancel := context.WithCancel(context.Background())
	return Model{
		visibleCount: pageSize,
		events:       events,
		ctx:          ctx,
		cancel:       cancel,
		// Real dimensions arrive with the first WindowSizeMsg and are applied by
		// syncDetailViewport; the log content is pre-wrapped, so SoftWrap stays off.
		logView: viewport.New(viewport.WithWidth(0), viewport.WithHeight(0)),
	}
}

// paginateItems sets the visible slice from allItems.
func (m *Model) paginateItems() {
	if m.visibleCount >= len(m.allItems) {
		m.items = m.allItems
	} else {
		m.items = m.allItems[:m.visibleCount]
	}
	if m.selected >= len(m.items) {
		m.selected = max(0, len(m.items)-1)
	}
}

// hasMore returns true if there are hidden items beyond the visible page.
func (m *Model) hasMore() bool {
	return m.visibleCount < len(m.allItems)
}

// selectedItem returns the currently selected display item, or nil.
func (m *Model) selectedItem() *displayItem {
	if m.selected < 0 || m.selected >= len(m.items) {
		return nil
	}
	return &m.items[m.selected]
}

// contentHeight is the number of rows available to the body between the banner
// and the help bar. Both viewContent and syncDetailViewport MUST use it: if the
// two ever compute a different height the viewport renders a frame the view
// then clips, which is what leaves stale rows on screen.
func (m Model) contentHeight() int {
	headerLines := strings.Count(m.renderBanner(), "\n")
	h := m.height - headerLines - 1 // -1 for the help bar
	if h < 0 {
		h = 0
	}
	return h
}

// syncDetailViewport rebuilds the log viewport's size and content for the
// current selection. resetToBottom forces the tail into view (entering detail);
// otherwise the tail is followed only if the user was already parked at the
// bottom.
func (m *Model) syncDetailViewport(resetToBottom bool) {
	item := m.selectedItem()
	if m.viewMode != viewDetail || item == nil || item.Primary() == nil {
		m.logView.SetContent("")
		return
	}

	// Capture the follow state BEFORE any resize. Shrinking the viewport raises
	// the old offset above the new max-bottom, so sampling AtBottom after
	// SetHeight reports false for a user who never scrolled and silently kills
	// tail-follow.
	wasAtBottom := m.logView.AtBottom()

	height := m.contentHeight()
	meta := renderDetailMeta(item, m.width, height, m.promptExpanded)
	metaLines := strings.Count(meta, "\n") + 1

	m.logView.SetWidth(m.width)
	m.logView.SetHeight(max(1, height-metaLines))

	if item.IsGroup() {
		m.logView.SetContent(buildGroupLogContent(item, m.width))
	} else {
		m.logView.SetContent(buildLogContent(item.Primary(), m.width))
	}

	if resetToBottom || wasAtBottom {
		m.logView.GotoBottom()
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

// hasRunning returns true if any session is running or queued, so the live
// timer keeps ticking (queued rows show a growing wait time).
func hasRunning(items []displayItem) bool {
	for _, item := range items {
		for _, s := range item.Sessions {
			if s.Status == "running" || s.Status == "queued" {
				return true
			}
		}
	}
	return false
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Keys this feature owns are handled FIRST, before the list switch. The
		// list cases match "j"/"k"/"g"/"G" unconditionally (their viewList test is
		// inside the case body), so anything routed after them would be swallowed
		// in detail mode and the advertised scroll keys would do nothing.
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit

		case "esc", "backspace":
			if m.viewMode == viewDetail {
				m.viewMode = viewList
				m.promptExpanded = false
				(&m).syncDetailViewport(false)
				return m, nil
			}

		case "p":
			if m.viewMode == viewDetail {
				m.promptExpanded = !m.promptExpanded
				// Meta height just changed, so the viewport must be resized —
				// this is the path that exercises capture-before-resize.
				(&m).syncDetailViewport(false)
				return m, nil
			}

		case "o":
			if m.viewMode == viewDetail {
				if item := (&m).selectedItem(); item != nil {
					if item.IsGroup() {
						openPublicGroupLogs(item.Sessions)
					} else if s := item.Primary(); s != nil && s.LogFile != "" {
						openPublicLog(s)
					}
				}
				return m, nil
			}

		case "x":
			if m.viewMode == viewDetail {
				if item := (&m).selectedItem(); item != nil {
					for _, s := range item.Sessions {
						// Queued sessions carry the waiting rival process's PID, so
						// SIGTERM cancels the queue wait (the process's signal handler
						// removes its ticket and fails the session).
						if (s.Status != "running" && s.Status != "queued") || s.PID <= 0 {
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
				return m, nil
			}

		case "g":
			if m.viewMode == viewDetail {
				m.logView.GotoTop()
				return m, nil
			}

		case "G":
			if m.viewMode == viewDetail {
				m.logView.GotoBottom()
				return m, nil
			}
		}

		// In detail mode every remaining key belongs to the viewport
		// (j/k/up/down/pgup/pgdn/space/u/d). Return immediately — falling
		// through to the list switch would move the list selection instead.
		if m.viewMode == viewDetail {
			var cmd tea.Cmd
			m.logView, cmd = m.logView.Update(msg)
			return m, cmd
		}

		// List mode only.
		switch msg.String() {
		case "j", "down":
			if m.selected < len(m.items)-1 {
				m.selected++
			}

		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}

		case "enter":
			if len(m.items) > 0 {
				m.viewMode = viewDetail
				m.promptExpanded = false
				// Open at the tail: live output and final verdicts both live at
				// the end of the log.
				(&m).syncDetailViewport(true)
			}

		case "g":
			m.selected = 0

		case "G":
			if len(m.items) > 0 {
				m.selected = len(m.items) - 1
			}

		case "l":
			if m.hasMore() {
				m.visibleCount += pageSize
				m.paginateItems()
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.viewMode == viewDetail {
			(&m).syncDetailViewport(false)
		}

	case SessionEvent:
		m.totalSessions = len(msg.Sessions)
		m.allItems = groupSessions(msg.Sessions)
		m.paginateItems()
		if m.viewMode == viewDetail {
			(&m).syncDetailViewport(false)
		}
		cmds := []tea.Cmd{waitForEvent(m.events)}
		if hasRunning(m.items) {
			cmds = append(cmds, tickCmd())
		}
		return m, tea.Batch(cmds...)

	case tickMsg:
		// Re-render for live timers and log tails. Keep ticking while running.
		if m.viewMode == viewDetail {
			(&m).syncDetailViewport(false)
		}
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

func openPublicLog(s *session.Session) {
	viewPath, err := createPublicLogView(s)
	if err != nil {
		return
	}
	openPublicLogPath(viewPath)
}

func openPublicGroupLogs(sessions []*session.Session) {
	viewPath, err := createPublicGroupLogView(sessions)
	if err != nil {
		return
	}
	openPublicLogPath(viewPath)
}

func openPublicLogPath(viewPath string) {
	if err := exec.Command("open", viewPath).Start(); err != nil {
		_ = os.Remove(viewPath)
		return
	}
	time.AfterFunc(10*time.Minute, func() { _ = os.Remove(viewPath) })
}

func createPublicLogView(s *session.Session) (string, error) {
	data, err := os.ReadFile(s.LogFile)
	if err != nil {
		return "", err
	}
	return createPublicTextView(config.PublicRuntimeLog(s.CLI, s.Model, string(data)))
}

func createPublicGroupLogView(sessions []*session.Session) (string, error) {
	var content strings.Builder
	for _, s := range sessions {
		label := groupLogLabel(s)
		if s.Status == "failed" && s.ErrorMsg != "" {
			label += " (FAILED)"
		}
		fmt.Fprintf(&content, "=== %s ===\n", label)
		if s.ErrorMsg != "" {
			fmt.Fprintf(&content, "Error: %s\n", config.PublicRuntimeError(s.CLI, s.Model, s.ErrorMsg))
		}
		data, err := os.ReadFile(s.LogFile)
		if err != nil {
			if s.ErrorMsg == "" {
				fmt.Fprintf(&content, "(log unavailable: %s)\n", err)
			}
		} else {
			content.WriteString(config.PublicRuntimeLog(s.CLI, s.Model, string(data)))
			if len(data) > 0 && data[len(data)-1] != '\n' {
				content.WriteByte('\n')
			}
		}
		content.WriteByte('\n')
	}
	return createPublicTextView(content.String())
}

func createPublicTextView(content string) (string, error) {
	file, err := os.CreateTemp("", "rival-log-*.txt")
	if err != nil {
		return "", err
	}
	viewPath := file.Name()
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		_ = os.Remove(viewPath)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(viewPath)
		return "", err
	}
	return viewPath, nil
}

// bannerLines is the ASCII logo for the TUI header.
var bannerLines = []string{
	`         _             __`,
	`   _____(_)   ______ _/ /`,
	"  / ___/ / | / / __ `/ /",
	` / /  / /| |/ / /_/ / /`,
	`/_/  /_/ |___/\__,_/_/`,
}

// bannerWidth is computed from the actual banner lines.
var bannerWidth = func() int {
	w := 0
	for _, l := range bannerLines {
		if n := len([]rune(l)); n > w {
			w = n
		}
	}
	return w
}()

// renderBanner returns the header block: logo left, stats right (if width >= 70).
func (m Model) renderBanner() string {
	// Count stats.
	running, queued, completed, failed := 0, 0, 0, 0
	for _, item := range m.allItems {
		for _, s := range item.Sessions {
			switch s.Status {
			case "running":
				running++
			case "queued":
				queued++
			case "completed":
				completed++
			case "failed":
				failed++
			}
		}
	}

	logo := strings.Join(bannerLines, "\n")
	styledLogo := bannerStyle.Render(logo)

	// If terminal is narrow, just show the logo.
	if m.width < 70 {
		return styledLogo + "\n"
	}

	// Stats on the right.
	var stats strings.Builder
	stats.WriteString(labelStyle.Render(Version))
	stats.WriteString("\n")
	stats.WriteString(fmt.Sprintf("  %s %d", runningStyle.Render("●"), running))
	if queued > 0 {
		stats.WriteString(fmt.Sprintf("  %s %d", queuedStyle.Render("◌"), queued))
	}
	stats.WriteString(fmt.Sprintf("  %s %d", completedStyle.Render("●"), completed))
	stats.WriteString(fmt.Sprintf("  %s %d", failedStyle.Render("●"), failed))
	stats.WriteString("\n")
	stats.WriteString(labelStyle.Render(fmt.Sprintf("  %d sessions", m.totalSessions)))

	// Pad stats to fill the gap between logo and stats.
	statsStr := stats.String()
	gap := m.width - bannerWidth - lipgloss.Width(statsStr) - 4
	if gap < 2 {
		gap = 2
	}
	spacer := strings.Repeat(" ", gap)

	// Join horizontally: logo lines + spacer + stats lines.
	logoLines := strings.Split(styledLogo, "\n")
	statsLines := strings.Split(statsStr, "\n")

	// Pad to same height.
	for len(statsLines) < len(logoLines) {
		statsLines = append(statsLines, "")
	}
	for len(logoLines) < len(statsLines) {
		logoLines = append(logoLines, strings.Repeat(" ", bannerWidth))
	}

	var out strings.Builder
	for i := range logoLines {
		sl := ""
		if i < len(statsLines) {
			sl = statsLines[i]
		}
		out.WriteString(logoLines[i])
		out.WriteString(spacer)
		out.WriteString(sl)
		out.WriteString("\n")
	}

	return out.String()
}

// View renders the UI. AltScreen is set on the view (bubbletea v2 dropped the
// tea.WithAltScreen program option).
func (m Model) View() tea.View {
	return altScreenView(m.viewContent())
}

// altScreenView wraps rendered content in an alt-screen tea.View.
func altScreenView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// viewContent renders the frame body.
func (m Model) viewContent() string {
	if m.quitting {
		return ""
	}

	if m.errText != "" {
		return "Error: " + m.errText
	}

	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	header := m.renderBanner()
	contentHeight := m.contentHeight()

	var content string
	var help string

	switch m.viewMode {
	case viewList:
		content = renderSessionList(m.items, m.selected, m.width, contentHeight, m.hasMore(), len(m.allItems)-len(m.items))
		helpText := "  j/k: navigate  enter: open  g/G: top/bottom"
		if m.hasMore() {
			helpText += "  l: load more"
		}
		helpText += "  q: quit"
		help = m.renderHelp(helpText, "  j/k: navigate  enter: open  q: quit")

	case viewDetail:
		// Update owns the log content — the view only draws what the viewport
		// already holds, so no file is read here.
		meta := renderDetailMeta(m.selectedItem(), m.width, contentHeight, m.promptExpanded)
		content = clipLines(meta+"\n"+m.logView.View(), contentHeight)
		help = m.renderHelp(
			"  j/k: scroll  g/G: top/bottom  p: prompt  o: open log  x: stop  esc: back  q: quit",
			"  j/k: scroll  g/G: top/bottom  esc: back  q: quit",
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header+content, help)
}

// renderHelp renders the help bar so it never exceeds the terminal width.
// lipgloss.JoinVertical pads every row to the widest line, so a help bar wider
// than the terminal drags the entire frame over-width and the terminal hard-wraps
// it — the exact corruption this feature exists to remove. Fall back to a short
// form, then to a display-width truncation.
func (m Model) renderHelp(full, short string) string {
	if m.width <= 0 {
		return ""
	}
	text := full
	if ansi.StringWidth(text) > m.width {
		text = short
	}
	if ansi.StringWidth(text) > m.width {
		text = ansi.Truncate(text, m.width, "")
	}
	return helpStyle.Render(text)
}

// clipLines hard-truncates content to at most maxLines lines.
func clipLines(s string, maxLines int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	return strings.Join(lines[:maxLines], "\n")
}
