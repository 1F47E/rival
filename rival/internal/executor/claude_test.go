package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeAuthHint(t *testing.T) {
	writeLog := func(t *testing.T, content string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "run.log")
		if err := os.WriteFile(p, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	tests := []struct {
		name    string
		log     string
		envAuth string
		envKey  string
		want    string // substring; "" = no hint
	}{
		{name: "credit balance, sub mode", log: "Credit balance is too low", envAuth: "", want: "subscription auth failed"},
		{name: "credit balance, api mode", log: "Credit balance is too low", envAuth: "api", envKey: "sk-x", want: "API auth failed"},
		{name: "login prompt", log: "Please run /login to continue", envAuth: "", want: "subscription auth failed"},
		{name: "invalid key", log: "Invalid API key provided", envAuth: "api", envKey: "sk-x", want: "API auth failed"},
		{name: "model failure is not auth", log: "model overloaded, retry later", envAuth: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RIVAL_CLAUDE_AUTH", tt.envAuth)
			t.Setenv("ANTHROPIC_API_KEY", tt.envKey)
			got := ClaudeAuthHint(writeLog(t, tt.log))
			if tt.want == "" {
				if got != "" {
					t.Fatalf("want no hint, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("hint %q does not contain %q", got, tt.want)
			}
		})
	}

	t.Run("missing log file", func(t *testing.T) {
		if got := ClaudeAuthHint(filepath.Join(t.TempDir(), "nope.log")); got != "" {
			t.Errorf("want empty, got %q", got)
		}
	})
}
