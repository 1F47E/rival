package review

import (
	"testing"

	"github.com/1F47E/rival/internal/config"
)

func TestRoleForCLI_Opencode(t *testing.T) {
	// opencode (GLM) gets the arch/security lens so the 3-reviewer roster isn't
	// all bug_hunter (codex + antigravity already are).
	if got := RoleForCLI("opencode"); got != RoleArchSecurity {
		t.Errorf("RoleForCLI(opencode) = %q, want %q", got, RoleArchSecurity)
	}
	// Sanity: the other mega reviewers are unchanged.
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
	in := func(clis ...string) []ReviewInput {
		var r []ReviewInput
		for _, c := range clis {
			r = append(r, ReviewInput{CLI: c})
		}
		return r
	}
	cases := []struct {
		name      string
		inputs    []ReviewInput
		wantCLI   string
		wantModel string
	}{
		{"codex preferred", in("opencode", "antigravity", "codex"), "codex", ""},
		{"antigravity when no codex", in("opencode", "antigravity"), "antigravity", ""},
		{"opencode when it's the only one", in("opencode"), "opencode", ""},
		{"empty when no preferred cli succeeded", in("gemini", "claude"), "", ""},
		{"empty on no inputs", nil, "", ""},
		{
			// Deterministic by ROSTER order (glm is first in the default roster),
			// not by which goroutine finished first — here deepseek-pro is listed
			// first in `inputs` (completion order) but glm must still win.
			"opencode judge picks highest roster model, not first-completed",
			[]ReviewInput{
				{CLI: "opencode", Model: "opencode/deepseek-v4-pro"},
				{CLI: "opencode", Model: "opencode/glm-5.2"},
			},
			"opencode", "opencode/glm-5.2",
		},
		{
			// If glm 429'd and only deepseek-pro survived, judge with deepseek-pro.
			"opencode judge falls to next roster model when top one failed",
			[]ReviewInput{
				{CLI: "opencode", Model: "opencode/deepseek-v4-pro"},
			},
			"opencode", "opencode/deepseek-v4-pro",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotCLI, gotModel := pickJudge(tc.inputs)
			if gotCLI != tc.wantCLI {
				t.Errorf("pickJudge(%v) cli = %q, want %q", tc.inputs, gotCLI, tc.wantCLI)
			}
			if gotModel != tc.wantModel {
				t.Errorf("pickJudge(%v) model = %q, want %q", tc.inputs, gotModel, tc.wantModel)
			}
		})
	}
}

func TestOpencodeVariantLevel_CoversAllEfforts(t *testing.T) {
	// Every valid effort must map to a non-empty opencode --variant so the
	// executor never falls back unexpectedly.
	for _, e := range config.ValidEfforts {
		if v := config.OpencodeVariantLevel[e]; v == "" {
			t.Errorf("OpencodeVariantLevel[%q] is empty; want a variant", e)
		}
	}
}
