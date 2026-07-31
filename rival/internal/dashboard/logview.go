package dashboard

import (
	"os"
	"strings"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
	"github.com/charmbracelet/x/ansi"
)

// logTabWidth is how many spaces one tab expands to. Tabs must become spaces
// before wrapping: a tab is one rune but many cells, so leaving it in place
// makes wrapped lines overflow the terminal.
const logTabWidth = 4

// sanitizeLogLine makes one raw log line safe to render inside a fixed-width
// pane. The step order is load-bearing: sanitizeLog splits on "\n", so every
// line of a CRLF log arrives with a trailing "\r". Trimming that single
// terminator first is what keeps the progress-frame rule (keep only what
// follows the last remaining "\r") from blanking every line of such a log.
func sanitizeLogLine(line string) string {
	line = strings.TrimSuffix(line, "\r")
	if idx := strings.LastIndex(line, "\r"); idx >= 0 {
		line = line[idx+1:]
	}
	line = ansi.Strip(line)
	line = strings.ReplaceAll(line, "\t", strings.Repeat(" ", logTabWidth))

	var b strings.Builder
	b.Grow(len(line))
	for _, r := range line {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func sanitizeLog(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = sanitizeLogLine(line)
	}
	return strings.Join(lines, "\n")
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
