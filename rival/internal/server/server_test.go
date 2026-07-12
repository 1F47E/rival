package server

import (
	"slices"
	"strings"
	"testing"
	"time"

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

func TestPairedPlanGroupUsesPublicModels(t *testing.T) {
	created := time.Now()
	later := created.Add(time.Millisecond)
	groups := groupSessions([]*session.Session{
		{ID: "b", GroupID: "paired", CLI: "fable", Mode: "plan", Model: config.FableModel, Status: "completed", QueuedAt: &later},
		{ID: "a", GroupID: "paired", CLI: "codex", Mode: "plan", Model: config.GPT56SolModel, Status: "completed", QueuedAt: &created},
	})
	if len(groups) != 1 {
		t.Fatalf("paired plan group count = %d, want 1", len(groups))
	}
	group := groups[0]
	if group.ID != "paired" {
		t.Fatalf("paired plan stable id = %q, want group id", group.ID)
	}
	if group.Kind != "plan" || group.CLI != "sol+fable" || group.Models != "sol + fable" {
		t.Fatalf("paired plan group labels = kind %q cli %q models %q", group.Kind, group.CLI, group.Models)
	}
	if len(group.Sessions) != 2 || group.Sessions[0].CLI != "sol" || group.Sessions[0].Model != "sol" || group.Sessions[1].CLI != "fable" || group.Sessions[1].Model != "fable" {
		t.Fatalf("paired plan public sessions = %+v", group.Sessions)
	}
}

func TestSingletonPlanKeepsLogicalGroupIdentity(t *testing.T) {
	groups := groupSessions([]*session.Session{
		{ID: "fable", GroupID: "degraded-plan", CLI: "fable", Mode: "plan", Model: config.FableModel, Status: "failed", ErrorMsg: "fable failed"},
	})
	if len(groups) != 1 {
		t.Fatalf("singleton plan group count = %d, want 1", len(groups))
	}
	group := groups[0]
	if !group.IsGroup || group.ID != "degraded-plan" || group.Kind != "plan" || group.CLI != "fable" || group.Sessions[0].ErrorMsg != "fable failed" {
		t.Fatalf("singleton plan group = %+v", group)
	}
}

func TestCuratedReviewGroupUsesPublicModels(t *testing.T) {
	created := time.Now()
	times := make([]time.Time, 4)
	for i := range times {
		times[i] = created.Add(time.Duration(i) * time.Millisecond)
	}
	sessions := []*session.Session{
		{ID: "d", GroupID: "review", CLI: "opencode", Mode: "megareview", Model: config.OpencodeGLMModel, Status: "completed", QueuedAt: &times[3]},
		{ID: "c", GroupID: "review", CLI: "opencode", Mode: "megareview", Model: config.OpencodeKimiK27Code, Status: "failed", ErrorMsg: "kimi failed", QueuedAt: &times[2]},
		{ID: "b", GroupID: "review", CLI: "opencode", Mode: "megareview", Model: config.OpencodeDeepSeekPro, Status: "completed", QueuedAt: &times[1]},
		{ID: "a", GroupID: "review", CLI: "codex", Mode: "megareview", Model: config.GPT56SolModel, Status: "completed", QueuedAt: &times[0]},
	}
	groups := groupSessions(sessions)
	if len(groups) != 1 {
		t.Fatalf("review group count = %d, want 1", len(groups))
	}
	group := groups[0]
	wantLabels := []string{"sol", "deepseek-v4-pro", "kimi-k2.7-code", "glm-5.2"}
	if group.Kind != "megareview" || group.CLI != strings.Join(wantLabels, "+") || group.Models != strings.Join(wantLabels, " + ") {
		t.Fatalf("review group labels = kind %q cli %q models %q", group.Kind, group.CLI, group.Models)
	}
	gotLabels := make([]string, 0, len(group.Sessions))
	for _, s := range group.Sessions {
		if s.CLI != s.Model {
			t.Fatalf("public session labels disagree: %+v", s)
		}
		gotLabels = append(gotLabels, s.Model)
	}
	if !slices.Equal(gotLabels, wantLabels) {
		t.Fatalf("public review sessions = %v, want %v", gotLabels, wantLabels)
	}
	if group.Sessions[2].ErrorMsg != "kimi failed" {
		t.Fatalf("non-primary failure missing from public group: %+v", group.Sessions[2])
	}
}

func TestIndexIncludesCuratedModelIcons(t *testing.T) {
	data, err := indexHTML.ReadFile("templates/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, label := range []string{"sol", "deepseek-v4-pro", "kimi-k2.7-code", "glm-5.2", "fable", "gemini-3.1-pro-preview", "gemini-3.5-flash"} {
		if !strings.Contains(html, label+"': '") && !strings.Contains(html, label+": '") {
			t.Errorf("web dashboard has no icon mapping for %q", label)
		}
	}
	for _, behavior := range []string{"const id = g.id", "g => g.id === id", "s.mode === 'consilium' ? 'JUDGE' : 'REVIEW'", "if (s.error) logText += 'Error: '"} {
		if !strings.Contains(html, behavior) {
			t.Errorf("web dashboard missing grouped-session behavior %q", behavior)
		}
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
