//go:build darwin

package procinfo

import "golang.org/x/sys/unix"

func startNanos(pid int) (int64, bool) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kp == nil {
		return 0, false
	}
	tv := kp.Proc.P_starttime // unix.Timeval: process creation time
	return tv.Sec*1_000_000_000 + int64(tv.Usec)*1000, true
}
