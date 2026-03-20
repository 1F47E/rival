package review

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/1F47E/rival/internal/config"
)

// BuildRolePrompt builds a role-specific review prompt by combining
// scope context with role-specific instructions and JSON output contract.
func BuildRolePrompt(role Role, scope string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Review scope: %s\n\n", scope))

	// Check for user-configured role prompt override.
	if override, ok := config.RolePromptOverride(string(role)); ok {
		sb.WriteString(override)
	} else {
		switch role {
		case RoleBugHunter:
			sb.WriteString(bugHunterInstructions())
		case RoleArchSecurity:
			sb.WriteString(archSecurityInstructions())
		case RoleCodeQuality:
			sb.WriteString(codeQualityInstructions())
		}
	}

	sb.WriteString(reviewerJSONContract())
	return sb.String()
}

// BuildConsiliumPrompt builds the judge prompt with all reviewer findings + scope context.
func BuildConsiliumPrompt(inputs []ReviewInput, scope string, threshold int) string {
	var sb strings.Builder

	sb.WriteString("# Consilium Judge — Final Code Review Verdict\n\n")
	sb.WriteString(fmt.Sprintf("Review scope: %s\n\n", scope))

	sb.WriteString(consiliumInstructions(threshold))

	// Reviewer findings
	sb.WriteString(fmt.Sprintf("## Reviewer Findings (%d reviewers)\n\n", len(inputs)))
	for _, input := range inputs {
		sb.WriteString(fmt.Sprintf("=== REVIEW FROM %s (%s) [role: %s] ===\n\n", input.CLI, input.Model, input.Role))
		if input.Parsed != nil {
			data, err := json.MarshalIndent(input.Parsed, "", "  ")
			if err == nil {
				sb.WriteString(string(data))
			} else {
				sb.WriteString(input.RawOutput)
			}
		} else {
			sb.WriteString(input.RawOutput)
		}
		sb.WriteString("\n\n=== END REVIEW ===\n\n")
	}

	sb.WriteString(consiliumJSONContract())
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

func archSecurityInstructions() string {
	return `## Role: Architecture & Security Adversarial Reviewer

You are the architecture and security adversarial reviewer for this code review.

Your job is to attack the change from angles that a code-level bug hunter may miss.

Focus on:
- architectural regressions
- broken user flows across multiple files
- incomplete refactors
- concurrency and lifecycle issues
- security and permission problems
- hidden impact on tests, configuration, and integration boundaries
- error handling gaps that create silent failures

Do not spend time on:
- formatting or naming
- trivial cleanup
- low-value debug leftovers unless they create real product risk

Rules:
- prioritize findings that require reasoning across files or flows
- challenge whether a new abstraction or refactor is actually complete
- report only issues supported by the provided context
- every finding must include exact file and line
- if the issue is mainly an unfinished flow, explain the broken user outcome clearly
- read the code in the review scope before producing findings

You are not the final judge. Your value is independent attack from a different angle.
`
}

func codeQualityInstructions() string {
	return `## Role: Code Quality & Developer Experience Reviewer

You are the code quality and developer experience reviewer for this code review.

Your job is to find issues that hurt readability, maintainability, and developer ergonomics.

Focus on:
- readability and naming clarity
- unnecessary complexity and over-engineering
- error message quality and developer-facing UX
- API ergonomics and consistency
- maintainability traps and future-proofing pitfalls
- missing documentation for non-obvious logic
- developer footguns and surprising behavior
- dead code and unused abstractions
- inconsistent patterns within the codebase
- poor separation of concerns

Do not spend time on:
- formatting or whitespace
- trivial style preferences
- performance unless it directly impacts DX (e.g., slow dev builds)

Rules:
- report only issues that would meaningfully improve a developer's experience working with this code
- prefer fewer, stronger findings over many weak ones
- every finding must include exact file and line
- if code is functional but confusing, explain the confusion clearly
- read the code in the review scope before producing findings

You are not the final judge. Your value is the DX perspective that other reviewers miss.
`
}

func consiliumInstructions(threshold int) string {
	return fmt.Sprintf(`## Instructions

You are the final judge for this code review.

You are given independent reviewer findings from different providers in structured JSON.

Your job is to:
- merge duplicate findings (same file + same line + same problem = one finding, all reporters in found_by)
- keep true high-signal issues
- drop weak, speculative, or redundant findings
- assign final severity and confidence
- produce a concise final review

Rules:
- Do not invent new findings that are absent from reviewer inputs.
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

`, threshold)
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

If the code is solid, return: {"summary": "...", "findings": []}
`
}

func consiliumJSONContract() string {
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
      "found_by": ["codex", "gemini", "claude"]
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
