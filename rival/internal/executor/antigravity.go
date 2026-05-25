package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
	"github.com/rs/zerolog/log"
)

// AntigravityPreflight checks that agy CLI is installed.
func AntigravityPreflight() error {
	if _, err := exec.LookPath("agy"); err != nil {
		return fmt.Errorf("agy CLI not installed. Install: curl -fsSL https://antigravity.google/cli/install.sh | bash")
	}
	return nil
}

// RunAntigravity executes a prompt through the Antigravity CLI (Gemini 3.5 Flash).
// The effort parameter is accepted for interface consistency but ignored — agy has no effort flag.
func RunAntigravity(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
	ensureTrustedWorkspace(workdir)

	args := []string{
		"-p",
		"--dangerously-skip-permissions",
		"--print-timeout", "10m",
	}

	fullPrompt := config.SystemPrompt + "\n\n" + config.BuildWorkdirPreamble(workdir) + "\n" + prompt
	return RunSubprocess(ctx, sess, "agy", args, nil, fullPrompt, mirror)
}

// ensureTrustedWorkspace adds workdir to agy's trustedWorkspaces if missing.
// Without this, agy silently produces 0 bytes for untrusted directories.
func ensureTrustedWorkspace(workdir string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	settingsPath := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")

	abs, err := filepath.Abs(workdir)
	if err != nil {
		return
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		data = []byte(`{"trustedWorkspaces":[]}`)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		settings = map[string]any{"trustedWorkspaces": []any{}}
	}

	workspaces, _ := settings["trustedWorkspaces"].([]any)
	for _, w := range workspaces {
		if s, ok := w.(string); ok && s == abs {
			return
		}
	}

	log.Info().Str("workdir", abs).Msg("adding workdir to agy trustedWorkspaces")
	settings["trustedWorkspaces"] = append(workspaces, abs)
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(settingsPath), 0755)
	_ = os.WriteFile(settingsPath, out, 0600)
}
