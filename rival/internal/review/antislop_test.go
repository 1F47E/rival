package review

import (
	"strings"
	"testing"
)

func TestFormatAntislopResultSingle(t *testing.T) {
	result := &PlanRunResult{Results: []PlanCLIResult{{
		CLI:   "codex",
		Model: "gpt-5.6-sol",
		Parsed: &PlanOutput{
			Summary: "One wrapper to cut.",
			Rating:  8,
			Findings: []ReviewerFinding{{
				File: "cmd/x.go", Line: 12, Severity: "medium", Category: "slop",
				Title: "pass-through wrapper", Body: "body", Suggestion: "inline it", Confidence: 8,
			}},
		},
	}}}

	out := FormatAntislopResult(result, "src/api/", false)
	for _, want := range []string{
		"═══ RIVAL ANTISLOP REVIEW ═══",
		"Scope: src/api/",
		"Leanness: 8/10",
		"pass-through wrapper",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Rating:") || strings.Contains(out, "File:") {
		t.Errorf("plan-flavor labels leaked into code-mode antislop output:\n%s", out)
	}
}

func TestFormatAntislopResultPlanModeEmpty(t *testing.T) {
	result := &PlanRunResult{Results: []PlanCLIResult{{
		CLI:    "fable",
		Model:  "fable",
		Parsed: &PlanOutput{Summary: "Lean.", Rating: 10},
	}}}

	out := FormatAntislopResult(result, "/tmp/plan.md", true)
	for _, want := range []string{"File: /tmp/plan.md", "Leanness: 10/10", "No slop found."} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "No bugs or gaps found.") {
		t.Errorf("plan-flavor empty line leaked:\n%s", out)
	}
}

func TestFormatAntislopResultMulti(t *testing.T) {
	result := &PlanRunResult{
		Results: []PlanCLIResult{
			{CLI: "codex", Model: "gpt-5.6-sol", Parsed: &PlanOutput{Summary: "s", Rating: 7}},
			{CLI: "fable", Model: "fable", Parsed: &PlanOutput{Summary: "f", Rating: 9}},
		},
		Skipped: []SkippedCLI{{CLI: "grok", Model: "grok", Reason: "unavailable"}},
	}

	out := FormatAntislopResult(result, "changed files (git auto-detect)", false)
	for _, want := range []string{
		"═══ RIVAL ANTISLOP REVIEW (",
		"Scope: changed files (git auto-detect)",
		"Leanness: 7/10",
		"Leanness: 9/10",
		"Skipped:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
