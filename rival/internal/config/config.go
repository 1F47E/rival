package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	CodexModel           = "gpt-5.5"
	GeminiModel          = "gemini-3.1-pro-preview"
	ClaudeModel          = "claude-opus-4-8[1m]"
	FableModel           = "claude-fable-5"
	AntigravityModel     = "gemini-3.5-flash"
	OpencodeModel        = "opencode/glm-5.2"
	ClaudeDockerImage    = "rival-claude"
	ClaudeDockerTokenEnv = "RIVAL_CLAUDE_TOKEN"

	DefaultEffort              = "xhigh"
	DefaultConfidenceThreshold = 6
	SessionDir                 = ".rival/sessions"
	QueueDir                   = ".rival/queue"
	PromptPreviewLen           = 100
	PromptDetailMaxLines       = 10

	DefaultMaxConcurrent = 1
	DefaultQueueTimeout  = 30 * time.Minute
	DefaultRunTimeout    = 30 * time.Minute
	QueuePollInterval    = 2 * time.Second
)

var ValidEfforts = []string{"low", "medium", "high", "xhigh"}

// ClaudeEffortLevel maps rival effort levels to claude CLI --effort values.
var ClaudeEffortLevel = map[string]string{
	"low":    "low",
	"medium": "medium",
	"high":   "max",
	"xhigh":  "max",
}

// OpencodeVariantLevel maps rival effort levels to opencode's --variant
// (provider-specific reasoning level: minimal | high | max).
var OpencodeVariantLevel = map[string]string{
	"low":    "minimal",
	"medium": "minimal",
	"high":   "high",
	"xhigh":  "max",
}

// EngineLabel returns a short human label for a reviewer engine, given its cli
// and model. opencode backs several models under the single cli "opencode", so
// its label comes from the model (the provider prefix is stripped, e.g.
// "opencode-go/glm-5.2" → "glm-5.2"). Fable (cli "claude"/"fable", model
// claude-fable-5) shows as "claude-fable". Everything else is just the cli.
func EngineLabel(cli, model string) string {
	switch {
	case cli == "opencode":
		return OpencodeShortLabel(model)
	case model == FableModel:
		return "claude-fable"
	default:
		return cli
	}
}

// OpencodeShortLabel strips the opencode provider prefix from a model id for
// display, e.g. "opencode-go/glm-5.2" → "glm-5.2", "opencode/deepseek-v4-pro" →
// "deepseek-v4-pro". An empty model falls back to "opencode".
func OpencodeShortLabel(model string) string {
	if model == "" {
		return "opencode"
	}
	if i := strings.LastIndex(model, "/"); i >= 0 && i+1 < len(model) {
		return model[i+1:]
	}
	return model
}

// OpencodeReviewer is one opencode-provided model run as a megareview reviewer.
// Model is the opencode model id (e.g. "opencode-go/glm-5.2"); Role is the
// review lens (a review.Role string) so the roster can diversify coverage across
// the models.
type OpencodeReviewer struct {
	Model string
	Role  string
}

// defaultOpencodeReviewers is the built-in opencode reviewer roster: three
// models via the OpenCode Zen provider (the "opencode/" prefix), each with a
// distinct role so they don't all hunt the same class of issue. Overridable via
// RIVAL_OPENCODE_MODELS. Zen billing uses RIVAL_OPENCODE_API_KEY (see
// OpencodeAPIKey); without a key opencode falls back to its own stored auth.
var defaultOpencodeReviewers = []OpencodeReviewer{
	{Model: "opencode/glm-5.2", Role: "arch_security"},
	{Model: "opencode/deepseek-v4-pro", Role: "bug_hunter"},
	{Model: "opencode/deepseek-v4-flash", Role: "code_quality"},
}

// OpencodeAPIKey returns the API key rival injects into the opencode provider
// config for reviewer runs (via OPENCODE_CONFIG_CONTENT), from
// RIVAL_OPENCODE_API_KEY. Empty means "let the opencode CLI use its own stored
// credential". The key is NEVER read from a repo file — only this env var — so a
// reviewed repo cannot supply it.
func OpencodeAPIKey() string {
	return strings.TrimSpace(os.Getenv("RIVAL_OPENCODE_API_KEY"))
}

