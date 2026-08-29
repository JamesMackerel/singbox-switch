package app

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestStateAtomicWriteAndPermissions(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "sing-box")
	mustWrite(t, binary, "binary")
	states := []State{
		{ConfigFile: "one.json", ConfigPath: dir, BinaryPath: binary},
		{ConfigFile: "two.json", ConfigPath: dir, BinaryPath: binary},
	}
	if err := writeState(dir, states[0]); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			if err := writeState(dir, states[i%2]); err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
		}
	}()
	for i := 0; i < 400; i++ {
		state, err := readState(dir)
		if err != nil {
			t.Fatalf("observed partial state: %v", err)
		}
		if state != states[0] && state != states[1] {
			t.Fatalf("observed unexpected state: %#v", state)
		}
	}
	wg.Wait()
	select {
	case err := <-errCh:
		t.Fatal(err)
	default:
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(statePath(dir))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("state mode = %o, want 600", info.Mode().Perm())
		}
	}
}

func TestStateRequiresAbsoluteBinary(t *testing.T) {
	if err := writeState(t.TempDir(), State{ConfigFile: "x.json", BinaryPath: "sing-box"}); err == nil {
		t.Fatal("relative binary unexpectedly accepted")
	}
}
