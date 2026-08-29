//go:build !windows

package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServiceProgramStopsAndCleansUpChild(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	binary := fakeSingBox(t, dir)
	mustWrite(t, filepath.Join(dir, "active.json"), `{}`)
	if err := writeState(dir, State{ConfigFile: "active.json", ConfigPath: dir, BinaryPath: binary}); err != nil {
		t.Fatal(err)
	}
	program := &serviceProgram{dir: dir, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	if err := program.Start(nil); err != nil {
		t.Fatal(err)
	}
	childPID := 0
	waitFor(t, 3*time.Second, func() bool {
		status, ok := daemonRunning(dir)
		if ok && status.ChildState == "running" {
			childPID = status.ChildPID
			return childPID > 0
		}
		return false
	})
	if err := program.Stop(nil); err != nil {
		t.Fatal(err)
	}
	status, err := readRuntimeStatus(dir)
	if err != nil {
		t.Fatal(err)
	}
	if status.ChildState != "stopped" || status.ChildPID != 0 {
		t.Fatalf("child was not cleaned up: %#v", status)
	}
	if processAlive(childPID) {
		t.Fatalf("child pid %d is still alive", childPID)
	}
	if _, err := os.Stat(filepath.Join(dir, lockFileName)); !os.IsNotExist(err) {
		t.Fatalf("daemon lock was not removed: %v", err)
	}
}
