package config

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMaxConcurrent(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want int
	}{
		{name: "unset uses two", env: "", want: 2},
		{name: "explicit override", env: "3", want: 3},
		{name: "zero falls back", env: "0", want: 2},
		{name: "negative falls back", env: "-1", want: 2},
		{name: "invalid falls back", env: "many", want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RIVAL_MAX_CONCURRENT", tt.env)
			if got := MaxConcurrent(); got != tt.want {
				t.Errorf("MaxConcurrent()=%d, want %d", got, tt.want)
			}
		})
	}
}

func TestRunTimeout(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want time.Duration
	}{
		{name: "unset → default", env: "", want: DefaultRunTimeout},
		{name: "explicit duration", env: "10m", want: 10 * time.Minute},
		{name: "zero disables", env: "0", want: 0},
		{name: "0s disables", env: "0s", want: 0},
		{name: "garbage → default", env: "banana", want: DefaultRunTimeout},
		{name: "negative → default", env: "-5m", want: DefaultRunTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RIVAL_RUN_TIMEOUT", tt.env)
			if got := RunTimeout(); got != tt.want {
				t.Errorf("RunTimeout()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestMaxRunWait(t *testing.T) {
	// queue 30m + 2*run 30m + 5m margin = 95m by default.
	t.Run("default", func(t *testing.T) {
		t.Setenv("RIVAL_QUEUE_TIMEOUT", "")
		t.Setenv("RIVAL_RUN_TIMEOUT", "")
		if got, want := MaxRunWait(), 95*time.Minute; got != want {
			t.Errorf("MaxRunWait()=%v, want %v", got, want)
		}
	})
	t.Run("scales with configured timeouts", func(t *testing.T) {
		t.Setenv("RIVAL_QUEUE_TIMEOUT", "10m")
		t.Setenv("RIVAL_RUN_TIMEOUT", "20m")
		// 10 + 2*20 + 5 = 55m
		if got, want := MaxRunWait(), 55*time.Minute; got != want {
			t.Errorf("MaxRunWait()=%v, want %v", got, want)
		}
	})
	t.Run("run timeout disabled → queue + margin only", func(t *testing.T) {
		t.Setenv("RIVAL_QUEUE_TIMEOUT", "30m")
		t.Setenv("RIVAL_RUN_TIMEOUT", "0")
		if got, want := MaxRunWait(), 35*time.Minute; got != want {
			t.Errorf("MaxRunWait()=%v, want %v", got, want)
		}
	})
}

func TestWithRunTimeout(t *testing.T) {
	t.Run("disabled returns no deadline", func(t *testing.T) {
		t.Setenv("RIVAL_RUN_TIMEOUT", "0")
		ctx, cancel := WithRunTimeout(context.Background(), 1)
		defer cancel()
		if _, ok := ctx.Deadline(); ok {
			t.Error("expected no deadline when timeout disabled")
		}
	})
	t.Run("mult scales the budget", func(t *testing.T) {
		t.Setenv("RIVAL_RUN_TIMEOUT", "10m")
		ctx, cancel := WithRunTimeout(context.Background(), 2)
		defer cancel()
		dl, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected a deadline")
		}
		// ~20m out; allow slack for test execution time.
		if remaining := time.Until(dl); remaining < 19*time.Minute || remaining > 20*time.Minute {
			t.Errorf("deadline ~20m expected, got %v remaining", remaining)
		}
	})
	t.Run("mult<=0 returns no deadline", func(t *testing.T) {
		t.Setenv("RIVAL_RUN_TIMEOUT", "10m")
		ctx, cancel := WithRunTimeout(context.Background(), 0)
		defer cancel()
		if _, ok := ctx.Deadline(); ok {
			t.Error("expected no deadline when mult<=0")
		}
	})
}

func TestClaudeAuth(t *testing.T) {
	tests := []struct {
		name    string
		envAuth string
		envKey  string
		want    string
		wantErr string
	}{
		{name: "default is subscription", envAuth: "", envKey: "sk-ant-xxx", want: ClaudeAuthSubscription},
		{name: "explicit subscription", envAuth: "subscription", want: ClaudeAuthSubscription},
		{name: "sub shorthand", envAuth: "sub", want: ClaudeAuthSubscription},
		{name: "api with key", envAuth: "api", envKey: "sk-ant-xxx", want: ClaudeAuthAPI},
		{name: "api without key fails", envAuth: "api", envKey: "", wantErr: "ANTHROPIC_API_KEY is empty"},
		{name: "garbage fails", envAuth: "oauth2", wantErr: "invalid RIVAL_CLAUDE_AUTH"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RIVAL_CLAUDE_AUTH", tt.envAuth)
			t.Setenv("ANTHROPIC_API_KEY", tt.envKey)
			got, err := ClaudeAuth()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpencodeReviewerList_Curated(t *testing.T) {
	// The legacy environment override must not reintroduce uncurated models.
	t.Setenv("RIVAL_OPENCODE_MODELS", "opencode/deepseek-v4-flash")
	got := OpencodeReviewerList()
	if len(got) != 3 {
		t.Fatalf("curated roster size = %d, want 3", len(got))
	}
	want := []OpencodeReviewer{
		{Model: OpencodeDeepSeekPro, Role: "bug_hunter"},
		{Model: OpencodeKimiK27Code, Role: "arch_security"},
		{Model: OpencodeGLMModel, Role: "code_quality"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("curated reviewer %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestOpencodeProviderConfigKeyFromEnv(t *testing.T) {
	t.Setenv("RIVAL_OPENCODE_API_KEY", "  sk-test-key  ")
	if got := OpencodeAPIKey(); got != "sk-test-key" {
		t.Errorf("OpencodeAPIKey() = %q, want trimmed sk-test-key", got)
	}
	t.Setenv("RIVAL_OPENCODE_API_KEY", "")
	if got := OpencodeAPIKey(); got != "" {
		t.Errorf("OpencodeAPIKey() with empty env = %q, want empty", got)
	}
}

func TestResolveReviewTargets_DefaultIsCuratedFourModelRoster(t *testing.T) {
	got, err := ResolveReviewTargets(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("default target count = %d, want 4: %+v", len(got), got)
	}
	if got[0].CLI != "codex" || got[0].Model != GPT56SolModel {
		t.Fatalf("first default target = %+v, want %s", got[0], GPT56SolModel)
	}
	for _, target := range got[1:] {
		if target.CLI != "opencode" {
			t.Fatalf("curated open-model target unexpectedly uses %q: %+v", target.CLI, got)
		}
	}
	if got[1].Model != OpencodeDeepSeekPro || got[2].Model != OpencodeKimiK27Code || got[3].Model != OpencodeGLMModel {
		t.Fatalf("unexpected default target order: %+v", got)
	}
}

func TestResolveReviewTargets_AliasesAndRoles(t *testing.T) {
	cases := []struct {
		selector string
		model    string
		role     string
	}{
		{"sol", GPT56SolModel, "bug_hunter"},
		{GPT56SolModel, GPT56SolModel, "bug_hunter"},
		{"deepseek", OpencodeDeepSeekPro, "bug_hunter"},
		{"deepseek-pro", OpencodeDeepSeekPro, "bug_hunter"},
		{"kimi", OpencodeKimiK27Code, "arch_security"},
		{"kimi-k2.7-code", OpencodeKimiK27Code, "arch_security"},
		{"glm", OpencodeGLMModel, "code_quality"},
		{"glm-5.2", OpencodeGLMModel, "code_quality"},
	}
	for _, tc := range cases {
		t.Run(tc.selector, func(t *testing.T) {
			got, err := ResolveReviewTargets([]string{tc.selector})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0].Model != tc.model || got[0].Role != tc.role {
				t.Fatalf("ResolveReviewTargets(%q) = %+v", tc.selector, got)
			}
			wantCLI := "opencode"
			if tc.model == GPT56SolModel {
				wantCLI = "codex"
			}
			if got[0].CLI != wantCLI {
				t.Fatalf("ResolveReviewTargets(%q) CLI = %q, want %q", tc.selector, got[0].CLI, wantCLI)
			}
		})
	}
}

func TestResolveReviewTargets_ExactOrderAndDedup(t *testing.T) {
	got, err := ResolveReviewTargets([]string{"glm,sol", "kimi", "deepseek", "glm"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0].Model != OpencodeGLMModel || got[1].Model != GPT56SolModel || got[2].Model != OpencodeKimiK27Code || got[3].Model != OpencodeDeepSeekPro {
		t.Fatalf("unexpected exact roster: %+v", got)
	}
}

func TestResolveReviewTargets_RejectsModelsOutsideCuratedSet(t *testing.T) {
	for _, selector := range []string{"codex", "deepseek-flash", "opencode/kimi-k2.6", "all", ""} {
		t.Run(selector, func(t *testing.T) {
			if _, err := ResolveReviewTargets([]string{selector}); err == nil {
				t.Fatalf("expected %q to be rejected", selector)
			}
		})
	}
}

func TestEngineLabel(t *testing.T) {
	cases := []struct{ cli, model, want string }{
		{"codex", GPT56SolModel, SolLabel},
		{"codex", "gpt-5.5", SolLabel},
		{"codex", "", SolLabel},
		{"antigravity", "gemini-3.5-flash", "gemini-3.5-flash"},
		{"claude", ClaudeModel, OpusLabel},
		{"claude", FableModel, FableLabel},
		{"claude", "claude-fable-4", FableLabel},
		{"fable", "", FableLabel},
		{"opencode", "opencode-go/glm-5.2", "glm-5.2"},
		{"opencode", "opencode-go/deepseek-v4-pro", "deepseek-v4-pro"},
		{"opencode", OpencodeKimiK27Code, "kimi-k2.7-code"},
		{"opencode", "", "opencode"},
	}
	for _, c := range cases {
		if got := EngineLabel(c.cli, c.model); got != c.want {
			t.Errorf("EngineLabel(%q,%q) = %q, want %q", c.cli, c.model, got, c.want)
		}
	}
}

func TestPublicRuntimeLogNormalizesOnlyRuntimeMetadata(t *testing.T) {
	raw := "OpenAI Codex v0.130.0\n--------\nmodel: gpt-5.5\nprovider: openai\n--------\nuser\n" +
		"inspect rival/cmd/command_codex.go\n" +
		"=== REVIEW FROM codex (gpt-5.5) [role: bug_hunter] ===\n"
	got := PublicRuntimeLog("codex", "gpt-5.5", raw)
	for _, want := range []string{
		"Sol runtime v0.130.0",
		"model: sol",
		"=== REVIEW FROM sol [role: bug_hunter] ===",
		"rival/cmd/command_codex.go",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("public log missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"OpenAI Codex", "model: gpt-5.5", "REVIEW FROM codex"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("public log exposes %q:\n%s", forbidden, got)
		}
	}
}

func TestPublicRuntimeErrorUsesPublicModelName(t *testing.T) {
	got := PublicRuntimeError("codex", GPT56SolModel, "Codex CLI failed for gpt-5.6-sol; run codex login")
	if strings.Contains(strings.ToLower(got), "codex") || strings.Contains(got, GPT56SolModel) || !strings.Contains(strings.ToLower(got), SolLabel) {
		t.Fatalf("public error was not normalized: %q", got)
	}
}
