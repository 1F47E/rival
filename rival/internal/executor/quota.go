package executor

import "strings"

// quotaSignatures are high-precision substrings that indicate a reviewer CLI
// hit a provider quota/rate limit. Some providers report these failures only
// in captured output, so matching is case-insensitive against the combined
// stdout+stderr log.
//
// These are deliberately specific to the provider error envelopes (not bare
// tokens like "429" or "rate limit") so a reviewer legitimately *describing*
// such a bug in its findings does not get misclassified as quota-exhausted.
var quotaSignatures = []string{
	"resource_exhausted (code 429)",
	"individual quota reached",
	"quota reached. contact your administrator",
	"enable overages",
	"insufficient_quota",
	"rate_limit_exceeded",
	"error 429 (too many requests)",
	"usage limit reached. upgrade",
}

// IsQuotaExhausted reports whether the captured CLI output indicates the
// provider rejected the request due to a quota/rate limit.
func IsQuotaExhausted(output string) bool {
	lower := strings.ToLower(output)
	for _, sig := range quotaSignatures {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}
