package cmd

import (
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/parser"
)

func TestGrokCommandIsPublic(t *testing.T) {
	if commandGrokCmd.Use != config.GrokLabel || commandGrokCmd.Hidden {
		t.Fatalf("command metadata = use %q hidden %v", commandGrokCmd.Use, commandGrokCmd.Hidden)
	}
	for _, flag := range []string{"workdir", "no-queue"} {
		if commandGrokCmd.Flags().Lookup(flag) == nil {
			t.Fatalf("command grok is missing the %q flag", flag)
		}
	}
}

// The sandbox is selected by the review boolean handed to executor.RunGrok, so
// a review-mode session that reported raw would run grok unsandboxed with
// --yolo. Pin both the mode derivation and the boolean that follows from it.
func TestGrokSessionModeDrivesReviewSandbox(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantMode   string
		wantReview bool
	}{
		{"plain prompt", "explain the auth flow", "raw", false},
		{"review without scope", "review", "review", true},
		{"review with scope", "review src/api/", "review", true},
		{"review with effort", "-re high review src/api/", "review", true},
		{"prompt with effort", "-re low explain the auth flow", "raw", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := parser.ParseGrokArgs(tt.raw)
			if err != nil {
				t.Fatalf("ParseGrokArgs(%q) error: %v", tt.raw, err)
			}

			mode := sessionMode(parsed.IsReview)
			if mode != tt.wantMode {
				t.Fatalf("mode = %q, want %q", mode, tt.wantMode)
			}
			if review := mode == "review"; review != tt.wantReview {
				t.Fatalf("review sandbox = %v, want %v", review, tt.wantReview)
			}
		})
	}
}

func TestGrokUsesConfiguredGrokEffortDefault(t *testing.T) {
	parsed, err := parser.ParseGrokArgs("review")
	if err != nil {
		t.Fatalf("ParseGrokArgs error: %v", err)
	}
	effort, err := config.ResolveEffort(config.GrokModel, parsed.Effort, config.DefaultReviewEffort)
	if err != nil {
		t.Fatalf("ResolveEffort error: %v", err)
	}
	if effort != "high" {
		t.Fatalf("effort = %q, want high", effort)
	}
}

func TestGrokUsageUsesOnlyPublicNaming(t *testing.T) {
	lower := strings.ToLower(grokUsage)
	if !strings.Contains(lower, "/rival-grok") {
		t.Fatal("usage must name /rival-grok")
	}
	if !strings.Contains(lower, "built-in: high") {
		t.Fatalf("usage missing the built-in effort fallback: %q", lower)
	}
}
