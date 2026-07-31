// Package logfmt makes raw CLI logs safe to display. Runtime logs are captured
// verbatim, so they carry ANSI escapes, carriage-return progress frames and
// stray control bytes that neither a terminal pane nor a browser renders
// sensibly. Both the TUI (internal/dashboard) and the web server
// (internal/server) share this one implementation; only the TUI additionally
// expands tabs, because the web page lets CSS tab-size do that job.
package logfmt

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// TabWidth is how many spaces ExpandTabs substitutes for one tab.
const TabWidth = 4

// SanitizeLine makes one raw log line safe to display: it keeps only the last
// progress frame, strips ANSI escape sequences, and drops C0 control chars and
// DEL. Tabs survive — expand them separately with ExpandTabs when the renderer
// has no tab semantics of its own.
//
// The step order is load-bearing: Sanitize splits on "\n", so every line of a
// CRLF log arrives with a trailing "\r". Trimming that single terminator first
// is what keeps the progress-frame rule (keep only what follows the last
// remaining "\r") from blanking every line of such a log.
func SanitizeLine(line string) string {
	line = strings.TrimSuffix(line, "\r")
	if idx := strings.LastIndex(line, "\r"); idx >= 0 {
		line = line[idx+1:]
	}
	line = ansi.Strip(line)

	var b strings.Builder
	b.Grow(len(line))
	for _, r := range line {
		if r == '\t' {
			b.WriteRune(r)
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Sanitize applies SanitizeLine to every line of raw, preserving newlines.
func Sanitize(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = SanitizeLine(line)
	}
	return strings.Join(lines, "\n")
}

// ExpandTabs replaces every tab with width spaces. Terminal panes need this
// before wrapping: a tab is one rune but many cells, so leaving it in place
// makes wrapped lines overflow. A non-positive width removes tabs entirely,
// matching strings.Repeat semantics.
func ExpandTabs(s string, width int) string {
	if width < 0 {
		width = 0
	}
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", width))
}
