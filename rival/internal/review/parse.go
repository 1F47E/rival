package review

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseReviewerOutput extracts structured JSON from raw CLI output.
// CLIs often wrap JSON in markdown fences or add prose before/after.
func ParseReviewerOutput(raw string) (*ReviewerOutput, error) {
	jsonStr := extractJSON(raw)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in output")
	}

	var out ReviewerOutput
	if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
		return nil, fmt.Errorf("unmarshal reviewer output: %w", err)
	}
	return &out, nil
}

// ParseConsiliumOutput extracts structured consilium JSON from raw CLI output.
func ParseConsiliumOutput(raw string) (*ConsiliumOutput, error) {
	jsonStr := extractJSON(raw)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in consilium output")
	}

	var out ConsiliumOutput
	if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
		return nil, fmt.Errorf("unmarshal consilium output: %w", err)
	}
	return &out, nil
}

// extractJSON finds the first valid JSON object in text.
// Handles markdown fences (```json ... ```), bare JSON, and prose wrapping.
func extractJSON(s string) string {
	// Try markdown fence first.
	if idx := strings.Index(s, "```json"); idx >= 0 {
		start := idx + len("```json")
		if end := strings.Index(s[start:], "```"); end >= 0 {
			return strings.TrimSpace(s[start : start+end])
		}
	}
	if idx := strings.Index(s, "```"); idx >= 0 {
		start := idx + len("```")
		// Skip optional language tag on same line.
		if nl := strings.Index(s[start:], "\n"); nl >= 0 {
			content := s[start+nl:]
			if end := strings.Index(content, "```"); end >= 0 {
				candidate := strings.TrimSpace(content[:end])
				if len(candidate) > 0 && candidate[0] == '{' {
					return candidate
				}
			}
		}
	}

	// Try to find bare JSON object — validate each candidate.
	remaining := s
	for {
		start := strings.Index(remaining, "{")
		if start < 0 {
			return ""
		}

		// Find matching closing brace.
		depth := 0
		inStr := false
		esc := false
		found := -1
		for i := start; i < len(remaining); i++ {
			if esc {
				esc = false
				continue
			}
			c := remaining[i]
			if c == '\\' && inStr {
				esc = true
				continue
			}
			if c == '"' {
				inStr = !inStr
				continue
			}
			if inStr {
				continue
			}
			switch c {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					found = i
				}
			}
			if found >= 0 {
				break
			}
		}

		if found < 0 {
			return ""
		}

		candidate := remaining[start : found+1]
		if json.Valid([]byte(candidate)) {
			return candidate
		}
		// Not valid JSON — skip past this '{' and try again.
		remaining = remaining[start+1:]
	}
}
