package review

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/1F47E/rival/internal/config"
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

// FormatPlanConsole renders a single-CLI PlanOutput for the caller: the header,
// the file, the 1-10 rating, the summary, then every finding grouped by severity
// bucket (crit→high→med→low) and numbered globally 1..N. No confidence filtering —
// all findings are returned.
func FormatPlanConsole(out *PlanOutput, file string) string {
	var sb strings.Builder
	sb.WriteString("\n═══ RIVAL PLAN REVIEW ═══\n\n")
	fmt.Fprintf(&sb, "File: %s\n", file)
	formatPlanBody(out, &sb)
	return sb.String()
}

// formatPlanBody writes the rating/summary/findings/tally for one PlanOutput into
// sb. It carries no header or File line, so it is shared by both the single-CLI
// (FormatPlanConsole) and multi-model (FormatPlanMultiConsole) renderers.
func formatPlanBody(out *PlanOutput, sb *strings.Builder) {
	fmt.Fprintf(sb, "Rating: %d/10\n\n", out.Rating)
	if s := strings.TrimSpace(out.Summary); s != "" {
		fmt.Fprintf(sb, "Summary: %s\n\n", s)
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
		return
	}

	for i, f := range findings {
		loc := f.File
		if f.Line > 0 {
			loc = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		fmt.Fprintf(sb, "%d. [%s] %s", i+1, displaySeverity(f.Severity), f.Title)
		if loc != "" {
			fmt.Fprintf(sb, " — %s", loc)
		}
		sb.WriteString("\n")
		if b := strings.TrimSpace(f.Body); b != "" {
			fmt.Fprintf(sb, "   %s\n", b)
		}
		if s := strings.TrimSpace(f.Suggestion); s != "" {
			fmt.Fprintf(sb, "   Fix: %s\n", s)
		}
		if f.Category != "" {
			fmt.Fprintf(sb, "   (%s, confidence %d)\n", f.Category, f.Confidence)
		} else {
			fmt.Fprintf(sb, "   (confidence %d)\n", f.Confidence)
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
	fmt.Fprintf(sb, "Findings: %d total — %d crit, %d high, %d med, %d low\n",
		len(findings), crit, high, med, low)
}

// FormatPlanResult renders a PlanRunResult for the caller. A single successful
// model reuses the original single-model layout (FormatPlanConsole when parsed,
// else the raw output). Two or more models use the multi-block layout. Any
// skipped models are surfaced.
func FormatPlanResult(result *PlanRunResult, file string) string {
	if result == nil || len(result.Results) == 0 {
		return "No plan review output.\n"
	}
	if len(result.Results) == 1 && len(result.Skipped) == 0 {
		r := result.Results[0]
		if r.Parsed != nil {
			return FormatPlanConsole(r.Parsed, file)
		}
		// Parse failed — preserve the raw model output while normalizing any
		// adapter banner to the concrete model name.
		return config.PublicRuntimeLog(r.CLI, r.Model, r.Raw)
	}
	return FormatPlanMultiConsole(result.Results, result.Skipped, file)
}

// PlanCLIResult is one CLI's plan-review outcome. Parsed is nil when the CLI's
// output could not be parsed into structured JSON, in which case Raw holds the
// unparsed CLI output so nothing the model produced is lost.
type PlanCLIResult struct {
	CLI    string
	Model  string
	Parsed *PlanOutput
	Raw    string
}

// planEngineLabel is the human-facing model name for a plan block. The adapter
// is intentionally never exposed in plan-review output.
func planEngineLabel(cli, model string) string {
	return config.EngineLabel(cli, model)
}

func planSkippedLabel(skipped SkippedCLI) string {
	return config.EngineLabel(skipped.CLI, skipped.Model)
}

// FormatPlanMultiConsole renders 2+ models' plan reviews as separate labelled
// blocks under one header, followed by any skipped models. A result whose Parsed
// is nil falls back to printing its Raw output so a parse failure never drops
// the model's work.
func FormatPlanMultiConsole(results []PlanCLIResult, skipped []SkippedCLI, file string) string {
	var sb strings.Builder

	labels := make([]string, 0, len(results))
	for _, r := range results {
		labels = append(labels, planEngineLabel(r.CLI, r.Model))
	}
	fmt.Fprintf(&sb, "\n═══ RIVAL PLAN REVIEW (%s) ═══\n\n", strings.Join(labels, " + "))
	fmt.Fprintf(&sb, "File: %s\n", file)

	for i, r := range results {
		fmt.Fprintf(&sb, "\n── %s ──\n\n", planEngineLabel(r.CLI, r.Model))
		if r.Parsed != nil {
			formatPlanBody(r.Parsed, &sb)
		} else {
			// Parse failed — emit normalized raw output so nothing is lost while
			// internal runtime identifiers stay out of the public result.
			sb.WriteString("(could not parse structured output — raw output below)\n\n")
			sb.WriteString(strings.TrimSpace(config.PublicRuntimeLog(r.CLI, r.Model, r.Raw)))
			sb.WriteString("\n")
		}
		if i < len(results)-1 {
			sb.WriteString("\n")
		}
	}

	if len(skipped) > 0 {
		sb.WriteString("\n")
		for _, s := range skipped {
			reason := config.PublicRuntimeError(s.CLI, s.Model, s.Reason)
			fmt.Fprintf(&sb, "Skipped: %s — %s\n", planSkippedLabel(s), reason)
		}
	}

	return sb.String()
}
