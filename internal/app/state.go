package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/BurntSushi/toml"
)

type State struct {
	BinaryPath string `toml:"binary_path"`
	ConfigPath string `toml:"config_path"`
	ConfigFile string `toml:"config_file"`
}

func statePath(dir string) string { return filepath.Join(dir, stateFileName) }

func readState(dir string) (State, error) {
	data, err := os.ReadFile(statePath(dir))
	if err != nil {
		return State{}, err
	}
	var state State
	if _, err := toml.Decode(string(data), &state); err != nil {
		return State{}, fmt.Errorf("decode state file: %w", err)
	}
	if state.BinaryPath != "" && !filepath.IsAbs(state.BinaryPath) {
		return State{}, errors.New("config.toml binary_path must be an absolute path")
	}
	if state.ConfigPath != "" && !filepath.IsAbs(state.ConfigPath) {
		return State{}, errors.New("config.toml config_path must be an absolute path")
	}
	return state, nil
}

func writeState(dir string, state State) error {
	if state.BinaryPath != "" && !filepath.IsAbs(state.BinaryPath) {
		return errors.New("refusing to write a relative sing-box path")
	}
	if state.ConfigPath != "" && !filepath.IsAbs(state.ConfigPath) {
		return errors.New("refusing to write a relative config path")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data := []byte("")
	if state.ConfigFile != "" {
		data = append(data, []byte("config_file = "+tomlQuote(state.ConfigFile)+"\n")...)
	}
	if state.BinaryPath != "" {
		data = append(data, []byte("binary_path = "+tomlQuote(state.BinaryPath)+"\n")...)
	}
	if state.ConfigPath != "" {
		data = append(data, []byte("config_path = "+tomlQuote(state.ConfigPath)+"\n")...)
	}
	tmp, err := os.CreateTemp(dir, ".singbox-switch-config-*")
	if err != nil {
		return fmt.Errorf("create temporary state: %w", err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temporary state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temporary state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := replaceFile(tmpName, statePath(dir)); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}
	ok = true
	return syncDirectory(dir)
}

func tomlQuote(value string) string {
	return strconv.Quote(value)
}
