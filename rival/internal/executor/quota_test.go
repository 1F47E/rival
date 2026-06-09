package executor

import "testing"

func TestIsQuotaExhausted(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{
			name: "agy 429 envelope",
			in:   `E0601 23:12:44 log.go:398] agent executor error: RESOURCE_EXHAUSTED (code 429): Individual quota reached. Contact your administrator to enable overages. Resets in 154h30m51s.`,
			want: true,
		},
		{
			name: "individual quota reached",
			in:   "Individual quota reached. Contact your administrator.",
			want: true,
		},
		{
			name: "enable overages hint",
			in:   "please enable overages to continue",
			want: true,
		},
		{
			name: "openai insufficient_quota",
			in:   `{"error":{"code":"insufficient_quota","message":"You exceeded your current quota"}}`,
			want: true,
		},
		{
			name: "rate_limit_exceeded",
			in:   "error: rate_limit_exceeded, retry later",
			want: true,
		},
		{
			name: "case insensitive",
			in:   "resource_exhausted (CODE 429)",
			want: true,
		},
		{
			name: "empty output is not quota",
			in:   "",
			want: false,
		},
		{
			name: "normal review output is not quota",
			in:   "[HIGH] handler.go:42 returns 429 to the client when the rate limit is hit; consider backoff.",
			want: false,
		},
		{
			name: "bare 429 in findings is not misclassified",
			in:   "The endpoint should return HTTP 429 (Too Many Requests) on rate limit, but currently returns 500.",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsQuotaExhausted(tt.in); got != tt.want {
				t.Errorf("IsQuotaExhausted(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
