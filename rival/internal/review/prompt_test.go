package review

import (
	"strings"
	"testing"
)

func TestBuildConsiliumPrompt_NilParsedBounded(t *testing.T) {
	bigRaw := strings.Repeat("X", 1_000_000)
	inputs := []ReviewInput{
		{CLI: "gemini", Model: "gemini-3.1-pro", Role: "arch_security", RawOutput: bigRaw},
		{CLI: "codex", Model: "gpt-5.5", Role: "bug_hunter", RawOutput: "small"},
	}
	prompt := BuildConsiliumPrompt(inputs, "the entire project", 6)
	if len(prompt) > 20_000 {
		t.Errorf("prompt too large: %d bytes", len(prompt))
	}
}

func TestBuildConsiliumPrompt_ParsedUsedVerbatim(t *testing.T) {
	inputs := []ReviewInput{
		{
			CLI: "codex", Model: "gpt-5.5", Role: "bug_hunter",
			Parsed: &ReviewerOutput{Summary: "all good", Findings: nil},
		},
	}
	prompt := BuildConsiliumPrompt(inputs, "src/", 6)
	if !strings.Contains(prompt, "all good") {
		t.Error("parsed summary not found in prompt")
	}
}

func TestFailedReviewerStub_TruncatesLongOutput(t *testing.T) {
	raw := strings.Repeat("A", 10_000)
	stub := failedReviewerStub("gemini", raw)
	if len(stub) > maxDebugTail+500 {
		t.Errorf("stub too large: %d bytes", len(stub))
	}
	if !strings.Contains(stub, "failed to produce structured JSON") {
		t.Error("stub missing failure message")
	}
}
