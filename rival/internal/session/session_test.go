package session

import (
	"testing"
	"time"

	"github.com/1F47E/rival/internal/config"
)

func TestSortGroupMembersUsesCreationOrderAndPutsJudgeLast(t *testing.T) {
	created := time.Now()
	second := created.Add(time.Millisecond)
	third := second.Add(time.Millisecond)
	sessions := []*Session{
		{ID: "judge", CLI: "codex", Model: config.GPT56SolModel, Mode: "consilium", QueuedAt: &third},
		{ID: "fable", CLI: "fable", Model: config.FableModel, Mode: "plan", QueuedAt: &second},
		{ID: "sol", CLI: "codex", Model: config.GPT56SolModel, Mode: "plan", QueuedAt: &created},
	}

	SortGroupMembers(sessions)
	for i, want := range []string{"sol", "fable", "judge"} {
		if sessions[i].ID != want {
			t.Fatalf("member %d = %q, want %q", i, sessions[i].ID, want)
		}
	}
}

func TestSortGroupMembersUsesCuratedFallbackForLegacySessions(t *testing.T) {
	sessions := []*Session{
		{ID: "fable", CLI: "fable", Model: config.FableModel, Mode: "plan"},
		{ID: "k3", CLI: "opencode", Model: config.KimiModel, Mode: "megareview"},
		{ID: "sol", CLI: "codex", Model: config.GPT56SolModel, Mode: "plan"},
		{ID: "deepseek", CLI: "opencode", Model: config.OpencodeDeepSeekPro, Mode: "megareview"},
	}

	SortGroupMembers(sessions)
	for i, want := range []string{"sol", "deepseek", "k3", "fable"} {
		if sessions[i].ID != want {
			t.Fatalf("legacy member %d = %q, want %q", i, sessions[i].ID, want)
		}
	}
}

func TestSortGroupMembersPreservesExplicitReviewerOrder(t *testing.T) {
	created := time.Now()
	later := created.Add(time.Millisecond)
	sessions := []*Session{
		{ID: "sol", CLI: "codex", Model: config.GPT56SolModel, Mode: "megareview", QueuedAt: &later},
		{ID: "k3", CLI: "opencode", Model: config.KimiModel, Mode: "megareview", QueuedAt: &created},
	}

	SortGroupMembers(sessions)
	if sessions[0].ID != "k3" || sessions[1].ID != "sol" {
		t.Fatalf("explicit reviewer order was not preserved: %s, %s", sessions[0].ID, sessions[1].ID)
	}
}
