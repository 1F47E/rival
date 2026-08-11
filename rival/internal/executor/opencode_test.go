package executor

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
)

func TestOpencodePreflight_K3RequiresKey(t *testing.T) {
	// This branch only fires when the opencode CLI is installed; skip otherwise
	// so the test is deterministic across machines.
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("opencode CLI not installed")
	}

	t.Setenv("MOONSHOT_API_KEY", "")
	t.Setenv("KIMI_API", "")
	if err := OpencodePreflightModel(config.KimiModel, ""); err == nil {
		t.Fatal("expected preflight error when K3 has no MOONSHOT_API_KEY")
	} else if !strings.Contains(err.Error(), "MOONSHOT_API_KEY") {
		t.Errorf("preflight error should name the missing key, got: %v", err)
	}

	t.Setenv("MOONSHOT_API_KEY", "sk-test")
	if err := OpencodePreflightModel(config.KimiModel, ""); err != nil {
		t.Errorf("preflight should pass with a key set, got: %v", err)
	}
}

func TestOpencodePreflightRejectsUnsupportedModel(t *testing.T) {
	if err := OpencodePreflightModel("custom/example-model", ""); err == nil {
		t.Fatal("unsupported OpenCode model was accepted")
	}
}

func TestOpencodeProviderConfig(t *testing.T) {
	if got := opencodeProviderConfig("custom/example-model", "sk-custom"); got != "" {
		t.Errorf("unsupported provider config = %s, want empty", got)
	}
	// Moonshot model → built-in provider "moonshotai".
	got := opencodeProviderConfig(config.KimiModel, "sk-moon")
	if !strings.Contains(got, `"moonshotai"`) {
		t.Errorf("moonshot provider config wrong: %s", got)
	}
	// Empty model or key → empty.
	if opencodeProviderConfig("", "k") != "" || opencodeProviderConfig(config.KimiModel, "") != "" {
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
	t.Setenv("MOONSHOT_API_KEY", "sk-test")
	first := strings.Join(opencodeRunEnvWith("session-a", config.KimiModel, "", OpencodeRunOpts{}), "\n")
	second := strings.Join(opencodeRunEnvWith("session-b", config.KimiModel, "", OpencodeRunOpts{}), "\n")
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
