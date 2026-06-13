package executor

import (
	"context"
	"testing"
	"time"

	"github.com/1F47E/rival/internal/session"
)

// TestRunSubprocess_ContextTimeoutKillsChild proves RIVAL_RUN_TIMEOUT's
// mechanism: a context deadline kills a hung child promptly instead of waiting
// for it to finish. Uses /bin/sleep so no provider quota is touched.
func TestRunSubprocess_ContextTimeoutKillsChild(t *testing.T) {
	// Keep all session file writes inside a temp HOME.
	t.Setenv("HOME", t.TempDir())

	sess, err := session.New("test", "raw", "none", "low", t.TempDir(), "", "", "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	// sleep would run 5s; the 100ms deadline must cut it short.
	result, runErr := RunSubprocess(ctx, sess, "sleep", []string{"5"}, nil, "", nil)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("expected child killed promptly, but it ran %v", elapsed)
	}
	if ctx.Err() != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", ctx.Err())
	}
	// A killed child surfaces as either a non-nil error or a nonzero exit code;
	// the contract we care about is "did not run to completion".
	if runErr == nil && result != nil && result.ExitCode == 0 {
		t.Errorf("expected nonzero/killed result, got clean exit 0")
	}
}
