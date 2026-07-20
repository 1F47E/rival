package review

import (
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
)

func TestFormatConsole_UsesConcreteSelectedModelLabels(t *testing.T) {
	output := &ConsiliumOutput{
		Summary:        "reviewed",
		Recommendation: Recommendation{Status: "approve", Summary: "solid"},
	}
	inputs := []ReviewInput{{
		CLI: "opencode", Model: config.OpencodeDeepSeekPro, Role: "code_quality",
	}}
	got := FormatConsole(output, inputs, 6, "opencode", config.OpencodeDeepSeekPro, nil)
	for _, want := range []string{
		"Reviewed by: deepseek-v4-pro (code_quality)",
		"Judge: deepseek-v4-pro (consilium)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("formatted review missing %q:\n%s", want, got)
		}
	}
}

func TestFormatConsole_UsesSolName(t *testing.T) {
	output := &ConsiliumOutput{
		Summary: "reviewed",
		Findings: []Finding{{
			File: "main.go", Line: 1, Severity: "high", Title: "bug", Body: "broken",
			FoundBy: []string{"codex", config.GPT56SolModel, "unselected-model"},
		}},
		Recommendation: Recommendation{Status: "approve", Summary: "solid"},
	}
	inputs := []ReviewInput{{CLI: "codex", Model: config.GPT56SolModel, Role: "bug_hunter"}}
	got := FormatConsole(output, inputs, 6, "codex", config.GPT56SolModel, nil)
	if !strings.Contains(got, "Reviewed by: "+config.SolLabel+" (bug_hunter)") ||
		!strings.Contains(got, "Judge: "+config.SolLabel+" (consilium)") ||
		!strings.Contains(got, "Found by: "+config.SolLabel) {
		t.Fatalf("formatted review does not use the model name:\n%s", got)
	}
	if strings.Contains(got, "codex") || strings.Contains(got, config.GPT56SolModel) || strings.Contains(got, "unselected-model") {
		t.Fatalf("formatted review leaked or accepted a non-public reporter:\n%s", got)
	}
}

func TestFormatSkipped_DistinguishesOpenCodeModels(t *testing.T) {
	got := formatSkipped([]SkippedCLI{
		{CLI: "opencode", Model: config.OpencodeDeepSeekPro, Reason: "failed"},
		{CLI: "opencode", Model: config.KimiModel, Reason: "failed"},
	})
	if !strings.Contains(got, "deepseek-v4-pro: failed") || !strings.Contains(got, "kimi-k3: failed") {
		t.Fatalf("skipped models are not distinguishable: %s", got)
	}
}

func TestPreferredJudgeForSingleSelectedModel(t *testing.T) {
	for _, target := range config.DefaultReviewTargets() {
		cli, model := preferredJudgeForTargets([]config.ReviewTarget{target})
		if cli != target.CLI || model != target.Model {
			t.Errorf("single target %+v chose judge %s/%s", target, cli, model)
		}
	}
}
