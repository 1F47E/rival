package cmd

import (
	"testing"
	"time"
)

// The update check must not sit on the startup path. A stale cache makes it
// perform an HTTP GET, which every command used to pay before doing any work.
func TestStartUpdateCheckDoesNotBlockStartup(t *testing.T) {
	start := time.Now()
	startUpdateCheck()
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("startUpdateCheck blocked for %v; it must return immediately", elapsed)
	}
	if updateCheckDone == nil {
		t.Fatal("startUpdateCheck did not record a completion channel")
	}
	waitForUpdateCheck()
}

// The join is bounded, so a hung check delays exit by at most its own budget.
func TestWaitForUpdateCheckIsBounded(t *testing.T) {
	updateCheckDone = make(chan struct{}) // never closed: simulates a hung check
	defer func() { updateCheckDone = nil }()

	start := time.Now()
	waitForUpdateCheck()
	elapsed := time.Since(start)

	if elapsed < updateCheckWait {
		t.Errorf("join returned after %v, before its %v budget", elapsed, updateCheckWait)
	}
	if elapsed > updateCheckWait+time.Second {
		t.Errorf("join took %v, well past its %v budget", elapsed, updateCheckWait)
	}
}

// A command that never started a check must not wait at all.
func TestWaitForUpdateCheckWithoutStartReturnsImmediately(t *testing.T) {
	updateCheckDone = nil
	start := time.Now()
	waitForUpdateCheck()
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("join waited %v with no check running", elapsed)
	}
}
