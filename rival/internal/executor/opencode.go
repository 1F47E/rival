package executor

import (
	"context"
	"fmt"
	"io"
	"os/exec"

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

// opencodeReadOnlyPermission is a read-only permission profile passed to opencode
// via OPENCODE_PERMISSION. A code reviewer reads repo content that may contain
// prompt-injection, so it must NOT be able to write files or run shell commands.
// This mirrors codex's `--sandbox read-only`: read/grep/glob/list are allowed
// (external_directory too — without it opencode auto-rejects reads outside its
// own dir and produces nothing), while edit/bash/task and web access are denied.
const opencodeReadOnlyPermission = `{"read":"allow","grep":"allow","glob":"allow","list":"allow","external_directory":"allow","edit":"deny","bash":"deny","task":"deny","webfetch":"deny","websearch":"deny"}`

// RunOpencode executes a prompt through the opencode CLI running GLM-5.2
// (config.OpencodeModel). opencode reads the prompt from stdin in non-interactive
// `run` mode; the effort is mapped to opencode's --variant (provider-specific
// reasoning level). It runs under a read-only permission profile (see
// opencodeReadOnlyPermission) rather than --dangerously-skip-permissions, so a
// prompt-injected repo cannot make the reviewer write files or run commands.
func RunOpencode(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
	variant := config.OpencodeVariantLevel[effort]
	if variant == "" {
		variant = "high"
	}

	args := []string{
		"run",
		"-m", config.OpencodeModel,
		"--variant", variant,
		"--dir", workdir,
	}
	env := []string{"OPENCODE_PERMISSION=" + opencodeReadOnlyPermission}

	fullPrompt := config.SystemPrompt + "\n\n" + config.BuildWorkdirPreamble(workdir) + "\n" + prompt
	// Drop any inherited OPENCODE_PERMISSION before appending ours. rival loads the
	// reviewed repo's .env (godotenv) into the process env, so a malicious repo
	// could otherwise ship a permissive OPENCODE_PERMISSION in its .env. Duplicate
	// env keys resolve to the LAST value (our read-only one, appended after base),
	// so this is defense-in-depth — but stripping it removes all reliance on env
	// ordering and keeps the child env clean.
	return RunSubprocess(ctx, sess, "opencode", args, env, fullPrompt, mirror, "OPENCODE_PERMISSION")
}
