package review

import (
	"sort"
	"strings"
)

// DefaultConfidenceThreshold is the minimum confidence to keep a finding.
const DefaultConfidenceThreshold = 6

// FilterByConfidence drops findings below the threshold.
func FilterByConfidence(findings []Finding, threshold int) []Finding {
	var kept []Finding
	for _, f := range findings {
		if f.Confidence >= threshold {
			kept = append(kept, f)
		}
	}
	return kept
}

var severityOrder = map[string]int{
	"critical": 0,
	"high":     1,
	"medium":   2,
	"low":      3,
}

func severityRank(s string) int {
	if rank, ok := severityOrder[strings.ToLower(s)]; ok {
		return rank
	}
	return 4
}

// SortFindings sorts by severity (critical first), then confidence (highest first).
func SortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		si := severityRank(findings[i].Severity)
		sj := severityRank(findings[j].Severity)
		if si != sj {
			return si < sj
		}
		return findings[i].Confidence > findings[j].Confidence
	})
}
