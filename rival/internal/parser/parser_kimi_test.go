package parser

import "testing"

// Every advertised effort must parse — the value is ignored downstream (K3
// runs max only), so rejecting "max"/"ultra" while the docs say "pinned to
// max regardless of -re" would be a trap.
func TestParseKimiArgsAcceptsAndIgnoresAllEffortNames(t *testing.T) {
	for _, effort := range kimiEffortNames {
		parsed, err := ParseKimiArgs("-re " + effort + " hello")
		if err != nil {
			t.Fatalf("-re %s: %v", effort, err)
		}
		if parsed.Effort != effort {
			t.Errorf("-re %s: parsed effort %q", effort, parsed.Effort)
		}
		if parsed.Prompt != "hello" {
			t.Errorf("-re %s: prompt %q, want hello", effort, parsed.Prompt)
		}
	}
}

func TestParseKimiArgsRejectsUnknownEffort(t *testing.T) {
	if _, err := ParseKimiArgs("-re bogus hello"); err == nil {
		t.Error("expected unknown effort to error")
	}
}

func TestParseKimiArgsDefaultsToMax(t *testing.T) {
	parsed, err := ParseKimiArgs("review src/")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Effort != "max" {
		t.Errorf("default effort = %q, want max", parsed.Effort)
	}
	if !parsed.IsReview || parsed.ReviewScope != "src/" {
		t.Errorf("review parse broken: %+v", parsed)
	}
}
