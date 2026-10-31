package runnerclient

import (
	"os/exec"
	"strconv"
	"time"
)

func configureCommandCancellation(command *exec.Cmd) {
	command.Cancel = func() error {
		err := exec.Command("taskkill", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F").Run()
		if err != nil {
			return command.Process.Kill()
		}
		return nil
	}
	command.WaitDelay = 3 * time.Second
}
