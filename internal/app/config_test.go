package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestScanConfigsAndSafeNames(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "moon1.json"), `{}`)
	mustWrite(t, filepath.Join(dir, "zeta.JSON"), `{}`)
	mustWrite(t, filepath.Join(dir, "notes.txt"), "ignored")
	configs, err := scanConfigs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 2 || configs[0].Name != "moon1" || configs[1].Name != "zeta" {
		t.Fatalf("unexpected configs: %#v", configs)
	}
	for _, name := range []string{"", ".", "..", "../moon1", `x\\y`, "moon1.json"} {
		if err := validateName(name); err == nil {
			t.Errorf("validateName(%q) unexpectedly succeeded", name)
		}
	}
	if _, err := findConfig(dir, "missing"); err == nil {
		t.Fatal("missing configuration unexpectedly found")
	}
}

func TestScanRejectsSymlinkOutsideDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated privileges on Windows")
	}
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	mustWrite(t, outside, `{}`)
	if err := os.Symlink(outside, filepath.Join(dir, "escape.json")); err != nil {
		t.Fatal(err)
	}
	configs, err := scanConfigs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 0 {
		t.Fatalf("outside symlink was scanned: %#v", configs)
	}
	if _, err := findConfig(dir, "escape"); err == nil {
		t.Fatal("outside symlink resolved as a valid configuration")
	}
}

func mustWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}
