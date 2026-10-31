//go:build !windows

package runnerclient

import (
	"os/exec"
	"syscall"
	"time"
)

// Native CLIs may launch a wrapper and a separate runtime. Cancel the whole
// process group so timeouts do not leave the runtime holding output pipes.
func configureCommandCancellation(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
	command.WaitDelay = 3 * time.Second
}
