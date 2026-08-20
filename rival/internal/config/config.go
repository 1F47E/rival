package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

const (
	GPT56SolModel = "gpt-5.6-sol"
	CodexModel    = GPT56SolModel // legacy internal alias
	FableModel    = "claude-fable-5"
	SolLabel      = "sol"
	FableLabel    = "fable"
	K3Label       = "kimi-k3"
	// K3CommandName is the cobra command word for K3. It differs from
	// K3Label, which is the public display and error name.
	K3CommandName        = "k3"
	KimiModel            = "moonshotai/kimi-k3" // Kimi K3 via OpenCode's built-in Moonshot AI provider
	GrokModel            = "grok-4.5"
	GrokLabel            = "grok"
	ClaudeDockerImage    = "rival-fable"
	ClaudeDockerTokenEnv = "RIVAL_CLAUDE_TOKEN"

	DefaultReviewEffort        = "high"
	DefaultPlanEffort          = "high"
	DefaultAntislopEffort      = "xhigh"
	DefaultConfidenceThreshold = 6
	SessionDir                 = ".rival/sessions"
	QueueDir                   = ".rival/queue"
	PromptPreviewLen           = 100
	PromptDetailMaxLines       = 10

	DefaultMaxConcurrent = 2
	DefaultQueueTimeout  = 30 * time.Minute
	DefaultRunTimeout    = 30 * time.Minute
	QueuePollInterval    = 2 * time.Second
)

// ValidEfforts is the one effort ladder every surface accepts and advertises.
//
// xhigh and ultra are NOT interchangeable: the codex runtime passes the value
// through verbatim and Sol treats ultra as its own reasoning level, so neither
// may be aliased to the other. Runtimes that expose a shorter menu clamp at
// their own boundary (see ClaudeEffortLevel and GrokEffort).
var ValidEfforts = []string{"low", "medium", "high", "xhigh", "ultra"}

// ClaudeEffortLevel maps rival effort levels to claude CLI --effort values.
var ClaudeEffortLevel = map[string]string{
	"low":    "low",
	"medium": "medium",
	"high":   "max",
	"xhigh":  "max",
	"ultra":  "max",
}

// OpencodeVariant returns K3's only provider-supported reasoning variant.
// Unknown models have no supported variant because K3 is Rival's sole
// OpenCode-backed model.
func OpencodeVariant(model, _ string) string {
	if model != KimiModel {
		return ""
	}
	return "max"
}

// ModelLabel returns the stable public name for a concrete model id. Runtime
// model ids stay internal so dashboards, console output, and API summaries use
// Rival's short model names consistently.
func ModelLabel(model string) string {
	switch model {
	case GPT56SolModel, SolLabel:
		return SolLabel
	case FableModel, FableLabel:
		return FableLabel
	case KimiModel, K3Label:
		return K3Label
	case GrokModel, GrokLabel:
		return GrokLabel
	default:
		return "retired-model"
	}
}