// validReviewerRoles are the roles BuildRolePrompt has instructions for. A role
// from RIVAL_OPENCODE_MODELS that isn't one of these would build a reviewer
// prompt with no role instructions, so unknown roles fall back to bug_hunter.
var validReviewerRoles = map[string]bool{
	"bug_hunter":    true,
	"arch_security": true,
	"code_quality":  true,
}

// validReviewerRole returns role if it is a known reviewer role, else
// "bug_hunter". (A user-configured role-prompt override in ~/.rival/config.yaml
// is honoured at prompt-build time regardless, so a custom role keyed there still
// works even though this normalizes the roster default.)
func validReviewerRole(role string) string {
	if validReviewerRoles[role] {
		return role
	}
	if _, ok := RolePromptOverride(role); ok {
		return role
	}
	return "bug_hunter"
}

// OpencodeReviewerList returns the opencode reviewer roster for megareview.
// RIVAL_OPENCODE_MODELS overrides the default: a comma-separated list of entries
// `model[:role]` (role defaults to "code_quality" when omitted; unknown roles are
// kept as-is and fall back to bug_hunter at prompt-build time). Duplicate models
// are dropped, preserving first-seen order. An empty/whitespace override yields
// the default roster (not an empty one), so a stray env value never disables
// opencode entirely.
func OpencodeReviewerList() []OpencodeReviewer {
	raw := strings.TrimSpace(os.Getenv("RIVAL_OPENCODE_MODELS"))
	if raw == "" {
		return defaultOpencodeReviewers
	}
	var out []OpencodeReviewer
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		model, role := part, "code_quality"
		// Split off an optional trailing :role. The model id itself contains a
		// slash (opencode-go/glm-5.2) but no colon, so the LAST colon (if any)
		// separates the role.
		if i := strings.LastIndex(part, ":"); i >= 0 {
			model = strings.TrimSpace(part[:i])
			if r := strings.TrimSpace(part[i+1:]); r != "" {
				role = r
			}
		}
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		out = append(out, OpencodeReviewer{Model: model, Role: validReviewerRole(role)})
	}
	if len(out) == 0 {
		return defaultOpencodeReviewers
	}
	return out
}

// SystemPrompt is prepended as a system instruction to all CLI invocations.
const SystemPrompt = `Answer the user's question directly. Do not offer follow-up options, menus, walkthroughs, or ask if they want more. No filler, no sign-offs. Just deliver the answer and stop.`

// WorkdirPreamble tells the CLI which project directory it's operating in.
const WorkdirPreamble = `You are working in project directory: {WORKDIR}
Use your tools to read files, run git commands, and explore the codebase as needed.
`

// BuildWorkdirPreamble returns the workdir preamble with the absolute path injected.
func BuildWorkdirPreamble(workdir string) string {
	abs, _ := filepath.Abs(workdir)
	return strings.ReplaceAll(WorkdirPreamble, "{WORKDIR}", abs)
}

// Gen3 only — thinkingLevel mapping.
var GeminiThinkingLevel = map[string]string{
	"low":    "LOW",
	"medium": "MEDIUM",
	"high":   "HIGH",
	"xhigh":  "HIGH",
}

// DiffReviewPreamble is prepended to ReviewPrompt when git auto-detects changed files.
// {FILES} is replaced with the newline-separated file list at runtime.
const DiffReviewPreamble = `The following files have uncommitted changes (or were changed in the last commit). Focus your review on these files, but read other project files as needed for context.

Changed files:
` + "```" + `
{FILES}
` + "```" + `
{DIFFSTAT}
`

// ReviewPrompt is the language-agnostic review template. {SCOPE} is replaced at runtime.
const ReviewPrompt = `You are a ruthless senior staff engineer doing a code review. Your job is to find real problems — not nitpick style.

Review scope: {SCOPE}

Read the code in the review scope. Then produce a review covering:

1. **Critical bugs** — logic errors, race conditions, data loss risks, unhandled edge cases
2. **Security vulnerabilities** — injection, auth bypass, secret exposure, SSRF, path traversal
3. **Architecture issues** — tight coupling, missing abstractions, scalability bottlenecks
4. **Performance problems** — N+1 queries, unnecessary allocations, missing indexes, blocking I/O
5. **Error handling gaps** — swallowed errors, missing retries, unclear failure modes

Rules:
- Only report issues you are confident about. No speculative nitpicks.
- For each issue: file path, line number (or range), severity (CRITICAL/HIGH/MEDIUM), one-line description, and a concrete fix suggestion.
- Group by severity, highest first.
- If the code is solid, say so briefly. Do not invent problems.
- Skip style, formatting, naming, and documentation unless they mask a real bug.`

