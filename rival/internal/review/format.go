package review

import (
	"fmt"
	"strings"

	"github.com/1F47E/rival/internal/config"
)

// FormatConsole formats the consilium output for terminal display.
func FormatConsole(output *ConsiliumOutput, inputs []ReviewInput, threshold int, judgeCLI, judgeModel string, skipped []SkippedCLI) string {
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
		if foundBy := publicFoundBy(f.FoundBy, inputs); len(foundBy) > 0 {
			sb.WriteString(fmt.Sprintf("  Found by: %s\n", strings.Join(foundBy, ", ")))
		}
		sb.WriteString("\n")
	}

	if len(output.Findings) == 0 {
		sb.WriteString("No findings above confidence threshold.\n\n")
	}

	sb.WriteString(fmt.Sprintf("Recommendation: %s — %s\n\n", output.Recommendation.Status, output.Recommendation.Summary))

	// Reviewer info — label by engine so opencode's 3 models are distinct
	// (glm-5.2, deepseek-v4-pro, …) instead of 3× "opencode".
	var reviewers []string
	for _, input := range inputs {
		reviewers = append(reviewers, fmt.Sprintf("%s (%s)", config.EngineLabel(input.CLI, input.Model), input.Role))
	}
	sb.WriteString(fmt.Sprintf("Reviewed by: %s\n", strings.Join(reviewers, ", ")))
	if len(skipped) > 0 {
		var parts []string
		for _, s := range skipped {
			reason := config.PublicRuntimeError(s.CLI, s.Model, s.Reason)
			parts = append(parts, fmt.Sprintf("%s (%s)", s.Label(), reason))
		}
		sb.WriteString(fmt.Sprintf("Skipped: %s\n", strings.Join(parts, ", ")))
	}
	sb.WriteString(fmt.Sprintf("Judge: %s (consilium)\n", config.EngineLabel(judgeCLI, judgeModel)))
	sb.WriteString(fmt.Sprintf("Findings: %d (threshold: %d)\n", len(output.Findings), threshold))

	return sb.String()
}

// publicFoundBy treats judge-provided attribution as untrusted. Only reviewers
// that succeeded in this invocation may be shown, and their adapter/model
// aliases are collapsed to the same public labels used everywhere else.
func publicFoundBy(foundBy []string, inputs []ReviewInput) []string {
	aliases := make(map[string]string)
	ambiguous := make(map[string]bool)
	addAlias := func(alias, label string) {
		alias = strings.ToLower(strings.TrimSpace(alias))
		if alias == "" || label == "" || ambiguous[alias] {
			return
		}
		if existing, ok := aliases[alias]; ok && existing != label {
			delete(aliases, alias)
			ambiguous[alias] = true
			return
		}
		aliases[alias] = label
	}

	for _, input := range inputs {
		label := config.EngineLabel(input.CLI, input.Model)
		addAlias(label, label)
		addAlias(input.Model, label)
		addAlias(input.CLI, label)
	}

	seen := make(map[string]bool)
	result := make([]string, 0, len(foundBy))
	for _, raw := range foundBy {
		label, ok := aliases[strings.ToLower(strings.TrimSpace(raw))]
		if !ok || seen[label] {
			continue
		}
		seen[label] = true
		result = append(result, label)
	}
	return result
}
