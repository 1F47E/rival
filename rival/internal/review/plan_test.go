package review

import (
	"strings"
	"testing"
)

// codexStylePlanLog mimics a Codex plan-review log: the CLI echoes the prompt
// (with the schema example, including its "rating" and placeholder finding) and
// then streams the real answer last.
const codexStylePlanLog = `OpenAI Codex
user
Plan document to review: /tmp/plan.md

Output schema:
` + "```json" + `
{
  "summary": "1-3 sentence overall assessment of the plan",
  "rating": 7,
  "findings": [
    {
      "file": "section or heading the issue is in (or the filename)",
      "line": 0,
      "severity": "critical|high|medium|low",
      "category": "bug|gap|ambiguity|scope|verification",
      "title": "one-line description of the issue",
      "confidence": 8
    }
  ]
}
` + "```" + `

codex
{"summary":"Solid plan with two real gaps.","rating":6,"findings":[{"file":"Migration","line":0,"severity":"high","category":"gap","title":"no rollback step","body":"plan adds a column but never describes how to revert","suggestion":"add a down migration","confidence":9}]}
tokens used 4321
`

func TestParsePlanOutput_IgnoresEchoedSchemaExample(t *testing.T) {
	out, err := ParsePlanOutput(codexStylePlanLog)
	if err != nil {
		t.Fatalf("ParsePlanOutput: %v", err)
	}
	if out.Summary != "Solid plan with two real gaps." {
		t.Fatalf("summary = %q, want the real answer (not the schema example)", out.Summary)
	}
	if out.Rating != 6 {
		t.Fatalf("rating = %d, want 6 (the real answer)", out.Rating)
	}
	if len(out.Findings) != 1 || out.Findings[0].Title != "no rollback step" {
		t.Fatalf("findings = %+v, want the real finding", out.Findings)
	}
}

func TestParsePlanOutput_RejectsOnlySchemaExample(t *testing.T) {
	schemaOnly := "```json\n" + `{"summary":"1-3 sentence overall assessment of the plan","rating":7,"findings":[{"file":"section or heading the issue is in (or the filename)","line":0,"severity":"critical|high|medium|low","category":"bug|gap|ambiguity|scope|verification","title":"one-line description of the issue","confidence":8}]}` + "\n```"
	if _, err := ParsePlanOutput(schemaOnly); err == nil {
		t.Fatal("expected error: the schema example must not be accepted as a real answer")
	}
}

func TestParsePlanOutput_RejectsUnrelatedJSON(t *testing.T) {
	// Has neither findings nor rating.
	if _, err := ParsePlanOutput(`{"event":"done","ok":true}`); err == nil {
		t.Fatal("expected error: unrelated JSON must be rejected")
	}
	// Has findings but no rating — a reviewer payload, not a plan payload.
	if _, err := ParsePlanOutput(`{"summary":"x","findings":[]}`); err == nil {
		t.Fatal("expected error: a payload without a rating key is not a plan payload")
	}
}

func TestParsePlanOutput_AcceptsCleanPlan(t *testing.T) {
	out, err := ParsePlanOutput(`prose {"summary":"Airtight.","rating":9,"findings":[]} prose`)
	if err != nil {
		t.Fatalf("ParsePlanOutput: %v", err)
	}
	if out.Rating != 9 || out.Summary != "Airtight." || len(out.Findings) != 0 {
		t.Fatalf("got %+v, want a clean plan review", out)
	}
}

func TestParsePlanOutput_DropsOnlyPlaceholderFindings(t *testing.T) {
	raw := `{"summary":"real","rating":5,"findings":[` +
		`{"file":"path/to/file","line":0,"severity":"critical|high|medium|low","category":"bug|gap|ambiguity|scope|verification","title":"x","confidence":8},` +
		`{"file":"Section 2","line":0,"severity":"medium","category":"gap","title":"real gap","body":"b","confidence":7}]}`
	out, err := ParsePlanOutput(raw)
	if err != nil {
		t.Fatalf("ParsePlanOutput: %v", err)
	}
	if len(out.Findings) != 1 || out.Findings[0].Title != "real gap" {
		t.Fatalf("got findings=%+v, want only the real one kept", out.Findings)
	}
}

func TestDisplaySeverity(t *testing.T) {
	cases := map[string]string{
		"critical": "crit",
		"CRITICAL": "crit",
		"high":     "high",
		"medium":   "med",
		"low":      "low",
		"weird":    "weird",
	}
	for in, want := range cases {
		if got := displaySeverity(in); got != want {
			t.Errorf("displaySeverity(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatPlanConsole_NumbersAndOrdersBySeverity(t *testing.T) {
	out := &PlanOutput{
		Summary: "two issues",
		Rating:  4,
		Findings: []ReviewerFinding{
			{File: "B", Severity: "low", Title: "minor", Confidence: 5},
			{File: "A", Line: 12, Severity: "critical", Title: "broken", Body: "bad", Suggestion: "fix it", Category: "bug", Confidence: 9},
			{File: "C", Severity: "medium", Title: "vague", Confidence: 6},
		},
	}
	s := FormatPlanConsole(out, "/tmp/plan.md")

	// Rating present.
	if !strings.Contains(s, "Rating: 4/10") {
		t.Errorf("missing rating line:\n%s", s)
	}
	// crit first, numbered 1, with short label and location.
	crit := strings.Index(s, "1. [crit] broken — A:12")
	med := strings.Index(s, "2. [med] vague")
	low := strings.Index(s, "3. [low] minor")
	if crit < 0 || med < 0 || low < 0 {
		t.Fatalf("expected numbered crit→med→low ordering, got:\n%s", s)
	}
	if crit >= med || med >= low {
		t.Errorf("findings not ordered crit→med→low:\n%s", s)
	}
	// Fix line and tally.
	if !strings.Contains(s, "Fix: fix it") {
		t.Errorf("missing suggestion line:\n%s", s)
	}
	if !strings.Contains(s, "Findings: 3 total — 1 crit, 0 high, 1 med, 1 low") {
		t.Errorf("missing/incorrect tally:\n%s", s)
	}
}

func TestFormatPlanConsole_CleanPlan(t *testing.T) {
	out := &PlanOutput{Summary: "Airtight.", Rating: 10}
	s := FormatPlanConsole(out, "/tmp/plan.md")
	if !strings.Contains(s, "Rating: 10/10") || !strings.Contains(s, "No bugs or gaps found.") {
		t.Errorf("clean plan render wrong:\n%s", s)
	}
}
