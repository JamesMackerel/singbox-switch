package app

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"time"
)

type managedProcess struct {
	cmd  *exec.Cmd
	done chan error
}

func startSingBox(binary, config string, stdout, stderr io.Writer, settle time.Duration) (*managedProcess, error) {
	cmd := exec.Command(binary, "run", "-c", config)
	// sing-box commonly has relative paths in its configuration (for example
	// cache-file: cache.db). A service manager may start us from /, which is
	// read-only on macOS and Linux. Keep all relative sing-box paths alongside
	// the selected configuration instead.
	cmd.Dir = filepath.Dir(config)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	configureChild(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start sing-box: %w", err)
	}
	proc := &managedProcess{cmd: cmd, done: make(chan error, 1)}
	go func() { proc.done <- cmd.Wait() }()
	timer := time.NewTimer(settle)
	defer timer.Stop()
	select {
	case err := <-proc.done:
		if err == nil {
			return nil, fmt.Errorf("sing-box exited during startup")
		}
		return nil, fmt.Errorf("sing-box exited during startup: %w", err)
	case <-timer.C:
		return proc, nil
	}
}

func stopSingBox(proc *managedProcess, timeout time.Duration) error {
	if proc == nil || proc.cmd == nil || proc.cmd.Process == nil {
		return nil
	}
	if err := terminateChild(proc.cmd.Process); err != nil {
		_ = proc.cmd.Process.Kill()
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-proc.done:
		return acceptableStopError(err)
	case <-timer.C:
		if err := proc.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("force-kill sing-box after timeout: %w", err)
		}
		<-proc.done
		return fmt.Errorf("sing-box did not stop within %s and was killed", timeout)
	}
}
