package executor

import (
	"context"
	"io"
	"os"
	"os/exec"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
)

// ClaudePreflight checks that claude is available (native or docker).
func ClaudePreflight() error {
	if _, err := exec.LookPath("claude"); err == nil {
		return nil
	}
	return ClaudeDockerPreflight()
}

// RunClaude executes a prompt through Claude CLI (native if available, docker otherwise).
func RunClaude(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
	if _, err := exec.LookPath("claude"); err == nil {
		sess.Mode = "native"
		return runClaudeNative(ctx, sess, prompt, effort, workdir, mirror)
	}
	sess.Mode = "docker"
	return RunClaudeDocker(ctx, sess, prompt, effort, workdir, mirror)
}

func runClaudeNative(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
	claudeEffort := config.ClaudeEffortLevel[effort]
	if claudeEffort == "" {
		claudeEffort = "max"
	}

	args := []string{
		"-p",
		"--model", config.ClaudeModel,
		"--effort", claudeEffort,
		"--output-format", "text",
		"--no-session-persistence",
		"--dangerously-skip-permissions",
		"--system-prompt", config.SystemPrompt,
	}

	env := os.Environ()
	return RunSubprocess(ctx, sess, "claude", args, env, prompt, mirror)
}
