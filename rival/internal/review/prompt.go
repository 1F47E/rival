package review

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/1F47E/rival/internal/config"
)

// BuildReviewerPrompt builds the reviewer prompt by combining scope context
// with the bug-hunter instructions and the JSON output contract.
func BuildReviewerPrompt(scope string, kind config.PromptKind) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Review scope: %s\n\n", scope))

	// Both prompts stay overridable through ~/.rival/config.yaml. An empty
	// override falls through rather than producing an empty prompt.
	key, builtin := "bug_hunter", bugHunterInstructions
	if kind == config.PromptSecurity {
		key, builtin = "security", securityInstructions
	}
	if override, ok := config.RolePromptOverride(key); ok && strings.TrimSpace(override) != "" {
		sb.WriteString(override)
	} else {
		sb.WriteString(builtin())
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
	sb.WriteString(reviewerLensMap(inputs))

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

// reviewerLensMap tells the judge which reviewer looked for what. Without it
// the judge cannot tell a finding that nobody corroborated from one that only
// a single reviewer was equipped to find.
func reviewerLensMap(inputs []ReviewInput) string {
	var sb strings.Builder
	sb.WriteString("## Reviewer lenses\n\n")
	for _, input := range inputs {
		fmt.Fprintf(&sb, "- %s reviewed for: %s\n", config.EngineLabel(input.CLI, input.Model), input.Prompt)
	}
	sb.WriteString(`
Reviewers can carry different lenses. Absence of corroboration from a reviewer
that was not looking for a class of defect is NOT evidence against a finding.
Do not lower confidence, and do not drop a finding, merely because only the
reviewer equipped to find it reported it.

The consensus bonus still applies whenever two reviewers independently report
the same issue, whatever lens each of them used.

`)
	return sb.String()
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

// securityInstructions is the vulnerability-hunting lens. It shares the JSON
// contract with the bug hunter so one parser and one formatter serve both.
//
// The twelve classes below are the taxonomy the plan review settled on. Each
// is asserted by a test, because a prompt that quietly loses a class produces
// a review that looks complete while never checking for it.
func securityInstructions() string {
	return `## Role: Security Reviewer

You are the security reviewer for this code review. Hunt exploitable
vulnerabilities, not style and not ordinary logic bugs.

Work through every class below. For each finding, state the attack: what an
attacker controls, what they reach, and what they get.

1. **Injection** — SQL, shell, template, LDAP, XPath, or NoSQL built from
   untrusted input; interpolation where a parameterized API exists.
2. **Authorization** — missing or wrong ownership checks; an identifier from
   the request used to fetch a record without proving the caller may see it
   (IDOR); privilege escalation through a mass-assigned field.
3. **Authentication** — guessable or missing session invalidation, tokens
   that never expire, comparison of secrets with a non-constant-time
   operation, credentials accepted from an untrusted source.
4. **Crypto** — a broken or ad-hoc algorithm, a static IV or nonce, a key
   derived from something predictable, randomness from a non-cryptographic
   source.
5. **Path traversal** — a filename or path segment from input reaching the
   filesystem without containment; archive extraction that trusts entry
   names.
6. **SSRF** — a URL, host, or port from input driving an outbound request;
   redirects followed into an internal network; metadata endpoints reachable.
7. **Deserialization** — untrusted input decoded into typed objects, or a
   format that can instantiate arbitrary types.
8. **Secret exposure** — credentials in logs, error text, URLs, or client
   responses; keys committed to the repository; a token widened beyond the
   scope it needs.
9. **Input validation** — missing bounds or type checks that reach memory,
   allocation, or a parser; an integer that can overflow into an index or a
   size.
10. **CSRF** — a state-changing route with no token or origin check, or one
    whose check can be bypassed by method or content type.
11. **Open redirect** — a redirect target taken from input without an
    allowlist, including the login-return case.
12. **Resource exhaustion** — an unbounded read, allocation, or loop driven
    by input; a regex that backtracks exponentially; a missing timeout or
    limit on work an attacker can trigger repeatedly.

Rules:
- Report only what you can tie to exact code. Name the file and the line.
- Prefer few strong findings over many weak ones. If a class does not apply
  to this code, say nothing about it rather than inventing a finding.
- If the code is genuinely sound, say so and return no findings.
- Do not report style, naming, or ordinary logic bugs. Another reviewer
  covers those.
`
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
