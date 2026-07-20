package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadSummaryFileSkipsLargePrompt(t *testing.T) {
	stored := Session{
		ID:            "summary-id",
		GroupID:       "summary-group",
		CLI:           "opencode",
		Mode:          "megareview",
		Model:         "test-model",
		Effort:        "high",
		Prompt:        strings.Repeat("large prompt ", 20000),
		PromptPreview: "large prompt preview",
		Status:        "running",
		StartTime:     time.Now(),
		WorkDir:       "/tmp/work",
		LogFile:       "/tmp/internal.log",
		PID:           123,
		PIDStart:      456,
		OwnerPID:      789,
		OwnerPIDStart: 101112,
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "session.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	got, err := LoadSummaryFile(path, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got.Prompt != "" {
		t.Fatalf("summary retained %d prompt bytes", len(got.Prompt))
	}
	if got.ID != stored.ID || got.GroupID != stored.GroupID || got.PromptPreview != stored.PromptPreview {
		t.Fatalf("summary identity = %+v", got)
	}
	if got.PID != stored.PID || got.PIDStart != stored.PIDStart || got.OwnerPID != stored.OwnerPID || got.OwnerPIDStart != stored.OwnerPIDStart {
		t.Fatalf("summary process metadata = %+v", got)
	}
	if got.LogFile != stored.LogFile || !got.StartTime.Equal(stored.StartTime) {
		t.Fatalf("summary runtime metadata = %+v", got)
	}
}
