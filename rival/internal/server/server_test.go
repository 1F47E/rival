package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
	for _, behavior := range []string{
		"const id = g.id",
		"g => g.id === id",
		"s.mode === 'consilium' ? 'JUDGE' : 'REVIEW'",
		"if (s.error) logText += 'Error: '",
		"Waiting for a queue slot",
		"unavailable = true",
		"s.status === 'failed' || unavailable",
		"syncOpenDetail(selected)",
		"state.detailGroup",
		"member-status",
		"window.addEventListener('hashchange'",
		"byId('detail-drawer').scrollTop = 0",
		"fetchRun(id",
		"Session detail is no longer available",
	} {
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

func TestGroupElapsedUsesWallClockSpanAcrossSequentialMembers(t *testing.T) {
	start := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	reviewerEnd := start.Add(4 * time.Minute)
	judgeStart := reviewerEnd
	judgeEnd := judgeStart.Add(3 * time.Minute)

	got := groupElapsed([]*session.Session{
		{Status: "completed", StartTime: start, EndTime: &reviewerEnd, Duration: "4m0s"},
		{Status: "failed", StartTime: judgeStart, EndTime: &judgeEnd, Duration: "3m0s"},
	})
	if got != "7m0s" {
		t.Fatalf("groupElapsed() = %q, want wall-clock span %q", got, "7m0s")
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

func TestSessionsAPIIsBoundedAndExcludesPrivateFields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := config.SessionDirPath()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}

	ids := []string{
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000002",
		"00000000-0000-4000-8000-000000000003",
	}
	for i, id := range ids {
		s := &session.Session{
			ID:            id,
			CLI:           "opencode",
			Mode:          "raw",
			Model:         "test-model",
			Effort:        "high",
			Prompt:        "private-prompt-" + id,
			PromptPreview: "preview-" + id,
			Status:        "completed",
			StartTime:     time.Now().Add(time.Duration(i) * time.Minute),
			WorkDir:       "/private/workdir",
			LogFile:       "/private/internal.log",
			PID:           4242,
		}
		if err := s.Save(); err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sessions?limit=1&offset=1", nil)
	rr := httptest.NewRecorder()
	New("test-version").ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}

	var got apiResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 1 || got.Pagination.Shown != 1 || got.Pagination.Offset != 1 || got.Pagination.TotalGroups != 3 {
		t.Fatalf("pagination response = %+v, groups = %d", got.Pagination, len(got.Groups))
	}
	if !got.Pagination.HasMore || !got.Pagination.HasPrevious {
		t.Fatalf("pagination directions = %+v", got.Pagination)
	}
	if got.Groups[0].Sessions[0].ID != ids[1] {
		t.Fatalf("middle page id = %q, want %q", got.Groups[0].Sessions[0].ID, ids[1])
	}

	body := rr.Body.String()
	for _, private := range []string{"private-prompt-", "/private/internal.log", `"pid"`, `"log_file"`} {
		if strings.Contains(body, private) {
			t.Errorf("list API leaked private field %q", private)
		}
	}
}

func TestRunAPIResolvesGroupOutsideListPagination(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(config.SessionDirPath(), 0700); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for index, id := range []string{
		"00000000-0000-4000-8000-000000000011",
		"00000000-0000-4000-8000-000000000012",
	} {
		s := &session.Session{
			ID:            id,
			GroupID:       "older-group",
			CLI:           "codex",
			Mode:          "megareview",
			Model:         config.GPT56SolModel,
			Effort:        "high",
			PromptPreview: "group prompt",
			Status:        []string{"completed", "failed"}[index],
			StartTime:     now.Add(time.Duration(index) * time.Second),
			WorkDir:       "/work",
		}
		if err := s.Save(); err != nil {
			t.Fatal(err)
		}
	}

	mux := New("test")
	req := httptest.NewRequest(http.MethodGet, "/api/run?id=older-group", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var group sessionGroup
	if err := json.Unmarshal(rr.Body.Bytes(), &group); err != nil {
		t.Fatal(err)
	}
	if group.ID != "older-group" || len(group.Sessions) != 2 || group.Status != "failed" {
		t.Fatalf("resolved group = %+v", group)
	}

	for _, tc := range []struct {
		method string
		target string
		want   int
	}{
		{http.MethodGet, "/api/run?id=missing", http.StatusNotFound},
		{http.MethodGet, "/api/run", http.StatusBadRequest},
		{http.MethodPost, "/api/run?id=older-group", http.StatusMethodNotAllowed},
	} {
		req := httptest.NewRequest(tc.method, tc.target, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != tc.want {
			t.Errorf("%s %s status = %d, want %d", tc.method, tc.target, rr.Code, tc.want)
		}
	}
}

func TestPromptResponseAndLogTailAreBounded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	largePrompt := strings.Repeat("prompt", (maxPromptSourceFileBytes/6)+1)
	stored := session.Session{Prompt: largePrompt}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	detail, err := loadPromptResponse(path, &session.Session{PromptPreview: "saved preview"})
	if err != nil {
		t.Fatal(err)
	}
	if !detail.Truncated || !detail.PreviewOnly || detail.Prompt != "saved preview" {
		t.Fatalf("large prompt response = %+v", detail)
	}

	logPath := filepath.Join(dir, "session.log")
	logData := []byte("prefix€" + strings.Repeat("x", maxLogTailBytes) + "tail")
	if err := os.WriteFile(logPath, logData, 0600); err != nil {
		t.Fatal(err)
	}
	tail, truncated, err := readLogTail(logPath, maxLogTailBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(tail) > maxLogTailBytes || !utf8.Valid(tail) || !strings.HasSuffix(string(tail), "tail") {
		t.Fatalf("log tail: truncated=%v bytes=%d utf8=%v suffix=%q", truncated, len(tail), utf8.Valid(tail), tail[max(0, len(tail)-8):])
	}
}

func TestListPageValidationAndCap(t *testing.T) {
	for _, tc := range []struct {
		target     string
		wantLimit  int
		wantOffset int
		wantErr    bool
	}{
		{target: "/api/sessions", wantLimit: defaultListLimit},
		{target: "/api/sessions?limit=99999&offset=12", wantLimit: maxListLimit, wantOffset: 12},
		{target: "/api/sessions?limit=0", wantErr: true},
		{target: "/api/sessions?offset=-1", wantErr: true},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.target, nil)
		limit, offset, err := listPage(req)
		if (err != nil) != tc.wantErr {
			t.Fatalf("listPage(%q) error = %v, wantErr %v", tc.target, err, tc.wantErr)
		}
		if err == nil && (limit != tc.wantLimit || offset != tc.wantOffset) {
			t.Fatalf("listPage(%q) = (%d, %d), want (%d, %d)", tc.target, limit, offset, tc.wantLimit, tc.wantOffset)
		}
	}
}
