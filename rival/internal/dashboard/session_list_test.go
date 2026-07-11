package dashboard

import (
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
)

func TestCLILabelUsesConcreteModelName(t *testing.T) {
	got := cliLabel("codex", config.GPT56SolModel, "review")
	if !strings.Contains(got, config.GPT56SolModel) {
		t.Fatalf("reviewer label = %q, want %s", got, config.GPT56SolModel)
	}
	if strings.Contains(strings.ToLower(got), "codex") {
		t.Fatalf("reviewer label exposes adapter name: %q", got)
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
