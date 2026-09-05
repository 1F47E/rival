package cmd

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/skills"
	"gopkg.in/yaml.v3"
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

func TestInstallForEachHost(t *testing.T) {
	for _, target := range []string{"claude", "codex"} {
		t.Run(target, func(t *testing.T) {
			home := t.TempDir()
			base, err := skillTargetBase(home, target)
			if err != nil {
				t.Fatal(err)
			}
			if err := installSkills(base, target, false); err != nil {
				t.Fatal(err)
			}
			for _, name := range skills.Names {
				content, err := os.ReadFile(filepath.Join(base, name, "SKILL.md"))
				if err != nil {
					t.Fatal(err)
				}
				source, version, err := readEmbeddedSkill(name)
				if err != nil {
					t.Fatal(err)
				}
				if parseVersion(string(content)) != version {
					t.Fatalf("lost installer version for %s", name)
				}
				if target == "claude" && string(content) != string(source) {
					t.Fatal("changed Claude skill contents")
				}
				if target == "codex" {
					parts := strings.SplitN(string(content), "---", 3)
					var frontmatter map[string]any
					if len(parts) != 3 || yaml.Unmarshal([]byte(parts[1]), &frontmatter) != nil {
						t.Fatal("invalid Codex frontmatter")
					}
					if frontmatter["name"] != name || frontmatter["description"] == "" || frontmatter["allowed-tools"] != nil {
						t.Fatalf("invalid Codex metadata for %s: %v", name, frontmatter)
					}
				}
			}
			// The same-version guard preserves local customization; --force
			// refreshes just the selected host's files.
			path := filepath.Join(base, "rival-review", "SKILL.md")
			original, _ := os.ReadFile(path)
			custom := append(append([]byte{}, original...), []byte("\nLocal customization\n")...)
			if err := os.WriteFile(path, custom, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := installSkills(base, target, false); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(path)
			if string(got) != string(custom) {
				t.Fatal("overwrote same-version customization without --force")
			}
			if err := installSkills(base, target, true); err != nil {
				t.Fatal(err)
			}
			got, _ = os.ReadFile(path)
			if string(got) != string(original) {
				t.Fatal("--force did not restore the shipped skill")
			}
			other := "codex"
			if target == "codex" {
				other = "claude"
			}
			otherBase, _ := skillTargetBase(home, other)
			if _, err := os.Stat(otherBase); !os.IsNotExist(err) {
				t.Fatal("touched the other host's skill directory")
			}
		})
	}
}

func TestSkillTargetBase(t *testing.T) {
	home := t.TempDir()
	for target, dir := range map[string]string{"claude": ".claude", "codex": ".agents"} {
		got, err := skillTargetBase(home, target)
		if err != nil || got != filepath.Join(home, dir, "skills") {
			t.Fatalf("target %s = %q, %v", target, got, err)
		}
	}
	if _, err := skillTargetBase(home, "unknown"); err == nil {
		t.Fatal("accepted unknown target")
	}
}

func TestRetiredSkillCleanupHashesStayConfigured(t *testing.T) {
	if len(retiredSkillNameHashes) != 2 {
		t.Fatalf("retired skill cleanup hash count = %d, want 2", len(retiredSkillNameHashes))
	}
}
