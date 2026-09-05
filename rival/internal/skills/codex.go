package skills

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Codex renders host-specific instructions while keeping each skill's identity,
// description, and version sourced from the shipped Claude skill.
func Codex(name string, source []byte) ([]byte, error) {
	parts := strings.SplitN(string(source), "---", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("missing skill frontmatter: %s", name)
	}
	var meta struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		Version     string `yaml:"version"`
		Hint        string `yaml:"argument-hint"`
	}
	if err := yaml.Unmarshal([]byte(parts[1]), &meta); err != nil {
		return nil, err
	}
	if meta.Name != name || meta.Description == "" || meta.Version == "" {
		return nil, fmt.Errorf("incomplete skill metadata: %s", name)
	}
	command := strings.TrimPrefix(name, "rival-")
	input := "Pass the user's arguments verbatim. With no arguments, show usage without starting a run. For code reviews, use the review keyword before the scope."
	switch name {
	case "rival-review":
		command = "megareview"
		input = "Pass the user's arguments verbatim. Empty input reviews git-detected changes. Options: -m/--model and -re/--effort. Preserve the explicit model roster; do not add reviewers."
	case "rival-fable":
		input = "Normalize input to [-re effort] review [scope]. Add review after an optional -re value unless already present. Empty arguments become review (git-detected changes)."
	case "rival-plan", "rival-plan-sol":
		models := "sol,fable"
		if name == "rival-plan-sol" {
			models = "sol"
		}
		command = "plan --model " + models + " --effort xhigh"
		input = "Input is one plan/spec Markdown file path, required. Pass it verbatim. This skill pins xhigh effort."
	case "rival-plan-fable":
		command = "plan --model fable"
		input = "Input is [-re effort] followed by one required plan/spec Markdown file path. Omitted effort uses configured Fable effort, with the plan command's low fallback."
	case "rival-antislop":
		input = "Pass arguments verbatim; empty input reviews git-detected changes. Options: -m sol|fable and -re/--effort. This is quality-only review, not a bug hunt."
	case "rival-security":
		input = "Input is a scope; empty input reviews git-detected changes. First run rival command security --which --workdir <repo>. Stop on failure; never substitute another security reviewer."
	case "rival-sol", "rival-astra", "rival-k3", "rival-grok":
	default:
		return nil, fmt.Errorf("no Codex command mapping for %s", name)
	}
	data, err := Files.ReadFile("codex.md.tmpl")
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("codex").Parse(string(data))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	err = tmpl.Execute(&out, map[string]string{
		"Name": name, "Description": strings.ReplaceAll(meta.Description, "/"+name, "$"+name),
		"Version": meta.Version, "Hint": meta.Hint, "Command": command, "Input": input,
	})
	return out.Bytes(), err
}
