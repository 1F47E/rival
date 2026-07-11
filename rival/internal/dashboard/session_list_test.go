package dashboard

import (
	"os"
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
)

func TestCLILabelUsesConcreteModelName(t *testing.T) {
	got := cliLabel("codex", config.GPT56SolModel, "review")
	if !strings.Contains(got, config.SolLabel) {
		t.Fatalf("reviewer label = %q, want %s", got, config.SolLabel)
	}
	if strings.Contains(strings.ToLower(got), "codex") || strings.Contains(strings.ToLower(got), "gpt-5.6") {
		t.Fatalf("reviewer label exposes adapter name: %q", got)
	}
}

func TestCreatePublicLogViewSanitizesRuntimeMetadata(t *testing.T) {
	rawPath := t.TempDir() + "/raw.log"
	raw := "OpenAI Codex v1\n--------\nmodel: " + config.GPT56SolModel + "\n--------\n"
	if err := os.WriteFile(rawPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	viewPath, err := createPublicLogView(&session.Session{CLI: "codex", Model: config.GPT56SolModel, LogFile: rawPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(viewPath) }()
	data, err := os.ReadFile(viewPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, config.GPT56SolModel) || strings.Contains(got, "Codex") || !strings.Contains(got, "Sol runtime") {
		t.Fatalf("opened log view was not normalized: %q", got)
	}
}

func TestGroupIconReflectsSelectedReviewerCount(t *testing.T) {
	tests := []struct {
		name     string
		sessions []*session.Session
		want     string
	}{
		{
			name: "one model plus judge",
			sessions: []*session.Session{
				{Mode: "megareview"}, {Mode: "consilium"},
			},
			want: "❯ mega",
		},
		{
			name: "three models plus judge",
			sessions: []*session.Session{
				{Mode: "megareview"}, {Mode: "megareview"}, {Mode: "megareview"}, {Mode: "consilium"},
			},
			want: "❯❯❯ mega",
		},
		{
			name: "four models plus judge",
			sessions: []*session.Session{
				{Mode: "megareview"}, {Mode: "megareview"}, {Mode: "megareview"}, {Mode: "megareview"}, {Mode: "consilium"},
			},
			want: "❯❯❯❯ mega",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := groupIcon(&displayItem{Sessions: tc.sessions}); got != tc.want {
				t.Fatalf("groupIcon() = %q, want %q", got, tc.want)
			}
		})
	}
}
