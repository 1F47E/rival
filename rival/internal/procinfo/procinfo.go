// Package procinfo provides PID liveness checks that are robust against PID
// reuse by pairing a PID with its process start time. After a process dies the
// OS may recycle its PID; comparing start times distinguishes the original
// process from an unrelated one that later inherited the number.
package procinfo

import "syscall"

// StartNanos returns the start time (Unix nanoseconds) of the process with the
// given pid, or (0, false) if it does not exist or cannot be inspected.
// Implemented per-platform (start_darwin.go, start_linux.go, start_other.go).
func StartNanos(pid int) (int64, bool) {
	return startNanos(pid)
}

// Alive reports whether pid is live AND is the same process that recorded
// wantStart. If wantStart is 0 (not recorded, or an unsupported platform) it
// degrades to a bare existence check. A recycled PID belongs to a process with
// a different start time, so this returns false for it — defeating PID reuse.
func Alive(pid int, wantStart int64) bool {
	if pid <= 0 || syscall.Kill(pid, 0) != nil {
		return false
	}
	if wantStart == 0 {
		return true // nothing to compare against — best effort
	}
	got, ok := StartNanos(pid)
	if !ok {
		return true // can't read start time right now — don't false-reap a live PID
	}
	return got == wantStart
}
