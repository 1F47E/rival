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
	GPT56SolModel        = "gpt-5.6-sol"
	CodexModel           = GPT56SolModel // legacy internal alias
	GeminiModel          = "gemini-3.1-pro-preview"
	ClaudeModel          = "claude-opus-4-8[1m]"
	FableModel           = "claude-fable-5"
	SolLabel             = "sol"
	OpusLabel            = "opus"
	FableLabel           = "fable"
	AntigravityModel     = "gemini-3.5-flash"
	OpencodeDeepSeekPro  = "opencode/deepseek-v4-pro"
	OpencodeModel        = OpencodeDeepSeekPro
	OpencodeKimiK27Code  = "opencode/kimi-k2.7-code"
	OpencodeGLMModel     = "opencode/glm-5.2"
	ClaudeDockerImage    = "rival-opus-fable"
	ClaudeDockerTokenEnv = "RIVAL_CLAUDE_TOKEN"

	DefaultEffort              = "xhigh"
	DefaultReviewEffort        = "high"
	DefaultPlanEffort          = "high"
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

// ValidEfforts includes xhigh for compatibility with existing commands and
// saved invocations. Review and plan help intentionally advertises the simpler
// low/medium/high/ultra ladder, with high as the default.
var ValidEfforts = []string{"low", "medium", "high", "xhigh"}

// ReviewEfforts is the public effort ladder shown by code-review and
// plan-review commands. xhigh remains accepted as a compatibility alias.
var ReviewEfforts = []string{"low", "medium", "high", "ultra"}

// ClaudeEffortLevel maps rival effort levels to claude CLI --effort values.
var ClaudeEffortLevel = map[string]string{
	"low":    "low",
	"medium": "medium",
	"high":   "max",
	"xhigh":  "max",
	"ultra":  "max",
}

// OpencodeVariant returns the provider-supported reasoning variant for a
// curated model and Rival effort. Kimi K2.7 Code currently advertises no named
// variants, so it must be launched without --variant. GLM exposes only high/max;
// DeepSeek exposes the full low/medium/high/max ladder.
func OpencodeVariant(model, effort string) string {
	switch model {
	case OpencodeKimiK27Code:
		return ""
	case OpencodeGLMModel:
		if effort == "xhigh" || effort == "ultra" {
			return "max"
		}
		return "high"
	default: // DeepSeek V4 Pro and the generic OpenCode fallback.
		switch effort {
		case "low", "medium", "high":
			return effort
		case "xhigh", "ultra":
			return "max"
		default:
			return "high"
		}
	}
}

// ModelLabel returns the stable public name for a concrete model id. Runtime
// model ids stay internal so dashboards, console output, and API summaries use
// Rival's short model names consistently.
func ModelLabel(model string) string {
	switch model {
	case GPT56SolModel:
		return SolLabel
	case ClaudeModel:
		return OpusLabel
	case FableModel:
		return FableLabel
	default:
		return OpencodeShortLabel(model)
	}
}

// EngineLabel returns a human-facing reviewer label. Review output names the
// selected model instead of the executable adapter used to launch it.
func EngineLabel(cli, model string) string {
	// Exact current ids win first because one shared adapter can launch both
	// Opus and Fable.
	switch model {
	case GPT56SolModel:
		return SolLabel
	case ClaudeModel:
		return OpusLabel
	case FableModel:
		return FableLabel
	}

	// Adapter identity is the reliable fallback for sessions written by older
	// releases with now-obsolete model ids.
	switch cli {
	case "codex":
		return SolLabel
	case "claude":
		if strings.Contains(strings.ToLower(model), FableLabel) {
			return FableLabel
		}
		return OpusLabel
	case "fable":
		return FableLabel
	}
	if model != "" {
		return ModelLabel(model)
	}
	return cli
}

// PublicRuntimeError removes internal adapter and concrete model identifiers
// from an error before it is shown to a user. Required executable paths and
// configuration keys remain untouched.
func PublicRuntimeError(cli, model, message string) string {
	message = replaceConcreteModelIDs(cli, model, message)
	label := EngineLabel(cli, model)
	switch cli {
	case "codex":
		return strings.NewReplacer(
			"OpenAI Codex", "Sol runtime",
			"Codex CLI", "Sol runtime",
			"codex CLI", "Sol runtime",
			"run codex login", "authenticate the Sol runtime",
			"codex exited", "sol exited",
			"start codex:", "start Sol runtime:",
			"subprocess codex:", "Sol runtime:",
			"Codex", "Sol",
		).Replace(message)
	case "claude", "fable":
		title := strings.ToUpper(label[:1]) + label[1:]
		return strings.NewReplacer(
			"Claude Code CLI", title+" runtime",
			"Claude CLI", title+" runtime",
			"claude CLI", label+" runtime",
			"claude requires Docker", title+" runtime requires Docker",
			"claude exited", label+" exited",
			"rival-claude", "rival-opus-fable",
			"start claude:", "start "+title+" runtime:",
			"subprocess claude:", title+" runtime:",
		).Replace(message)
	default:
		return message
	}
}

