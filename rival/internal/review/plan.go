package review

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// PlanOutput is the structured JSON emitted by `rival command plan`. It mirrors
// ReviewerOutput but carries a 1-10 Rating and is parsed independently so the
// plan command stays decoupled from the multi-CLI consilium types.
type PlanOutput struct {
	Summary  string            `json:"summary"`
	Rating   int               `json:"rating"`
	Findings []ReviewerFinding `json:"findings"`
}

// planExampleSummary is the exact summary string from the PlanReviewPrompt schema
// example. A real plan review never emits this verbatim, so it lets us skip the
// echoed schema and pick the model's real answer.
const planExampleSummary = "1-3 sentence overall assessment of the plan"

// ParsePlanOutput extracts the plan reviewer's structured JSON from raw CLI
// output. Like ParseReviewerOutput, the CLI echoes the prompt (with the schema
// example) and prints unrelated JSON, so we scan every top-level JSON object and
// take the LAST genuine plan payload — it must carry both "findings" and "rating"
// keys and must not be the schema example itself.
func ParsePlanOutput(raw string) (*PlanOutput, error) {
	objs := jsonObjects(raw)
	var lastErr error
	for i := len(objs) - 1; i >= 0; i-- {
		c := objs[i]
		// A genuine plan payload carries summary + rating + findings. Requiring all
		// three rejects unrelated/tool JSON and reviewer (non-plan) payloads.
		if !hasJSONKey(c, "findings") || !hasJSONKey(c, "rating") || !hasJSONKey(c, "summary") {
			continue
		}
		var out PlanOutput
		if err := json.Unmarshal([]byte(c), &out); err != nil {
			lastErr = err
			continue
		}
		if strings.TrimSpace(out.Summary) == planExampleSummary {
			continue // the echoed schema example, not a real answer
		}
		if out.Rating < 1 || out.Rating > 10 {
			continue // not a real 1-10 plan rating; likely noise or a partial echo
		}
		out.Findings = dropPlaceholderReviewerFindings(out.Findings)
		return &out, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("no valid plan JSON payload (last decode error: %w)", lastErr)
	}
	return nil, fmt.Errorf("no plan JSON payload found in output")
}

// displaySeverity maps the canonical severity word to the short label shown in
// plan output (crit/high/med/low). Unknown values pass through unchanged.
func displaySeverity(canonical string) string {
	switch strings.ToLower(canonical) {
	case "critical":
		return "crit"
	case "high":
		return "high"
	case "medium":
		return "med"
	case "low":
		return "low"
	default:
		return strings.ToLower(canonical)
	}
}

// FormatPlanConsole renders a PlanOutput for the caller: the 1-10 rating, the
// summary, then every finding grouped by severity bucket (crit→high→med→low) and
// numbered globally 1..N. No confidence filtering — all findings are returned.
func FormatPlanConsole(out *PlanOutput, file string) string {
	var sb strings.Builder

	sb.WriteString("\n═══ RIVAL PLAN REVIEW ═══\n\n")
	sb.WriteString(fmt.Sprintf("File: %s\n", file))
	sb.WriteString(fmt.Sprintf("Rating: %d/10\n\n", out.Rating))
	if s := strings.TrimSpace(out.Summary); s != "" {
		sb.WriteString(fmt.Sprintf("Summary: %s\n\n", s))
	}

	// Stable sort by severity (crit first), then confidence (highest first), so
	// findings come out grouped without a separate bucketing pass.
	findings := make([]ReviewerFinding, len(out.Findings))
	copy(findings, out.Findings)
	sort.SliceStable(findings, func(i, j int) bool {
		ri, rj := severityRank(findings[i].Severity), severityRank(findings[j].Severity)
		if ri != rj {
			return ri < rj
		}
		return findings[i].Confidence > findings[j].Confidence
	})

	if len(findings) == 0 {
		sb.WriteString("No bugs or gaps found.\n")
		return sb.String()
	}

	for i, f := range findings {
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] %s", i+1, displaySeverity(f.Severity), f.Title))
		if loc != "" {
			sb.WriteString(fmt.Sprintf(" — %s", loc))
		}
		sb.WriteString("\n")
		if b := strings.TrimSpace(f.Body); b != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", b))
		}
		if s := strings.TrimSpace(f.Suggestion); s != "" {
			sb.WriteString(fmt.Sprintf("   Fix: %s\n", s))
		}
		if f.Category != "" {
			sb.WriteString(fmt.Sprintf("   (%s, confidence %d)\n", f.Category, f.Confidence))
		} else {
			sb.WriteString(fmt.Sprintf("   (confidence %d)\n", f.Confidence))
		}
		sb.WriteString("\n")
	}

	// Severity tally for a quick read.
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
	sb.WriteString(fmt.Sprintf("Findings: %d total — %d crit, %d high, %d med, %d low\n",
		len(findings), crit, high, med, low))

	return sb.String()
}
