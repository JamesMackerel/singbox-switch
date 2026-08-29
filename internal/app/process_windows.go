//go:build windows

package app

import (
	"errors"
	"os"
	"os/exec"
)

func configureChild(cmd *exec.Cmd) {}

// os.Interrupt is not implemented for Windows processes. Kill is the only
// reliable cross-version primitive available without introducing a console.
func terminateChild(process *os.Process) error { return process.Kill() }

func acceptableStopError(err error) error {
	var exitErr *exec.ExitError
	if err == nil || errors.As(err, &exitErr) {
		return nil
	}
	return err
}
