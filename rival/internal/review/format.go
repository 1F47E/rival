package review

import (
	"fmt"
	"strings"
)

// FormatConsole formats the consilium output for terminal display.
func FormatConsole(output *ConsiliumOutput, inputs []ReviewInput, threshold int, judgeCLI string, skipped []SkippedCLI) string {
	var sb strings.Builder

	sb.WriteString("\n═══ RIVAL REVIEW ═══\n\n")

	sb.WriteString(fmt.Sprintf("Summary: %s\n\n", output.Summary))

	for _, f := range output.Findings {
		sev := strings.ToUpper(f.Severity)
		sb.WriteString(fmt.Sprintf("[%s] %s:%d — %s\n", sev, f.File, f.Line, f.Title))
		sb.WriteString(fmt.Sprintf("  %s\n", f.Body))
		if f.Suggestion != "" {
			sb.WriteString(fmt.Sprintf("  Fix: %s\n", f.Suggestion))
		}
		if len(f.FoundBy) > 0 {
			sb.WriteString(fmt.Sprintf("  Found by: %s\n", strings.Join(f.FoundBy, ", ")))
		}
		sb.WriteString("\n")
	}

	if len(output.Findings) == 0 {
		sb.WriteString("No findings above confidence threshold.\n\n")
	}

	sb.WriteString(fmt.Sprintf("Recommendation: %s — %s\n\n", output.Recommendation.Status, output.Recommendation.Summary))

	// Reviewer info
	var reviewers []string
	for _, input := range inputs {
		reviewers = append(reviewers, fmt.Sprintf("%s (%s)", input.CLI, input.Role))
	}
	sb.WriteString(fmt.Sprintf("Reviewed by: %s\n", strings.Join(reviewers, ", ")))
	if len(skipped) > 0 {
		var parts []string
		for _, s := range skipped {
			parts = append(parts, fmt.Sprintf("%s (%s)", s.CLI, s.Reason))
		}
		sb.WriteString(fmt.Sprintf("Skipped: %s\n", strings.Join(parts, ", ")))
	}
	sb.WriteString(fmt.Sprintf("Judge: %s (consilium)\n", judgeCLI))
	sb.WriteString(fmt.Sprintf("Findings: %d (threshold: %d)\n", len(output.Findings), threshold))

	return sb.String()
}
