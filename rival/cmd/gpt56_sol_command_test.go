package cmd

import (
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
)

func TestGPT56SolCommandsArePublicAndDefaultHigh(t *testing.T) {
	if commandGPT56SolCmd.Use != config.GPT56SolModel || commandGPT56SolCmd.Hidden {
		t.Fatalf("command metadata = use %q hidden %v", commandGPT56SolCmd.Use, commandGPT56SolCmd.Hidden)
	}
	if runGPT56SolCmd.Use != config.GPT56SolModel || runGPT56SolCmd.Hidden {
		t.Fatalf("run metadata = use %q hidden %v", runGPT56SolCmd.Use, runGPT56SolCmd.Hidden)
	}

	effort := runGPT56SolCmd.Flags().Lookup("effort")
	if effort == nil || effort.DefValue != "high" {
		t.Fatalf("run effort default = %v, want high", effort)
	}
}

func TestLegacyStandaloneCommandsAreHidden(t *testing.T) {
	if !commandCodexCmd.Hidden || !runCodexCmd.Hidden {
		t.Fatal("legacy standalone commands must stay hidden")
	}
}

func TestGPT56SolUsageUsesOnlyModelNaming(t *testing.T) {
	lower := strings.ToLower(gpt56SolUsage)
	if !strings.Contains(lower, config.GPT56SolModel) {
		t.Fatalf("usage must name %s", config.GPT56SolModel)
	}
	if strings.Contains(lower, "codex") {
		t.Fatal("usage exposes a runtime name instead of the model name")
	}
	for _, want := range []string{"high (default)", "ultra"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("usage missing %q", want)
		}
	}
}
