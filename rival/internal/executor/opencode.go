package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
)

// OpencodePreflight checks that the opencode CLI is installed.
func OpencodePreflight() error {
	if _, err := exec.LookPath("opencode"); err != nil {
		return fmt.Errorf("opencode CLI not installed. Install: https://opencode.ai (brew install sst/tap/opencode)")
	}
	return nil
}

// opencodeReadOnlyPermission is a read-only, workdir-scoped permission profile
// passed to opencode via OPENCODE_PERMISSION. A code reviewer reads repo content
// that may contain prompt-injection, so it must NOT write files, run shell
// commands, OR read outside the reviewed workdir. read/grep/glob/list are allowed
// (opencode auto-scopes these to the workdir + its own tool-output dirs);
// external_directory is DENIED so a prompt-injected repo can't make the reviewer
// read host secrets (~/.aws/credentials, ~/.ssh, a sibling repo's .env) and
// exfiltrate them through the review output / logs / consilium prompt. edit/bash/
// task and web access are denied. Verified: in-workdir reads work,
// out-of-workdir reads are blocked by the external_directory deny rule.
const opencodeReadOnlyPermission = `{"read":"allow","grep":"allow","glob":"allow","list":"allow","external_directory":"deny","edit":"deny","bash":"deny","task":"deny","webfetch":"deny","websearch":"deny"}`

// RunOpencode executes a prompt through the opencode CLI running the given model
// (e.g. "opencode-go/glm-5.2"). opencode reads the prompt from stdin in
// non-interactive `run` mode; the effort is mapped to opencode's --variant
// (provider-specific reasoning level). It runs under a read-only permission
// profile (see opencodeReadOnlyPermission) rather than --dangerously-skip-permissions,
// so a prompt-injected repo cannot make the reviewer write files or run commands.
// An empty model falls back to config.OpencodeModel.
func RunOpencode(ctx context.Context, sess *session.Session, prompt, effort, workdir, model string, mirror io.Writer) (*Result, error) {
	if model == "" {
		model = config.OpencodeModel
	}
	variant := config.OpencodeVariantLevel[effort]
	if variant == "" {
		variant = "high"
	}

	args := []string{
		"run",
		// --pure runs without external plugins / project-controlled config, so a
		// reviewed repo's own .opencode config can't re-enable denied tools or
		// otherwise weaken the read-only sandbox. (OPENCODE_PERMISSION already
		// wins over project config, but this removes all reliance on that.)
		"--pure",
		"-m", model,
		"--variant", variant,
		"--dir", workdir,
	}
	env := []string{"OPENCODE_PERMISSION=" + opencodeReadOnlyPermission}

	// If a rival-managed opencode API key is set, inject it into the provider
	// config for THIS model's provider (e.g. "opencode" = Zen, "opencode-go" =
	// Go). The opencode CLI's auth.json resolution for the Zen provider is
	// unreliable, but a provider-config override always works. The key comes from
	// RIVAL_OPENCODE_API_KEY (never hardcoded) and rides in via OPENCODE_CONFIG_CONTENT.
	if key := config.OpencodeAPIKey(); key != "" {
		if cfg := opencodeProviderConfig(model, key); cfg != "" {
			env = append(env, "OPENCODE_CONFIG_CONTENT="+cfg)
		}
	}

	fullPrompt := config.SystemPrompt + "\n\n" + config.BuildWorkdirPreamble(workdir) + "\n" + prompt
	// Drop any inherited OPENCODE_PERMISSION / OPENCODE_CONFIG_CONTENT before
	// appending ours. rival loads the reviewed repo's .env (godotenv) into the
	// process env, so a malicious repo could otherwise ship a permissive
	// OPENCODE_PERMISSION or a config that weakens the sandbox. (safeEnv already
	// strips the OPENCODE_ prefix, so this is belt-and-suspenders.)
	return RunSubprocess(ctx, sess, "opencode", args, env, fullPrompt, mirror, "OPENCODE_PERMISSION", "OPENCODE_CONFIG_CONTENT")
}

// opencodeProviderConfig returns an OPENCODE_CONFIG_CONTENT JSON string that sets
// the API key on the provider that serves the given model. The provider id is the
// part of the model before the first "/" (e.g. "opencode-go/glm-5.2" → provider
// "opencode-go"); a model with no "/" defaults to the "opencode" (Zen) provider.
// Returns "" if the model or key is empty.
func opencodeProviderConfig(model, key string) string {
	if model == "" || key == "" {
		return ""
	}
	provider := "opencode"
	if i := strings.Index(model, "/"); i > 0 {
		provider = model[:i]
	}
	cfg := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"provider": map[string]any{
			provider: map[string]any{
				"options": map[string]any{"apiKey": key},
			},
		},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return string(b)
}
