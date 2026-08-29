package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type daemon struct {
	dir          string
	stdout       io.Writer
	stderr       io.Writer
	pollInterval time.Duration
	settleTime   time.Duration
	stopTimeout  time.Duration
	restartDelay time.Duration

	proc   *managedProcess
	active State
	status RuntimeStatus
}

func newDaemon(dir string, stdout, stderr io.Writer) *daemon {
	return &daemon{
		dir:          dir,
		stdout:       stdout,
		stderr:       stderr,
		pollInterval: 250 * time.Millisecond,
		settleTime:   750 * time.Millisecond,
		stopTimeout:  5 * time.Second,
		restartDelay: time.Second,
		status: RuntimeStatus{
			DaemonPID:  os.Getpid(),
			ChildState: "stopped",
		},
	}
}

func (d *daemon) run(ctx context.Context) error {
	if err := os.MkdirAll(d.dir, 0755); err != nil {
		return err
	}
	release, err := acquireDaemonLock(d.dir)
	if err != nil {
		return err
	}
	defer release()
	defer func() {
		if err := stopSingBox(d.proc, d.stopTimeout); err != nil {
			fmt.Fprintln(d.stderr, "stop sing-box:", err)
		}
		d.proc = nil
		d.status.ChildPID = 0
		d.status.ChildState = "stopped"
		_ = writeRuntimeStatus(d.dir, d.status)
	}()

	d.reconcile()
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()
	for {
		var exited <-chan error
		if d.proc != nil {
			exited = d.proc.done
		}
		select {
		case <-ctx.Done():
			return nil
		case err := <-exited:
			d.proc = nil
			d.status.ChildPID = 0
			d.status.ChildState = "crashed"
			d.status.LastError = exitMessage(err)
			_ = writeRuntimeStatus(d.dir, d.status)
			fmt.Fprintln(d.stderr, d.status.LastError)
			if !waitContext(ctx, d.restartDelay) {
				return nil
			}
			d.reconcile()
		case <-ticker.C:
			d.reconcile()
		}
	}
}

func (d *daemon) reconcile() {
	desired, err := readState(d.dir)
	if err != nil {
		d.setError(fmt.Errorf("read desired state: %w", err))
		return
	}
	d.status.Requested = desired.ConfigFile
	config, err := validateState(d.dir, desired)
	if err != nil {
		d.setError(err)
		return
	}
	if d.proc != nil && desired == d.active {
		d.status.ChildState = "running"
		_ = writeRuntimeStatus(d.dir, d.status)
		return
	}

	old := d.active
	if d.proc != nil {
		d.status.ChildState = "stopping"
		_ = writeRuntimeStatus(d.dir, d.status)
		if err := stopSingBox(d.proc, d.stopTimeout); err != nil {
			fmt.Fprintln(d.stderr, "stop old sing-box:", err)
		}
		d.proc = nil
		d.status.ChildPID = 0
	}

	d.status.ChildState = "starting"
	_ = writeRuntimeStatus(d.dir, d.status)
	proc, err := startSingBox(desired.BinaryPath, config.Path, d.stdout, d.stderr, d.settleTime)
	if err == nil {
		d.proc = proc
		d.active = desired
		d.status.Active = desired.ConfigFile
		d.status.ChildPID = proc.cmd.Process.Pid
		d.status.ChildState = "running"
		d.status.LastError = ""
		_ = writeRuntimeStatus(d.dir, d.status)
		return
	}

	startErr := fmt.Errorf("start configuration %s: %w", desired.ConfigFile, err)
	d.setError(startErr)
	if old.ConfigFile == "" || old == desired {
		return
	}
	// use writes the requested state before the daemon applies it. Restore the
	// last running state if the requested child cannot become stable.
	if err := writeState(d.dir, old); err != nil {
		d.setError(fmt.Errorf("%v; restore state: %w", startErr, err))
		return
	}
	oldConfig, err := validateState(d.dir, old)
	if err != nil {
		d.setError(fmt.Errorf("%v; validate previous state: %w", startErr, err))
		return
	}
	proc, err = startSingBox(old.BinaryPath, oldConfig.Path, d.stdout, d.stderr, d.settleTime)
	if err != nil {
		d.setError(fmt.Errorf("%v; restart previous configuration: %w", startErr, err))
		return
	}
	d.proc = proc
	d.active = old
	d.status.Requested = old.ConfigFile
	d.status.Active = old.ConfigFile
	d.status.ChildPID = proc.cmd.Process.Pid
	d.status.ChildState = "running"
	d.status.LastError = startErr.Error()
	_ = writeRuntimeStatus(d.dir, d.status)
}

func (d *daemon) setError(err error) {
	d.status.ChildState = "failed"
	d.status.LastError = err.Error()
	_ = writeRuntimeStatus(d.dir, d.status)
	fmt.Fprintln(d.stderr, err)
}

func validateState(dir string, state State) (Config, error) {
	if state.BinaryPath == "" {
		return Config{}, errors.New("config.toml binary_path is required; run `singbox-switch config binary-path <path>`")
	}
	if state.ConfigPath == "" {
		return Config{}, errors.New("config.toml config_path is required; run `singbox-switch config config-path <path>`")
	}
	if _, err := os.Stat(state.ConfigPath); err != nil {
		return Config{}, fmt.Errorf("config_path %q: %w", state.ConfigPath, err)
	}
	config, err := configFromFilename(state.ConfigPath, state.ConfigFile)
	if err != nil {
		return Config{}, fmt.Errorf("invalid selected configuration: %w", err)
	}
	resolved, err := resolveBinary(state.BinaryPath)
	if err != nil {
		return Config{}, err
	}
	if resolved != state.BinaryPath {
		return Config{}, fmt.Errorf("state binary_path must be absolute and canonical")
	}
	return config, nil
}

func checkConfig(binary string, config Config) error {
	cmd := exec.Command(binary, "check", "-c", config.Path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("configuration %s failed validation: %s", config.Name, message)
	}
	return nil
}

func exitMessage(err error) string {
	if err == nil {
		return "sing-box exited unexpectedly"
	}
	return "sing-box crashed: " + err.Error()
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
