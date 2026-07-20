package cmd

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveSkillDirsByHashRemovesOnlyExactMatches(t *testing.T) {
	base := t.TempDir()
	retired := "rival-retired-fixture"
	kept := "rival-current-fixture"
	for _, name := range []string{retired, kept} {
		if err := os.Mkdir(filepath.Join(base, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(retired)))
	removed, err := removeSkillDirsByHash(base, map[string]struct{}{sum: {}})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(filepath.Join(base, retired)); !os.IsNotExist(err) {
		t.Fatalf("retired directory still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, kept)); err != nil {
		t.Fatalf("current directory was removed: %v", err)
	}
}

func TestRetiredSkillCleanupHashesStayConfigured(t *testing.T) {
	if len(retiredSkillNameHashes) != 2 {
		t.Fatalf("retired skill cleanup hash count = %d, want 2", len(retiredSkillNameHashes))
	}
}
