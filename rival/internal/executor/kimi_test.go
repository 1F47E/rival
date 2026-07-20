package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
)

// dropEnv entries ending in "_" are prefix drops — "AWS_" must catch the whole
// credential family (SESSION_TOKEN, PROFILE, WEB_IDENTITY_TOKEN_FILE, …), not
// just the three canonical names; exact entries must not over-match.
func TestDropMatchesPrefixAndExact(t *testing.T) {
	cases := []struct {
		kv   string
		want bool
	}{
		{"AWS_ACCESS_KEY_ID=x", true},
		{"AWS_WEB_IDENTITY_TOKEN_FILE=/path", true},
		{"AWS_PROFILE=default", true},
		{"OPENAI_API_KEY=sk-x", true},
		{"OPENAI_API_KEY_BACKUP=sk-x", false}, // exact entry, different name
		{"AWSOME_VAR=x", false},               // prefix requires the underscore
		{"PATH=/usr/bin", false},
	}
	for _, tc := range cases {
		if got := dropMatches(tc.kv, kimiDropEnv); got != tc.want {
			t.Errorf("dropMatches(%q) = %v, want %v", tc.kv, got, tc.want)
		}
	}
}

// KIMI_API is rival's Moonshot auth source (godotenv loads it into rival's own
// env) and no child CLI needs any KIMI_* var — the key reaches opencode via
// OPENCODE_CONFIG_CONTENT. The prefix block keeps the raw key out of every
// child environment.
func TestKimiEnvPrefixIsBlocked(t *testing.T) {
	t.Setenv("KIMI_API", "sk-secret")
	for _, kv := range safeEnv() {
		if strings.HasPrefix(kv, "KIMI_") {
			t.Errorf("safeEnv leaked %q", kv)
		}
	}
}

// Review mode keeps the zero-value read-only reviewer defaults; raw mode goes
// full-auto with the credential strip. Both carry the Moonshot key.
func TestKimiRunOptsByMode(t *testing.T) {
	t.Setenv("KIMI_API", "test-key")

	review := kimiRunOpts("review", t.TempDir())
	if review.Permission != "" {
		t.Errorf("review mode must keep the read-only default profile, got %q", review.Permission)
	}
	if len(review.DropEnv) != 0 {
		t.Errorf("review mode needs no extra drops (bash is denied), got %v", review.DropEnv)
	}
	if review.APIKey != "test-key" {
		t.Errorf("review APIKey = %q, want test-key", review.APIKey)
	}

	raw := kimiRunOpts("raw", t.TempDir())
	if raw.Permission != opencodeFullAutoPermission {
		t.Errorf("raw mode must run full-auto, got %q", raw.Permission)
	}
	if len(raw.DropEnv) != len(kimiDropEnv) {
		t.Errorf("raw mode must strip credentials, got %v", raw.DropEnv)
	}
}

// A moonshot model must receive the Moonshot key, never the Zen key — the
// provider-config injection is keyed on the model's provider prefix.
func TestMoonshotModelUsesKimiKeyNotZen(t *testing.T) {
	t.Setenv("KIMI_API", "sk-moonshot")
	t.Setenv("RIVAL_OPENCODE_API_KEY", "sk-zen")

	env := strings.Join(opencodeRunEnvWith("sess-1", config.KimiModel, "", OpencodeRunOpts{}), "\n")
	if !strings.Contains(env, "sk-moonshot") {
		t.Errorf("moonshot model env missing KIMI_API key: %s", env)
	}
	if strings.Contains(env, "sk-zen") {
		t.Errorf("moonshot model env must not carry the Zen key: %s", env)
	}
	if !strings.Contains(env, `"moonshot"`) {
		t.Errorf("provider config must target the moonshot provider: %s", env)
	}

	// Zen models keep the Zen key.
	zen := strings.Join(opencodeRunEnvWith("sess-2", "opencode/glm-5.2", "", OpencodeRunOpts{}), "\n")
	if !strings.Contains(zen, "sk-zen") {
		t.Errorf("zen model env missing Zen key: %s", zen)
	}
}

// The megareview k3 path has no explicit APIKey override, so the moonshot
// fallback must run the .env walk-up from the workdir — a review launched
// from a project subdirectory has to find the repo-root KIMI_API.
func TestMoonshotFallbackWalksUpFromWorkdir(t *testing.T) {
	t.Setenv("KIMI_API", "")
	t.Setenv("RIVAL_OPENCODE_API_KEY", "sk-zen")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("KIMI_API=sk-walkup\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub", "dir")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatal(err)
	}
	env := strings.Join(opencodeRunEnvWith("sess-4", config.KimiModel, sub, OpencodeRunOpts{}), "\n")
	if !strings.Contains(env, "sk-walkup") {
		t.Errorf("moonshot fallback did not walk up to the workdir .env: %s", env)
	}
	if strings.Contains(env, "sk-zen") {
		t.Errorf("moonshot fallback must not use the Zen key: %s", env)
	}
	if err := OpencodePreflightModel(config.KimiModel, sub); err != nil {
		t.Errorf("preflight should find the walked-up key: %v", err)
	}
}

func TestKimiRawEnvUsesFullAutoPermission(t *testing.T) {
	t.Setenv("KIMI_API", "test-key")
	env := strings.Join(opencodeRunEnvWith("sess-3", config.KimiModel, "", kimiRunOpts("raw", t.TempDir())), "\n")
	if !strings.Contains(env, "OPENCODE_PERMISSION="+opencodeFullAutoPermission) {
		t.Errorf("raw env missing full-auto permission: %s", env)
	}
}
