package executor

import (
	"os/exec"
	"strings"
	"testing"
)

func TestOpencodePreflight_ZenRosterRequiresKey(t *testing.T) {
	// This branch only fires when the opencode CLI is installed; skip otherwise
	// so the test is deterministic across machines.
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("opencode CLI not installed")
	}

	// Default roster uses OpenCode Zen ("opencode/") models. With no key, preflight
	// must fail with an actionable message instead of letting each reviewer fail
	// mid-run with an opaque "Missing API key".
	t.Setenv("RIVAL_OPENCODE_API_KEY", "")
	if err := OpencodePreflight(); err == nil {
		t.Fatal("expected preflight error when Zen roster has no RIVAL_OPENCODE_API_KEY")
	} else if !strings.Contains(err.Error(), "RIVAL_OPENCODE_API_KEY") {
		t.Errorf("preflight error should name the missing key, got: %v", err)
	}

	// With a key set, the Zen-key check passes.
	t.Setenv("RIVAL_OPENCODE_API_KEY", "sk-test")
	if err := OpencodePreflight(); err != nil {
		t.Errorf("preflight should pass with a key set, got: %v", err)
	}

	// A specifically selected non-Zen model doesn't require the key.
	t.Setenv("RIVAL_OPENCODE_API_KEY", "")
	if err := OpencodePreflightModel("opencode-go/glm-5.2"); err != nil {
		t.Errorf("non-Zen model should not require the key, got: %v", err)
	}
}

func TestOpencodeProviderConfig(t *testing.T) {
	// Zen model → provider "opencode".
	got := opencodeProviderConfig("opencode/glm-5.2", "sk-zen")
	if !strings.Contains(got, `"opencode"`) || !strings.Contains(got, "sk-zen") {
		t.Errorf("zen provider config wrong: %s", got)
	}
	// Go model → provider "opencode-go".
	got = opencodeProviderConfig("opencode-go/glm-5.2", "sk-go")
	if !strings.Contains(got, `"opencode-go"`) {
		t.Errorf("go provider config wrong: %s", got)
	}
	// Moonshot model → provider "moonshot".
	got = opencodeProviderConfig("moonshot/kimi-k3", "sk-moon")
	if !strings.Contains(got, `"moonshot"`) {
		t.Errorf("moonshot provider config wrong: %s", got)
	}
	// Empty model or key → empty.
	if opencodeProviderConfig("", "k") != "" || opencodeProviderConfig("m", "") != "" {
		t.Error("empty model/key should yield empty config")
	}
}

func TestOpencodeRunArgs_UsesOnlySupportedVariants(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		effort    string
		want      string
		noVariant bool
	}{
		{name: "deepseek max", model: "opencode/deepseek-v4-pro", effort: "xhigh", want: "--variant max"},
		{name: "deepseek ultra", model: "opencode/deepseek-v4-pro", effort: "ultra", want: "--variant max"},
		{name: "deepseek medium", model: "opencode/deepseek-v4-pro", effort: "medium", want: "--variant medium"},
		{name: "glm low clamps high", model: "opencode/glm-5.2", effort: "low", want: "--variant high"},
		{name: "glm ultra", model: "opencode/glm-5.2", effort: "ultra", want: "--variant max"},
		{name: "kimi has no named variant", model: "opencode/kimi-k2.7-code", effort: "xhigh", noVariant: true},
		{name: "kimi-k3 pins max", model: "moonshot/kimi-k3", effort: "max", want: "--variant max"},
		{name: "kimi-k3 pins max at low", model: "moonshot/kimi-k3", effort: "low", want: "--variant max"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			joined := strings.Join(opencodeRunArgs(tc.model, tc.effort, "/repo"), " ")
			if tc.noVariant {
				if strings.Contains(joined, "--variant") {
					t.Fatalf("Kimi args contain unsupported variant: %s", joined)
				}
				return
			}
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("args %q do not contain %q", joined, tc.want)
			}
		})
	}
}

func TestOpencodeRunEnv_IsolatesSessionDatabases(t *testing.T) {
	t.Setenv("RIVAL_OPENCODE_API_KEY", "sk-test")
	first := strings.Join(opencodeRunEnv("session-a", "opencode/deepseek-v4-pro"), "\n")
	second := strings.Join(opencodeRunEnv("session-b", "opencode/kimi-k2.7-code"), "\n")
	if !strings.Contains(first, "OPENCODE_DB=rival-session-a.db") {
		t.Fatalf("first env missing isolated DB: %s", first)
	}
	if !strings.Contains(second, "OPENCODE_DB=rival-session-b.db") {
		t.Fatalf("second env missing isolated DB: %s", second)
	}
	if first == second {
		t.Fatal("different sessions received identical OpenCode environments")
	}
}
