package parser

import (
	"strings"
	"testing"
)

func TestParseArgs_Empty(t *testing.T) {
	for _, input := range []string{"", "  ", "\t\n"} {
		r, err := ParseGPT56SolArgs(input)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if !r.IsEmpty {
			t.Errorf("expected IsEmpty for %q", input)
		}
		if r.Effort != "" {
			t.Errorf("expected omitted effort for configured default resolution, got %q", r.Effort)
		}
	}
}

func TestParseArgs_RawPrompt(t *testing.T) {
	r, err := ParseGPT56SolArgs("explain the auth flow")
	if err != nil {
		t.Fatal(err)
	}
	if r.IsEmpty || r.IsReview {
		t.Error("expected raw prompt")
	}
	if r.Prompt != "explain the auth flow" {
		t.Errorf("unexpected prompt: %q", r.Prompt)
	}
	if r.Effort != "" {
		t.Errorf("expected omitted effort for configured default resolution, got %q", r.Effort)
	}
}

func TestParseArgs_EffortWithPrompt(t *testing.T) {
	r, err := ParseGPT56SolArgs("-re xhigh find bugs in main.go")
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

func TestParseArgs_UltraEffort(t *testing.T) {
	r, err := ParseGPT56SolArgs("-re ultra review")
	if err != nil {
		t.Fatal(err)
	}
	if r.Effort != "ultra" || !r.IsReview {
		t.Fatalf("unexpected parse result: %+v", r)
	}
}

func TestParseArgs_InvalidEffort(t *testing.T) {
	_, err := ParseGPT56SolArgs("-re maximum review")
	if err == nil {
		t.Fatal("expected error for invalid effort")
	}
	if !strings.Contains(err.Error(), "invalid effort") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseArgs_ReviewAlone(t *testing.T) {
	r, err := ParseGPT56SolArgs("review")
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
	r, err := ParseGPT56SolArgs("review src/")
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
	r, err := ParseGPT56SolArgs(`review "only THIS file xxx"`)
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
	r, err := ParseGPT56SolArgs("-re high review")
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
	r, err := ParseGPT56SolArgs("-re high review src/api/")
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
	r, err := ParseGPT56SolArgs("-re high")
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

func TestParseArgs_AutoScope(t *testing.T) {
	// "review" alone → AutoScope=true
	r, err := ParseGPT56SolArgs("review")
	if err != nil {
		t.Fatal(err)
	}
	if !r.AutoScope {
		t.Error("expected AutoScope=true for bare review")
	}

	// "review src/" → AutoScope=false
	r, err = ParseGPT56SolArgs("review src/")
	if err != nil {
		t.Fatal(err)
	}
	if r.AutoScope {
		t.Error("expected AutoScope=false when explicit scope given")
	}

	// "-re high review" → AutoScope=true
	r, err = ParseGPT56SolArgs("-re high review")
	if err != nil {
		t.Fatal(err)
	}
	if !r.AutoScope {
		t.Error("expected AutoScope=true for -re high review")
	}
}

func TestParseReviewArgs_AutoScope(t *testing.T) {
	// Empty megareview arguments run the default roster against git scope.
	r, err := ParseReviewArgs("")
	if err != nil {
		t.Fatal(err)
	}
	if !r.AutoScope || r.IsEmpty {
		t.Fatalf("empty megareview args should auto-scope, got %+v", r)
	}
	if r.Effort != "" {
		t.Fatalf("empty megareview effort = %q, want configured-default sentinel", r.Effort)
	}

	// Empty scope in megareview → AutoScope=true
	r, err = ParseReviewArgs("-re high")
	if err != nil {
		t.Fatal(err)
	}
	if !r.AutoScope {
		t.Error("expected AutoScope=true for megareview with no scope")
	}

	// Explicit scope → AutoScope=false
	r, err = ParseReviewArgs("src/api/")
	if err != nil {
		t.Fatal(err)
	}
	if r.AutoScope {
		t.Error("expected AutoScope=false when explicit scope given")
	}
}

func TestParseReviewArgs_ModelSelection(t *testing.T) {
	t.Run("GPT-5.6-Sol full model name", func(t *testing.T) {
		r, err := ParseReviewArgs("-m gpt-5.6-sol -re ultra")
		if err != nil {
			t.Fatal(err)
		}
		if !r.AutoScope || r.Effort != "ultra" || len(r.Models) != 1 || r.Models[0] != "gpt-5.6-sol" {
			t.Fatalf("unexpected parse result: %+v", r)
		}
	})

	t.Run("model only auto-scopes", func(t *testing.T) {
		r, err := ParseReviewArgs("-m deepseek")
		if err != nil {
			t.Fatal(err)
		}
		if !r.AutoScope || len(r.Models) != 1 || r.Models[0] != "deepseek" {
			t.Fatalf("unexpected parse result: %+v", r)
		}
	})

	t.Run("flags in either order and comma list", func(t *testing.T) {
		r, err := ParseReviewArgs("--model=k3,deepseek --effort high src/api and reports")
		if err != nil {
			t.Fatal(err)
		}
		if r.Effort != "high" || r.AutoScope || r.ReviewScope != "src/api and reports" {
			t.Fatalf("unexpected parse result: %+v", r)
		}
		if len(r.Models) != 2 || r.Models[0] != "k3" || r.Models[1] != "deepseek" {
			t.Fatalf("unexpected models: %v", r.Models)
		}

		r, err = ParseReviewArgs("-re low -m k3 -m deepseek src/")
		if err != nil {
			t.Fatal(err)
		}
		if r.Effort != "low" || len(r.Models) != 2 || r.Models[1] != "deepseek" {
			t.Fatalf("unexpected repeated model parse: %+v", r)
		}

		r, err = ParseReviewArgs("src/api/ -m deepseek -re medium")
		if err != nil {
			t.Fatal(err)
		}
		if r.ReviewScope != "src/api/" || r.Effort != "medium" || len(r.Models) != 1 || r.Models[0] != "deepseek" {
			t.Fatalf("trailing flags must not become scope text: %+v", r)
		}
	})

	t.Run("double dash escapes scope", func(t *testing.T) {
		r, err := ParseReviewArgs("-m k3 -- -generated/path")
		if err != nil {
			t.Fatal(err)
		}
		if r.ReviewScope != "-generated/path" {
			t.Fatalf("scope = %q", r.ReviewScope)
		}
	})
}

func TestParseReviewArgs_ModelOptionErrors(t *testing.T) {
	for _, raw := range []string{"-m", "--model=", "-m -re high", "--model k3,,deepseek", "--unknown value"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseReviewArgs(raw); err == nil {
				t.Fatalf("expected %q to fail", raw)
			}
		})
	}
}

func TestParseReviewArgs_Help(t *testing.T) {
	for _, raw := range []string{"-h", "--help"} {
		r, err := ParseReviewArgs(raw)
		if err != nil {
			t.Fatal(err)
		}
		if !r.IsEmpty {
			t.Fatalf("%s should request usage output, got %+v", raw, r)
		}
	}
}
