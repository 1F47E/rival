package executor

import (
	"strings"
	"testing"

	"github.com/1F47E/rival/internal/config"
)

func TestGrokEffort(t *testing.T) {
	cases := []struct {
		effort  string
		want    string
		wantErr bool
	}{
		{effort: "low", want: "low"},
		{effort: "medium", want: "medium"},
		{effort: "high", want: "high"},
		// grok-4.5 has no reasoning level above high; rival's richer menu clamps down.
		{effort: "xhigh", want: "high"},
		{effort: "ultra", want: "high"},
		{effort: "max", want: "high"},
		{effort: "minimal", want: "low"},
		{effort: "none", want: "low"},
		{effort: "", wantErr: true},
		{effort: "turbo", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.effort, func(t *testing.T) {
			got, err := grokEffort(tc.effort)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("effort %q was accepted, got %q", tc.effort, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("effort %q rejected: %v", tc.effort, err)
			}
			if got != tc.want {
				t.Fatalf("effort %q mapped to %q, want %q", tc.effort, got, tc.want)
			}
		})
	}
}

func TestGrokRunArgs_AlwaysPassesPromptFileAndRuntimeFlags(t *testing.T) {
	args, err := grokRunArgs(config.GrokModel, "/tmp/rival-grok-1.md", "high", "", false)
	if err != nil {
		t.Fatalf("grokRunArgs: %v", err)
	}
	joined := strings.Join(args, " ")

	for _, want := range []string{
		"--prompt-file /tmp/rival-grok-1.md",
		"-m " + config.GrokModel,
		"--effort high",
		"--output-format plain",
		"--no-auto-update",
		"--yolo",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %s", want, joined)
		}
	}
	// The prompt travels by file, never inline: a bare -p would both duplicate it
	// and expose it in the process table.
	for _, arg := range args {
		if arg == "-p" {
			t.Errorf("args used a bare -p instead of --prompt-file: %s", joined)
		}
	}
	if strings.Contains(joined, "--cwd") {
		t.Errorf("empty workdir must not emit --cwd: %s", joined)
	}
	if strings.Contains(joined, "--sandbox") {
		t.Errorf("raw run must not emit a sandbox flag: %s", joined)
	}
}

func TestGrokRunArgs_WorkdirAndReviewSandbox(t *testing.T) {
	cases := []struct {
		name        string
		workdir     string
		review      bool
		wantCwd     bool
		wantSandbox bool
	}{
		{name: "raw_no_workdir"},
		{name: "raw_with_workdir", workdir: "/repo", wantCwd: true},
		{name: "review_no_workdir", review: true, wantSandbox: true},
		{name: "review_with_workdir", workdir: "/repo", review: true, wantCwd: true, wantSandbox: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args, err := grokRunArgs(config.GrokModel, "/tmp/p.md", "medium", tc.workdir, tc.review)
			if err != nil {
				t.Fatalf("grokRunArgs: %v", err)
			}
			joined := strings.Join(args, " ")

			gotCwd := strings.Contains(joined, "--cwd "+tc.workdir)
			if gotCwd != tc.wantCwd {
				t.Errorf("--cwd %q present=%v, want %v: %s", tc.workdir, gotCwd, tc.wantCwd, joined)
			}
			gotSandbox := strings.Contains(joined, "--sandbox read-only")
			if gotSandbox != tc.wantSandbox {
				t.Errorf("--sandbox read-only present=%v, want %v: %s", gotSandbox, tc.wantSandbox, joined)
			}
		})
	}
}

func TestGrokRunArgs_PropagatesEffortError(t *testing.T) {
	args, err := grokRunArgs(config.GrokModel, "/tmp/p.md", "turbo", "/repo", true)
	if err == nil {
		t.Fatalf("invalid effort was accepted: %v", args)
	}
	if args != nil {
		t.Fatalf("invalid effort returned args: %v", args)
	}
}

func TestGrokFullPrompt_MatchesSharedComposition(t *testing.T) {
	const prompt = "review this diff"
	want := config.SystemPrompt + "\n\n" + config.BuildWorkdirPreamble("/repo") + "\n" + prompt
	if got := grokFullPrompt(prompt, "/repo"); got != want {
		t.Fatalf("grokFullPrompt mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}
