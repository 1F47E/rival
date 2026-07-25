package review

import (
	"reflect"
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/executor"
)

// The reviewer/judge/preflight switches used to be inline, so a new CLI could
// silently land in the default (error) arm. These tests pin each switch's
// dispatch table by name rather than executing a provider binary.

func TestReviewDispatch_KnownCLIs(t *testing.T) {
	for _, cli := range []string{"codex", "opencode", config.GrokLabel} {
		t.Run(cli, func(t *testing.T) {
			if _, ok := preflightFor(cli); !ok {
				t.Errorf("preflightFor(%q) fell into the unsupported arm", cli)
			}
			if _, ok := reviewerRunnerFor(cli); !ok {
				t.Errorf("reviewerRunnerFor(%q) fell into the unsupported arm", cli)
			}
			if _, ok := judgeRunnerFor(cli); !ok {
				t.Errorf("judgeRunnerFor(%q) fell into the unsupported arm", cli)
			}
		})
	}
}

func TestReviewDispatch_UnknownCLIIsUnsupported(t *testing.T) {
	for _, cli := range []string{"", "claude", "gemini", "grok-4.5"} {
		t.Run(cli, func(t *testing.T) {
			if _, ok := preflightFor(cli); ok {
				t.Errorf("preflightFor(%q) unexpectedly resolved", cli)
			}
			if _, ok := reviewerRunnerFor(cli); ok {
				t.Errorf("reviewerRunnerFor(%q) unexpectedly resolved", cli)
			}
			if _, ok := judgeRunnerFor(cli); ok {
				t.Errorf("judgeRunnerFor(%q) unexpectedly resolved", cli)
			}
		})
	}
}

// Grok's preflight arm must be grok's own, not another CLI's. The binary is not
// installed in CI, so the error text is the observable proof of which preflight
// ran.
func TestReviewDispatch_GrokPreflightIsGroksOwn(t *testing.T) {
	preflight, ok := preflightFor(config.GrokLabel)
	if !ok {
		t.Fatal("grok has no preflight arm")
	}
	err := preflight(config.GrokModel, "")
	if err == nil {
		// A machine with grok installed and authenticated: nothing to assert
		// beyond the arm resolving, which the check above already did.
		t.Skip("grok runtime is installed and authenticated on this machine")
	}
	if !strings.Contains(err.Error(), config.GrokLabel) {
		t.Errorf("grok preflight error %q does not come from the grok arm", err)
	}
}

// The consilium contract is "the session carries the concrete model to judge
// with; the adapter falls back to its default if that is empty". The grok
// adapter must honor it like the opencode one, so it must not discard its model
// argument. Argv itself is asserted in the executor package
// (TestGrokRunArgs_ThreadsExplicitModel); here we pin that the adapter is wired
// to the model-taking entry point rather than the default-model wrapper.
func TestGrokAdapter_UsesModelTakingEntryPoint(t *testing.T) {
	if grokReviewRun == nil {
		t.Fatal("grok adapter has no run function")
	}
	if reflect.ValueOf(grokReviewRun).Pointer() != reflect.ValueOf(executor.RunGrokModel).Pointer() {
		t.Error("grok adapter is not wired to executor.RunGrokModel, so its model argument is dropped")
	}
}

// The grok reviewer/judge arms must clamp rival's effort ladder onto grok's
// low/medium/high before the run, and reject anything unclampable, so a session
// never records an effort grok was not sent.
func TestGrokReviewEffort_ClampsLadder(t *testing.T) {
	cases := []struct{ in, want string }{
		{"low", "low"},
		{"medium", "medium"},
		{"high", "high"},
		{"xhigh", "high"},
		{"ultra", "high"},
		{"max", "high"},
		{"minimal", "low"},
	}
	for _, tc := range cases {
		got, err := grokReviewEffort(tc.in)
		if err != nil {
			t.Errorf("grokReviewEffort(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("grokReviewEffort(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if _, err := grokReviewEffort("bogus"); err == nil {
		t.Error("grokReviewEffort accepted an effort grok cannot run")
	}
}

// reviewerEffortFor is what newReviewerSession records; for grok it must equal
// the clamped value actually handed to the executor.
func TestReviewerEffortFor_GrokRecordsClampedEffort(t *testing.T) {
	got, err := reviewerEffortFor(config.GrokLabel, config.GrokModel, "ultra")
	if err != nil {
		t.Fatalf("reviewerEffortFor(grok, ultra): %v", err)
	}
	if got != "high" {
		t.Fatalf("grok reviewer effort = %q, want the clamped %q", got, "high")
	}
	sent, err := grokReviewEffort(got)
	if err != nil {
		t.Fatalf("clamped effort %q was rejected by the executor clamp: %v", got, err)
	}
	if sent != got {
		t.Fatalf("recorded effort %q != effort sent to grok %q", got, sent)
	}
}

// Non-grok reviewers keep resolving through config only.
func TestReviewerEffortFor_SolUnchanged(t *testing.T) {
	got, err := reviewerEffortFor("codex", config.GPT56SolModel, "ultra")
	if err != nil {
		t.Fatal(err)
	}
	want, err := config.ResolveEffort(config.GPT56SolModel, "ultra", config.DefaultReviewEffort)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("sol reviewer effort = %q, want %q", got, want)
	}
}

func TestReviewerEffortFor_PropagatesConfigError(t *testing.T) {
	if _, err := reviewerEffortFor("codex", config.GPT56SolModel, "bogus"); err == nil {
		t.Error("an invalid effort override must not be silently accepted")
	}
}
