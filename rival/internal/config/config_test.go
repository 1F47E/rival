package config

import (
	"context"
	"strings"
	"testing"
	"time"
)

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
