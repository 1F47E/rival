package dashboard

import (
	"os"
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
)

func TestGroupDetailReservesSpaceForEveryPlanLog(t *testing.T) {
	writeLog := func(name, content string) string {
		t.Helper()
		path := t.TempDir() + "/" + name
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	item := &displayItem{Sessions: []*session.Session{
		{ID: "sol", GroupID: "paired-plan", CLI: "codex", Model: config.GPT56SolModel, Mode: "plan", Status: "completed", Prompt: strings.Repeat("long plan context ", 100), LogFile: writeLog("sol.log", "sol output\n")},
		{ID: "fable", GroupID: "paired-plan", CLI: "fable", Model: config.FableModel, Mode: "plan", Status: "completed", LogFile: writeLog("fable.log", "fable output\n")},
	}}

	got := renderGroupDetailView(item, 80, 24, false)
	for _, heading := range []string{"SOL REVIEW", "FABLE REVIEW"} {
		if !strings.Contains(got, heading) {
			t.Fatalf("24-line grouped detail omitted %q:\n%s", heading, got)
		}
	}
	if lines := strings.Count(got, "\n") + 1; lines > 24 {
		t.Fatalf("group detail rendered %d lines, want at most 24", lines)
	}
}

func TestGroupLogsDistinguishJudgeAndIncludeAllMembers(t *testing.T) {
	writeLog := func(name, content string) string {
		t.Helper()
		path := t.TempDir() + "/" + name
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	sessions := []*session.Session{
		{CLI: "codex", Model: config.GPT56SolModel, Mode: "megareview", Status: "completed", LogFile: writeLog("review.log", "review body\n")},
		{CLI: "codex", Model: config.GPT56SolModel, Mode: "consilium", Status: "completed", LogFile: writeLog("judge.log", "judge body\n")},
	}
	path, err := createPublicGroupLogView(sessions)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(path) }()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{"SOL REVIEW", "review body", "SOL JUDGE", "judge body"} {
		if !strings.Contains(got, want) {
			t.Fatalf("combined group log missing %q:\n%s", want, got)
		}
	}
}

func TestDetailViewHandlesTinyTerminal(t *testing.T) {
	m := New()
	defer m.cancel()
	m.width = 80
	m.height = 1
	m.viewMode = viewDetail
	m.items = []displayItem{{Sessions: []*session.Session{
		{ID: "tiny", CLI: "fable", Model: config.FableModel, Mode: "plan", Status: "running"},
	}}}
	m.allItems = m.items

	_ = m.View()
}
