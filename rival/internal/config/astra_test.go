package config

import "testing"

// Astra and Sol share the codex adapter, so EngineLabel's `cli == "codex"`
// fallback would label every Astra run "sol" if the exact id were not matched
// first. This is the same collision that once labelled Grok as K3.
func TestAstraIsNotLabelledSol(t *testing.T) {
	if got := EngineLabel("codex", AstraModel); got != AstraLabel {
		t.Errorf("EngineLabel(codex, astra) = %q, want %q", got, AstraLabel)
	}
	if got := EngineLabel("codex", GPT56SolModel); got != SolLabel {
		t.Errorf("EngineLabel(codex, sol) = %q, want %q", got, SolLabel)
	}
	if got := ModelLabel(AstraModel); got != AstraLabel {
		t.Errorf("ModelLabel(astra) = %q, want %q", got, AstraLabel)
	}
}

// The concrete id must never reach a public log, and normalization must be
// idempotent.
func TestAstraIDNormalizesToItsLabel(t *testing.T) {
	raw := "banner from " + AstraModel + " done"
	once := PublicRuntimeLog("codex", AstraModel, raw)
	if contains(once, AstraModel) {
		t.Errorf("concrete astra id leaked: %q", once)
	}
	if !contains(once, AstraLabel) {
		t.Errorf("astra not normalized: %q", once)
	}
	if twice := PublicRuntimeLog("codex", AstraModel, once); twice != once {
		t.Errorf("not idempotent:\nonce:  %q\ntwice: %q", once, twice)
	}
}

// The user asked for xhigh, and an unknown effort label would be rejected by
// config validation.
func TestAstraDefaultsToXhigh(t *testing.T) {
	if got := DefaultEffortForModel(AstraModel); got != "xhigh" {
		t.Errorf("default effort = %q, want xhigh", got)
	}
	if !knownEffortModel(AstraLabel) {
		t.Error("efforts.astra would be rejected as an unknown model")
	}
}

// The command path passes its own fallback, which short-circuits
// builtinModelEffort — the direct DefaultEffortForModel test above does not
// cover it, and an early build regressed here.
func TestAstraCommandPathKeepsXhigh(t *testing.T) {
	got, err := ResolveEffort(AstraModel, "", "")
	if err != nil {
		t.Fatalf("ResolveEffort: %v", err)
	}
	if got != "xhigh" {
		t.Errorf("command-path effort = %q, want xhigh", got)
	}
}

// -m astra must select the codex adapter with Astra's concrete id.
func TestAstraSelectableInMegareview(t *testing.T) {
	targets, err := ResolveReviewTargets([]string{"astra"})
	if err != nil {
		t.Fatalf("ResolveReviewTargets: %v", err)
	}
	if len(targets) != 1 || targets[0].Model != AstraModel || targets[0].CLI != "codex" {
		t.Fatalf("got %+v", targets)
	}
	if targets[0].Prompt != PromptBugHunter {
		t.Errorf("astra should run the bug-hunter lens, got %v", targets[0].Prompt)
	}
}
