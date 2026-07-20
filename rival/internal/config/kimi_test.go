package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKimiAPIKeyFromPrefersEnv(t *testing.T) {
	t.Setenv("KIMI_API", "env-key")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("KIMI_API=file-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := KimiAPIKeyFrom(dir); got != "env-key" {
		t.Errorf("KimiAPIKeyFrom = %q, want env-key (env must win over workdir .env)", got)
	}
}

// rival is routinely invoked from a subdirectory of the project holding the
// key; the workdir .env fallback keeps preflight working there.
func TestKimiAPIKeyFromFallsBackToWorkdirEnvFile(t *testing.T) {
	t.Setenv("KIMI_API", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("KIMI_API=file-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := KimiAPIKeyFrom(dir); got != "file-key" {
		t.Errorf("KimiAPIKeyFrom = %q, want file-key", got)
	}
	if got := KimiAPIKeyFrom(t.TempDir()); got != "" {
		t.Errorf("KimiAPIKeyFrom(no .env) = %q, want empty", got)
	}
}

// Every rival effort — including the "max" the k3 path records — must pin the
// K3 variant to max: the default OpencodeVariant branch would otherwise map
// unknown efforts to --variant high, which K3 does not support.
func TestOpencodeVariantKimiK3PinsMax(t *testing.T) {
	for _, effort := range []string{"low", "medium", "high", "xhigh", "ultra", "max", ""} {
		if got := OpencodeVariant(KimiModel, effort); got != "max" {
			t.Errorf("OpencodeVariant(%s, %q) = %q, want max", KimiModel, effort, got)
		}
	}
}

// k3/kimi-k3 select Kimi K3 via the Moonshot provider; the historical "kimi"
// alias stays on the curated kimi-k2.7-code and must not change meaning.
func TestResolveReviewTargetsK3Selector(t *testing.T) {
	for _, alias := range []string{"k3", "kimi-k3"} {
		got, err := ResolveReviewTargets([]string{alias})
		if err != nil {
			t.Fatalf("%s: %v", alias, err)
		}
		if len(got) != 1 || got[0].CLI != "opencode" || got[0].Model != KimiModel {
			t.Errorf("%s resolved to %+v, want opencode/%s", alias, got, KimiModel)
		}
	}
	got, err := ResolveReviewTargets([]string{"kimi"})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Model != OpencodeKimiK27Code {
		t.Errorf("alias kimi must stay on %s, got %s", OpencodeKimiK27Code, got[0].Model)
	}
}

// A workdir that is a subdirectory of the project (e.g. rival/ under the repo
// root) must still find the repo root's .env by walking up.
func TestKimiAPIKeyFromWalksUpToParentEnvFile(t *testing.T) {
	t.Setenv("KIMI_API", "")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("KIMI_API=parent-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "rival", "internal")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatal(err)
	}
	if got := KimiAPIKeyFrom(sub); got != "parent-key" {
		t.Errorf("KimiAPIKeyFrom(subdir) = %q, want parent-key", got)
	}
}
