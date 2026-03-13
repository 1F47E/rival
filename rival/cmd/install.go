package cmd

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/1F47E/rival/internal/skills"
	"github.com/spf13/cobra"
)

var forceInstall bool

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Claude Code skills to ~/.claude/skills/",
	RunE:  runInstall,
}

func init() {
	installCmd.Flags().BoolVar(&forceInstall, "force", false, "overwrite without prompting")
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	targetBase := filepath.Join(home, ".claude", "skills")
	fmt.Printf("Installing skills to %s\n\n", targetBase)

	var installed, updated, skipped int

	for _, name := range skills.Names {
		srcContent, srcVersion, err := readEmbeddedSkill(name)
		if err != nil {
			return fmt.Errorf("read embedded skill %s: %w", name, err)
		}

		targetDir := filepath.Join(targetBase, name)
		targetFile := filepath.Join(targetDir, "SKILL.md")

		if _, err := os.Stat(targetFile); os.IsNotExist(err) {
			// New install
			if err := writeSkill(targetDir, targetFile, srcContent); err != nil {
				return err
			}
			fmt.Printf("  ✓ %s — installed (v%s)\n", name, srcVersion)
			installed++
			continue
		}

		// Existing — compare versions
		existingContent, err := os.ReadFile(targetFile)
		if err != nil {
			return fmt.Errorf("read %s: %w", targetFile, err)
		}
		dstVersion := parseVersion(string(existingContent))

		if srcVersion == dstVersion && !forceInstall {
			fmt.Printf("  · %s — already up to date (v%s)\n", name, srcVersion)
			skipped++
			continue
		}

		if !forceInstall {
			fmt.Printf("  ? %s — update v%s → v%s? [y/N] ", name, dstVersion, srcVersion)
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				fmt.Printf("    skipped\n")
				skipped++
				continue
			}
		}

		if err := writeSkill(targetDir, targetFile, srcContent); err != nil {
			return err
		}
		fmt.Printf("  ✓ %s — updated (v%s → v%s)\n", name, dstVersion, srcVersion)
		updated++
	}

	fmt.Println()
	fmt.Printf("Done: %d installed, %d updated, %d up to date\n", installed, updated, skipped)
	return nil
}

func readEmbeddedSkill(name string) ([]byte, string, error) {
	content, err := fs.ReadFile(skills.Files, filepath.Join(name, "SKILL.md"))
	if err != nil {
		return nil, "", err
	}
	return content, parseVersion(string(content)), nil
}

func writeSkill(dir, file string, content []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(file, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", file, err)
	}
	return nil
}

// parseVersion extracts the version: field from YAML frontmatter.
// Frontmatter is between the first and second "---" lines.
func parseVersion(content string) string {
	lines := strings.Split(content, "\n")
	inFrontmatter := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break // end of frontmatter
		}
		if inFrontmatter && strings.HasPrefix(trimmed, "version:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "version:"))
		}
	}
	return "unknown"
}
