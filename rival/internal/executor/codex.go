package executor

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
)

// CodexPreflight checks that codex is installed and authenticated.
func CodexPreflight() error {
	if _, err := exec.LookPath("codex"); err != nil {
		return fmt.Errorf("codex CLI not installed. Install: npm install -g @openai/codex")
	}

	cmd := exec.Command("codex", "login", "status")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("codex not authenticated. Run: codex login\n%s", string(out))
	}
	return nil
}

// RunCodex executes a prompt through the Codex CLI.
func RunCodex(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
	args := []string{
		"exec",
		"-C", workdir,
		"-m", config.CodexModel,
		"-c", fmt.Sprintf("model_reasoning_effort=%s", effort),
		"--sandbox", "read-only",
		"--ephemeral",
		"--color", "never",
		"-",
	}

	return RunSubprocess(ctx, sess, "codex", args, nil, prompt, mirror)
}
