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
		name   string
		inputs []ReviewInput
		want   string
	}{
		{"codex preferred", in("opencode", "antigravity", "codex"), "codex"},
		{"antigravity when no codex", in("opencode", "antigravity"), "antigravity"},
		{"opencode when it's the only one", in("opencode"), "opencode"},
		{"empty when no preferred cli succeeded", in("gemini", "claude"), ""},
		{"empty on no inputs", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickJudge(tc.inputs); got != tc.want {
				t.Errorf("pickJudge(%v) = %q, want %q", tc.inputs, got, tc.want)
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
