package cmd

import (
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestReviewCommandsExposeModelFlag(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags *pflag.FlagSet
	}{
		{name: "rival review", flags: reviewCmd.Flags()},
		{name: "rival command megareview", flags: commandMegareviewCmd.Flags()},
	} {
		flag := tc.flags.Lookup("model")
		if flag == nil {
			t.Fatalf("%s has no --model flag", tc.name)
		}
		if flag.Shorthand != "m" {
			t.Errorf("%s --model shorthand = %q, want m", tc.name, flag.Shorthand)
		}
	}
}

func TestReviewCommandDefaultsToHighEffort(t *testing.T) {
	flag := reviewCmd.Flags().Lookup("effort")
	if flag == nil {
		t.Fatal("review command has no --effort flag")
	}
	if flag.DefValue != config.DefaultReviewEffort {
		t.Fatalf("review effort default = %q, want %q", flag.DefValue, config.DefaultReviewEffort)
	}
	if flag.DefValue != "high" {
		t.Fatalf("review effort default = %v, want high", flag)
	}
}

func TestMegareviewUsageNamesModelsAndUltra(t *testing.T) {
	for _, want := range []string{"gpt-5.6-sol", "DeepSeek V4 Pro", "Kimi K2.7 Code", "GLM-5.2", "high (default)", "ultra"} {
		if !strings.Contains(megareviewUsage, want) {
			t.Errorf("megareview usage missing %q", want)
		}
	}
}

func TestModelSelectionFlag_RejectsExplicitEmpty(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringSliceP("model", "m", nil, "models")
	if err := cmd.ParseFlags([]string{"--model="}); err != nil {
		t.Fatal(err)
	}
	_, changed, err := modelSelectionFlag(cmd)
	if !changed {
		t.Fatal("explicit --model= was not detected as changed")
	}
	if err == nil || !strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("expected actionable empty-model error, got %v", err)
	}
}

func TestModelSelectionFlag_ParsesCommaList(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringSliceP("model", "m", nil, "models")
	if err := cmd.ParseFlags([]string{"-m", "deepseek,kimi"}); err != nil {
		t.Fatal(err)
	}
	models, changed, err := modelSelectionFlag(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || len(models) != 2 || models[0] != "deepseek" || models[1] != "kimi" {
		t.Fatalf("unexpected selection: changed=%v models=%v", changed, models)
	}
}
