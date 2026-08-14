package review

import "github.com/1F47E/rival/internal/config"

// ReviewerOutput is the structured JSON every reviewer must emit.
type ReviewerOutput struct {
	Summary  string            `json:"summary"`
	Findings []ReviewerFinding `json:"findings"`
}

// ReviewerFinding is a single finding from one reviewer (pre-judge).
type ReviewerFinding struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Suggestion string `json:"suggestion,omitempty"`
	Confidence int    `json:"confidence"`
}

// ReviewInput holds the output from a single CLI reviewer for the consilium.
type ReviewInput struct {
	CLI   string `json:"cli"`
	Model string `json:"model"`
	// Prompt is the lens this reviewer ran. The judge needs it: a finding
	// only one reviewer could have produced must not be discounted for
	// lacking corroboration from a reviewer that was not looking for it.
	Prompt    config.PromptKind `json:"-"`
	RawOutput string            `json:"raw_output"`
	Parsed    *ReviewerOutput   `json:"parsed,omitempty"`
}

// Finding is a single code review finding (consilium judge output).
type Finding struct {
	File       string   `json:"file"`
	Line       int      `json:"line"`
	Severity   string   `json:"severity"`
	Category   string   `json:"category,omitempty"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Suggestion string   `json:"suggestion,omitempty"`
	Confidence int      `json:"confidence"`
	FoundBy    []string `json:"found_by"`
}

// Recommendation is the consilium's overall verdict.
type Recommendation struct {
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

// ConsiliumOutput is the structured output from the consilium judge.
type ConsiliumOutput struct {
	Summary        string         `json:"summary"`
	Findings       []Finding      `json:"findings"`
	Recommendation Recommendation `json:"recommendation"`
	ReviewerCount  int            `json:"reviewer_count"`
}
