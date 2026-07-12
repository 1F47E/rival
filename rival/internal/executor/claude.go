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

// ClaudePreflight checks that claude is available (native or docker).
func ClaudePreflight() error {
	if _, lookErr := exec.LookPath("claude"); lookErr == nil {
		return nil
	}
	return ClaudeDockerPreflight()
}

// RunClaude executes a prompt through Claude CLI (native if available, docker otherwise)
// using the default Claude model.
func RunClaude(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
	return RunClaudeModel(ctx, sess, prompt, effort, workdir, config.ClaudeModel, mirror)
}

// RunFable executes a prompt through the Claude CLI using the Fable model.
// Fable runs through the same `claude` binary (native or docker) and the same
// auth — only the --model string differs.
func RunFable(ctx context.Context, sess *session.Session, prompt, effort, workdir string, mirror io.Writer) (*Result, error) {
	return RunClaudeModel(ctx, sess, prompt, effort, workdir, config.FableModel, mirror)
}

// RunClaudeModel runs a prompt through the Claude CLI with an explicit model id,
// auto-selecting native (claude on PATH) vs docker.
func RunClaudeModel(ctx context.Context, sess *session.Session, prompt, effort, workdir, model string, mirror io.Writer) (*Result, error) {
	var result *Result
	var err error
	if _, lookErr := exec.LookPath("claude"); lookErr == nil {
		setClaudeTransportMode(sess, "native")
		result, err = runClaudeNative(ctx, sess, prompt, effort, workdir, model, mirror)
	} else {
		setClaudeTransportMode(sess, "docker")
		result, err = RunClaudeDocker(ctx, sess, prompt, effort, workdir, model, mirror)
	}
	if err != nil {
		label := config.EngineLabel("claude", model)
		return nil, fmt.Errorf("%s runtime: %s", label, config.PublicRuntimeError("claude", model, err.Error()))
	}
	return result, nil
}

// setClaudeTransportMode records the transport for ordinary model runs while
// preserving a plan session's task identity throughout its live execution.
func setClaudeTransportMode(sess *session.Session, transport string) {
	if sess.Mode != "plan" {
		sess.Mode = transport
	}
}

func runClaudeNative(ctx context.Context, sess *session.Session, prompt, effort, workdir, model string, mirror io.Writer) (*Result, error) {
	auth, err := config.ClaudeAuth()
	if err != nil {
		return nil, err
	}
	sess.Account = auth
	log.Info().Str("session", sess.ID).Str("auth", auth).Str("model", config.EngineLabel("claude", model)).Msg("model native auth mode")

	// Subscription mode: the claude CLI is already authed via /login. An
	// inherited ANTHROPIC_API_KEY would silently win over that login and bill
	// API credits — strip it so billing stays on the subscription.
	var dropEnv []string
	if auth == config.ClaudeAuthSubscription {
		dropEnv = []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"}
	}

	claudeEffort := config.ClaudeEffortLevel[effort]
	if claudeEffort == "" {
		claudeEffort = "max"
	}

	args := []string{
		"-p",
		"--model", model,
		"--effort", claudeEffort,
		"--output-format", "text",
		"--no-session-persistence",
		"--dangerously-skip-permissions",
		"--system-prompt", config.SystemPrompt,
	}

	fullPrompt := config.BuildWorkdirPreamble(workdir) + "\n" + prompt
	return RunSubprocess(ctx, sess, "claude", args, nil, fullPrompt, mirror, dropEnv...)
}

// claudeAuthMarkers are CLI output fragments that indicate an auth/billing
// failure rather than a model failure.
var claudeAuthMarkers = []string{
	"Credit balance is too low",
	"Invalid API key",
	"Please run /login",
	"not logged in",
	"OAuth token has expired",
	"authentication_error",
}

// ClaudeAuthHint inspects a failed native run's log for auth/billing errors
// and returns an actionable, auth-mode-specific explanation ("" if none).
func ClaudeAuthHint(logFile string) string {
	data, err := os.ReadFile(logFile)
	if err != nil {
		return ""
	}
	text := string(data)
	matched := false
	for _, m := range claudeAuthMarkers {
		if strings.Contains(text, m) {
			matched = true
			break
		}
	}
	if !matched {
		return ""
	}
	auth, err := config.ClaudeAuth()
	if err != nil {
		return err.Error()
	}
	if auth == config.ClaudeAuthAPI {
		return "rival: API auth failed (RIVAL_CLAUDE_AUTH=api) — check ANTHROPIC_API_KEY and its credit balance, or unset RIVAL_CLAUDE_AUTH to use the claude CLI subscription login"
	}
	return "rival: subscription auth failed — run `claude` once and /login (Pro/Max), or set RIVAL_CLAUDE_AUTH=api with a funded ANTHROPIC_API_KEY"
}
