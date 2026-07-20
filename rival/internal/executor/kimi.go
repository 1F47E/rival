package executor

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
)

// KimiPreflight checks that the opencode CLI is installed and a Moonshot API
// key is available (env / cwd .env / workdir .env walk-up — see
// config.KimiAPIKeyFrom). Kimi K3 runs through opencode's moonshot provider
// with the key injected per run via OPENCODE_CONFIG_CONTENT; without it the
// run fails mid-flight with an opaque provider auth error, so preflight turns
// that into one clear hint.
func KimiPreflight(workdir string) error {
	if _, err := exec.LookPath("opencode"); err != nil {
		return fmt.Errorf("opencode CLI not installed. Install: https://opencode.ai (brew install sst/tap/opencode)")
	}
	if config.KimiAPIKeyFrom(workdir) == "" {
		return fmt.Errorf("KIMI_API is not set — add KIMI_API=<moonshot api key> to the project .env (workdir or current directory) or export it")
	}
	return nil
}

// kimiDropEnv lists credential vars stripped from the kimi child in raw
// (full-auto) mode, where every tool call is allowed — a prompt-injected repo
// could otherwise read any inherited secret via `env` and exfiltrate it. The
// opencode child needs none of these; its auth arrives via
// OPENCODE_CONFIG_CONTENT. Applied only in raw mode: review mode runs under
// the read-only profile where bash is denied. This shrinks the blast radius
// but is NOT a sandbox in raw mode: the agent can still run arbitrary
// commands as the user.
var kimiDropEnv = []string{
	"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN",
	"GEMINI_API_KEY", "GOOGLE_API_KEY", "GOOGLE_APPLICATION_CREDENTIALS",
	"RIVAL_OPENCODE_API_KEY", "RIVAL_CLAUDE_TOKEN",
	// Trailing underscore = prefix drop: catches SESSION_TOKEN, PROFILE,
	// WEB_IDENTITY_TOKEN_FILE, container credential URIs — the whole family.
	"AWS_",
	"GITHUB_TOKEN", "GH_TOKEN", "GITLAB_TOKEN",
}

// RunKimi executes a prompt with Kimi K3 through the opencode CLI
// (moonshot/kimi-k3, the first-party Moonshot API, 1M context). The reasoning
// variant is pinned to max by OpencodeVariant — K3 is a thinking-only model
// whose API accepts no other level, so the requested rival effort is ignored.
// Permissions follow the session mode: review runs under the same mechanical
// read-only OPENCODE_PERMISSION profile as the megareview reviewers; raw
// prompts run full-auto (every tool allowed, per the original request) with
// known credential env vars stripped as blast-radius reduction.
func RunKimi(ctx context.Context, sess *session.Session, prompt, workdir string, mirror io.Writer) (*Result, error) {
	return RunOpencodeWith(ctx, sess, prompt, "max", workdir, config.KimiModel, kimiRunOpts(sess.Mode, workdir), mirror)
}

// kimiRunOpts selects the permission profile and env hardening for one run by
// session mode. Review keeps the zero-value read-only reviewer defaults; only
// the API key differs (Moonshot instead of Zen).
func kimiRunOpts(mode, workdir string) OpencodeRunOpts {
	opts := OpencodeRunOpts{APIKey: config.KimiAPIKeyFrom(workdir)}
	if mode != "review" {
		opts.Permission = opencodeFullAutoPermission
		opts.DropEnv = kimiDropEnv
	}
	return opts
}