// EngineLabel returns a human-facing reviewer label. Review output names the
// selected model instead of the executable adapter used to launch it.
func EngineLabel(cli, model string) string {
	// Exact current ids win first.
	switch model {
	case GPT56SolModel:
		return SolLabel
	case FableModel:
		return FableLabel
	case KimiModel:
		return K3Label
	case GrokModel:
		return GrokLabel
	}

	// Adapter identity is the reliable fallback for sessions written by older
	// releases with now-obsolete model ids.
	switch cli {
	case "codex":
		return SolLabel
	case GrokLabel:
		return GrokLabel
	case "claude", "fable", "opencode":
		return "retired-model"
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
			"rival-claude", "rival-fable",
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
	text = strings.ReplaceAll(text, FableModel, FableLabel)
	text = strings.ReplaceAll(text, KimiModel, K3Label)
	text = strings.ReplaceAll(text, GrokModel, GrokLabel)
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
	if strings.Contains(lowerIdentity, "retired-model") {
		return prefix + "retired-model" + role
	}
	switch strings.ToLower(reviewer) {
	case "codex":
		reviewer = SolLabel
	case "claude":
		if strings.Contains(lowerIdentity, FableLabel) {
			reviewer = FableLabel
		} else {
			reviewer = "retired-model"
		}
	case "opencode":
		if strings.Contains(lowerIdentity, K3Label) {
			reviewer = K3Label
		} else {
			reviewer = "retired-model"
		}
	case GPT56SolModel:
		reviewer = SolLabel
	case FableModel:
		reviewer = FableLabel
	case KimiModel:
		reviewer = K3Label
	case GrokLabel, GrokModel:
		reviewer = GrokLabel
	}
	return prefix + reviewer + role
}

// ReviewTarget is one concrete reviewer selected for a megareview run. CLI is
// the internal executable adapter and Model is the concrete model id.
// User-facing output always uses Model.
type ReviewTarget struct {
	CLI   string
	Model string
	// Prompt selects the lens this reviewer runs. The zero value is the bug
	// hunter, so only targets that need a different lens set it.
	Prompt PromptKind
}

// DefaultReviewTargets returns the curated two-model megareview roster. The
// order is also the consilium judge preference order.
func DefaultReviewTargets() []ReviewTarget {
	// K3 left the default roster on 2026-08-14. It stays selectable with
	// -m k3, where it always carries the security lens.
	return []ReviewTarget{
		{CLI: "codex", Model: GPT56SolModel},
	}
}

// ResolveReviewTargets resolves per-invocation model selectors to an exact,
// ordered reviewer roster. With no selectors the curated default roster is
// returned. With selectors, ONLY the named reviewers are returned. Selectors
// may be repeated or comma-separated.
//
// Friendly aliases:
//   - sol (the exact runtime model id remains accepted for compatibility)
//   - k3, kimi-k3
//   - grok (opt-in; absent from the default roster)
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
			expanded = []ReviewTarget{{CLI: "codex", Model: GPT56SolModel}}
		case "k3", "kimi-k3":
			// K3 only ever runs the security lens, in any roster.
			// Kimi K3 runs through the Moonshot AI provider and needs its API key.
			expanded = []ReviewTarget{{CLI: "opencode", Model: KimiModel, Prompt: PromptSecurity}}
		case GrokLabel:
			// Opt-in only: grok never joins the default roster.
			expanded = []ReviewTarget{{CLI: GrokLabel, Model: GrokModel}}
		default:
			return nil, fmt.Errorf("unknown review model %q; use one of: sol, kimi-k3, grok", raw)
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

// KimiAPIKeyFrom returns the Moonshot AI API key for K3 runs.
// MOONSHOT_API_KEY is the canonical OpenCode variable; KIMI_API remains a
// backward-compatible alias. Rival checks the process environment first, then
// walks up from workdir looking for either entry in a project .env. The key is
// injected per run into OpenCode's built-in moonshotai provider through
// OPENCODE_CONFIG_CONTENT and is never written to on-disk OpenCode config.
func KimiAPIKeyFrom(workdir string) string {
	for _, name := range []string{"MOONSHOT_API_KEY", "KIMI_API"} {
		if key := strings.TrimSpace(os.Getenv(name)); key != "" {
			return key
		}
	}
	if workdir == "" {
		return ""
	}
	dir, err := filepath.Abs(workdir)
	if err != nil {
		return ""
	}
	home, _ := os.UserHomeDir()
	for i := 0; i < 8; i++ {
		if vars, err := godotenv.Read(filepath.Join(dir, ".env")); err == nil {
			for _, name := range []string{"MOONSHOT_API_KEY", "KIMI_API"} {
				if key := strings.TrimSpace(vars[name]); key != "" {
					return key
				}
			}
		}
		parent := filepath.Dir(dir)
		if dir == home || parent == dir {
			break
		}
		dir = parent
	}
	return ""
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

// antislopJSONContract is the output contract shared by both antislop prompts.
// It matches PlanReviewPrompt's schema (summary + rating + findings) so
// review.ParsePlanOutput parses antislop output unchanged. The example summary
// string is intentionally byte-identical to PlanReviewPrompt's — the parser
// uses that exact string to skip prompt echoes.
const antislopJSONContract = `Output: respond with EXACTLY ONE JSON object and nothing else (no prose before or after, no markdown fences). Schema:

{
  "summary": "1-3 sentence overall assessment of the plan",
  "rating": 7,
  "findings": [
    {
      "file": "path/to/file",
      "line": 42,
      "severity": "critical|high|medium|low",
      "category": "reuse|simplify|efficiency|altitude|compat|reinvention|slop|yagni",
      "title": "one-line description of the cut",
      "body": "what is slop or over-engineered, and what it costs",
      "suggestion": "the concrete cut, merge, defer, or replacement",
      "confidence": 8
    }
  ]
}

"rating" is LEANNESS from 1 (mostly slop / heavily over-engineered) to 10 (nothing left to cut). Severity is the impact of the cut: critical = a large unnecessary subsystem or dependency; high = significant dead weight (unneeded abstraction, layer, or compat path); medium = real but contained slop; low = minor cleanup. "line" may be 0 when not applicable. Sort findings by severity, highest first.`

// AntislopCodePrompt is the quality-only code review template used by
// `rival command antislop` in code mode. {SCOPE} is replaced at runtime.
// Derived from Claude Code's built-in /simplify skill (reuse, simplification,
// efficiency, altitude), extended with over-engineering and AI-slop angles.
// Report-only: the reviewer proposes cuts; the caller applies them.
const AntislopCodePrompt = `You are a ruthless senior staff engineer doing a QUALITY-ONLY review: hunt slop and over-engineering, not bugs. Do NOT report correctness bugs, security issues, or missing features — other reviews cover those. Skip style and formatting nitpicks entirely.

Review scope: {SCOPE}

Read the code in the review scope (use your tools; read enclosing functions and neighboring files for context). Work through every angle below, in sequence, in this same pass. For every finding name the concrete cut or replacement.

1. **Reuse & DRY** — new code that re-implements something the codebase already has: search shared/utility modules and files adjacent to the change, and name the existing helper to call instead. Duplicated logic across files or functions is a finding even when each copy is individually fine — name the single home for it.

2. **Simplification** — unnecessary complexity: redundant or derivable state, copy-paste with slight variation, deep nesting, dead code left behind. Name the simpler form that does the same job.

3. **Efficiency** — wasted work: redundant computation or repeated I/O, independent operations run sequentially, blocking work added to startup or hot paths, long-lived objects built from closures that keep the whole enclosing scope alive. Name the cheaper alternative.

4. **Altitude** — changes implemented at the wrong depth: special cases layered on shared infrastructure are a sign the fix is not deep enough — prefer generalizing the underlying mechanism over adding special cases.

5. **Backward-compat hoarding** — compat shims, legacy fallbacks, deprecated-but-kept paths, versioned duplicates (doThingV2), re-export layers kept "just in case". Default stance: delete the old path and migrate the callers. Spare compat code ONLY when a named external consumer (published API, on-disk format, wire protocol) depends on it — name that consumer, otherwise recommend the cut.

6. **Library reinvention** — hand-rolled implementations of what a well-established library already does (parsers, retry/backoff, date math, semver, globbing, and the like). Prefer the language stdlib and the project's existing dependencies before proposing a new one. Name the exact replacement package or function.

7. **Slop signatures** — the telltale patterns of generated code:
- Comment slop: comments narrating the obvious, docstrings restating the signature, section banners, comments justifying the change to a reviewer. Keep only comments stating a constraint the code cannot show.
- Silent-fallback slop: unrequested graceful degradation — quiet defaults on missing config, empty catch blocks returning a zero value. Flag as unspecified behavior masking failures; ask "where was this fallback specified?" rather than asserting what the intended behavior is.
- Wrapper/pass-through slop: functions whose body is a single call, interfaces with one implementation, getters over public fields, grab-bag utils/helpers layers.
- Speculative generality: options nobody passes, parameters always called with the same value, generics instantiated at one type, both directions implemented when only one is needed. Verify via call-site search before reporting.

Rules:
- Only report issues you verified against the code. Every finding cites an exact file and line.
- Prefer fewer, stronger findings over many weak ones.
- If the code is already lean, say so in the summary, give a high rating, and return few or zero findings. Do not invent problems.

` + antislopJSONContract

// AntislopPlanPrompt is the quality-only plan/spec review template used by
// `rival command antislop` in plan mode. {FILE} is replaced with the absolute
// path at the call site. Same charter as AntislopCodePrompt at plan altitude:
// the reviewer produces a cut list, never a bug list.
const AntislopPlanPrompt = `You are a ruthless senior staff engineer reviewing an engineering PLAN / SPEC document (not source code) for slop and over-engineering ONLY. Do NOT report bugs, logic flaws, or gaps — other reviews cover those. Your output is a cut list: what to remove, merge, or defer so the plan ships the stated goal and nothing else.

Plan document to review: {FILE}

Read the file in full (use your tools; read referenced project files when they settle whether something already exists). Judge every cut against the plan's own stated goal. Work through every angle below:

1. **Scope creep & YAGNI** — features, phases, and abstractions the stated goal does not need; work that can be deferred without loss. Each finding is a concrete cut, merge, or defer with justification.

2. **Gold-plating & premature optimization** — caching layers, performance work, configurability, and generality scheduled before anything demands them.

3. **Backward-compat hoarding** — migration shims, compat layers, and deprecation periods planned for interfaces with no named external consumer. Name the consumer or recommend the cut.

4. **Library reinvention** — steps that spec building something a well-established library or an existing module in the same codebase already provides. Name the exact replacement.

5. **DRY at plan level** — the same mechanism designed twice in different sections of the plan; consolidate to one design with one owner.

6. **Ceremony padding** — boilerplate sections that exist because plans "should" have them: risk matrices, rollback plans, observability phases, "Future Considerations" — for work whose size does not warrant them.

Rules:
- Only report cuts you are confident about; quote or cite the section/heading each one lives in ("file" holds the section or heading, "line" may be 0).
- A lean plan gets a high rating and few or zero findings. Do not invent problems, and do not pad this review.

` + antislopJSONContract

// PromptKind selects which reviewer prompt a target runs. The zero value is
// the bug hunter, so an unset field keeps the existing behavior.
type PromptKind int

const (
	PromptBugHunter PromptKind = iota
	PromptSecurity
)

// String names the lens for the consilium judge, which needs to know that
// reviewers looked for different things.
func (k PromptKind) String() string {
	if k == PromptSecurity {
		return "security"
	}
	return "bug hunting"
}

// SecurityModel describes one model that can run the security review. Both
// entries run through the OpenCode adapter, so the differences between them
// are data rather than code.
type SecurityModel struct {
	// Name is the config value: "k3" or "grok".
	Name string
	// Model is the upstream model id.
	Model string
	// Selector is what `opencode -m` receives. OpenCode splits it at the
	// first slash to choose the provider, so for OpenRouter-hosted models
	// this differs from Model.
	Selector string
	// Provider names the block in the generated OpenCode config.
	Provider string
	// BaseURL is the provider endpoint. Empty means OpenCode's own default,
	// which is what the built-in Moonshot provider uses.
	BaseURL string
	// KeyEnv is the environment variable holding the API key.
	KeyEnv string
	// Label is the public name shown in output. It must not collide with any
	// other model's label or concrete id.
	Label string
	// Variant is the reasoning level passed to the provider.
	Variant string
}

// SecurityReviewerK3 and SecurityReviewerGrok are the accepted values of the
// security.reviewer config key.
const (
	SecurityReviewerK3   = "k3"
	SecurityReviewerGrok = "grok"
)

// GrokOpenRouterModel is Grok 4.6 served through OpenRouter. It is a
// different runtime from GrokModel, which reaches xAI through the grok CLI.
const (
	GrokOpenRouterModel    = "x-ai/grok-4.6"
	GrokOpenRouterSelector = "openrouter/x-ai/grok-4.6"
	// GrokOpenRouterLabel deliberately differs from GrokLabel. The two Groks
	// are separate models on separate runtimes with separate credentials, and
	// a shared label would make their sessions and logs indistinguishable.
	GrokOpenRouterLabel = "grok-4.6-openrouter"
	openRouterBaseURL   = "https://openrouter.ai/api/v1"
)

// securityModels is the registry. Adding an entry here is all a new security
// model needs: the adapter reads provider, key, selector, and variant from it.
var securityModels = map[string]SecurityModel{
	SecurityReviewerK3: {
		Name:     SecurityReviewerK3,
		Model:    KimiModel,
		Selector: KimiModel,
		Provider: "moonshotai",
		KeyEnv:   "MOONSHOT_API_KEY",
		Label:    K3Label,
		Variant:  "max",
	},
	SecurityReviewerGrok: {
		Name:     SecurityReviewerGrok,
		Model:    GrokOpenRouterModel,
		Selector: GrokOpenRouterSelector,
		Provider: "openrouter",
		BaseURL:  openRouterBaseURL,
		KeyEnv:   "OPENROUTER_API_KEY",
		Label:    GrokOpenRouterLabel,
		Variant:  "xhigh",
	},
}

// SecurityReviewerNames lists the accepted config values, for error messages.
func SecurityReviewerNames() []string {
	return []string{SecurityReviewerK3, SecurityReviewerGrok}
}

// ResolveSecurityModel returns the model that runs the security review. An
// empty config value means K3.
func ResolveSecurityModel() (SecurityModel, error) {
	name := SecurityReviewerK3
	if userConfig != nil {
		if configured := strings.TrimSpace(userConfig.Security.Reviewer); configured != "" {
			name = strings.ToLower(configured)
		}
	}
	entry, ok := securityModels[name]
	if !ok {
		return SecurityModel{}, fmt.Errorf("invalid security.reviewer %q, must be one of: %s",
			name, strings.Join(SecurityReviewerNames(), ", "))
	}
	return entry, nil
}

// ConfiguredSecurityReviewer returns the raw config value, or "" when unset.
func ConfiguredSecurityReviewer() string {
	if userConfig == nil {
		return ""
	}
	return strings.TrimSpace(userConfig.Security.Reviewer)
}

// OpenCodeEntryFor looks up a registry entry by concrete model id. The
// megareview uses it so an explicit -m selection never consults the security
// config: with security.reviewer set to grok, `-m k3` must still run K3.
func OpenCodeEntryFor(model string) (SecurityModel, bool) {
	for _, entry := range securityModels {
		if entry.Model == model {
			return entry, true
		}
	}
	return SecurityModel{}, false
}

// SecurityAPIKeyFrom resolves an entry's API key. Precedence matches
// KimiAPIKeyFrom: the process environment first, then the nearest .env found
// walking up from workdir.
func SecurityAPIKeyFrom(entry SecurityModel, workdir string) string {
	if key := strings.TrimSpace(os.Getenv(entry.KeyEnv)); key != "" {
		return key
	}
	if entry.Name == SecurityReviewerK3 {
		// K3 keeps its legacy alias.
		if key := strings.TrimSpace(os.Getenv("KIMI_API")); key != "" {
			return key
		}
	}
	if workdir == "" {
		return ""
	}
	dir, err := filepath.Abs(workdir)
	if err != nil {
		return ""
	}
	home, _ := os.UserHomeDir()
	for i := 0; i < 8; i++ {
		if vars, err := godotenv.Read(filepath.Join(dir, ".env")); err == nil {
			if key := strings.TrimSpace(vars[entry.KeyEnv]); key != "" {
				return key
			}
		}
		parent := filepath.Dir(dir)
		if dir == home || parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

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

// MaxConcurrent returns how many reviews may run at once (RIVAL_MAX_CONCURRENT, default 2).
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
// SecurityConfig selects which model runs the security review.
type SecurityConfig struct {
	Reviewer string `yaml:"reviewer"`
}

type UserConfig struct {
	Claude   ClaudeConfig      `yaml:"claude"`
	Security SecurityConfig    `yaml:"security"`
	Efforts  map[string]string `yaml:"efforts"`
	Roles    map[string]string `yaml:"roles"`
}

var userConfig *UserConfig
var userConfigErr error

// LoadUserConfig reads ~/.rival/config.yaml if it exists.
func LoadUserConfig() {
	userConfig = nil
	userConfigErr = nil
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".rival", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			userConfigErr = fmt.Errorf("read %s: %w", path, err)
		}
		return
	}
	var cfg UserConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		userConfigErr = fmt.Errorf("parse %s: %w", path, err)
		return
	}
	for label, raw := range cfg.Efforts {
		effort := strings.ToLower(strings.TrimSpace(raw))
		if !knownEffortModel(label) {
			userConfigErr = fmt.Errorf("invalid effort model %q in %s; use one of: sol, kimi-k3, fable, grok", label, path)
			return
		}
		if !validConfiguredModelEffort(label, effort) {
			allowed := "low, medium, high, xhigh, ultra"
			if label == "kimi-k3" {
				allowed = "max"
			}
			userConfigErr = fmt.Errorf("invalid effort %q for %s in %s; use one of: %s", raw, label, path, allowed)
			return
		}
		cfg.Efforts[label] = effort
	}
	// Validate the security reviewer here, not only where it is resolved, so a
	// typo fails every command instead of waiting for a security run.
	if configured := strings.TrimSpace(cfg.Security.Reviewer); configured != "" {
		name := strings.ToLower(configured)
		if _, ok := securityModels[name]; !ok {
			userConfigErr = fmt.Errorf("invalid security.reviewer %q in %s; use one of: %s",
				configured, path, strings.Join(SecurityReviewerNames(), ", "))
			return
		}
		cfg.Security.Reviewer = name
	}
	userConfig = &cfg
}

// UserConfigError reports an invalid ~/.rival/config.yaml. Commands fail
// before doing any queue, session, or provider work rather than silently
// ignoring a typo in a requested model default.
func UserConfigError() error {
	return userConfigErr
}

// RolePromptOverride returns the user-configured prompt for a role, if any.
func RolePromptOverride(role string) (string, bool) {
	if userConfig == nil {
		return "", false
	}
	v, ok := userConfig.Roles[role]
	return v, ok
}

// DefaultEffortForModel returns the configured default for a concrete model id
// or public model label. Invalid user configuration is reported by
// UserConfigError before command side effects begin.
//
// Kimi K3 is thinking-only and supports exactly max. Other current models
// accept Rival's low/medium/high/xhigh/ultra ladder.
func DefaultEffortForModel(model string) string {
	label := ModelLabel(model)
	builtin := builtinModelEffort(label)
	if userConfig == nil {
		return builtin
	}
	effort, ok := userConfig.Efforts[label]
	if !ok {
		return builtin
	}
	return effort
}

func builtinModelEffort(label string) string {
	switch label {
	case SolLabel:
		return DefaultReviewEffort
	case "kimi-k3":
		return "max"
	case FableLabel:
		return "medium"
	case GrokLabel:
		// grok-4.5's menu is low/medium/high, and high is its own default.
		return "high"
	default:
		return DefaultReviewEffort
	}
}

func knownEffortModel(label string) bool {
	switch label {
	case SolLabel, "kimi-k3", FableLabel, GrokLabel:
		return true
	default:
		return false
	}
}

func validConfiguredModelEffort(label, effort string) bool {
	if label == "kimi-k3" {
		return effort == "max"
	}
	return IsValidEffort(effort)
}

// ResolveEffort applies the documented precedence for one concrete model:
// explicit invocation override, then ~/.rival/config.yaml, then the supplied
// surface-specific fallback. Kimi K3 remains pinned to max because that
// provider exposes no other reasoning level.
func ResolveEffort(model, override, fallback string) (string, error) {
	label := ModelLabel(model)
	override = strings.ToLower(strings.TrimSpace(override))
	if override != "" {
		if label == "kimi-k3" {
			return "max", nil
		}
		if !IsValidEffort(override) {
			return "", fmt.Errorf("invalid effort %q for %s", override, label)
		}
		return override, nil
	}
	if userConfig != nil {
		if effort, ok := userConfig.Efforts[label]; ok {
			return effort, nil
		}
	}
	fallback = strings.ToLower(strings.TrimSpace(fallback))
	if fallback == "" {
		fallback = builtinModelEffort(label)
	}
	if label == "kimi-k3" {
		return "max", nil
	}
	if !IsValidEffort(fallback) {
		return "", fmt.Errorf("invalid fallback effort %q for %s", fallback, label)
	}
	return fallback, nil
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
