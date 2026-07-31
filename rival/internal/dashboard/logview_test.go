package dashboard

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/session"
	"github.com/charmbracelet/x/ansi"
)

func TestSanitizeLog(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"tab expands to four spaces", "if x {\n\treturn\n}", "if x {\n    return\n}"},
		{"progress frames collapse to the last", "10%\r99%", "99%"},
		{"csi color stripped", "\x1b[31mred\x1b[0m", "red"},
		{"osc bel stripped", "\x1b]0;title\x07text", "text"},
		{"osc st stripped", "\x1b]8;;url\x1b\\link", "link"},
		{"backspace and nul dropped", "a\x08b\x00c", "abc"},
		{"newlines preserved", "one\ntwo\nthree", "one\ntwo\nthree"},
		{"plain text unchanged", "plain log line", "plain log line"},
		// CRLF: sanitizeLog splits on "\n", so each line still ends in "\r".
		// The trailing-terminator trim must run before the progress-frame rule
		// or every line of a CRLF log collapses to empty.
		{"crlf lines survive", "alpha\r\nbeta\r\n", "alpha\nbeta\n"},
		{"trailing frame keeps its text", "working...\r", "working..."},
		{"crlf plus progress frames", "10%\r99%\r\n", "99%\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeLog(tc.raw); got != tc.want {
				t.Fatalf("sanitizeLog(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// The original bug: wrapping by rune count let tabs and wide runes push lines
// past the pane width, so the terminal hard-wrapped them itself and bubbletea's
// repaint desynced. Every emitted line must fit in wrapWidth display cells.
func TestWrapLogLinesDisplayWidth(t *testing.T) {
	logPath := t.TempDir() + "/session.log"
	raw := strings.Join([]string{
		"func main() {",
		"\tif err := run(); err != nil {",
		"\t\tlog.Fatal(err)",
		"\t}",
		"}",
		strings.Repeat("あ", 50),
		"\x1b[31m" + strings.Repeat("error detail ", 20) + "\x1b[0m",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &session.Session{CLI: "codex", Model: "gpt-5", LogFile: logPath}

	for _, width := range []int{20, 40, 80} {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			lines := wrapLogLines(s, width)
			if len(lines) == 0 {
				t.Fatalf("wrapLogLines(width=%d) returned no lines", width)
			}
			for i, line := range lines {
				if got := ansi.StringWidth(line); got > width {
					t.Fatalf("line %d width = %d, want <= %d: %q", i, got, width, line)
				}
			}
		})
	}
}

func TestWrapLogLinesEmptyAndMissing(t *testing.T) {
	empty := t.TempDir() + "/empty.log"
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := wrapLogLines(&session.Session{LogFile: empty}, 80); got != nil {
		t.Fatalf("empty log = %q, want nil", got)
	}
	if got := wrapLogLines(&session.Session{LogFile: t.TempDir() + "/missing.log"}, 80); got != nil {
		t.Fatalf("missing log = %q, want nil", got)
	}
}
