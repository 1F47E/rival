//go:build !windows

package cmd

import (
	"os/exec"
	"syscall"
)

// setDetachAttr puts the re-exec'd child in its own session + process group
// (setsid) so a process-group kill of the launching shell cannot reach it.
func setDetachAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
