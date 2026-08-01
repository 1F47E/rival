package cmd

import (
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/parser"
)

func TestAntislopMode(t *testing.T) {
	tests := []struct {
		name      string
		scope     string
		autoScope bool
		escaped   bool
		planMode  bool
		planPath  string
		wantErr   bool
	}{
		{"auto scope is code mode", "the entire project", true, false, false, "", false},
		{"explicit scope is code mode", "src/api/", false, false, false, "", false},
		{"plan token starts plan mode", "plan plans/x/plan.md", false, false, true, "plans/x/plan.md", false},
		{"path with spaces preserved", "plan plans/my feature/plan v1.md", false, false, true, "plans/my feature/plan v1.md", false},
		{"bare plan is a usage error", "plan", false, false, false, "", true},
		{"dot-slash plan is code mode", "./plan", false, false, false, "", false},
		{"plan prefix in a longer word is code mode", "planner/", false, false, false, "", false},
		{"escaped plan scope is code mode", "plan handling in the parser", false, true, false, "", false},
		{"escaped bare plan is code mode, not an error", "plan", false, true, false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			planMode, planPath, err := antislopMode(tt.scope, tt.autoScope, tt.escaped)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tt.wantErr)
			}
			if planMode != tt.planMode || planPath != tt.planPath {
				t.Fatalf("got (%v, %q), want (%v, %q)", planMode, planPath, tt.planMode, tt.planPath)
			}
		})
	}
}

// The skill-facing grammar reaches antislop through parser.ParseReviewArgs;
// assert the combinations the skills document actually produce.
func TestAntislopStdinGrammar(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		planMode bool
		planPath string
		effort   string
		models   []string
	}{
		{"empty input is auto code mode", "", false, "", "", nil},
		{"plan with options", "-re high -m fable plan plans/x/p.md", true, "plans/x/p.md", "high", []string{"fable"}},
		{"scope with model list", "-m sol,fable src/", false, "", "", []string{"sol", "fable"}},
		{"escaped dash scope", "-- -weird/dir", false, "", "", nil},
		{"escaped plan-word scope stays code mode", "-- plan handling in the parser", false, "", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parser.ParseReviewArgs(tt.raw)
			if err != nil {
				t.Fatalf("ParseReviewArgs: %v", err)
			}
			planMode, planPath, err := antislopMode(parsed.ReviewScope, parsed.AutoScope, parsed.Escaped)
			if err != nil {
				t.Fatalf("antislopMode: %v", err)
			}
			if planMode != tt.planMode || planPath != tt.planPath {
				t.Fatalf("mode: got (%v, %q), want (%v, %q)", planMode, planPath, tt.planMode, tt.planPath)
			}
			if parsed.Effort != tt.effort {
				t.Fatalf("effort: got %q, want %q", parsed.Effort, tt.effort)
			}
			if len(parsed.Models) != len(tt.models) {
				t.Fatalf("models: got %v, want %v", parsed.Models, tt.models)
			}
		})
	}
}

func TestAntislopDefaultModelIsSolOnly(t *testing.T) {
	clis, err := parsePlanModels(defaultAntislopModels)
	if err != nil {
		t.Fatalf("parsePlanModels: %v", err)
	}
	if len(clis) != 1 || clis[0] != "codex" {
		t.Fatalf("got %v, want sol only (codex adapter)", clis)
	}
}

func TestBuildAntislopCodePromptExplicitScope(t *testing.T) {
	prompt, target, display := buildAntislopCodePrompt("src/api/", false, t.TempDir())
	if !strings.Contains(prompt, "Review scope: src/api/") {
		t.Fatalf("scope not substituted:\n%s", prompt[:200])
	}
	if strings.Contains(prompt, "{SCOPE}") {
		t.Fatal("placeholder left in prompt")
	}
	if target != "src/api/" || display != "src/api/" {
		t.Fatalf("got target=%q display=%q", target, display)
	}
}

func TestBuildAntislopCodePromptNoChangesFallsBackToProject(t *testing.T) {
	// A fresh temp dir is not a git repo, so auto-detect finds nothing.
	prompt, target, display := buildAntislopCodePrompt("the entire project", true, t.TempDir())
	if !strings.Contains(prompt, "Review scope: the entire project") {
		t.Fatalf("full-project fallback missing:\n%s", prompt[:200])
	}
	if target != "the entire project" || display != "the entire project" {
		t.Fatalf("got target=%q display=%q", target, display)
	}
	if !strings.Contains(prompt, config.AntislopCodePrompt[strings.Index(config.AntislopCodePrompt, "Rules:"):][:20]) {
		t.Fatal("prompt does not end with the antislop template")
	}
}
