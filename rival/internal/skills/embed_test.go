package skills

import (
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
		"version: 3.20.0",
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
