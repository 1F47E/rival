package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
	"github.com/rs/zerolog/log"
)

// GrokPreflight checks that grok is installed and authenticated. The auth file
// is resolved from the real home directory rather than $GROK_HOME: that prefix
// is blocked from child environments (see blockedEnvPrefixes), so honoring it
// here would check a location the run can never use.
func GrokPreflight() error {
	if _, err := exec.LookPath("grok"); err != nil {
		return fmt.Errorf("%s runtime is not installed", config.GrokLabel)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("%s authentication is unavailable: %w; run `grok login`", config.GrokLabel, err)
	}
	authFile := filepath.Join(home, ".grok", "auth.json")
	if _, err := os.Stat(authFile); err != nil {
		return fmt.Errorf("%s authentication is unavailable (%s not found); run `grok login`", config.GrokLabel, authFile)
	}
	return nil
}

// RunGrok executes a prompt with grok-4.5. Unlike codex, the grok CLI does not
// read the prompt from stdin, so the composed prompt is handed over in a temp
// file — this also keeps it out of the process table. review selects the
// read-only sandbox used by review pipelines.
func RunGrok(ctx context.Context, sess *session.Session, prompt, effort, workdir string, review bool, mirror io.Writer) (*Result, error) {
	promptFile, err := os.CreateTemp("", "rival-grok-*.md")
	if err != nil {
		return nil, fmt.Errorf("%s runtime: create prompt file: %w", config.GrokLabel, err)
	}
	defer func() {
		if removeErr := os.Remove(promptFile.Name()); removeErr != nil && !os.IsNotExist(removeErr) {
			log.Warn().Err(removeErr).Str("file", promptFile.Name()).Msg("failed to remove grok prompt file")
		}
	}()

	if _, err := io.WriteString(promptFile, grokFullPrompt(prompt, workdir)); err != nil {
		_ = promptFile.Close()
		return nil, fmt.Errorf("%s runtime: write prompt file: %w", config.GrokLabel, err)
	}
	if err := promptFile.Close(); err != nil {
		return nil, fmt.Errorf("%s runtime: close prompt file: %w", config.GrokLabel, err)
	}

	args, err := grokRunArgs(config.GrokModel, promptFile.Name(), effort, workdir, review)
	if err != nil {
		return nil, fmt.Errorf("%s runtime: %w", config.GrokLabel, err)
	}

	// The prompt is already in the file; stdin carries nothing.
	result, err := RunSubprocess(ctx, sess, "grok", args, nil, "", mirror)
	if err != nil {
		return nil, fmt.Errorf("%s runtime: %s", config.GrokLabel, err.Error())
	}
	return result, nil
}

// grokFullPrompt composes the prompt exactly as the other executors do, so a
// grok run sees the same system prompt and workdir preamble as Sol or Fable.
func grokFullPrompt(prompt, workdir string) string {
	return config.SystemPrompt + "\n\n" + config.BuildWorkdirPreamble(workdir) + "\n" + prompt
}

func grokRunArgs(model, promptFile, effort, workdir string, review bool) ([]string, error) {
	mapped, err := GrokEffort(effort)
	if err != nil {
		return nil, err
	}

	args := []string{
		"--prompt-file", promptFile,
		"-m", model,
		"--effort", mapped,
		"--output-format", "plain",
		"--no-auto-update",
		"--yolo",
	}
	if workdir != "" {
		args = append(args, "--cwd", workdir)
	}
	if review {
		args = append(args, "--sandbox", "read-only")
	}
	return args, nil
}

// GrokEffort maps rival's effort menu onto grok-4.5's own low/medium/high.
// Levels above high clamp to high and levels below low clamp to low rather
// than failing a run over a level the model simply does not expose; an
// unrecognized value is still an error so typos do not silently downgrade.
// Exported so callers can record the clamped value on the session instead of
// the level the user asked for.
func GrokEffort(effort string) (string, error) {
	switch effort {
	case "low", "medium", "high":
		return effort, nil
	case "xhigh", "ultra", "max":
		return "high", nil
	case "minimal", "none":
		return "low", nil
	case "":
		return "", fmt.Errorf("effort is required for %s", config.GrokLabel)
	default:
		return "", fmt.Errorf("unsupported %s effort %q; use one of: low, medium, high", config.GrokLabel, effort)
	}
}
