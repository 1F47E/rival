package parser

import (
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
)

func TestParseArgs_Empty(t *testing.T) {
	for _, input := range []string{"", "  ", "\t\n"} {
		r, err := ParseCodexArgs(input)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if !r.IsEmpty {
			t.Errorf("expected IsEmpty for %q", input)
		}
		if r.Effort != config.DefaultEffort {
			t.Errorf("expected default effort, got %q", r.Effort)
		}
	}
}

func TestParseArgs_RawPrompt(t *testing.T) {
	r, err := ParseCodexArgs("explain the auth flow")
	if err != nil {
		t.Fatal(err)
	}
	if r.IsEmpty || r.IsReview {
		t.Error("expected raw prompt")
	}
	if r.Prompt != "explain the auth flow" {
		t.Errorf("unexpected prompt: %q", r.Prompt)
	}
	if r.Effort != "medium" {
		t.Errorf("unexpected effort: %q", r.Effort)
	}
}

func TestParseArgs_EffortWithPrompt(t *testing.T) {
	r, err := ParseCodexArgs("-re xhigh find bugs in main.go")
	if err != nil {
		t.Fatal(err)
	}
	if r.Effort != "xhigh" {
		t.Errorf("expected xhigh, got %q", r.Effort)
	}
	if r.Prompt != "find bugs in main.go" {
		t.Errorf("unexpected prompt: %q", r.Prompt)
	}
}

func TestParseArgs_InvalidEffort(t *testing.T) {
	_, err := ParseCodexArgs("-re ultra review")
	if err == nil {
		t.Fatal("expected error for invalid effort")
	}
	if !strings.Contains(err.Error(), "invalid effort") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseArgs_ReviewAlone(t *testing.T) {
	r, err := ParseCodexArgs("review")
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsReview {
		t.Error("expected IsReview")
	}
	if r.ReviewScope != "the entire project" {
		t.Errorf("expected default scope, got %q", r.ReviewScope)
	}
	if !strings.Contains(r.Prompt, "Review scope: the entire project") {
		t.Error("prompt should contain review scope")
	}
}

func TestParseArgs_ReviewWithScope(t *testing.T) {
	r, err := ParseCodexArgs("review src/")
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsReview {
		t.Error("expected IsReview")
	}
	if r.ReviewScope != "src/" {
		t.Errorf("unexpected scope: %q", r.ReviewScope)
	}
}

func TestParseArgs_ReviewQuotedScope(t *testing.T) {
	r, err := ParseCodexArgs(`review "only THIS file xxx"`)
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsReview {
		t.Error("expected IsReview")
	}
	if r.ReviewScope != `"only THIS file xxx"` {
		t.Errorf("unexpected scope: %q", r.ReviewScope)
	}
}

func TestParseArgs_EffortWithReview(t *testing.T) {
	r, err := ParseCodexArgs("-re high review")
	if err != nil {
		t.Fatal(err)
	}
	if r.Effort != "high" {
		t.Errorf("expected high, got %q", r.Effort)
	}
	if !r.IsReview {
		t.Error("expected IsReview")
	}
	if r.ReviewScope != "the entire project" {
		t.Errorf("expected default scope, got %q", r.ReviewScope)
	}
}

func TestParseArgs_EffortWithReviewAndScope(t *testing.T) {
	r, err := ParseCodexArgs("-re high review src/api/")
	if err != nil {
		t.Fatal(err)
	}
	if r.Effort != "high" {
		t.Errorf("expected high, got %q", r.Effort)
	}
	if !r.IsReview {
		t.Error("expected IsReview")
	}
	if r.ReviewScope != "src/api/" {
		t.Errorf("unexpected scope: %q", r.ReviewScope)
	}
}

func TestParseArgs_EffortAlone(t *testing.T) {
	r, err := ParseCodexArgs("-re high")
	if err != nil {
		t.Fatal(err)
	}
	if r.Effort != "high" {
		t.Errorf("expected high, got %q", r.Effort)
	}
	if !r.IsEmpty {
		t.Error("expected IsEmpty when only -re flag provided")
	}
}

func TestParseGeminiArgs_Identical(t *testing.T) {
	r, err := ParseGeminiArgs("-re xhigh review src/")
	if err != nil {
		t.Fatal(err)
	}
	if r.Effort != "xhigh" || !r.IsReview || r.ReviewScope != "src/" {
		t.Errorf("gemini parser mismatch: effort=%q review=%v scope=%q", r.Effort, r.IsReview, r.ReviewScope)
	}
}
