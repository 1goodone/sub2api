//go:build linux

package reauthruntime

import (
	"os/exec"
	"syscall"
	"time"
)

func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
	cmd.Cancel = func() error { return syscall.Kill(cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 7 * time.Second
}
