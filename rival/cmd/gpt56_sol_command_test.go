package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
	"github.com/spf13/cobra"
)

func TestSolCommandsArePublicAndDeferToConfiguredEffort(t *testing.T) {
	if commandSolCmd.Use != config.SolLabel || commandSolCmd.Hidden {
		t.Fatalf("command metadata = use %q hidden %v", commandSolCmd.Use, commandSolCmd.Hidden)
	}
	if runSolCmd.Use != config.SolLabel || runSolCmd.Hidden {
		t.Fatalf("run metadata = use %q hidden %v", runSolCmd.Use, runSolCmd.Hidden)
	}

	effort := runSolCmd.Flags().Lookup("effort")
	if effort == nil || effort.DefValue != "" {
		t.Fatalf("run effort default = %v, want configured-default sentinel", effort)
	}
}

func TestLegacyStandaloneCommandsAreHidden(t *testing.T) {
	if !commandGPT56SolCmd.Hidden || !runGPT56SolCmd.Hidden || !commandCodexCmd.Hidden || !runCodexCmd.Hidden {
		t.Fatal("legacy standalone commands must stay hidden")
	}
	if !commandClaudeCmd.Hidden || !runClaudeCmd.Hidden {
		t.Fatal("legacy opus adapter commands must stay hidden")
	}
}

func TestLegacyStandaloneHelpUsesOnlyPublicNames(t *testing.T) {
	tests := []struct {
		name      string
		alias     *cobra.Command
		want      string
		forbidden []string
	}{
		{"versioned Sol command", commandGPT56SolCmd, "rival command sol", []string{"gpt-5.6"}},
		{"versioned Sol run", runGPT56SolCmd, "rival run sol", []string{"gpt-5.6"}},
		{"legacy Sol command adapter", commandCodexCmd, "rival command sol", []string{"codex", "gpt-5.6"}},
		{"legacy Sol run adapter", runCodexCmd, "rival run sol", []string{"codex", "gpt-5.6"}},
		{"legacy Opus command adapter", commandClaudeCmd, "rival command opus", []string{"claude"}},
		{"legacy Opus run adapter", runClaudeCmd, "rival run opus", []string{"claude"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			tt.alias.SetOut(&output)
			tt.alias.SetErr(&output)
			defer tt.alias.SetOut(nil)
			defer tt.alias.SetErr(nil)

			tt.alias.HelpFunc()(tt.alias, nil)
			got := strings.ToLower(output.String())
			if !strings.Contains(got, tt.want) {
				t.Fatalf("help = %q, want public usage %q", got, tt.want)
			}
			for _, forbidden := range tt.forbidden {
				if strings.Contains(got, forbidden) {
					t.Fatalf("help exposes hidden name %q: %q", forbidden, got)
				}
			}
		})
	}
}

func TestSolUsageUsesOnlyPublicModelNaming(t *testing.T) {
	lower := strings.ToLower(solUsage)
	if !strings.Contains(lower, "/rival-sol") {
		t.Fatal("usage must name /rival-sol")
	}
	for _, hidden := range []string{"codex", "gpt-5.6", "rival-gpt"} {
		if strings.Contains(lower, hidden) {
			t.Fatalf("usage exposes hidden runtime/model name %q", hidden)
		}
	}
	for _, want := range []string{"built-in: high", "ultra"} {
		if !strings.Contains(lower, want) {
			t.Fatalf("usage missing %q", want)
		}
	}
}

func TestOpusCommandsArePublic(t *testing.T) {
	if commandOpusCmd.Use != config.OpusLabel || commandOpusCmd.Hidden {
		t.Fatalf("command metadata = use %q hidden %v", commandOpusCmd.Use, commandOpusCmd.Hidden)
	}
	if runOpusCmd.Use != config.OpusLabel || runOpusCmd.Hidden {
		t.Fatalf("run metadata = use %q hidden %v", runOpusCmd.Use, runOpusCmd.Hidden)
	}
	if lower := strings.ToLower(opusUsage); strings.Contains(lower, "claude") || !strings.Contains(lower, "rival run opus") || !strings.Contains(lower, "built-in: xhigh") {
		t.Fatalf("opus usage exposes an adapter name or lacks public name: %q", lower)
	}
}
