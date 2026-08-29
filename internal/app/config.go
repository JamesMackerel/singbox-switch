package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Config struct {
	Name     string
	Filename string
	Path     string
}

func scanConfigs(dir string) ([]Config, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("scan config directory %q: %w", dir, err)
	}
	configs := make([]Config, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if err := validateName(name); err != nil {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		resolvedPath, err := secureConfigPath(dir, path)
		if err != nil {
			continue
		}
		configs = append(configs, Config{Name: name, Filename: entry.Name(), Path: resolvedPath})
	}
	sort.Slice(configs, func(i, j int) bool { return configs[i].Name < configs[j].Name })
	return configs, nil
}

func validateName(name string) error {
	if name == "" || name == "." || name == ".." {
		return errors.New("configuration name is empty or reserved")
	}
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\\`) {
		return fmt.Errorf("unsafe configuration name %q", name)
	}
	if strings.HasSuffix(strings.ToLower(name), ".json") {
		return fmt.Errorf("configuration name must omit .json: %q", name)
	}
	if strings.ContainsRune(name, '\x00') {
		return fmt.Errorf("unsafe configuration name %q", name)
	}
	return nil
}

func findConfig(dir, name string) (Config, error) {
	if err := validateName(name); err != nil {
		return Config{}, err
	}
	configs, err := scanConfigs(dir)
	if err != nil {
		return Config{}, err
	}
	for _, config := range configs {
		if config.Name == name {
			return config, nil
		}
	}
	return Config{}, fmt.Errorf("configuration %q does not exist in %s", name, dir)
}

func configFromFilename(dir, filename string) (Config, error) {
	if filepath.Base(filename) != filename || !strings.EqualFold(filepath.Ext(filename), ".json") {
		return Config{}, fmt.Errorf("unsafe configuration filename %q", filename)
	}
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	config, err := findConfig(dir, name)
	if err != nil {
		return Config{}, err
	}
	if config.Filename != filename {
		return Config{}, fmt.Errorf("configuration filename %q does not match disk entry %q", filename, config.Filename)
	}
	return config, nil
}

func secureConfigPath(dir, path string) (string, error) {
	dirReal, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("resolve config directory: %w", err)
	}
	pathReal, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve config path: %w", err)
	}
	rel, err := filepath.Rel(dirReal, pathReal)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("configuration %q is outside config directory", path)
	}
	info, err := os.Stat(pathReal)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("configuration %q is not a regular file", path)
	}
	return pathReal, nil
}
