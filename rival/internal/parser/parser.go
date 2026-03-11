package parser

import (
	"fmt"
	"strings"

	"github.com/1F47E/rival/internal/config"
)

// ParseResult holds the parsed user arguments.
type ParseResult struct {
	Effort      string
	IsReview    bool
	ReviewScope string
	Prompt      string
	IsEmpty     bool
}

// ParseCodexArgs parses raw arguments for the codex command.
// Grammar: [-re level] [review [scope] | prompt]
func ParseCodexArgs(raw string) (*ParseResult, error) {
	return parseArgs(raw)
}

// ParseGeminiArgs parses raw arguments for the gemini command.
// Identical grammar to codex (no -m flag in v1).
func ParseGeminiArgs(raw string) (*ParseResult, error) {
	return parseArgs(raw)
}

func parseArgs(raw string) (*ParseResult, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return &ParseResult{Effort: config.DefaultEffort, IsEmpty: true}, nil
	}

	result := &ParseResult{Effort: config.DefaultEffort}

	// Step 1: Parse -re flag.
	if strings.HasPrefix(s, "-re ") {
		rest := strings.TrimSpace(s[4:])
		parts := strings.SplitN(rest, " ", 2)
		effort := parts[0]
		if !config.IsValidEffort(effort) {
			return nil, fmt.Errorf("invalid effort level %q, must be one of: %s", effort, strings.Join(config.ValidEfforts, ", "))
		}
		result.Effort = effort
		if len(parts) > 1 {
			s = strings.TrimSpace(parts[1])
		} else {
			s = ""
		}
	}

	// Step 2: Check for review subcommand.
	lower := strings.ToLower(s)
	if lower == "review" || strings.HasPrefix(lower, "review ") {
		result.IsReview = true
		scope := strings.TrimSpace(s[len("review"):])
		if scope == "" {
			scope = "the entire project"
		}
		result.ReviewScope = scope
		result.Prompt = strings.ReplaceAll(config.ReviewPrompt, "{SCOPE}", scope)
		return result, nil
	}

	// Step 3: Otherwise it's a raw prompt.
	if s == "" {
		result.IsEmpty = true
		return result, nil
	}
	result.Prompt = s
	return result, nil
}
