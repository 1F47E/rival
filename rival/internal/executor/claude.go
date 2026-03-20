package executor

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
)

// ClaudePreflight checks that claude CLI is installed.
func ClaudePreflight() error {
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("claude CLI not installed. Install: https://docs.anthropic.com/en/docs/claude-code/overview")
	}
	return nil
}

// RunClaude executes a prompt through the Claude Code CLI.
func RunClaude(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
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

	fullPrompt := prompt
	return RunSubprocess(ctx, sess, "claude", args, nil, fullPrompt, mirror)
}
