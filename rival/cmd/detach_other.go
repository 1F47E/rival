//go:build windows

package cmd

import "os/exec"

// setDetachAttr is a no-op on Windows: syscall.SysProcAttr has no Setsid field.
// rival targets darwin+linux for releases; this keeps the package compilable
// for GOOS=windows tooling/CI rather than providing real detach semantics.
func setDetachAttr(*exec.Cmd) {}
