package review

import (
	"context"
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
)

// realPlanJSON is a minimal valid plan payload ParsePlanOutput accepts.
const realPlanJSON = `{"summary":"ok plan","rating":7,"findings":[]}`

func TestAssemblePlanResults_AllFailed(t *testing.T) {
	batch := []planCLIRun{
		{CLI: "codex", ExitCode: 1},
		{CLI: "fable", Err: errString("boom")},
	}
	if _, err := assemblePlanResults(batch, nil); err == nil {
		t.Fatal("expected error when every CLI fails")
	}
}

func TestAssemblePlanResults_OneSkippedOneOK(t *testing.T) {
	batch := []planCLIRun{
		{CLI: "codex", Model: config.CodexModel, Raw: realPlanJSON, ExitCode: 0},
	}
	// fable was unavailable at preflight → pre-run skipped list.
	pre := []SkippedCLI{{CLI: "fable", Reason: "claude not found"}}

	res, err := assemblePlanResults(batch, pre)
	if err != nil {
		t.Fatalf("assemblePlanResults: %v", err)
	}
	if len(res.Results) != 1 || res.Results[0].CLI != "codex" {
		t.Fatalf("want 1 codex result, got %+v", res.Results)
	}
	if res.Results[0].Parsed == nil || res.Results[0].Parsed.Rating != 7 {
		t.Fatalf("codex result not parsed: %+v", res.Results[0])
	}
	if len(res.Skipped) != 1 || res.Skipped[0].CLI != "fable" {
		t.Fatalf("want fable skipped preserved, got %+v", res.Skipped)
	}
}

func TestAssemblePlanResults_NonzeroExitSkips(t *testing.T) {
	batch := []planCLIRun{
		{CLI: "codex", Model: config.CodexModel, Raw: realPlanJSON, ExitCode: 0},
		{CLI: "fable", Model: config.FableModel, Raw: "partial", ExitCode: 2},
	}
	res, err := assemblePlanResults(batch, nil)
	if err != nil {
		t.Fatalf("assemblePlanResults: %v", err)
	}
	if len(res.Results) != 1 || res.Results[0].CLI != "codex" {
		t.Fatalf("want only codex kept, got %+v", res.Results)
	}
	if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Reason, "exited with code 2") {
		t.Fatalf("want fable skipped with exit reason, got %+v", res.Skipped)
	}
}

func TestAssemblePlanResults_QuotaSkips(t *testing.T) {
	batch := []planCLIRun{
		{CLI: "codex", Model: config.CodexModel, Raw: "error: insufficient_quota", ExitCode: 0},
		{CLI: "fable", Model: config.FableModel, Raw: realPlanJSON, ExitCode: 0},
	}
	res, err := assemblePlanResults(batch, nil)
	if err != nil {
		t.Fatalf("assemblePlanResults: %v", err)
	}
	if len(res.Results) != 1 || res.Results[0].CLI != "fable" {
		t.Fatalf("want only fable kept (codex quota'd), got %+v", res.Results)
	}
	if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Reason, "429") {
		t.Fatalf("want codex quota-skipped, got %+v", res.Skipped)
	}
}

func TestAssemblePlanResults_ParseFailKeepsRaw(t *testing.T) {
	// Exit 0, no quota, but output has no parseable plan payload → keep Raw, nil Parsed.
	batch := []planCLIRun{
		{CLI: "codex", Model: config.CodexModel, Raw: "just some prose, no json", ExitCode: 0},
	}
	res, err := assemblePlanResults(batch, nil)
	if err != nil {
		t.Fatalf("assemblePlanResults: %v", err)
	}
	if len(res.Results) != 1 {
		t.Fatalf("want 1 result kept on parse failure, got %+v", res.Results)
	}
	if res.Results[0].Parsed != nil {
		t.Fatalf("want nil Parsed on parse failure, got %+v", res.Results[0].Parsed)
	}
	if res.Results[0].Raw != "just some prose, no json" {
		t.Fatalf("raw not preserved: %q", res.Results[0].Raw)
	}
}

func TestPlanEngineLabel(t *testing.T) {
	if got := planEngineLabel("codex", config.CodexModel); got != "codex" {
		t.Errorf("codex label = %q, want codex", got)
	}
	// Fable's session CLI is "fable" here, but the model id is what tags it.
	if got := planEngineLabel("fable", config.FableModel); got != "claude-fable" {
		t.Errorf("fable label = %q, want claude-fable", got)
	}
}

