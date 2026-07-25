package skills

import (
	"slices"
	"strings"
	"testing"
)

func TestPlanSolSkillPinsUltraEffort(t *testing.T) {
	data, err := Files.ReadFile("rival-plan-sol/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"version: ",
		"argument-hint: \"<path-to-plan.md>\"",
		"rival command plan --model sol --effort ultra --detach",
		"Always run at **ultra**",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("plan-sol skill missing %q", want)
		}
	}
	for _, forbidden := range []string{"defaults to **high**", "[-re high|ultra]"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("plan-sol skill still advertises optional effort %q", forbidden)
		}
	}
}

func TestGrokSkillIsEmbedded(t *testing.T) {
	const name = "rival-grok"
	if !slices.Contains(Names, name) {
		t.Fatalf("grok skill %q is not active", name)
	}
	if slices.Contains(Deprecated, name) {
		t.Fatalf("grok skill %q is deprecated", name)
	}
	data, err := Files.ReadFile(name + "/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"version: ",
		"name: rival-grok",
		"rival command grok --detach --workdir",
		"rival wait --log <rival_err>",
		"grok login",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("grok skill missing %q", want)
		}
	}
	// A hardcoded watcher timeout re-introduces the bound that
	// RIVAL_QUEUE_TIMEOUT/RIVAL_RUN_TIMEOUT already own.
	if strings.Contains(content, "rival wait --log <rival_err> --timeout") {
		t.Error("grok skill hardcodes a --timeout on the watcher")
	}
}

func TestPlanSkillRunsBothModelsAtUltra(t *testing.T) {
	const name = "rival-plan"
	if !slices.Contains(Names, name) {
		t.Fatalf("paired plan skill %q is not active", name)
	}
	if slices.Contains(Deprecated, name) {
		t.Fatalf("paired plan skill %q is still deprecated", name)
	}
	data, err := Files.ReadFile(name + "/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"name: rival-plan",
		"Sol and Fable",
		"rival command plan --model sol,fable --effort ultra --detach",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("paired plan skill missing %q", want)
		}
	}
}
