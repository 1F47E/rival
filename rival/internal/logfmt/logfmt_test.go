package logfmt

import "testing"

func TestSanitize(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"tabs survive sanitize", "if x {\n\treturn\n}", "if x {\n\treturn\n}"},
		{"progress frames collapse to the last", "10%\r99%", "99%"},
		{"csi color stripped", "\x1b[31mred\x1b[0m", "red"},
		{"osc bel stripped", "\x1b]0;title\x07text", "text"},
		{"osc st stripped", "\x1b]8;;url\x1b\\link", "link"},
		{"backspace and nul dropped", "a\x08b\x00c", "abc"},
		{"newlines preserved", "one\ntwo\nthree", "one\ntwo\nthree"},
		{"plain text unchanged", "plain log line", "plain log line"},
		// CRLF: Sanitize splits on "\n", so each line still ends in "\r". The
		// trailing-terminator trim must run before the progress-frame rule or
		// every line of a CRLF log collapses to empty.
		{"crlf lines survive", "alpha\r\nbeta\r\n", "alpha\nbeta\n"},
		{"trailing frame keeps its text", "working...\r", "working..."},
		{"crlf plus progress frames", "10%\r99%\r\n", "99%\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sanitize(tc.raw); got != tc.want {
				t.Fatalf("Sanitize(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestExpandTabs(t *testing.T) {
	if got := ExpandTabs("a\tb", TabWidth); got != "a    b" {
		t.Fatalf("ExpandTabs = %q", got)
	}
	if got := ExpandTabs("a\tb", 0); got != "ab" {
		t.Fatalf("ExpandTabs width 0 = %q", got)
	}
	if got := ExpandTabs("a\tb", -1); got != "ab" {
		t.Fatalf("ExpandTabs negative width = %q", got)
	}
}
