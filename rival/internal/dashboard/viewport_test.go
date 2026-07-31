package dashboard

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
	"github.com/charmbracelet/x/ansi"
)

// key builds a KeyPressMsg for a single-rune or named key.
func key(s string) tea.KeyPressMsg {
	if len([]rune(s)) == 1 {
		return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
	}
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	default:
		panic("unhandled key " + s)
	}
}

// upperKey builds a shifted single-rune keypress (e.g. "G").
func upperKey(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r), Mod: tea.ModShift}
}

func writeTempLog(t *testing.T, name, content string) string {
	t.Helper()
	path := t.TempDir() + "/" + name
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// nastyLog mixes tabs, CJK, and ANSI escapes — everything that made the old
// rune-counting wrap overflow the terminal.
const nastyLog = "\tfunc main() {\n\t\tfmt.Println(\"日本語のテキストはとても幅が広いですね、これは長い行です\")\n\x1b[31m\terror: something went terribly wrong in a very long line that must wrap\x1b[0m\n\t}\n"

func newTestModel(t *testing.T, logBody string) Model {
	t.Helper()
	m := New()
	t.Cleanup(m.cancel)
	m.items = []displayItem{{Sessions: []*session.Session{{
		ID:        "widthtest",
		CLI:       "codex",
		Model:     config.GPT56SolModel,
		Mode:      "review",
		Effort:    "ultra",
		Status:    "running",
		StartTime: time.Now(),
		Prompt:    strings.Repeat("review this repository carefully ", 40),
		LogFile:   writeTempLog(t, "widthtest.log", logBody),
	}}}}
	m.allItems = m.items
	return m
}

func assertFrameWidth(t *testing.T, m Model, label string) {
	t.Helper()
	for i, line := range strings.Split(m.viewContent(), "\n") {
		if w := ansi.StringWidth(line); w > m.width {
			t.Fatalf("%s: line %d width %d exceeds terminal width %d: %q", label, i, w, m.width, line)
		}
	}
}

func TestViewNeverExceedsWidth(t *testing.T) {
	body := nastyLog + strings.Repeat("padding line to make the log long enough to scroll\n", 200)

	for _, width := range []int{60, 80, 120} {
		m := newTestModel(t, body)

		updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		m = updated.(Model)
		assertFrameWidth(t, m, "list mode")

		updated, _ = m.Update(key("enter"))
		m = updated.(Model)
		if m.viewMode != viewDetail {
			t.Fatalf("width %d: enter did not switch to detail mode", width)
		}
		assertFrameWidth(t, m, "detail mode")

		// Expanded prompt is the widest meta state.
		updated, _ = m.Update(key("p"))
		m = updated.(Model)
		assertFrameWidth(t, m, "detail mode, prompt expanded")
	}
}

func TestDetailViewportStickyBottom(t *testing.T) {
	logPath := writeTempLog(t, "sticky.log", strings.Repeat("initial line\n", 200))

	m := New()
	t.Cleanup(m.cancel)
	m.items = []displayItem{{Sessions: []*session.Session{{
		ID:        "sticky",
		CLI:       "codex",
		Model:     config.GPT56SolModel,
		Mode:      "review",
		Effort:    "ultra",
		Status:    "running",
		StartTime: time.Now(),
		Prompt:    strings.Repeat("sticky prompt text ", 30),
		LogFile:   logPath,
	}}}}
	m.allItems = m.items

	appendLine := func(marker string) {
		t.Helper()
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(marker + "\n"); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	tick := func(m Model) Model {
		t.Helper()
		updated, _ := m.Update(tickMsg(time.Now()))
		return updated.(Model)
	}

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = updated.(Model)

	// Enter detail through the real key path, not by calling sync directly.
	updated, _ = m.Update(key("enter"))
	m = updated.(Model)
	if m.viewMode != viewDetail {
		t.Fatal("enter did not switch to detail mode")
	}
	if !m.logView.AtBottom() {
		t.Fatal("detail view did not open at the tail")
	}

	// Tail follows while pinned to the bottom.
	appendLine("MARKER-ONE")
	m = tick(m)
	if !strings.Contains(m.logView.GetContent(), "MARKER-ONE") {
		t.Fatal("tick did not re-read the log: MARKER-ONE missing from viewport content")
	}
	if !m.logView.AtBottom() {
		t.Fatal("viewport lost the bottom after appending while pinned")
	}

	// Scroll up: position must hold as the log grows.
	updated, _ = m.Update(key("k"))
	m = updated.(Model)
	if m.logView.AtBottom() {
		t.Fatal("k did not scroll the viewport up")
	}
	offsetBefore := m.logView.YOffset()

	appendLine("MARKER-TWO")
	m = tick(m)
	if m.logView.AtBottom() {
		t.Fatal("viewport snapped back to the bottom although the user had scrolled up")
	}
	if got := m.logView.YOffset(); got != offsetBefore {
		t.Fatalf("scroll offset moved from %d to %d while scrolled up", offsetBefore, got)
	}
	if !strings.Contains(m.logView.GetContent(), "MARKER-TWO") {
		t.Fatal("tick did not re-read the log while scrolled up: MARKER-TWO missing")
	}

	// Back to the bottom, then shrink the terminal. This is the regression test
	// for capture-before-resize: sampling AtBottom after SetHeight reports false
	// and kills tail-follow.
	updated, _ = m.Update(upperKey('G'))
	m = updated.(Model)
	if !m.logView.AtBottom() {
		t.Fatal("G did not jump to the bottom")
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 18})
	m = updated.(Model)
	if !m.logView.AtBottom() {
		t.Fatal("resize while pinned to the bottom lost tail-follow")
	}

	// Prompt expansion shrinks the viewport the same way.
	updated, _ = m.Update(key("p"))
	m = updated.(Model)
	if !m.promptExpanded {
		t.Fatal("p did not expand the prompt")
	}
	if !m.logView.AtBottom() {
		t.Fatal("prompt expansion while pinned to the bottom lost tail-follow")
	}
}

func TestDetailScrollKeysReachTheViewport(t *testing.T) {
	m := newTestModel(t, strings.Repeat("scrollable line\n", 300))

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(Model)
	updated, _ = m.Update(key("enter"))
	m = updated.(Model)

	before := m.logView.YOffset()
	updated, _ = m.Update(key("k"))
	m = updated.(Model)
	if m.logView.YOffset() >= before {
		t.Fatalf("k did not scroll the log up (offset %d -> %d)", before, m.logView.YOffset())
	}
	if m.selected != 0 {
		t.Fatalf("k moved the list selection in detail mode: selected=%d", m.selected)
	}

	// g jumps to the top of the log, not the top of the list.
	updated, _ = m.Update(key("g"))
	m = updated.(Model)
	if !m.logView.AtTop() {
		t.Fatal("g did not jump the viewport to the top")
	}
}
