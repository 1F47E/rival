package executor

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
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
	if err := OpencodePreflightModel("custom/example-model", ""); err != nil {
		t.Errorf("non-Zen model should not require the key, got: %v", err)
	}
}

func TestOpencodeProviderConfig(t *testing.T) {
	// Zen model → provider "opencode".
	got := opencodeProviderConfig(config.OpencodeDeepSeekPro, "sk-zen")
	if !strings.Contains(got, `"opencode"`) || !strings.Contains(got, "sk-zen") {
		t.Errorf("zen provider config wrong: %s", got)
	}
	// An arbitrary provider prefix is preserved.
	got = opencodeProviderConfig("custom/example-model", "sk-custom")
	if !strings.Contains(got, `"custom"`) {
		t.Errorf("custom provider config wrong: %s", got)
	}
	// Moonshot model → built-in provider "moonshotai".
	got = opencodeProviderConfig(config.KimiModel, "sk-moon")
	if !strings.Contains(got, `"moonshotai"`) {
		t.Errorf("moonshot provider config wrong: %s", got)
	}
	// Empty model or key → empty.
	if opencodeProviderConfig("", "k") != "" || opencodeProviderConfig("m", "") != "" {
		t.Error("empty model/key should yield empty config")
	}
}

func TestOpencodeRunArgs_UsesOnlySupportedVariants(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		effort string
		want   string
	}{
		{name: "deepseek max", model: config.OpencodeDeepSeekPro, effort: "xhigh", want: "--variant max"},
		{name: "deepseek ultra", model: config.OpencodeDeepSeekPro, effort: "ultra", want: "--variant max"},
		{name: "deepseek medium", model: config.OpencodeDeepSeekPro, effort: "medium", want: "--variant medium"},
		{name: "kimi-k3 pins max", model: config.KimiModel, effort: "max", want: "--variant max"},
		{name: "kimi-k3 pins max at low", model: config.KimiModel, effort: "low", want: "--variant max"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			joined := strings.Join(opencodeRunArgs(tc.model, tc.effort, "/repo"), " ")
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("args %q do not contain %q", joined, tc.want)
			}
		})
	}
}

func TestOpencodeRunEnv_IsolatesSessionDatabases(t *testing.T) {
	t.Setenv("RIVAL_OPENCODE_API_KEY", "sk-test")
	first := strings.Join(opencodeRunEnv("session-a", config.OpencodeDeepSeekPro), "\n")
	second := strings.Join(opencodeRunEnv("session-b", config.KimiModel), "\n")
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
