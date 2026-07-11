package executor

import (
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
