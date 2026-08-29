//go:build !windows

package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testDaemon(t *testing.T, dir string) (*daemon, context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	d := newDaemon(dir, &bytes.Buffer{}, &bytes.Buffer{})
	d.pollInterval = 15 * time.Millisecond
	d.settleTime = 25 * time.Millisecond
	d.stopTimeout = 400 * time.Millisecond
	d.restartDelay = 30 * time.Millisecond
	done := make(chan error, 1)
	go func() { done <- d.run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("daemon shutdown: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("daemon did not stop")
		}
	})
	return d, cancel, done
}

func TestDaemonSwitchRollbackOnStartupFailure(t *testing.T) {
	dir := t.TempDir()
	binary := fakeSingBox(t, dir)
	mustWrite(t, filepath.Join(dir, "old.json"), `{}`)
	mustWrite(t, filepath.Join(dir, "bad.json"), `start-fail`)
	old := State{ConfigFile: "old.json", ConfigPath: dir, BinaryPath: binary}
	if err := writeState(dir, old); err != nil {
		t.Fatal(err)
	}
	testDaemon(t, dir)
	waitFor(t, 2*time.Second, func() bool {
		status, ok := daemonRunning(dir)
		return ok && status.Active == "old.json" && status.ChildState == "running"
	})
	var output bytes.Buffer
	if err := runUse(dir, "bad", &output); err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("use should report startup rollback, got %v", err)
	}
	waitFor(t, 3*time.Second, func() bool {
		state, err := readState(dir)
		status, ok := daemonRunning(dir)
		return err == nil && ok && state == old && status.Active == "old.json" &&
			status.ChildState == "running" && strings.Contains(status.LastError, "start configuration bad.json")
	})
}

func TestUseWaitsForRunningDaemonToActivate(t *testing.T) {
	dir := t.TempDir()
	binary := fakeSingBox(t, dir)
	mustWrite(t, filepath.Join(dir, "a.json"), `{}`)
	mustWrite(t, filepath.Join(dir, "b.json"), `{}`)
	if err := writeState(dir, State{ConfigFile: "a.json", ConfigPath: dir, BinaryPath: binary}); err != nil {
		t.Fatal(err)
	}
	testDaemon(t, dir)
	waitFor(t, 2*time.Second, func() bool {
		status, ok := daemonRunning(dir)
		return ok && status.Active == "a.json" && status.ChildState == "running"
	})
	var output bytes.Buffer
	if err := runUse(dir, "b", &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "selected and activated b") {
		t.Fatalf("unexpected output: %q", output.String())
	}
}

func TestDaemonRestartsCrashedChild(t *testing.T) {
	dir := t.TempDir()
	binary := fakeSingBox(t, dir)
	logPath := filepath.Join(dir, "starts.log")
	t.Setenv("FAKE_SINGBOX_LOG", logPath)
	mustWrite(t, filepath.Join(dir, "crasher.json"), `crash`)
	if err := writeState(dir, State{ConfigFile: "crasher.json", ConfigPath: dir, BinaryPath: binary}); err != nil {
		t.Fatal(err)
	}
	testDaemon(t, dir)
	waitFor(t, 3*time.Second, func() bool {
		data, _ := os.ReadFile(logPath)
		return strings.Count(string(data), "start crasher.json") >= 2
	})
}

func TestDaemonRapidSwitchUsesLatestState(t *testing.T) {
	dir := t.TempDir()
	binary := fakeSingBox(t, dir)
	for _, name := range []string{"a", "b", "c"} {
		mustWrite(t, filepath.Join(dir, name+".json"), `{}`)
	}
	if err := writeState(dir, State{ConfigFile: "a.json", ConfigPath: dir, BinaryPath: binary}); err != nil {
		t.Fatal(err)
	}
	testDaemon(t, dir)
	waitFor(t, 2*time.Second, func() bool {
		status, ok := daemonRunning(dir)
		return ok && status.Active == "a.json"
	})
	if err := writeState(dir, State{ConfigFile: "b.json", ConfigPath: dir, BinaryPath: binary}); err != nil {
		t.Fatal(err)
	}
	if err := writeState(dir, State{ConfigFile: "c.json", ConfigPath: dir, BinaryPath: binary}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool {
		status, ok := daemonRunning(dir)
		return ok && status.Active == "c.json" && status.ChildState == "running"
	})
}

func TestDaemonRunsSingBoxWithConfigDirectoryAsWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	binary := fakeSingBox(t, dir)
	mustWrite(t, filepath.Join(dir, "cache.json"), `cache-test`)
	if err := writeState(dir, State{ConfigFile: "cache.json", ConfigPath: dir, BinaryPath: binary}); err != nil {
		t.Fatal(err)
	}
	testDaemon(t, dir)
	waitFor(t, 2*time.Second, func() bool {
		status, ok := daemonRunning(dir)
		return ok && status.Active == "cache.json" && status.ChildState == "running"
	})
	if _, err := os.Stat(filepath.Join(dir, "cache.db")); err != nil {
		t.Fatalf("sing-box did not run from config directory: %v", err)
	}
}

func TestDaemonRestartReadsPersistedState(t *testing.T) {
	dir := t.TempDir()
	binary := fakeSingBox(t, dir)
	mustWrite(t, filepath.Join(dir, "saved.json"), `{}`)
	if err := writeState(dir, State{ConfigFile: "saved.json", ConfigPath: dir, BinaryPath: binary}); err != nil {
		t.Fatal(err)
	}
	d1, cancel1, done1 := testDaemonNoCleanup(t, dir)
	_ = d1
	waitFor(t, 2*time.Second, func() bool {
		status, ok := daemonRunning(dir)
		return ok && status.Active == "saved.json"
	})
	cancel1()
	if err := <-done1; err != nil {
		t.Fatal(err)
	}
	testDaemon(t, dir)
	waitFor(t, 2*time.Second, func() bool {
		status, ok := daemonRunning(dir)
		return ok && status.Active == "saved.json" && status.ChildState == "running"
	})
}

func testDaemonNoCleanup(t *testing.T, dir string) (*daemon, context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	d := newDaemon(dir, &bytes.Buffer{}, &bytes.Buffer{})
	d.pollInterval = 15 * time.Millisecond
	d.settleTime = 25 * time.Millisecond
	d.stopTimeout = 400 * time.Millisecond
	d.restartDelay = 30 * time.Millisecond
	done := make(chan error, 1)
	go func() { done <- d.run(ctx) }()
	return d, cancel, done
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
