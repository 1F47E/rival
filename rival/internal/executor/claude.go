package executor

import (
	"context"
	"io"
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

// RunClaude executes a prompt through Claude CLI (native if available, docker otherwise)
// using the default Claude model.
func RunClaude(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
	return RunClaudeModel(ctx, sess, prompt, effort, workdir, config.ClaudeModel, mirror)
}

// RunFable executes a prompt through the Claude CLI using the Fable model.
// Fable runs through the same `claude` binary (native or docker) and the same
// auth — only the --model string differs.
func RunFable(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
	return RunClaudeModel(ctx, sess, prompt, effort, workdir, config.FableModel, mirror)
}

// RunClaudeModel runs a prompt through the Claude CLI with an explicit model id,
// auto-selecting native (claude on PATH) vs docker.
func RunClaudeModel(ctx context.Context, sess *session.Session, prompt, effort, workdir, model string, mirror io.Writer) (*Result, error) {
	if _, err := exec.LookPath("claude"); err == nil {
		sess.Mode = "native"
		return runClaudeNative(ctx, sess, prompt, effort, workdir, model, mirror)
	}
	sess.Mode = "docker"
	return RunClaudeDocker(ctx, sess, prompt, effort, workdir, model, mirror)
}

func runClaudeNative(ctx context.Context, sess *session.Session, prompt, effort, workdir, model string, mirror io.Writer) (*Result, error) {
	claudeEffort := config.ClaudeEffortLevel[effort]
	if claudeEffort == "" {
		claudeEffort = "max"
	}

	args := []string{
		"-p",
		"--model", model,
		"--effort", claudeEffort,
		"--output-format", "text",
		"--no-session-persistence",
		"--dangerously-skip-permissions",
		"--system-prompt", config.SystemPrompt,
	}

	fullPrompt := config.BuildWorkdirPreamble(workdir) + "\n" + prompt
	return RunSubprocess(ctx, sess, "claude", args, nil, fullPrompt, mirror)
}
