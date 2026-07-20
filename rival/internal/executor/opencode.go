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

// OpencodePreflight checks that the opencode CLI is installed and, when the
// reviewer roster uses OpenCode Zen models ("opencode/" prefix), that a Zen API
// key is configured. The opencode CLI's own Zen auth resolution is unreliable, so
// rival injects RIVAL_OPENCODE_API_KEY per run — without it every Zen reviewer
// fails mid-run with an opaque "Missing API key". Failing preflight here turns
// that into one clear, actionable skip reason instead.
func OpencodePreflight() error {
	for _, reviewer := range config.OpencodeReviewerList() {
		// The curated roster carries no moonshot models, so the empty workdir
		// (which limits KIMI_API resolution to the process env) is irrelevant.
		if err := OpencodePreflightModel(reviewer.Model, ""); err != nil {
			return err
		}
	}
	return nil
}

// OpencodePreflightModel validates one selected OpenCode model. workdir seeds
// the KIMI_API .env walk-up for moonshot models (see config.KimiAPIKeyFrom);
// pass "" when no workdir context exists.
func OpencodePreflightModel(model, workdir string) error {
	if _, err := exec.LookPath("opencode"); err != nil {
		return fmt.Errorf("opencode CLI not installed. Install: https://opencode.ai (brew install sst/tap/opencode)")
	}
	if strings.HasPrefix(model, "opencode/") && config.OpencodeAPIKey() == "" {
		return fmt.Errorf("OpenCode Zen model %s requires RIVAL_OPENCODE_API_KEY — export your Zen key", model)
	}
	if strings.HasPrefix(model, "moonshot/") && config.KimiAPIKeyFrom(workdir) == "" {
		return fmt.Errorf("model %s requires KIMI_API (Moonshot provider) — add it to the project .env or export it", model)
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

// opencodeFullAutoPermission allows every tool except out-of-workdir native
// reads. Used only by the standalone kimi runner's raw-prompt mode, where the
// user explicitly asked for a full-auto agent that can edit files and run
// commands in the workdir. external_directory is denied to keep the native
// file tools on the documented "in the workdir" promise — bash being allowed
// means this is defense-in-depth, not containment (a shell can read anything
// the user can). Review mode never uses this profile.
const opencodeFullAutoPermission = `{"read":"allow","grep":"allow","glob":"allow","list":"allow","external_directory":"deny","edit":"allow","bash":"allow","task":"allow","webfetch":"allow","websearch":"allow"}`

// OpencodeRunOpts customizes one opencode execution beyond the reviewer
// defaults. Zero values keep megareview behavior exactly: read-only
// permission, RIVAL_OPENCODE_API_KEY provider injection, no extra env drops.
type OpencodeRunOpts struct {
	Permission string   // OPENCODE_PERMISSION JSON; "" = read-only reviewer profile
	APIKey     string   // api key for this model's provider; "" = config.OpencodeAPIKey()
	DropEnv    []string // extra vars/prefixes stripped from the child (see dropMatches)
}

// RunOpencode executes a prompt through the opencode CLI running the given model
// (e.g. "opencode-go/glm-5.2"). opencode reads the prompt from stdin in
// non-interactive `run` mode; the effort is mapped to opencode's --variant
// (provider-specific reasoning level). It runs under a read-only permission
// profile (see opencodeReadOnlyPermission) rather than --dangerously-skip-permissions,
// so a prompt-injected repo cannot make the reviewer write files or run commands.
// An empty model falls back to config.OpencodeModel.
func RunOpencode(ctx context.Context, sess *session.Session, prompt, effort, workdir, model string, mirror io.Writer) (*Result, error) {
	return RunOpencodeWith(ctx, sess, prompt, effort, workdir, model, OpencodeRunOpts{}, mirror)
}

// RunOpencodeWith is RunOpencode with per-call overrides (see OpencodeRunOpts).
// The standalone kimi runner uses it for its full-auto raw mode and its
// moonshot-provider key; megareview reviewers stay on the zero-value defaults.
func RunOpencodeWith(ctx context.Context, sess *session.Session, prompt, effort, workdir, model string, opts OpencodeRunOpts, mirror io.Writer) (*Result, error) {
	if model == "" {
		model = config.OpencodeModel
	}
	args := opencodeRunArgs(model, effort, workdir)
	env := opencodeRunEnvWith(sess.ID, model, workdir, opts)

	fullPrompt := config.SystemPrompt + "\n\n" + config.BuildWorkdirPreamble(workdir) + "\n" + prompt
	// Drop any inherited OPENCODE_PERMISSION / OPENCODE_CONFIG_CONTENT before
	// appending ours. rival loads the reviewed repo's .env (godotenv) into the
	// process env, so a malicious repo could otherwise ship a permissive
	// OPENCODE_PERMISSION or a config that weakens the sandbox. (safeEnv already
	// strips the OPENCODE_ prefix, so this is belt-and-suspenders.)
	drop := append([]string{"OPENCODE_PERMISSION", "OPENCODE_CONFIG_CONTENT", "OPENCODE_DB"}, opts.DropEnv...)
	return RunSubprocess(ctx, sess, "opencode", args, env, fullPrompt, mirror, drop...)
}

func opencodeRunArgs(model, effort, workdir string) []string {
	args := []string{
		"run",
		// --pure runs without external plugins / project-controlled config, so a
		// reviewed repo's own .opencode config can't re-enable denied tools or
		// otherwise weaken the read-only sandbox. (OPENCODE_PERMISSION already
		// wins over project config, but this removes all reliance on that.)
		"--pure",
		"-m", model,
	}
	if variant := config.OpencodeVariant(model, effort); variant != "" {
		args = append(args, "--variant", variant)
	}
	args = append(args, "--dir", workdir)
	return args
}

func opencodeRunEnv(sessionID, model string) []string {
	return opencodeRunEnvWith(sessionID, model, "", OpencodeRunOpts{})
}

func opencodeRunEnvWith(sessionID, model, workdir string, opts OpencodeRunOpts) []string {
	permission := opts.Permission
	if permission == "" {
		permission = opencodeReadOnlyPermission
	}
	env := []string{
		"OPENCODE_PERMISSION=" + permission,
		// Give each reviewer its OWN opencode session DB. The megareview runs
		// several opencode processes at once and they otherwise share one SQLite
		// DB (WAL + 5s busy_timeout), which intermittently loses the write lock —
		// observed as a reviewer failing with "database is locked" (exit 1). A
		// per-session DB (keyed on the unique session ID) removes all contention.
		"OPENCODE_DB=rival-" + sessionID + ".db",
	}

	// Inject an API key into the provider config for THIS model's provider
	// (e.g. "opencode" = Zen, "moonshot" = Moonshot). The opencode CLI's
	// auth.json resolution for the Zen provider is unreliable, but a
	// provider-config override always works. Callers may supply a
	// provider-specific key (kimi → KIMI_API); the default is the
	// rival-managed RIVAL_OPENCODE_API_KEY. Never hardcoded; rides in via
	// OPENCODE_CONFIG_CONTENT.
	key := opts.APIKey
	if key == "" {
		if strings.HasPrefix(model, "moonshot/") {
			// Moonshot models must never receive the Zen key. Without an
			// explicit override, fall back to KIMI_API (process env, then the
			// .env walk-up from workdir) — this is what makes the k3
			// megareview selector work, including from subdirectories.
			key = config.KimiAPIKeyFrom(workdir)
		} else {
			key = config.OpencodeAPIKey()
		}
	}
	if key != "" {
		if cfg := opencodeProviderConfig(model, key); cfg != "" {
			env = append(env, "OPENCODE_CONFIG_CONTENT="+cfg)
		}
	}
	return env
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
