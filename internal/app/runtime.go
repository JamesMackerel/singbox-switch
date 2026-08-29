package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type RuntimeStatus struct {
	DaemonPID  int       `json:"daemon_pid"`
	ChildPID   int       `json:"child_pid,omitempty"`
	Requested  string    `json:"requested_config,omitempty"`
	Active     string    `json:"active_config,omitempty"`
	ChildState string    `json:"child_state"`
	LastError  string    `json:"last_error,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func runtimePath(dir string) string { return filepath.Join(dir, runtimeFileName) }

func readRuntimeStatus(dir string) (RuntimeStatus, error) {
	data, err := os.ReadFile(runtimePath(dir))
	if err != nil {
		return RuntimeStatus{}, err
	}
	var status RuntimeStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return RuntimeStatus{}, fmt.Errorf("decode runtime status: %w", err)
	}
	return status, nil
}

func writeRuntimeStatus(dir string, status RuntimeStatus) error {
	status.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, ".singbox-switch-runtime-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := replaceFile(name, runtimePath(dir)); err != nil {
		return err
	}
	ok = true
	return nil
}

func daemonRunning(dir string) (RuntimeStatus, bool) {
	status, err := readRuntimeStatus(dir)
	if err != nil || status.DaemonPID <= 0 || time.Since(status.UpdatedAt) > 5*time.Second {
		return RuntimeStatus{}, false
	}
	if !processAlive(status.DaemonPID) {
		return RuntimeStatus{}, false
	}
	return status, true
}

func acquireDaemonLock(dir string) (func(), error) {
	path := filepath.Join(dir, lockFileName)
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			_, writeErr := fmt.Fprintf(file, "%d\n", os.Getpid())
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				_ = os.Remove(path)
				return nil, errors.Join(writeErr, closeErr)
			}
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		data, readErr := os.ReadFile(path)
		var pid int
		if readErr == nil {
			_, _ = fmt.Sscanf(string(data), "%d", &pid)
		}
		if pid > 0 && processAlive(pid) {
			return nil, fmt.Errorf("another singbox-switch daemon is already running (pid %d)", pid)
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("remove stale daemon lock: %w", err)
		}
	}
	return nil, errors.New("could not acquire daemon lock")
}
