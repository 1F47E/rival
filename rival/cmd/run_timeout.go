package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/1F47E/rival/internal/config"
)

// runTimeoutFailMsg returns the session-failure message for a run that errored.
// When the run's context hit the RIVAL_RUN_TIMEOUT deadline, it produces a
// clear "run timeout" message so a hung provider CLI is distinguishable from a
// normal failure; otherwise it returns fallback (the provider's own error).
//
// runCtx is the timeout-bounded context passed to the executor. cause is the
// error returned by the executor call (may be nil when only the exit code was
// nonzero — a hung CLI killed by ctx cancellation usually surfaces as a
// nonzero exit, so the runCtx.Err() check covers both).
func runTimeoutFailMsg(runCtx context.Context, fallback string) string {
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return fmt.Sprintf("run timeout after %s (RIVAL_RUN_TIMEOUT) — provider CLI did not finish", config.RunTimeout())
	}
	return fallback
}