func TestFormatPlanResult_SingleParsed(t *testing.T) {
	res := &PlanRunResult{Results: []PlanCLIResult{
		{CLI: "codex", Model: config.CodexModel, Parsed: &PlanOutput{Summary: "s", Rating: 8}},
	}}
	out := FormatPlanResult(res, "/tmp/plan.md")
	if !strings.Contains(out, "═══ RIVAL PLAN REVIEW ═══") || !strings.Contains(out, "Rating: 8/10") {
		t.Errorf("single-parsed render wrong:\n%s", out)
	}
	// Single-CLI must NOT use the multi header.
	if strings.Contains(out, "RIVAL PLAN REVIEW (") {
		t.Errorf("single result should use the single-CLI header:\n%s", out)
	}
}

func TestFormatPlanResult_SingleParseFailReturnsRaw(t *testing.T) {
	res := &PlanRunResult{Results: []PlanCLIResult{
		{CLI: "codex", Model: config.CodexModel, Parsed: nil, Raw: "RAW CODEX OUTPUT"},
	}}
	out := FormatPlanResult(res, "/tmp/plan.md")
	if out != "RAW CODEX OUTPUT" {
		t.Errorf("parse-fail single result must return raw verbatim, got:\n%s", out)
	}
}

func TestFormatPlanResult_MultiBlocksAndSkipped(t *testing.T) {
	res := &PlanRunResult{
		Results: []PlanCLIResult{
			{CLI: "codex", Model: config.CodexModel, Parsed: &PlanOutput{Summary: "cx", Rating: 6}},
			{CLI: "fable", Model: config.FableModel, Parsed: nil, Raw: "fable raw dump"},
		},
		Skipped: []SkippedCLI{{CLI: "antigravity", Reason: "n/a"}},
	}
	out := FormatPlanResult(res, "/tmp/plan.md")
	if !strings.Contains(out, "RIVAL PLAN REVIEW (codex + claude-fable)") {
		t.Errorf("multi header missing engines:\n%s", out)
	}
	if !strings.Contains(out, "── codex ("+config.CodexModel+") ──") {
		t.Errorf("codex block header missing:\n%s", out)
	}
	if !strings.Contains(out, "── claude-fable ("+config.FableModel+") ──") {
		t.Errorf("fable block header missing (should be claude-fable):\n%s", out)
	}
	// Fable block had no parsed output → raw fallback shown.
	if !strings.Contains(out, "fable raw dump") {
		t.Errorf("fable raw fallback missing:\n%s", out)
	}
	if !strings.Contains(out, "Skipped: antigravity — n/a") {
		t.Errorf("skipped line missing:\n%s", out)
	}
}

func TestAssemblePlanResults_ErrUsesReason(t *testing.T) {
	// A timeout-style failure carries a Reason that must surface in Skipped,
	// instead of the bare error text.
	batch := []planCLIRun{
		{CLI: "fable", Err: errString("context deadline exceeded"), Reason: "fable run timeout after 30m (RIVAL_RUN_TIMEOUT) — provider CLI did not finish", ExitCode: -1},
		{CLI: "codex", Model: config.CodexModel, Raw: realPlanJSON, ExitCode: 0},
	}
	res, err := assemblePlanResults(batch, nil)
	if err != nil {
		t.Fatalf("assemblePlanResults: %v", err)
	}
	if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Reason, "RIVAL_RUN_TIMEOUT") {
		t.Fatalf("want fable skipped with RIVAL_RUN_TIMEOUT reason, got %+v", res.Skipped)
	}
}

func TestRunPlanCLI_RestoresPlanMode(t *testing.T) {
	// Isolate the sessions dir (SessionDirPath uses $HOME) so this test never
	// writes into the user's real ~/.rival/sessions.
	t.Setenv("HOME", t.TempDir())

	// The fable executor overwrites sess.Mode to the transport ("native"); the
	// terminal session must be recorded as a plan session regardless.
	sess, err := session.NewQueued("fable", "plan", config.FableModel, "xhigh", t.TempDir(), "p", "/tmp/plan.md", "g")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.MarkRunning(); err != nil {
		t.Fatal(err)
	}
	ex := planExecutor{
		preflight: func(string) error { return nil },
		run: func(_ context.Context, s *session.Session, _, _, _, _ string) (string, int, error) {
			s.Mode = "native" // simulate the Claude executor clobbering mode
			return realPlanJSON, 0, nil
		},
	}
	out := runPlanCLI(context.Background(), ex, sess, "fable", "p", "xhigh", t.TempDir())
	if out.ExitCode != 0 {
		t.Fatalf("run failed: %+v", out)
	}
	if sess.Mode != "plan" {
		t.Fatalf("sess.Mode = %q, want plan (restored after fable transport overwrote it)", sess.Mode)
	}
}

// errString is a tiny error type so tests can build a planCLIRun.Err without fmt.
type errString string

func (e errString) Error() string { return string(e) }
