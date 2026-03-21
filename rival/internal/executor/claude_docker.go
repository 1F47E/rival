package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
	"github.com/rs/zerolog/log"
)

const claudeDockerImage = "rival-claude"

// Embedded Dockerfile content — written to temp file for auto-build.
const claudeDockerfile = `FROM node:22-slim
RUN npm install -g @anthropic-ai/claude-code
WORKDIR /workspace
ENTRYPOINT ["claude"]
`

// ClaudeDockerPreflight checks docker is available, token is set, and image exists (auto-builds if missing).
func ClaudeDockerPreflight() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not installed")
	}

	token := os.Getenv(config.ClaudeDockerTokenEnv)
	if token == "" {
		return fmt.Errorf("%s env var not set. Generate with: claude setup-token", config.ClaudeDockerTokenEnv)
	}

	// Check docker daemon is running.
	cmd := exec.Command("docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker daemon not running: %w", err)
	}

	// Check image exists, auto-build if missing.
	inspectCmd := exec.Command("docker", "image", "inspect", claudeDockerImage)
	inspectCmd.Stdout = nil
	inspectCmd.Stderr = nil
	if err := inspectCmd.Run(); err != nil {
		log.Info().Msg("rival-claude docker image not found, building...")
		if buildErr := buildClaudeDockerImage(); buildErr != nil {
			return fmt.Errorf("auto-build docker image: %w", buildErr)
		}
		log.Info().Msg("rival-claude docker image built successfully")
	}

	return nil
}

func buildClaudeDockerImage() error {
	// Write embedded Dockerfile to temp file.
	tmpFile, err := os.CreateTemp("", "rival-claude-dockerfile-*")
	if err != nil {
		return fmt.Errorf("create temp dockerfile: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(claudeDockerfile); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write dockerfile: %w", err)
	}
	tmpFile.Close()

	// Build image.
	cmd := exec.Command("docker", "build", "-t", claudeDockerImage, "-f", tmpFile.Name(), ".")
	cmd.Stdout = os.Stderr // Show build progress on stderr.
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}
	return nil
}

// RunClaudeDocker executes a prompt through the Claude Code CLI inside a Docker container.
func RunClaudeDocker(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
	token := os.Getenv(config.ClaudeDockerTokenEnv)
	if token == "" {
		return nil, fmt.Errorf("%s env var not set", config.ClaudeDockerTokenEnv)
	}

	claudeEffort := config.ClaudeEffortLevel[effort]
	if claudeEffort == "" {
		claudeEffort = "max"
	}

	// Ensure workdir is absolute for Docker volume mount.
	absWorkdir := workdir
	if !strings.HasPrefix(absWorkdir, "/") {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working dir: %w", err)
		}
		absWorkdir = wd + "/" + workdir
	}

	args := []string{
		"run", "--rm", "-i",
		"-v", absWorkdir + ":/workspace",
		"-w", "/workspace",
		"-e", "ANTHROPIC_AUTH_TOKEN=" + token,
		claudeDockerImage,
		// Claude args (entrypoint is "claude"):
		"-p",
		"--model", config.ClaudeModel,
		"--effort", claudeEffort,
		"--output-format", "text",
		"--no-session-persistence",
		"--dangerously-skip-permissions",
		"--system-prompt", config.SystemPrompt,
	}

	return RunSubprocess(ctx, sess, "docker", args, nil, prompt, mirror)
}
