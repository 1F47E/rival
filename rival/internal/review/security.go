package review

import (
	"fmt"
	"sort"
	"strings"

	"github.com/1F47E/rival/internal/config"
)

// ValidateSecurityOutput reports whether a parsed payload is a usable security
// review.
//
// ParseReviewerOutput only checks that the JSON decodes and carries the
// expected keys, so `{"summary":"","findings":null}` parses cleanly and would
// otherwise format as a clean review. For a security gate that is the worst
// possible failure: it reports "no vulnerabilities" when nothing was
// reviewed. A caller must treat a validation failure exactly like a parse
// failure.
func ValidateSecurityOutput(out *ReviewerOutput) error {
	return ValidateSecurityResult(out, "")
}

// ValidateSecurityResult validates a payload against the raw output it came
// from. The raw text matters: the reviewer prompt contains a parseable clean
// example, `{"summary": "No issues found.", "findings": []}`, so a run that
// merely echoes the prompt without reviewing anything parses successfully and
// looks like a clean review. For a security gate that is the worst failure
// available, so an echoed prompt is rejected rather than trusted.
func ValidateSecurityResult(out *ReviewerOutput, raw string) error {
	if out == nil {
		return fmt.Errorf("no structured output")
	}
	if raw != "" && looksLikeEchoedPrompt(out, raw) {
		return fmt.Errorf("output repeats the prompt's own example rather than a review")
	}
	if strings.TrimSpace(out.Summary) == "" {
		return fmt.Errorf("summary is empty")
	}
	for i, f := range out.Findings {
		if strings.TrimSpace(f.File) == "" {
			return fmt.Errorf("finding %d has no file", i+1)
		}
		if strings.TrimSpace(f.Title) == "" {
			return fmt.Errorf("finding %d has no title", i+1)
		}
		if severityRank(f.Severity) > 3 {
			return fmt.Errorf("finding %d has an unknown severity %q", i+1, f.Severity)
		}
	}
	return nil
}

// promptEchoMarkers are phrases that appear in the security prompt itself. A
// review quoting one is possible, so they are only evidence alongside the
// example summary.
var promptEchoMarkers = []string{
	"## Role: Security Reviewer",
	"Work through every class below",
}

// looksLikeEchoedPrompt reports whether the output is the prompt reflected
// back. A genuine clean review carries the same summary text, so the summary
// alone is not enough: the raw output must also contain the prompt's own
// instructions, which a reviewer has no reason to reproduce.
func looksLikeEchoedPrompt(out *ReviewerOutput, raw string) bool {
	if strings.TrimSpace(out.Summary) != "No issues found." || len(out.Findings) > 0 {
		return false
	}
	for _, marker := range promptEchoMarkers {
		if strings.Contains(raw, marker) {
			return true
		}
	}
	return false
}

// FormatSecurityResult renders a security review, or falls back to the raw log
// when the payload is unusable. It owns that choice because the inner
// formatter has neither the raw output nor the CLI the normalizer needs.
func FormatSecurityResult(parsed *ReviewerOutput, raw, cli, model, scope string) string {
	if err := ValidateSecurityResult(parsed, raw); err != nil {
		var sb strings.Builder
		fmt.Fprintf(&sb, "\n═══ RIVAL SECURITY REVIEW — UNUSABLE OUTPUT ═══\n\n")
		fmt.Fprintf(&sb, "Model: %s\nScope: %s\nProblem: %s\n\n", config.EngineLabel(cli, model), scope, err)
		sb.WriteString("The model ran but did not return a review this can trust.\n")
		sb.WriteString("Raw output follows for diagnosis.\n\n")
		sb.WriteString(config.PublicRuntimeLog(cli, model, raw))
		return sb.String()
	}
	return FormatSecurityConsole(parsed, config.EngineLabel(cli, model), scope)
}

// FormatSecurityConsole renders a validated security review.
func FormatSecurityConsole(out *ReviewerOutput, model, scope string) string {
	var sb strings.Builder
	sb.WriteString("\n═══ RIVAL SECURITY REVIEW ═══\n\n")
	fmt.Fprintf(&sb, "Model: %s\n", model)
	fmt.Fprintf(&sb, "Scope: %s\n\n", scope)
	if summary := strings.TrimSpace(out.Summary); summary != "" {
		fmt.Fprintf(&sb, "Summary: %s\n\n", summary)
	}

	if len(out.Findings) == 0 {
		sb.WriteString("No vulnerabilities found.\n")
		return sb.String()
	}

	findings := make([]ReviewerFinding, len(out.Findings))
	copy(findings, out.Findings)
	sort.SliceStable(findings, func(i, j int) bool {
		ri, rj := severityRank(findings[i].Severity), severityRank(findings[j].Severity)
		if ri != rj {
			return ri < rj
		}
		return findings[i].Confidence > findings[j].Confidence
	})

	for i, f := range findings {
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		fmt.Fprintf(&sb, "%d. [%s] %s", i+1, displaySeverity(f.Severity), f.Title)
		if loc != "" {
			fmt.Fprintf(&sb, " — %s", loc)
		}
		sb.WriteString("\n")
		if body := strings.TrimSpace(f.Body); body != "" {
			fmt.Fprintf(&sb, "   %s\n", body)
		}
		if fix := strings.TrimSpace(f.Suggestion); fix != "" {
			fmt.Fprintf(&sb, "   Fix: %s\n", fix)
		}
		if f.Category != "" {
			fmt.Fprintf(&sb, "   (%s, confidence %d)\n", f.Category, f.Confidence)
		} else {
			fmt.Fprintf(&sb, "   (confidence %d)\n", f.Confidence)
		}
		sb.WriteString("\n")
	}

	var crit, high, med, low int
	for _, f := range findings {
		switch severityRank(f.Severity) {
		case 0:
			crit++
		case 1:
			high++
		case 2:
			med++
		default:
			low++
		}
	}
	fmt.Fprintf(&sb, "Findings: %d total — %d crit, %d high, %d med, %d low\n",
		len(findings), crit, high, med, low)
	return sb.String()
}
