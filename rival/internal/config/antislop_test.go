package config

import (
	"strings"
	"testing"
)

// The antislop prompts must carry their runtime placeholder, the JSON contract
// keys ParsePlanOutput requires, the antislop category enum, and the exact
// example-summary string the parser uses to skip prompt echoes.
func TestAntislopPromptTemplates(t *testing.T) {
	const (
		exampleSummary = `"summary": "1-3 sentence overall assessment of the plan"`
		categoryEnum   = "reuse|simplify|efficiency|altitude|compat|reinvention|slop|yagni"
	)

	tests := []struct {
		name        string
		prompt      string
		placeholder string
		other       string
	}{
		{"code", AntislopCodePrompt, "{SCOPE}", "{FILE}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.prompt, tt.placeholder) {
				t.Errorf("missing placeholder %s", tt.placeholder)
			}
			if strings.Contains(tt.prompt, tt.other) {
				t.Errorf("contains foreign placeholder %s", tt.other)
			}
			for _, key := range []string{`"summary"`, `"rating"`, `"findings"`, exampleSummary, categoryEnum} {
				if !strings.Contains(tt.prompt, key) {
					t.Errorf("missing %q", key)
				}
			}
		})
	}

	// The echo-skip only works if the example summary matches PlanReviewPrompt's.
	if !strings.Contains(PlanReviewPrompt, exampleSummary) {
		t.Errorf("PlanReviewPrompt example summary diverged from the antislop contract")
	}
}
