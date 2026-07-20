package review

import (
	"context"
	"os"
	"path/filepath"
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
		{CLI: "codex", Model: config.GPT56SolModel, Raw: realPlanJSON, ExitCode: 0},
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
		{CLI: "codex", Model: config.GPT56SolModel, Raw: realPlanJSON, ExitCode: 0},
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
		{CLI: "codex", Model: config.GPT56SolModel, Raw: "error: insufficient_quota", ExitCode: 0},
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
		{CLI: "codex", Model: config.GPT56SolModel, Raw: "just some prose, no json", ExitCode: 0},
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

func TestAssemblePlanResults_EmptyOutputSkips(t *testing.T) {
	// An exit-0 run that wrote nothing must be skipped, not treated as a
	// successful (but empty) plan review.
	batch := []planCLIRun{
		{CLI: "fable", Model: config.FableModel, Raw: "   \n  ", ExitCode: 0},
		{CLI: "codex", Model: config.GPT56SolModel, Raw: realPlanJSON, ExitCode: 0},
	}
	res, err := assemblePlanResults(batch, nil)
	if err != nil {
		t.Fatalf("assemblePlanResults: %v", err)
	}
	if len(res.Results) != 1 || res.Results[0].CLI != "codex" {
		t.Fatalf("want only codex kept (fable empty), got %+v", res.Results)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].CLI != "fable" {
		t.Fatalf("want fable skipped for empty output, got %+v", res.Skipped)
	}
}

func TestPlanEngineLabel(t *testing.T) {
	if got := planEngineLabel("codex", config.GPT56SolModel); got != config.SolLabel {
		t.Errorf("sol label = %q, want %q", got, config.SolLabel)
	}
	if got := planEngineLabel("fable", config.FableModel); got != config.FableLabel {
		t.Errorf("fable label = %q, want %q", got, config.FableLabel)
	}
}

func TestPlanFailureReasonUsesModelName(t *testing.T) {
	got := planFailureReason("codex", "Codex CLI not installed; run codex login")
	if !strings.Contains(strings.ToLower(got), config.SolLabel) {
		t.Fatalf("failure reason missing model name: %q", got)
	}
	if strings.Contains(strings.ToLower(got), "codex") {
		t.Fatalf("failure reason leaked adapter name: %q", got)
	}
	fable := planFailureReason("fable", config.FableModel+" failed in Claude CLI")
	lowerFable := strings.ToLower(fable)
	if strings.Count(lowerFable, config.FableLabel) != 2 || strings.Contains(lowerFable, "claude") {
		t.Fatalf("model-facing normalization did not use public fable name: %q", fable)
	}
}

func TestFormatPlanResult_SingleParsed(t *testing.T) {
	res := &PlanRunResult{Results: []PlanCLIResult{
		{CLI: "codex", Model: config.GPT56SolModel, Parsed: &PlanOutput{Summary: "s", Rating: 8}},
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
		{CLI: "codex", Model: config.GPT56SolModel, Parsed: nil, Raw: "Codex raw output"},
	}}
	out := FormatPlanResult(res, "/tmp/plan.md")
	if out != "Sol runtime raw output" {
		t.Errorf("parse-fail single result must preserve raw content with a model-facing label, got:\n%s", out)
	}
}

func TestFormatPlanResult_MultiBlocksAndSkipped(t *testing.T) {
	res := &PlanRunResult{
		Results: []PlanCLIResult{
			{CLI: "codex", Model: config.GPT56SolModel, Parsed: &PlanOutput{Summary: "cx", Rating: 6}},
			{CLI: "fable", Model: config.FableModel, Parsed: nil, Raw: "Claude raw dump"},
		},
		Skipped: []SkippedCLI{{CLI: "opencode", Model: config.OpencodeDeepSeekPro, Reason: "n/a"}},
	}
	out := FormatPlanResult(res, "/tmp/plan.md")
	if !strings.Contains(out, "RIVAL PLAN REVIEW ("+config.SolLabel+" + "+config.FableLabel+")") {
		t.Errorf("multi header missing engines:\n%s", out)
	}
	if !strings.Contains(out, "── "+config.SolLabel+" ──") {
		t.Errorf("sol block header missing:\n%s", out)
	}
	if !strings.Contains(out, "── "+config.FableLabel+" ──") {
		t.Errorf("fable block header missing:\n%s", out)
	}
	// Fable block had no parsed output → raw fallback shown.
	if !strings.Contains(out, "Fable runtime raw dump") {
		t.Errorf("fable raw fallback missing:\n%s", out)
	}
	if !strings.Contains(out, "Skipped: deepseek-v4-pro — n/a") {
		t.Errorf("skipped line missing:\n%s", out)
	}
	if strings.Contains(strings.ToLower(out), "codex") {
		t.Errorf("plan output must use model names, not adapter names:\n%s", out)
	}
}

func TestAssemblePlanResults_ErrUsesReason(t *testing.T) {
	// A timeout-style failure carries a Reason that must surface in Skipped,
	// instead of the bare error text.
	batch := []planCLIRun{
		{CLI: "fable", Err: errString("context deadline exceeded"), Reason: config.FableModel + " run timeout after 30m (RIVAL_RUN_TIMEOUT) — model did not finish", ExitCode: -1},
		{CLI: "codex", Model: config.GPT56SolModel, Raw: realPlanJSON, ExitCode: 0},
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
	sess, err := session.NewQueued("fable", "plan", config.FableModel, "high", t.TempDir(), "p", "/tmp/plan.md", "g")
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
	out := runPlanCLI(context.Background(), ex, sess, "fable", "p", t.TempDir())
	if out.ExitCode != 0 {
		t.Fatalf("run failed: %+v", out)
	}
	if sess.Mode != "plan" {
		t.Fatalf("sess.Mode = %q, want plan (restored after fable transport overwrote it)", sess.Mode)
	}
}

func TestRunPlanReviewResolvesPerModelEfforts(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		override   string
		clis       []string
		want       map[string]string
	}{
		{
			name: "sol uses native fallback",
			clis: []string{"codex"},
			want: map[string]string{"codex": "high"},
		},
		{
			name: "fable alone uses low native fallback",
			clis: []string{"fable"},
			want: map[string]string{"fable": "low"},
		},
		{
			name: "paired native plan uses high for both",
			clis: []string{"codex", "fable"},
			want: map[string]string{"codex": "high", "fable": "high"},
		},
		{
			name:       "configured defaults resolve independently",
			configYAML: "efforts:\n  sol: low\n  fable: ultra\n",
			clis:       []string{"codex", "fable"},
			want:       map[string]string{"codex": "low", "fable": "ultra"},
		},
		{
			name:       "explicit override wins for every model",
			configYAML: "efforts:\n  sol: low\n  fable: medium\n",
			override:   "ultra",
			clis:       []string{"codex", "fable"},
			want:       map[string]string{"codex": "ultra", "fable": "ultra"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loadPlanTestConfig(t, tc.configYAML)

			type observation struct {
				cli           string
				sessionEffort string
				runEffort     string
			}
			observed := make(chan observation, len(tc.clis))
			ex := planExecutor{
				preflight: func(string) error { return nil },
				run: func(_ context.Context, sess *session.Session, cli, _, effort, _ string) (string, int, error) {
					observed <- observation{cli: cli, sessionEffort: sess.Effort, runEffort: effort}
					return realPlanJSON, 0, nil
				},
			}

			_, err := runPlanReview(
				context.Background(),
				ex,
				"/tmp/plan.md",
				tc.override,
				t.TempDir(),
				"efforts",
				true,
				tc.clis,
			)
			if err != nil {
				t.Fatalf("runPlanReview: %v", err)
			}

			for range tc.clis {
				got := <-observed
				want := tc.want[got.cli]
				if got.sessionEffort != want {
					t.Errorf("%s session effort = %q, want %q", got.cli, got.sessionEffort, want)
				}
				if got.runEffort != got.sessionEffort {
					t.Errorf("%s executor effort = %q, want session effort %q", got.cli, got.runEffort, got.sessionEffort)
				}
			}
		})
	}
}

func TestRunPlanReviewPreservesRequestedOrderWhenFableFinishesFirst(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fableDone := make(chan struct{})
	ex := planExecutor{
		preflight: func(string) error { return nil },
		run: func(_ context.Context, _ *session.Session, cli, _, _, _ string) (string, int, error) {
			if cli == "fable" {
				close(fableDone)
			} else {
				<-fableDone
			}
			return realPlanJSON, 0, nil
		},
	}
	result, err := runPlanReview(context.Background(), ex, "/tmp/plan.md", "ultra", t.TempDir(), "ordered", true, []string{"codex", "fable"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 2 || result.Results[0].CLI != "codex" || result.Results[1].CLI != "fable" {
		t.Fatalf("plan results lost requested order: %+v", result.Results)
	}
}

func loadPlanTestConfig(t *testing.T, contents string) {
	t.Helper()

	home := t.TempDir()
	// Restore the process-global config only after t.Setenv has restored HOME.
	t.Cleanup(config.LoadUserConfig)
	t.Setenv("HOME", home)

	if contents != "" {
		dir := filepath.Join(home, ".rival")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create config dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(contents), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}

	config.LoadUserConfig()
	if err := config.UserConfigError(); err != nil {
		t.Fatalf("load config: %v", err)
	}
}

// errString is a tiny error type so tests can build a planCLIRun.Err without fmt.
type errString string

func (e errString) Error() string { return string(e) }
