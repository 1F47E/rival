package gitscope

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s (%v)", args, out, err)
		}
	}
	run("init")
	run("checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func TestResolve_DirtyFiles(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("git", "-C", dir, "add", "b.go").Run(); err != nil {
		t.Fatal(err)
	}

	files := Resolve(dir)
	if files == "" {
		t.Fatal("expected dirty files, got empty")
	}
	if files != "b.go" {
		t.Errorf("expected 'b.go', got %q", files)
	}
}

func TestResolve_LastCommit(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "c.go"), []byte("package c"), 0644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	run("add", ".")
	run("commit", "-m", "add c")

	files := Resolve(dir)
	if files == "" {
		t.Fatal("expected last commit files, got empty")
	}
	if files != "c.go" {
		t.Errorf("expected 'c.go', got %q", files)
	}
}

func TestResolve_UntrackedFiles(t *testing.T) {
	dir := initRepo(t)
	// Create a new file without staging — should still be detected.
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package new"), 0644); err != nil {
		t.Fatal(err)
	}

	files := Resolve(dir)
	if files == "" {
		t.Fatal("expected untracked file, got empty")
	}
	if files != "new.go" {
		t.Errorf("expected 'new.go', got %q", files)
	}
}

func TestResolve_ModifiedUnstaged(t *testing.T) {
	dir := initRepo(t)
	// Modify an existing tracked file without staging.
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a // modified"), 0644); err != nil {
		t.Fatal(err)
	}

	files := Resolve(dir)
	if files == "" {
		t.Fatal("expected modified file, got empty")
	}
	if files != "a.go" {
		t.Errorf("expected 'a.go', got %q", files)
	}
}

func TestResolve_SingleCommitClean(t *testing.T) {
	// Repo with exactly one commit, clean working tree → should return "".
	dir := initRepo(t) // initRepo creates one commit
	files := Resolve(dir)
	if files != "" {
		t.Errorf("expected empty for single-commit clean repo, got %q", files)
	}
}

func TestResolve_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	files := Resolve(dir)
	if files != "" {
		t.Errorf("expected empty for non-git dir, got %q", files)
	}
}
