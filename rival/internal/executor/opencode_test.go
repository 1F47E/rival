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
	t.Setenv("RIVAL_OPENCODE_MODELS", "") // default (Zen) roster
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

	// A non-Zen roster (opencode-go/) doesn't require the key.
	t.Setenv("RIVAL_OPENCODE_API_KEY", "")
	t.Setenv("RIVAL_OPENCODE_MODELS", "opencode-go/glm-5.2")
	if err := OpencodePreflight(); err != nil {
		t.Errorf("non-Zen roster should not require the key, got: %v", err)
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
	// Empty model or key → empty.
	if opencodeProviderConfig("", "k") != "" || opencodeProviderConfig("m", "") != "" {
		t.Error("empty model/key should yield empty config")
	}
}
