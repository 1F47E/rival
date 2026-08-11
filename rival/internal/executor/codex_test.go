package executor

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
)

func TestCodexRunArgs_UsesExplicitSolModelAndEffort(t *testing.T) {
	for _, effort := range []string{"high", "ultra"} {
		t.Run(effort, func(t *testing.T) {
			joined := strings.Join(codexRunArgs(config.GPT56SolModel, effort, "/repo"), " ")
			if !strings.Contains(joined, "-m "+config.GPT56SolModel) {
				t.Fatalf("args do not select %s: %s", config.GPT56SolModel, joined)
			}
			if !strings.Contains(joined, "model_reasoning_effort="+effort) {
				t.Fatalf("args do not preserve effort %s: %s", effort, joined)
			}
			if !strings.Contains(joined, "--sandbox read-only") {
				t.Fatalf("args lost read-only sandbox: %s", joined)
			}
		})
	}
}

func TestRunCodexModelRejectsUnsupportedModel(t *testing.T) {
	result, err := RunCodexModel(
		context.Background(),
		nil,
		"review",
		"high",
		"/repo",
		"retired-model",
		io.Discard,
	)
	if err == nil {
		t.Fatal("unsupported Sol model was accepted")
	}
	if result != nil {
		t.Fatalf("unsupported Sol model returned a result: %#v", result)
	}
}

// Sol exposes ultra as its own reasoning level, distinct from xhigh. Unifying
// the ladder must not alias the two: the value reaches the runtime verbatim.
func TestCodexPassesUltraAndXhighThroughUnaliased(t *testing.T) {
	for _, effort := range []string{"xhigh", "ultra"} {
		args := codexRunArgs(config.GPT56SolModel, effort, "/tmp")
		want := "model_reasoning_effort=" + effort
		if !strings.Contains(strings.Join(args, " "), want) {
			t.Errorf("codex args for %q missing %q: %v", effort, want, args)
		}
	}
}