// PublicRuntimeLog normalizes runtime banners and concrete model ids while
// preserving model output, including any source paths that contain an adapter
// name. Persisted logs stay lossless; every user-facing log reader calls this.
func PublicRuntimeLog(cli, model, raw string) string {
	if raw == "" {
		return raw
	}
	raw = replaceConcreteModelIDs(cli, model, raw)
	label := EngineLabel(cli, model)
	title := label
	if title != "" {
		title = strings.ToUpper(title[:1]) + title[1:]
	}

	lines := strings.SplitAfter(raw, "\n")
	bannerSeen := false
	headerOpen := true
	delimiters := 0
	for i, line := range lines {
		ending := ""
		body := line
		if strings.HasSuffix(body, "\n") {
			body = strings.TrimSuffix(body, "\n")
			ending = "\n"
		}
		trimmed := strings.TrimSpace(body)
		leading := body[:len(body)-len(strings.TrimLeft(body, " \t"))]

		switch cli {
		case "codex":
			if strings.HasPrefix(trimmed, "OpenAI Codex") {
				trimmed = "Sol runtime" + strings.TrimPrefix(trimmed, "OpenAI Codex")
				body = leading + trimmed
				bannerSeen = true
			} else if i == 0 && strings.HasPrefix(trimmed, "Codex ") {
				trimmed = "Sol runtime " + strings.TrimPrefix(trimmed, "Codex ")
				body = leading + trimmed
				bannerSeen = true
			}
		case "claude", "fable":
			if strings.HasPrefix(trimmed, "Claude Code") {
				trimmed = title + " runtime" + strings.TrimPrefix(trimmed, "Claude Code")
				body = leading + trimmed
				bannerSeen = true
			} else if i == 0 && strings.HasPrefix(trimmed, "Claude ") {
				trimmed = title + " runtime " + strings.TrimPrefix(trimmed, "Claude ")
				body = leading + trimmed
				bannerSeen = true
			}
		}

		if bannerSeen && headerOpen && strings.HasPrefix(strings.ToLower(trimmed), "model:") {
			body = leading + "model: " + label
		}
		if strings.HasPrefix(trimmed, "=== REVIEW FROM ") {
			body = leading + publicReviewHeader(trimmed)
		}

		if trimmed == "--------" {
			delimiters++
			if delimiters >= 2 {
				headerOpen = false
			}
		}
		if strings.EqualFold(trimmed, "user") {
			headerOpen = false
		}
		lines[i] = body + ending
	}
	return strings.Join(lines, "")
}

func replaceConcreteModelIDs(cli, model, text string) string {
	if model != "" {
		text = strings.ReplaceAll(text, model, EngineLabel(cli, model))
	}
	text = strings.ReplaceAll(text, GPT56SolModel, SolLabel)
	text = strings.ReplaceAll(text, ClaudeModel, OpusLabel)
	text = strings.ReplaceAll(text, FableModel, FableLabel)
	return text
}

