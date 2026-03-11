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

// GeminiPreflight checks that gemini CLI is installed.
func GeminiPreflight() error {
	if _, err := exec.LookPath("gemini"); err != nil {
		return fmt.Errorf("gemini CLI not installed. Install: npm install -g @google/gemini-cli")
	}
	return nil
}

// RunGemini executes a prompt through the Gemini CLI.
func RunGemini(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
	// Create temp dir for GEMINI_HOME.
	tmpDir, err := os.MkdirTemp("", "rival-gemini-*")
	if err != nil {
		return nil, fmt.Errorf("create gemini temp dir: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
			log.Warn().Err(removeErr).Str("dir", tmpDir).Msg("failed to clean up gemini temp dir")
		}
	}()

	// Write settings.json with thinkingLevel config (gen3 only).
	level := config.GeminiThinkingLevel[effort]
	if level == "" {
		level = "MEDIUM"
	}

	settings := map[string]any{
		"modelConfigs": map[string]any{
			"customAliases": map[string]any{
				config.GeminiModel: map[string]any{
					"extends": "chat-base-3",
					"modelConfig": map[string]any{
						"model": config.GeminiModel,
						"generateContentConfig": map[string]any{
							"thinkingConfig": map[string]any{
								"thinkingLevel": level,
							},
						},
					},
				},
			},
		},
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("marshal gemini settings: %w", err)
	}

	settingsPath := filepath.Join(tmpDir, "settings.json")
	if err := os.WriteFile(settingsPath, settingsJSON, 0600); err != nil {
		return nil, fmt.Errorf("write gemini settings: %w", err)
	}

	args := []string{
		"-m", config.GeminiModel,
		"--sandbox",
	}

	env := []string{
		fmt.Sprintf("GEMINI_HOME=%s", tmpDir),
	}

	return RunSubprocess(ctx, sess, "gemini", args, env, prompt, mirror)
}
