package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgsGlobalOptionsAnywhere(t *testing.T) {
	opts, command, err := parseArgs([]string{"use", "moon1"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Help || opts.Follow {
		t.Fatalf("unexpected options: %#v", opts)
	}
	if strings.Join(command, " ") != "use moon1" {
		t.Fatalf("unexpected command: %#v", command)
	}
}

func TestUseValidationFailureDoesNotChangeState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SINGBOX_CONFIG_DIR", dir)
	binary := fakeSingBox(t, dir)
	mustWrite(t, filepath.Join(dir, "old.json"), `{}`)
	mustWrite(t, filepath.Join(dir, "bad.json"), `check-fail`)
	old := State{ConfigFile: "old.json", ConfigPath: dir, BinaryPath: binary}
	if err := writeState(dir, old); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := Run([]string{"use", "bad"}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "failed validation") {
		t.Fatalf("expected validation error, got %v", err)
	}
	state, err := readState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if state != old {
		t.Fatalf("state changed after check failure: %#v", state)
	}
}

func TestCheckRejectsDamagedJSONAndUseWithoutService(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SINGBOX_CONFIG_DIR", dir)
	binary := fakeSingBox(t, dir)
	mustWrite(t, filepath.Join(dir, "broken.json"), `{broken`)
	if err := writeState(dir, State{ConfigPath: dir, BinaryPath: binary}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"check", "broken"}, &out, &out); err == nil {
		t.Fatal("damaged JSON unexpectedly passed check")
	}
	mustWrite(t, filepath.Join(dir, "good.json"), `{}`)
	out.Reset()
	if err := Run([]string{"good"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "service is not running") {
		t.Fatalf("unexpected output: %s", out.String())
	}
	state, err := readState(dir)
	if err != nil || state.ConfigFile != "good.json" {
		t.Fatalf("state = %#v, err = %v", state, err)
	}
}

func TestUseMissingConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SINGBOX_CONFIG_DIR", dir)
	binary := fakeSingBox(t, dir)
	if err := writeState(dir, State{ConfigPath: dir, BinaryPath: binary}); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{"use", "missing"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigDirectoryUsesStrictXDGPriority(t *testing.T) {
	configOverride := filepath.Join(t.TempDir(), "explicit")
	xdg := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("SINGBOX_CONFIG_DIR", configOverride)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	resolved, err := resolveConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if resolved != configOverride {
		t.Fatalf("SINGBOX_CONFIG_DIR was not highest priority: %s", resolved)
	}
	t.Setenv("SINGBOX_CONFIG_DIR", "")
	resolved, err = resolveConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(xdg, "singbox-switch")
	if resolved != want {
		t.Fatalf("XDG_CONFIG_HOME path = %s, want %s", resolved, want)
	}
}

func TestConfigDirFlagIsRejected(t *testing.T) {
	if _, _, err := parseArgs([]string{"--config-dir", "/tmp/config", "list"}); err == nil {
		t.Fatal("--config-dir unexpectedly accepted")
	}
}

func TestInitAndConfigPersistsPaths(t *testing.T) {
	dir := t.TempDir()
	singboxDir := t.TempDir()
	t.Setenv("SINGBOX_CONFIG_DIR", dir)
	binary := fakeSingBox(t, dir)
	mustWrite(t, filepath.Join(singboxDir, "moon.json"), `{}`)
	var out bytes.Buffer
	if err := Run([]string{"init"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	state, err := readState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if state.ConfigFile != "" || state.BinaryPath != "" || state.ConfigPath != "" {
		t.Fatalf("init state = %#v", state)
	}
	if err := Run([]string{"config", "binary-path", binary}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"config", "config-path", singboxDir}, &out, &out); err != nil {
		t.Fatal(err)
	}
	state, err = readState(dir)
	if err != nil || state.BinaryPath != binary || state.ConfigPath != singboxDir {
		t.Fatalf("config state = %#v, err = %v", state, err)
	}
	if err := Run([]string{"use", "moon"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	state, err = readState(dir)
	if err != nil || state.ConfigFile != "moon.json" {
		t.Fatalf("use state = %#v, err = %v", state, err)
	}
	data, err := os.ReadFile(statePath(dir))
	if err != nil || !strings.Contains(string(data), "config_file = \"moon.json\"") {
		t.Fatalf("config.toml = %q, err = %v", data, err)
	}
}

func TestBinaryPathMustBeExplicit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SINGBOX_CONFIG_DIR", dir)
	singboxDir := t.TempDir()
	mustWrite(t, filepath.Join(singboxDir, "moon.json"), `{}`)
	if err := Run([]string{"init"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"config", "config-path", singboxDir}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	err := Run([]string{"check", "moon"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "path is not configured") {
		t.Fatalf("expected explicit binary error, got %v", err)
	}
}