func publicReviewHeader(line string) string {
	const prefix = "=== REVIEW FROM "
	rest := strings.TrimPrefix(line, prefix)
	roleAt := strings.Index(rest, " [role:")
	if roleAt < 0 {
		return line
	}
	identity := rest[:roleAt]
	role := rest[roleAt:]
	fields := strings.Fields(identity)
	if len(fields) == 0 {
		return line
	}
	reviewer := strings.Trim(fields[0], "()")
	lowerIdentity := strings.ToLower(identity)
	switch strings.ToLower(reviewer) {
	case "codex":
		reviewer = SolLabel
	case "claude":
		if strings.Contains(lowerIdentity, FableLabel) {
			reviewer = FableLabel
		} else {
			reviewer = OpusLabel
		}
	case GPT56SolModel:
		reviewer = SolLabel
	case ClaudeModel:
		reviewer = OpusLabel
	case FableModel:
		reviewer = FableLabel
	}
	return prefix + reviewer + role
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

// defaultOpencodeReviewers is the intentionally curated three-model roster,
// ordered by judge preference. Each model gets a distinct review lens.
var defaultOpencodeReviewers = []OpencodeReviewer{
	{Model: OpencodeDeepSeekPro, Role: "bug_hunter"},
	{Model: OpencodeKimiK27Code, Role: "arch_security"},
	{Model: OpencodeGLMModel, Role: "code_quality"},
}

// ReviewTarget is one concrete reviewer selected for a megareview run. CLI is
// the internal executable adapter, Model is the concrete model id, and Role
// controls the review lens. User-facing output always uses Model.
type ReviewTarget struct {
	CLI   string
	Model string
	Role  string
}

// DefaultReviewTargets returns the curated four-model megareview roster. The
// order is also the consilium judge preference order.
func DefaultReviewTargets() []ReviewTarget {
	targets := []ReviewTarget{{CLI: "codex", Model: GPT56SolModel, Role: "bug_hunter"}}
	for _, reviewer := range OpencodeReviewerList() {
		targets = append(targets, ReviewTarget{CLI: "opencode", Model: reviewer.Model, Role: reviewer.Role})
	}
	return targets
}

// ResolveReviewTargets resolves per-invocation model selectors to an exact,
// ordered reviewer roster. With no selectors the curated default roster is
// returned. With selectors, ONLY the named reviewers are returned. Selectors
// may be repeated or comma-separated.
//
// Friendly aliases:
//   - sol (the exact runtime model id remains accepted for compatibility)
//   - deepseek, deepseek-pro, deepseek-v4-pro
//   - kimi, kimi-code, kimi-k2.7-code
//   - glm, glm-5.2
//
// Per-run selection intentionally stays on this curated set.
func ResolveReviewTargets(selectors []string) ([]ReviewTarget, error) {
	var flat []string
	for _, value := range selectors {
		for _, selector := range strings.Split(value, ",") {
			selector = strings.TrimSpace(selector)
			if selector == "" {
				return nil, fmt.Errorf("model selector cannot be empty")
			}
			flat = append(flat, selector)
		}
	}
	if len(flat) == 0 {
		return DefaultReviewTargets(), nil
	}

	var targets []ReviewTarget
	seen := map[string]bool{}
	appendTarget := func(target ReviewTarget) {
		key := target.CLI + "\x00" + target.Model
		if seen[key] {
			return
		}
		seen[key] = true
		targets = append(targets, target)
	}

	for _, raw := range flat {
		alias := strings.ToLower(strings.TrimSpace(raw))

		var expanded []ReviewTarget
		switch alias {
		case SolLabel, GPT56SolModel:
			expanded = []ReviewTarget{{CLI: "codex", Model: GPT56SolModel, Role: "bug_hunter"}}
		case "deepseek", "deepseek-pro", "deepseek-v4-pro":
			expanded = []ReviewTarget{{CLI: "opencode", Model: OpencodeDeepSeekPro, Role: "bug_hunter"}}
		case "kimi", "kimi-code", "kimi-k2.7", "kimi-k2.7-code":
			expanded = []ReviewTarget{{CLI: "opencode", Model: OpencodeKimiK27Code, Role: "arch_security"}}
		case "glm", "glm-5.2":
			expanded = []ReviewTarget{{CLI: "opencode", Model: OpencodeGLMModel, Role: "code_quality"}}
		default:
			return nil, fmt.Errorf("unknown review model %q; use one of: sol, deepseek-v4-pro, kimi-k2.7-code, glm-5.2", raw)
		}
		for _, target := range expanded {
			appendTarget(target)
		}
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("no review models selected")
	}
	return targets, nil
}

// OpencodeAPIKey returns the required OpenCode Zen API key that Rival injects
// into reviewer runs via OPENCODE_CONFIG_CONTENT. The key is read only from
// RIVAL_OPENCODE_API_KEY, never from a reviewed repository.
func OpencodeAPIKey() string {
	return strings.TrimSpace(os.Getenv("RIVAL_OPENCODE_API_KEY"))
}

// OpencodeReviewerList returns a copy of the curated roster. Per-run selection
// is handled by ResolveReviewTargets rather than process-wide environment state.
func OpencodeReviewerList() []OpencodeReviewer {
	return append([]OpencodeReviewer(nil), defaultOpencodeReviewers...)
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
	"ultra":  "HIGH",
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

// IsValidReviewEffort validates review and plan effort values. It intentionally
// accepts xhigh so existing invocations keep working even though new help text
// presents ultra as the top-level choice.
func IsValidReviewEffort(e string) bool {
	return IsValidEffort(e) || e == "ultra"
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
