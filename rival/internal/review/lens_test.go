package review

import (
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
)

// The judge must be told which reviewer used which lens. Without it, a
// security finding that only K3 could produce looks uncorroborated and can be
// filtered out below the confidence threshold.
func TestConsiliumPromptNamesEachReviewersLens(t *testing.T) {
	inputs := []ReviewInput{
		{CLI: "codex", Model: config.GPT56SolModel, Prompt: config.PromptBugHunter,
			Parsed: &ReviewerOutput{Summary: "s"}},
		{CLI: "opencode", Model: config.KimiModel, Prompt: config.PromptSecurity,
			Parsed: &ReviewerOutput{Summary: "s"}},
	}
	got := BuildConsiliumPrompt(inputs, "src/", 6)

	if !strings.Contains(got, config.SolLabel+" reviewed for: bug hunting") {
		t.Errorf("sol's lens missing from the prompt:\n%s", got)
	}
	if !strings.Contains(got, config.K3Label+" reviewed for: security") {
		t.Errorf("k3's lens missing from the prompt:\n%s", got)
	}
	if !strings.Contains(got, "is NOT evidence against a finding") {
		t.Error("the prompt does not tell the judge how to treat a single-lens finding")
	}
	// The bonus must survive: two reviewers can find the same defect through
	// different lenses, and that agreement is still meaningful.
	if !strings.Contains(got, "whatever lens each of them used") {
		t.Error("the prompt discards cross-lens corroboration")
	}
}

// A single-lens roster still renders correctly.
func TestConsiliumPromptWithOneLens(t *testing.T) {
	inputs := []ReviewInput{
		{CLI: "codex", Model: config.GPT56SolModel, Prompt: config.PromptBugHunter,
			Parsed: &ReviewerOutput{Summary: "s"}},
	}
	got := BuildConsiliumPrompt(inputs, "src/", 6)
	if strings.Contains(got, "reviewed for: security") {
		t.Errorf("a bug-hunter-only roster claims a security lens:\n%s", got)
	}
}

// The lens must survive the whole path: resolver, target, plan, result, input.
func TestLensSurvivesFromSelectorToJudge(t *testing.T) {
	targets, err := config.ResolveReviewTargets([]string{"sol,k3"})
	if err != nil {
		t.Fatal(err)
	}
	var inputs []ReviewInput
	for _, target := range targets {
		// Mirrors what the runner builds after a reviewer completes.
		inputs = append(inputs, ReviewInput{
			CLI: target.CLI, Model: target.Model, Prompt: target.Prompt,
			Parsed: &ReviewerOutput{Summary: "s"},
		})
	}
	got := BuildConsiliumPrompt(inputs, "src/", 6)
	if !strings.Contains(got, config.K3Label+" reviewed for: security") {
		t.Errorf("the security lens did not survive to the judge prompt:\n%s", got)
	}
}
