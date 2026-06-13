package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// detachedEnv marks the re-exec'd child so it doesn't detach again.
const detachedEnv = "RIVAL_DETACHED"

// maybeDetach re-execs rival into its own process session (setsid) and exits
// the parent immediately, printing the child PID to stderr as
// "rival: detached pid=N".
//
// Why: Claude Code skills launch rival from shells it tears down with a
// process-group kill when the skill's forked context ends — even mid-review,
// even while queued (the v3.13.0 background-shell pattern died exactly this
// way). A setsid'd child lives in its own session and process group, so the
// teardown cannot reach it; the launching shell returns instantly and callers
// poll the printed PID instead of holding a long-lived shell.
//
// Stdin/stdout/stderr file descriptors are inherited verbatim, so redirects
// like `< prompt > out 2> err` keep working in the child. Intended for
// non-interactive use (skills); a TTY-attached child would lose its
// controlling terminal.
//
// Called from the root PersistentPreRun before any session/queue side effects.
func detachIfRequested(detach bool) {
	if !detach || os.Getenv(detachedEnv) == "1" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "rival: detach failed: %v\n", err)
		os.Exit(1)
	}
	child := exec.Command(exe, os.Args[1:]...)
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Env = append(os.Environ(), detachedEnv+"=1")
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "rival: detach failed: %v\n", err)
		os.Exit(1)
	}
	if _, err := fmt.Fprintf(os.Stderr, "rival: detached pid=%d\n", child.Process.Pid); err != nil {
		// The caller can never learn the PID — don't leave an untrackable
		// child behind.
		_ = child.Process.Kill()
		os.Exit(1)
	}
	os.Exit(0)
}
