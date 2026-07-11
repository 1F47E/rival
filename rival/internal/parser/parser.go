package parser

import (
	"fmt"
	"strings"

	"github.com/1F47E/rival/internal/config"
)

// ParseResult holds the parsed user arguments.
type ParseResult struct {
	Effort      string
	Models      []string // exact megareview roster selectors; nil means configured default
	IsReview    bool
	AutoScope   bool // true when review has no explicit scope (use git detection)
	ReviewScope string
	Prompt      string
	IsEmpty     bool
}

// ParseGPT56SolArgs parses raw arguments for the gpt-5.6-sol command.
// Grammar: [-re level] [review [scope] | prompt]. It defaults to high and
// accepts ultra as the deepest public effort level.
func ParseGPT56SolArgs(raw string) (*ParseResult, error) {
	return parseArgsWithEffort(raw, config.DefaultReviewEffort, config.IsValidReviewEffort, config.ReviewEfforts)
}

// ParseCodexArgs is retained for internal compatibility with older callers.
func ParseCodexArgs(raw string) (*ParseResult, error) {
	return ParseGPT56SolArgs(raw)
}

// ParseGeminiArgs parses raw arguments for the gemini command.
// It uses the common standalone-command grammar (no -m flag in v1).
func ParseGeminiArgs(raw string) (*ParseResult, error) {
	return parseArgs(raw)
}

// ParseClaudeArgs parses raw arguments for the claude command.
func ParseClaudeArgs(raw string) (*ParseResult, error) {
	return parseArgs(raw)
}

// ParseAntigravityArgs parses raw arguments for the antigravity command.
func ParseAntigravityArgs(raw string) (*ParseResult, error) {
	return parseArgs(raw)
}

// ParseFableArgs parses raw arguments for the fable command (claude-fable-5).
// Identical grammar to claude.
func ParseFableArgs(raw string) (*ParseResult, error) {
	return parseArgs(raw)
}

func parseArgs(raw string) (*ParseResult, error) {
	return parseArgsWithEffort(raw, config.DefaultEffort, config.IsValidEffort, config.ValidEfforts)
}

func parseArgsWithEffort(raw, defaultEffort string, validEffort func(string) bool, effortNames []string) (*ParseResult, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return &ParseResult{Effort: defaultEffort, IsEmpty: true}, nil
	}

	result := &ParseResult{Effort: defaultEffort}

	// Step 1: Parse -re flag.
	if strings.HasPrefix(s, "-re ") {
		rest := strings.TrimSpace(s[4:])
		parts := strings.SplitN(rest, " ", 2)
		effort := parts[0]
		if !validEffort(effort) {
			return nil, fmt.Errorf("invalid effort level %q, must be one of: %s", effort, strings.Join(effortNames, ", "))
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
			result.AutoScope = true
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
