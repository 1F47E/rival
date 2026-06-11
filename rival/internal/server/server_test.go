package server

import (
	"testing"

	"github.com/1F47E/rival/internal/session"
)

func TestGroupSessions(t *testing.T) {
	tests := []struct {
		name        string
		sessions    []*session.Session
		wantGroups  int
		wantIsGroup []bool // per resulting group, in order
		wantCLI     []string
	}{
		{
			name:     "empty",
			sessions: nil,
			wantGroups: 0,
		},
		{
			name: "two solo sessions stay separate",
			sessions: []*session.Session{
				{ID: "a", CLI: "codex", Model: "gpt-5.5", Status: "completed"},
				{ID: "b", CLI: "gemini", Model: "gemini-3.1", Status: "completed"},
			},
			wantGroups:  2,
			wantIsGroup: []bool{false, false},
			wantCLI:     []string{"codex", "gemini"},
		},
		{
			name: "shared GroupID collapses into one mega row",
			sessions: []*session.Session{
				{ID: "a", GroupID: "g1", CLI: "codex", Model: "gpt-5.5", Status: "completed"},
				{ID: "b", GroupID: "g1", CLI: "antigravity", Model: "gemini-3.1", Status: "completed"},
			},
			wantGroups:  1,
			wantIsGroup: []bool{true},
			wantCLI:     []string{"mega"},
		},
		{
			name: "mixed: one mega group + one solo",
			sessions: []*session.Session{
				{ID: "a", GroupID: "g1", CLI: "codex", Model: "gpt-5.5", Status: "completed"},
				{ID: "b", GroupID: "g1", CLI: "antigravity", Model: "gemini-3.1", Status: "completed"},
				{ID: "c", CLI: "claude", Model: "opus", Status: "running"},
			},
			wantGroups:  2,
			wantIsGroup: []bool{true, false},
			wantCLI:     []string{"mega", "claude"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupSessions(tt.sessions)
			if len(got) != tt.wantGroups {
				t.Fatalf("groupSessions() returned %d groups, want %d", len(got), tt.wantGroups)
			}
			for i := range got {
				if got[i].IsGroup != tt.wantIsGroup[i] {
					t.Errorf("group %d IsGroup = %v, want %v", i, got[i].IsGroup, tt.wantIsGroup[i])
				}
				if got[i].CLI != tt.wantCLI[i] {
					t.Errorf("group %d CLI = %q, want %q", i, got[i].CLI, tt.wantCLI[i])
				}
			}
		})
	}
}

func TestGroupStatus(t *testing.T) {
	tests := []struct {
		name     string
		statuses []string
		want     string
	}{
		{"all completed", []string{"completed", "completed"}, "completed"},
		{"any running wins", []string{"completed", "running", "failed"}, "running"},
		{"failed over completed", []string{"completed", "failed"}, "failed"},
		{"single failed", []string{"failed"}, "failed"},
		{"queued over failed and completed", []string{"completed", "queued", "failed"}, "queued"},
		{"running over queued", []string{"queued", "running"}, "running"},
		{"single queued", []string{"queued"}, "queued"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sessions []*session.Session
			for _, s := range tt.statuses {
				sessions = append(sessions, &session.Session{Status: s})
			}
			if got := groupStatus(sessions); got != tt.want {
				t.Errorf("groupStatus(%v) = %q, want %q", tt.statuses, got, tt.want)
			}
		})
	}
}

func TestGroupModels_Dedupes(t *testing.T) {
	sessions := []*session.Session{
		{Model: "gpt-5.5"},
		{Model: "gemini-3.1"},
		{Model: "gpt-5.5"}, // duplicate
		{Model: ""},        // skipped
	}
	got := groupModels(sessions)
	want := "gpt-5.5 + gemini-3.1"
	if got != want {
		t.Errorf("groupModels() = %q, want %q", got, want)
	}
}
