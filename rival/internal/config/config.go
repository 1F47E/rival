package config

import (
	"os"
	"path/filepath"
)

const (
	CodexModel  = "gpt-5.4"
	GeminiModel = "gemini-3.1-pro-preview"

	DefaultEffort         = "high"
	SessionDir            = ".rival/sessions"
	PromptPreviewLen      = 100
	PromptDetailMaxLines  = 10
)

var ValidEfforts = []string{"low", "medium", "high", "xhigh"}

// SystemPrompt is prepended as a system instruction to all CLI invocations.
const SystemPrompt = `Answer the user's question directly. Do not offer follow-up options, menus, walkthroughs, or ask if they want more. No filler, no sign-offs. Just deliver the answer and stop.`

// Gen3 only — thinkingLevel mapping.
var GeminiThinkingLevel = map[string]string{
	"low":    "LOW",
	"medium": "MEDIUM",
	"high":   "HIGH",
	"xhigh":  "HIGH",
}

// ReviewPrompt is the language-agnostic review template. {SCOPE} is replaced at runtime.
const ReviewPrompt = `You are a ruthless senior staff engineer doing a code review. Your job is to find real problems — not nitpick style.

Review scope: {SCOPE}

Read the code in the review scope. Then produce a review covering:

1. **Critical bugs** — logic errors, race conditions, data loss risks, unhandled edge cases
2. **Security vulnerabilities** — injection, auth bypass, secret exposure, SSRF, path traversal
3. **Architecture issues** — tight coupling, missing abstractions, scalability bottlenecks
4. **Performance problems** — N+1 queries, unnecessary allocations, missing indexes, blocking I/O
5. **Error handling gaps** — swallowed errors, missing retries, unclear failure modes

Rules:
- Only report issues you are confident about. No speculative nitpicks.
- For each issue: file path, line number (or range), severity (CRITICAL/HIGH/MEDIUM), one-line description, and a concrete fix suggestion.
- Group by severity, highest first.
- If the code is solid, say so briefly. Do not invent problems.
- Skip style, formatting, naming, and documentation unless they mask a real bug.`

// IsValidEffort checks if the given effort level is in the allowlist.
func IsValidEffort(e string) bool {
	for _, v := range ValidEfforts {
		if v == e {
			return true
		}
	}
	return false
}

// SessionDirPath returns the absolute path to ~/.rival/sessions.
func SessionDirPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", SessionDir)
	}
	return filepath.Join(home, SessionDir)
}
