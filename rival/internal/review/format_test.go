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
		CLI: "opencode", Model: config.OpencodeGLMModel, Role: "code_quality",
	}}
	got := FormatConsole(output, inputs, 6, "opencode", config.OpencodeGLMModel, nil)
	for _, want := range []string{
		"Reviewed by: glm-5.2 (code_quality)",
		"Judge: glm-5.2 (consilium)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("formatted review missing %q:\n%s", want, got)
		}
	}
}

func TestFormatConsole_UsesGPTModelName(t *testing.T) {
	output := &ConsiliumOutput{
		Summary:        "reviewed",
		Recommendation: Recommendation{Status: "approve", Summary: "solid"},
	}
	inputs := []ReviewInput{{CLI: "codex", Model: config.GPT56SolModel, Role: "bug_hunter"}}
	got := FormatConsole(output, inputs, 6, "codex", config.GPT56SolModel, nil)
	if !strings.Contains(got, "Reviewed by: "+config.GPT56SolModel+" (bug_hunter)") ||
		!strings.Contains(got, "Judge: "+config.GPT56SolModel+" (consilium)") {
		t.Fatalf("formatted review does not use the model name:\n%s", got)
	}
}

func TestFormatSkipped_DistinguishesOpenCodeModels(t *testing.T) {
	got := formatSkipped([]SkippedCLI{
		{CLI: "opencode", Model: config.OpencodeDeepSeekPro, Reason: "failed"},
		{CLI: "opencode", Model: config.OpencodeKimiK27Code, Reason: "failed"},
	})
	if !strings.Contains(got, "deepseek-v4-pro: failed") || !strings.Contains(got, "kimi-k2.7-code: failed") {
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