// PlanReviewPrompt is the plan/spec review template used by `rival command plan`.
// It targets a single planning/spec markdown document (NOT source code) and asks
// codex to rate it and surface bugs + gaps. {FILE} is replaced with the absolute
// path at the call site. The model must emit ONE JSON object matching the contract
// below so the output can be parsed structurally (see review.ParsePlanOutput).
const PlanReviewPrompt = `You are a ruthless senior staff engineer reviewing an engineering PLAN / SPEC document (not source code). Your job is to find real problems that would make this plan fail, mislead an implementer, or ship the wrong thing — not to nitpick wording.

Plan document to review: {FILE}

Read the file in full (use your tools). Judge it as an implementation blueprint. Look for:

1. **Bugs / logic flaws** — steps that are wrong, contradictory, out of order, or that would break when implemented as written.
2. **Gaps** — missing steps, unhandled edge cases, undefined error/failure behavior, absent rollback/migration/auth/validation, things the plan silently assumes.
3. **Ambiguity** — instructions vague enough that two engineers would build different things; unstated assumptions; undefined terms.
4. **Scope / feasibility** — unrealistic claims, hidden dependencies, under-estimated work, or parts that conflict with how the rest of the system (as described) works.
5. **Verification gaps** — no way to tell if the plan succeeded; missing tests, acceptance criteria, or rollback checks.

Rules:
- Only report issues you are confident are real. No speculative nitpicks, no style/grammar comments.
- If the plan is genuinely solid, say so in the summary and return few or zero findings. Do not invent problems.
- Rate the plan overall from 1 (unimplementable / dangerously wrong) to 10 (airtight, ready to execute).

Output: respond with EXACTLY ONE JSON object and nothing else (no prose before or after, no markdown fences). Schema:

{
  "summary": "1-3 sentence overall assessment of the plan",
  "rating": 7,
  "findings": [
    {
      "file": "section or heading the issue is in (or the filename)",
      "line": 0,
      "severity": "critical|high|medium|low",
      "category": "bug|gap|ambiguity|scope|verification",
      "title": "one-line description of the issue",
      "body": "what is wrong and why it matters for implementation",
      "suggestion": "concrete fix or what to add",
      "confidence": 8
    }
  ]
}

Severity guidance: critical = plan is wrong/will cause data loss or a broken build if followed; high = significant gap or flaw that blocks correct implementation; medium = real ambiguity or missing detail an implementer will trip on; low = minor gap or clarification. "line" may be 0 when not applicable. Sort findings by severity, highest first.`

// IsValidEffort checks if the given effort level is in the allowlist.
func IsValidEffort(e string) bool {
	for _, v := range ValidEfforts {
		if v == e {
			return true
		}
	}
	return false
}

// SessionDirPath returns the absolute path to ~/.rival/sessions.
func SessionDirPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", SessionDir)
	}
	return filepath.Join(home, SessionDir)
}

// QueueDirPath returns the absolute path to ~/.rival/queue.
func QueueDirPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", QueueDir)
	}
	return filepath.Join(home, QueueDir)
}

// Claude auth modes for native runs (RIVAL_CLAUDE_AUTH).
const (
	ClaudeAuthSubscription = "subscription" // CLI's own /login (Pro/Max) — the default
	ClaudeAuthAPI          = "api"          // explicit ANTHROPIC_API_KEY billing
)

