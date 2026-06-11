//go:build !darwin && !linux

package procinfo

func startNanos(pid int) (int64, bool) {
	return 0, false
}
