package server

import (
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
)

func TestGroupSessions(t *testing.T) {
	tests := []struct {
		name        string
		sessions    []*session.Session
		wantGroups  int
		wantIsGroup []bool // per resulting group, in order
		wantCLI     []string
		wantKind    []string // "" for solo groups
	}{
		{
			name:       "empty",
			sessions:   nil,
			wantGroups: 0,
		},
		{
			name: "two solo sessions stay separate",
			sessions: []*session.Session{
				{ID: "a", CLI: "codex", Model: config.GPT56SolModel, Status: "completed"},
				{ID: "b", CLI: "gemini", Model: "gemini-3.1", Status: "completed"},
			},
			wantGroups:  2,
			wantIsGroup: []bool{false, false},
			wantCLI:     []string{config.SolLabel, "gemini-3.1"},
			wantKind:    []string{"", ""},
		},
		{
			name: "shared GroupID collapses into one mega row",
			sessions: []*session.Session{
				{ID: "a", GroupID: "g1", CLI: "codex", Mode: "megareview", Model: config.GPT56SolModel, Status: "completed"},
				{ID: "b", GroupID: "g1", CLI: "antigravity", Mode: "megareview", Model: "gemini-3.1", Status: "completed"},
			},
			wantGroups:  1,
			wantIsGroup: []bool{true},
			wantCLI:     []string{config.SolLabel + "+gemini-3.1"},
			wantKind:    []string{"megareview"},
		},
		{
			name: "plan group is labelled with concrete model names",
			sessions: []*session.Session{
				{ID: "a", GroupID: "gp", CLI: "codex", Mode: "plan", Model: config.CodexModel, Status: "completed"},
				{ID: "b", GroupID: "gp", CLI: "fable", Mode: "plan", Model: config.FableModel, Status: "completed"},
			},
			wantGroups:  1,
			wantIsGroup: []bool{true},
			wantCLI:     []string{config.SolLabel + "+" + config.FableLabel},
			wantKind:    []string{"plan"},
		},
		{
			name: "mixed: one mega group + one solo",
			sessions: []*session.Session{
				{ID: "a", GroupID: "g1", CLI: "codex", Mode: "megareview", Model: config.GPT56SolModel, Status: "completed"},
				{ID: "b", GroupID: "g1", CLI: "antigravity", Mode: "megareview", Model: "gemini-3.1", Status: "completed"},
				{ID: "c", CLI: "claude", Model: "opus", Status: "running"},
			},
			wantGroups:  2,
			wantIsGroup: []bool{true, false},
			wantCLI:     []string{config.SolLabel + "+gemini-3.1", "opus"},
			wantKind:    []string{"megareview", ""},
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
				if tt.wantKind != nil && got[i].Kind != tt.wantKind[i] {
					t.Errorf("group %d Kind = %q, want %q", i, got[i].Kind, tt.wantKind[i])
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
		{Model: config.GPT56SolModel},
		{Model: "gemini-3.1"},
		{Model: config.GPT56SolModel}, // duplicate
		{Model: ""},                   // skipped
	}
	got := groupModels(sessions)
	want := config.SolLabel + " + gemini-3.1"
	if got != want {
		t.Errorf("groupModels() = %q, want %q", got, want)
	}
}

func TestPublicSessionsUsesOnlyPublicNames(t *testing.T) {
	original := &session.Session{CLI: "codex", Model: config.GPT56SolModel, ErrorMsg: "Codex CLI failed for " + config.GPT56SolModel}
	got := publicSessions([]*session.Session{original})
	if len(got) != 1 || got[0].CLI != config.SolLabel || got[0].Model != config.SolLabel {
		t.Fatalf("public session = %+v, want sol labels", got)
	}
	if original.CLI != "codex" || original.Model != config.GPT56SolModel {
		t.Fatal("publicSessions mutated persisted session metadata")
	}
	if strings.Contains(got[0].ErrorMsg, "Codex") || strings.Contains(got[0].ErrorMsg, config.GPT56SolModel) {
		t.Fatalf("public session error was not normalized: %q", got[0].ErrorMsg)
	}
}

func TestPublicLogDataNormalizesRuntimeBanner(t *testing.T) {
	raw := []byte("OpenAI Codex v1\n--------\nmodel: " + config.GPT56SolModel + "\n--------\n")
	data, ok := publicLogData("a", raw, []*session.Session{{ID: "a", CLI: "codex", Model: config.GPT56SolModel}})
	if !ok {
		t.Fatal("expected matching session metadata")
	}
	got := string(data)
	if strings.Contains(got, config.GPT56SolModel) || strings.Contains(got, "Codex") || !strings.Contains(got, "Sol runtime") {
		t.Fatalf("public log was not normalized: %q", got)
	}
	if _, ok := publicLogData("missing", raw, nil); ok {
		t.Fatal("orphan log must fail closed without session metadata")
	}
}
