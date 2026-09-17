//go:build !windows

package supervisor

import (
	"os/exec"
	"syscall"
)

func configureProcess(cmd *exec.Cmd)  { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func stopProcess(cmd *exec.Cmd) error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

func containProcess(cmd *exec.Cmd) (func(), error) { return func() { _ = stopProcess(cmd) }, nil }
