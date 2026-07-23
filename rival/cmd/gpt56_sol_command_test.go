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

func TestFableCommandsArePublic(t *testing.T) {
	if commandFableCmd.Use != config.FableLabel || commandFableCmd.Hidden {
		t.Fatalf("command metadata = use %q hidden %v", commandFableCmd.Use, commandFableCmd.Hidden)
	}
	if runFableCmd.Use != config.FableLabel || runFableCmd.Hidden {
		t.Fatalf("run metadata = use %q hidden %v", runFableCmd.Use, runFableCmd.Hidden)
	}
	if lower := strings.ToLower(fableUsage); !strings.Contains(lower, "/rival-fable") || !strings.Contains(lower, "built-in default: medium") {
		t.Fatalf("fable usage lacks public name or effort fallback: %q", lower)
	}
}

func TestModelCommandParentsRejectUnknownRunnerNames(t *testing.T) {
	for _, parent := range []*cobra.Command{runCmd, commandCmd} {
		if err := parent.ValidateArgs([]string{"retired-runner"}); err == nil {
			t.Fatalf("%s accepted an unknown runner name", parent.CommandPath())
		}
		if err := parent.ValidateArgs(nil); err != nil {
			t.Fatalf("%s rejected an empty invocation: %v", parent.CommandPath(), err)
		}
	}
}
