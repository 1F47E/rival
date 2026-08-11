package review

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/1F47E/rival/internal/config"
)

// BuildReviewerPrompt builds the reviewer prompt by combining scope context
// with the bug-hunter instructions and the JSON output contract.
func BuildReviewerPrompt(scope string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Review scope: %s\n\n", scope))

	// A user can still override the reviewer prompt through the "bug_hunter"
	// key in ~/.rival/config.yaml, which is the only role name the config
	// surface ever accepted.
	if override, ok := config.RolePromptOverride("bug_hunter"); ok {
		sb.WriteString(override)
	} else {
		sb.WriteString(bugHunterInstructions())
	}

	sb.WriteString(reviewerJSONContract())
	return sb.String()
}

// BuildConsiliumPrompt builds the judge prompt with all reviewer findings + scope context.
func BuildConsiliumPrompt(inputs []ReviewInput, scope string, threshold int) string {
	var sb strings.Builder
	reviewerLabels := make([]string, 0, len(inputs))
	for _, input := range inputs {
		reviewerLabels = append(reviewerLabels, config.EngineLabel(input.CLI, input.Model))
	}

	sb.WriteString("# Consilium Judge — Final Code Review Verdict\n\n")
	sb.WriteString(fmt.Sprintf("Review scope: %s\n\n", scope))

	sb.WriteString(consiliumInstructions(threshold, reviewerLabels))

	// Reviewer findings
	sb.WriteString(fmt.Sprintf("## Reviewer Findings (%d reviewers)\n\n", len(inputs)))
	for _, input := range inputs {
		label := config.EngineLabel(input.CLI, input.Model)
		sb.WriteString(fmt.Sprintf("=== REVIEW FROM %s ===\n\n", label))
		if input.Parsed != nil {
			if data, err := json.MarshalIndent(input.Parsed, "", "  "); err == nil {
				sb.WriteString(string(data))
			} else {
				sb.WriteString(failedReviewerStub(label, input.RawOutput))
			}
		} else {
			sb.WriteString(failedReviewerStub(label, input.RawOutput))
		}
		sb.WriteString("\n\n=== END REVIEW ===\n\n")
	}

	sb.WriteString(consiliumJSONContract(reviewerLabels))
	return sb.String()
}

func bugHunterInstructions() string {
	return `## Role: Implementation Bug Hunter

You are the implementation bug hunter for this code review.

Your job is to find concrete code-level defects with high confidence.

Focus on:
- logic bugs
- broken state transitions
- incorrect assumptions
- missing edge-case handling
- wrong wiring between layers
- compile/build-break risks visible from the provided context
- race conditions
- data loss risks

AI-generated code checklist (check these explicitly):
- hallucinated imports: verify every import exists in the project's dependency tree
- happy-path-only logic: for every external call (DB, API, filesystem), check what happens on null/empty/error/timeout
- N+1 patterns: database or API calls inside loops, missing pagination on list queries
- shallow test assertions: tests that check truthiness instead of specific values, or only verify no-throw

Do not spend time on:
- style or formatting
- minor cleanup
- speculative architecture opinions

Rules:
- report only issues you can tie to exact code
- prefer fewer, stronger findings over many weak ones
- every finding must include exact file and line
- if a behavior looks incomplete but not clearly broken, do not upgrade it beyond medium
- if you are not confident, omit it
- read the code in the review scope before producing findings

You are not the final judge. Optimize for true positives, not completeness.
`
}

func consiliumInstructions(threshold int, reviewerLabels []string) string {
	return fmt.Sprintf(`## Instructions

You are the final judge for this code review.

You are given independent findings from different reviewer models in structured JSON.

Your job is to:
- merge duplicate findings (same file + same line + same problem = one finding, all reporters in found_by)
- keep true high-signal issues
- drop weak, speculative, or redundant findings
- assign final severity and confidence
- produce a concise final review

Rules:
- Do not invent new findings that are absent from reviewer inputs.
- In found_by, use only the exact concrete reviewer labels shown in the REVIEW FROM headers, never the generic label "opencode".
- Allowed found_by labels for this run: %s.
- For each finding, include only reviewers that independently reported that specific issue; never copy the complete allowed-label list by default.
- Prefer findings supported by multiple reviewers. Consensus bonus: findings reported by 2+ reviewers independently get +2 confidence.
- If only one reviewer reported an issue, keep it only if the evidence is concrete and confidence >= %d.
- Prioritize product regressions, correctness, build-breaks, security, and broken critical flows.
- De-prioritize cleanup, style, and low-value noise.
- Every finding MUST reference an exact file path and line number.
- Keep the final output short and dense.

Severity levels:
- critical: Data loss, security vulnerability, crash in production
- high: Significant bug, race condition, missing error handling
- medium: Logic issue, performance problem, architectural concern
- low: Minor issue, edge case, improvement suggestion

`, strings.Join(reviewerLabels, ", "), threshold)
}

func reviewerJSONContract() string {
	return `## Output Format

Return JSON only. No prose, no markdown, no explanation outside the JSON. Your entire response must be a single valid JSON object matching this schema:

` + "```json" + `
{
  "summary": "1-3 sentence reviewer summary",
  "findings": [
    {
      "file": "path/to/file",
      "line": 42,
      "severity": "critical|high|medium|low",
      "category": "bug|security|performance|concurrency|architecture|tests|ux",
      "title": "brief title",
      "body": "concrete explanation tied to code",
      "suggestion": "concrete fix",
      "confidence": 8
    }
  ]
}
` + "```" + `

If the code is solid, return: {"summary": "No issues found.", "findings": []}
`
}

const maxDebugTail = 2048

func failedReviewerStub(cli, rawOutput string) string {
	tail := rawOutput
	if len(tail) > maxDebugTail {
		tail = tail[len(tail)-maxDebugTail:]
	}
	stub := struct {
		Summary   string `json:"summary"`
		Findings  []any  `json:"findings"`
		DebugTail string `json:"debug_tail,omitempty"`
	}{
		Summary:   fmt.Sprintf("%s failed to produce structured JSON output", cli),
		Findings:  []any{},
		DebugTail: strings.TrimSpace(tail),
	}
	data, err := json.MarshalIndent(stub, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"summary":"%s failed: internal marshal error","findings":[]}`, cli)
	}
	return string(data)
}

func consiliumJSONContract(reviewerLabels []string) string {
	if len(reviewerLabels) == 0 {
		reviewerLabels = []string{"reviewer-label"}
	}
	labelsJSON, _ := json.Marshal(reviewerLabels[:1])
	return `## Output Format

Return JSON only. No prose, no markdown, no explanation outside the JSON. Your entire response must be a single valid JSON object matching this schema:

` + "```json" + `
{
  "summary": "1-3 sentence overall review summary",
  "findings": [
    {
      "file": "path/to/file",
      "line": 42,
      "severity": "critical|high|medium|low",
      "category": "bug|security|performance|concurrency|architecture|tests|ux",
      "title": "brief title",
      "body": "concrete explanation tied to code",
      "suggestion": "concrete fix",
      "confidence": 8,
	  "found_by": ` + string(labelsJSON) + `
    }
  ],
  "recommendation": {
    "status": "approve|request_changes|comment",
    "summary": "1-2 sentence recommendation"
  }
}
` + "```" + `
`
}
