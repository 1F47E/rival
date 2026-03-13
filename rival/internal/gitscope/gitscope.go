package gitscope

import (
	"os/exec"
	"strings"
)

// Resolve detects which files to review based on git state.
// Priority: dirty files (staged+unstaged+untracked) → last commit → empty string.
// Returns newline-separated file list, or "" if not a git repo or no changes found.
func Resolve(workdir string) string {
	// Try dirty files first: tracked changes (staged + unstaged modifications).
	tracked, _ := gitCmd(workdir, "diff", "--name-only", "HEAD")
	// Also catch untracked new files (not yet git-added).
	untracked, _ := gitCmd(workdir, "ls-files", "--others", "--exclude-standard")

	files := mergeFileLists(strings.TrimSpace(tracked), strings.TrimSpace(untracked))
	if files != "" {
		return files
	}

	// Working tree is clean — try last commit.
	out, err := gitCmd(workdir, "diff", "--name-only", "HEAD~1", "HEAD")
	if err == nil {
		files = strings.TrimSpace(out)
		if files != "" {
			return files
		}
	}

	return ""
}

// mergeFileLists combines two newline-separated file lists, deduplicating.
func mergeFileLists(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	// Deduplicate using a set.
	seen := make(map[string]struct{})
	var result []string
	for _, f := range strings.Split(a, "\n") {
		f = strings.TrimSpace(f)
		if f != "" {
			seen[f] = struct{}{}
			result = append(result, f)
		}
	}
	for _, f := range strings.Split(b, "\n") {
		f = strings.TrimSpace(f)
		if f != "" {
			if _, ok := seen[f]; !ok {
				result = append(result, f)
			}
		}
	}
	return strings.Join(result, "\n")
}

func gitCmd(workdir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workdir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
