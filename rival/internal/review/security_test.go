package review

import (
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
)

// ParseReviewerOutput accepts any JSON carrying the right keys, so a payload
// can parse and still be unusable. A security gate that formats these as a
// clean review reports "no vulnerabilities" when nothing was reviewed.
func TestValidateSecurityOutputRejectsUnusablePayloads(t *testing.T) {
	tests := []struct {
		name string
		out  *ReviewerOutput
	}{
		{"nil", nil},
		{"empty summary", &ReviewerOutput{Summary: "  "}},
		{"finding with no file", &ReviewerOutput{
			Summary:  "found things",
			Findings: []ReviewerFinding{{Title: "sqli", Severity: "high"}},
		}},
		{"finding with no title", &ReviewerOutput{
			Summary:  "found things",
			Findings: []ReviewerFinding{{File: "a.go", Severity: "high"}},
		}},
		{"unknown severity", &ReviewerOutput{
			Summary:  "found things",
			Findings: []ReviewerFinding{{File: "a.go", Title: "sqli", Severity: "spicy"}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSecurityOutput(tt.out); err == nil {
				t.Error("expected this payload to be rejected")
			}
		})
	}
}

// A clean review is valid: no findings is a real answer, unlike no summary.
func TestValidateSecurityOutputAcceptsACleanReview(t *testing.T) {
	out := &ReviewerOutput{Summary: "No vulnerabilities found."}
	if err := ValidateSecurityOutput(out); err != nil {
		t.Errorf("a clean review was rejected: %v", err)
	}
}

func TestFormatSecurityResultFallsBackOnUnusableOutput(t *testing.T) {
	got := FormatSecurityResult(nil, "provider exploded", "opencode", config.KimiModel, "src/")
	if !strings.Contains(got, "UNUSABLE OUTPUT") {
		t.Errorf("fallback header missing:\n%s", got)
	}
	if !strings.Contains(got, "provider exploded") {
		t.Errorf("raw output was discarded:\n%s", got)
	}
	if strings.Contains(got, "No vulnerabilities found") {
		t.Error("unusable output was reported as a clean review")
	}
}

func TestFormatSecurityConsoleOrdersBySeverity(t *testing.T) {
	out := &ReviewerOutput{
		Summary: "two issues",
		Findings: []ReviewerFinding{
			{File: "b.go", Line: 2, Severity: "medium", Title: "open redirect", Confidence: 7},
			{File: "a.go", Line: 1, Severity: "critical", Title: "sql injection", Confidence: 9},
		},
	}
	got := FormatSecurityConsole(out, config.K3Label, "src/")
	crit := strings.Index(got, "sql injection")
	med := strings.Index(got, "open redirect")
	if crit == -1 || med == -1 {
		t.Fatalf("findings missing:\n%s", got)
	}
	if crit > med {
		t.Errorf("critical finding rendered after the medium one:\n%s", got)
	}
	if !strings.Contains(got, "1 crit, 0 high, 1 med, 0 low") {
		t.Errorf("severity tally wrong:\n%s", got)
	}
}

func TestFormatSecurityConsoleCleanReview(t *testing.T) {
	got := FormatSecurityConsole(&ReviewerOutput{Summary: "nothing found"}, config.K3Label, "src/")
	if !strings.Contains(got, "No vulnerabilities found.") {
		t.Errorf("clean review should say so explicitly:\n%s", got)
	}
}
