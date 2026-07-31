package dashboard

import (
	"fmt"
	"os"
	"strings"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/logfmt"
	"github.com/1F47E/rival/internal/session"
	"github.com/charmbracelet/x/ansi"
)

// sanitizeLog strips terminal control sequences and then expands tabs. The TUI
// needs the tab expansion the web path skips: a tab is one rune but many cells,
// so leaving it in place makes wrapped lines overflow the terminal.
func sanitizeLog(raw string) string {
	return logfmt.ExpandTabs(logfmt.Sanitize(raw), logfmt.TabWidth)
}

// wrapLogLines reads one session log, applies public model naming, strips
// terminal control sequences, and hard-wraps by display width so wide runes
// and tabs cannot push a line past wrapWidth.
func wrapLogLines(s *session.Session, wrapWidth int) []string {
	data, err := os.ReadFile(s.LogFile)
	if err != nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}

	text := sanitizeLog(config.PublicRuntimeLog(s.CLI, s.Model, string(data)))
	text = strings.TrimRight(text, "\n")
	if wrapWidth > 0 {
		text = ansi.Hardwrap(text, wrapWidth, true)
	}
	return strings.Split(text, "\n")
}

// buildLogContent renders one session's whole log, pre-wrapped to width. There
// is no line budget: the caller scrolls this content in a viewport.
func buildLogContent(s *session.Session, width int) string {
	if s == nil {
		return labelStyle.Render("(empty log)")
	}
	lines := wrapLogLines(s, width)
	if len(lines) == 0 {
		return labelStyle.Render("(empty log)")
	}
	return strings.Join(lines, "\n")
}

// buildGroupLogContent renders every group member's heading, runtime error and
// full log back to back, pre-wrapped to width. Like buildLogContent it has no
// line budget — the viewport scrolls it.
func buildGroupLogContent(item *displayItem, width int) string {
	if item == nil {
		return labelStyle.Render("(empty log)")
	}
	var b strings.Builder
	for _, sess := range item.Sessions {
		label := groupLogLabel(sess)
		if sess.Status == "failed" && sess.ErrorMsg != "" {
			label += " (FAILED)"
		}
		b.WriteString(titleStyle.Render(fmt.Sprintf("=== %s ===", label)))
		b.WriteString("\n")

		if sess.Status == "failed" && sess.ErrorMsg != "" {
			message := config.PublicRuntimeError(sess.CLI, sess.Model, sess.ErrorMsg)
			for _, line := range wrapText(message, width) {
				b.WriteString(failedStyle.Render(line))
				b.WriteString("\n")
			}
		}

		logLines := wrapLogLines(sess, width)
		if len(logLines) == 0 {
			b.WriteString(labelStyle.Render("(empty log)"))
			b.WriteString("\n")
		} else {
			b.WriteString(strings.Join(logLines, "\n"))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}