// ClaudeAuth returns the auth mode for native claude/fable runs.
// Default is subscription: the claude CLI is already authed via /login, and an
// inherited ANTHROPIC_API_KEY must never silently switch billing to API
// credits. API billing is opt-in via RIVAL_CLAUDE_AUTH=api and then requires
// ANTHROPIC_API_KEY to be set. Any other value is a hard error — auth must be
// explicit, never guessed.
func ClaudeAuth() (string, error) {
	switch v := os.Getenv("RIVAL_CLAUDE_AUTH"); v {
	case "", ClaudeAuthSubscription, "sub":
		return ClaudeAuthSubscription, nil
	case ClaudeAuthAPI:
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			return "", fmt.Errorf("RIVAL_CLAUDE_AUTH=api but ANTHROPIC_API_KEY is empty — set the key or unset RIVAL_CLAUDE_AUTH to use the claude CLI subscription login")
		}
		return ClaudeAuthAPI, nil
	default:
		return "", fmt.Errorf("invalid RIVAL_CLAUDE_AUTH=%q — use %q (default) or %q", v, ClaudeAuthSubscription, ClaudeAuthAPI)
	}
}

// MaxConcurrent returns how many reviews may run at once (RIVAL_MAX_CONCURRENT, default 1).
func MaxConcurrent() int {
	if v := os.Getenv("RIVAL_MAX_CONCURRENT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return DefaultMaxConcurrent
}

// QueueTimeout returns the max time to wait for a queue slot (RIVAL_QUEUE_TIMEOUT, default 30m).
func QueueTimeout() time.Duration {
	if v := os.Getenv("RIVAL_QUEUE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return DefaultQueueTimeout
}

// MaxRunWait returns a safe upper bound on how long a detached run can legitimately
// take end-to-end: the full queue wait plus the worst-case run budget (megareview
// runs two phases, so 2× RunTimeout), plus a small margin for process startup,
// stdout flush, and reaper cycles. `rival wait` uses this as its default timeout
// so it never gives up on a run that is still within its configured limits.
// When RunTimeout is disabled (0), only the queue wait + margin is bounded.
func MaxRunWait() time.Duration {
	margin := 5 * time.Minute
	return QueueTimeout() + 2*RunTimeout() + margin
}

// RunTimeout returns the max wall-clock a single provider run may take once it
// holds a queue slot (RIVAL_RUN_TIMEOUT, default 30m). This is the hard
// guarantee that a detached rival always terminates even if the provider CLI
// hangs. The clock starts after slot promotion, so queue wait does not eat it.
// Set RIVAL_RUN_TIMEOUT=0 to disable (no timeout — returns 0); an unset or
// unparseable value falls back to the default.
func RunTimeout() time.Duration {
	v := os.Getenv("RIVAL_RUN_TIMEOUT")
	if v == "" {
		return DefaultRunTimeout
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return DefaultRunTimeout
	}
	if d < 0 {
		return DefaultRunTimeout
	}
	return d // d == 0 → caller treats as "no timeout"
}

// WithRunTimeout derives a context bounded by mult×RunTimeout(). mult scales the
// budget for multi-phase pipelines (e.g. megareview = 2: reviewers + judge).
// When RunTimeout() is 0 (disabled) it returns ctx with a no-op cancel.
func WithRunTimeout(ctx context.Context, mult int) (context.Context, context.CancelFunc) {
	d := RunTimeout()
	if d <= 0 || mult <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, time.Duration(mult)*d)
}

// QueueDisabled reports whether queueing is bypassed via RIVAL_NO_QUEUE.
func QueueDisabled() bool {
	v := os.Getenv("RIVAL_NO_QUEUE")
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// ClaudeConfig holds claude-specific settings.
type ClaudeConfig struct {
	Subscription string `yaml:"subscription"` // "team" or "personal"
}

// UserConfig holds optional user configuration from ~/.rival/config.yaml.
type UserConfig struct {
	Claude ClaudeConfig      `yaml:"claude"`
	Roles  map[string]string `yaml:"roles"`
}

var userConfig *UserConfig

// LoadUserConfig reads ~/.rival/config.yaml if it exists.
func LoadUserConfig() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".rival", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var cfg UserConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return
	}
	userConfig = &cfg
}

// RolePromptOverride returns the user-configured prompt for a role, if any.
func RolePromptOverride(role string) (string, bool) {
	if userConfig == nil {
		return "", false
	}
	v, ok := userConfig.Roles[role]
	return v, ok
}

// ClaudeSubscription returns the configured subscription type ("team", "personal", or "").
func ClaudeSubscription() string {
	if userConfig == nil {
		return ""
	}
	return userConfig.Claude.Subscription
}

func init() {
	LoadUserConfig()
}
