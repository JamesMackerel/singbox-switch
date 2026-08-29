//go:build !windows

package app

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateChild(process *os.Process) error {
	return syscall.Kill(-process.Pid, syscall.SIGTERM)
}

func acceptableStopError(err error) error {
	var exitErr *exec.ExitError
	if err == nil || errors.As(err, &exitErr) {
		return nil
	}
	return err
}
