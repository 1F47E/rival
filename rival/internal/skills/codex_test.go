package skills

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEverySkillHasValidCodexVariant(t *testing.T) {
	for _, name := range Names {
		t.Run(name, func(t *testing.T) {
			data, err := CodexSkill(name, "9.8.7")
			if err != nil {
				t.Fatal(err)
			}
			parts := strings.SplitN(string(data), "---", 3)
			if len(parts) != 3 {
				t.Fatal("missing frontmatter")
			}
			var header struct {
				Name        string            `yaml:"name"`
				Description string            `yaml:"description"`
				Metadata    map[string]string `yaml:"metadata"`
			}
			if err := yaml.Unmarshal([]byte(parts[1]), &header); err != nil {
				t.Fatal(err)
			}
			if header.Name != name || header.Description == "" || header.Metadata["version"] != "9.8.7" {
				t.Fatalf("invalid header: %+v", header)
			}
			for _, unsupported := range []string{"$ARGUMENTS", "run_in_background", "Write tool", "{{COMMAND}}"} {
				if strings.Contains(string(data), unsupported) {
					t.Fatalf("unresolved or host-incompatible instruction %q", unsupported)
				}
			}
		})
	}
	if _, err := CodexSkill("rival-unknown", "1"); err == nil {
		t.Fatal("missing command accepted")
	}
}
