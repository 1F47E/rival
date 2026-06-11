//go:build linux

package procinfo

import (
	"os"
	"strconv"
	"strings"
)

// clockTicksPerSec is the kernel USER_HZ. It is effectively always 100 on
// Linux; we hardcode it because cgo (sysconf(_SC_CLK_TCK)) is disabled here.
const clockTicksPerSec = 100

// startNanos returns the process start time as nanoseconds SINCE BOOT (not
// wall-clock). /proc/<pid>/stat field 22 (starttime) is in clock ticks since
// boot and is fixed for a process's whole lifetime. We deliberately do NOT add
// the system boot time (/proc/stat btime): btime is derived from the wall clock
// and shifts on NTP steps / manual date changes / suspend-resume, so adding it
// would make the same live process report different start times before vs.
// after a clock step — false-reaping a live queue holder. The value only ever
// has to be self-consistent on one running machine, so a boot-relative epoch is
// both sufficient and stable.
func startNanos(pid int) (int64, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}
	// The comm field (2) may contain spaces/parens; everything after the last
	// ')' is space-separated, with starttime as field 22 overall → index 19
	// within the post-')' slice (fields 3..).
	s := string(data)
	rparen := strings.LastIndexByte(s, ')')
	if rparen < 0 {
		return 0, false
	}
	fields := strings.Fields(s[rparen+1:])
	// fields[0] is state (field 3); starttime is field 22 → fields[19].
	if len(fields) < 20 {
		return 0, false
	}
	ticks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return 0, false
	}
	// ticks → ns since boot. +1 so a process that started at exactly tick 0
	// never collides with the (0, false) "unavailable" sentinel.
	return ticks*(1_000_000_000/clockTicksPerSec) + 1, true
}
