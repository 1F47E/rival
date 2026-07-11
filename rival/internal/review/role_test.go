package review

import (
	"testing"

	"github.com/1F47E/rival/internal/config"
)

func TestRoleForCLI_Opencode(t *testing.T) {
	if got := RoleForCLI("opencode"); got != RoleBugHunter {
		t.Errorf("RoleForCLI(opencode) = %q, want %q", got, RoleBugHunter)
	}
	// Sanity: legacy CLI fallback mappings remain unchanged.
	if got := RoleForCLI("codex"); got != RoleBugHunter {
		t.Errorf("RoleForCLI(codex) = %q, want bug_hunter", got)
	}
	if got := RoleForCLI("antigravity"); got != RoleBugHunter {
		t.Errorf("RoleForCLI(antigravity) = %q, want bug_hunter", got)
	}
}

func TestModelForCLI_Opencode(t *testing.T) {
	if got := modelForCLI("opencode"); got != config.OpencodeModel {
		t.Errorf("modelForCLI(opencode) = %q, want %q", got, config.OpencodeModel)
	}
}

func TestPickJudge(t *testing.T) {
	cases := []struct {
		name      string
		inputs    []ReviewInput
		targets   []config.ReviewTarget
		wantCLI   string
		wantModel string
	}{
		{
			"default judge picks GPT-5.6-Sol regardless of completion order",
			[]ReviewInput{
				{CLI: "opencode", Model: config.OpencodeGLMModel},
				{CLI: "codex", Model: config.GPT56SolModel},
				{CLI: "opencode", Model: config.OpencodeDeepSeekPro},
				{CLI: "opencode", Model: config.OpencodeKimiK27Code},
			},
			config.DefaultReviewTargets(),
			"codex", config.GPT56SolModel,
		},
		{
			"requested order controls OpenCode judge",
			[]ReviewInput{
				{CLI: "opencode", Model: config.OpencodeKimiK27Code},
				{CLI: "opencode", Model: config.OpencodeGLMModel},
			},
			[]config.ReviewTarget{
				{CLI: "opencode", Model: config.OpencodeGLMModel, Role: "code_quality"},
				{CLI: "opencode", Model: config.OpencodeKimiK27Code, Role: "arch_security"},
			},
			"opencode", config.OpencodeGLMModel,
		},
		{
			"default judge falls through to DeepSeek when GPT-5.6-Sol failed",
			[]ReviewInput{{CLI: "opencode", Model: config.OpencodeDeepSeekPro}},
			config.DefaultReviewTargets(),
			"opencode", config.OpencodeDeepSeekPro,
		},
		{
			"single GPT-5.6-Sol success judges itself",
			[]ReviewInput{{CLI: "codex", Model: config.GPT56SolModel}},
			[]config.ReviewTarget{{CLI: "codex", Model: config.GPT56SolModel, Role: "bug_hunter"}},
			"codex", config.GPT56SolModel,
		},
		{
			"empty on no successful inputs",
			nil,
			config.DefaultReviewTargets(),
			"", "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotCLI, gotModel := pickJudge(tc.inputs, tc.targets)
			if gotCLI != tc.wantCLI {
				t.Errorf("pickJudge(%v) cli = %q, want %q", tc.inputs, gotCLI, tc.wantCLI)
			}
			if gotModel != tc.wantModel {
				t.Errorf("pickJudge(%v) model = %q, want %q", tc.inputs, gotModel, tc.wantModel)
			}
		})
	}
}

func TestOpencodeVariant_PerCuratedModel(t *testing.T) {
	cases := []struct{ model, effort, want string }{
		{config.OpencodeDeepSeekPro, "low", "low"},
		{config.OpencodeDeepSeekPro, "medium", "medium"},
		{config.OpencodeDeepSeekPro, "xhigh", "max"},
		{config.OpencodeDeepSeekPro, "ultra", "max"},
		{config.OpencodeKimiK27Code, "xhigh", ""},
		{config.OpencodeKimiK27Code, "ultra", ""},
		{config.OpencodeGLMModel, "low", "high"},
		{config.OpencodeGLMModel, "xhigh", "max"},
		{config.OpencodeGLMModel, "ultra", "max"},
	}
	for _, tc := range cases {
		if got := config.OpencodeVariant(tc.model, tc.effort); got != tc.want {
			t.Errorf("OpencodeVariant(%q, %q) = %q, want %q", tc.model, tc.effort, got, tc.want)
		}
	}
}
