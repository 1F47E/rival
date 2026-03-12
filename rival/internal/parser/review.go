package parser

import (
	"fmt"
	"strings"

	"github.com/1F47E/rival/internal/config"
)

// ParseReviewArgs parses raw arguments for the megareview command.
// Grammar: [-re level] [scope] — always a review, no "review" keyword needed.
func ParseReviewArgs(raw string) (*ParseResult, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return &ParseResult{Effort: config.DefaultEffort, IsEmpty: true}, nil
	}

	result := &ParseResult{Effort: config.DefaultEffort, IsReview: true}

	// Parse -re flag.
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

	// Remaining text is the scope.
	scope := s
	if scope == "" {
		scope = "the entire project"
	}
	result.ReviewScope = scope
	result.Prompt = strings.ReplaceAll(config.ReviewPrompt, "{SCOPE}", scope)
	return result, nil
}
