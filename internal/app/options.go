package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	configDirName   = "singbox-switch"
	stateFileName   = "config.toml"
	runtimeFileName = ".singbox-switch-runtime.json"
	lockFileName    = ".singbox-switch-daemon.lock"
)

type Options struct {
	Follow bool
	Help   bool
}

func parseArgs(args []string) (Options, []string, error) {
	var opts Options
	command := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-f" || a == "--follow":
			opts.Follow = true
		case a == "-h" || a == "--help":
			opts.Help = true
		case strings.HasPrefix(a, "-"):
			return Options{}, nil, fmt.Errorf("unknown option %q", a)
		default:
			command = append(command, a)
		}
	}
	return opts, command, nil
}

func resolveConfigDir() (string, error) {
	if override := os.Getenv("SINGBOX_CONFIG_DIR"); override != "" {
		return filepath.Abs(override)
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate user home directory: %w", err)
	}
	if runtime.GOOS == "darwin" {
		if base == "" {
			base = filepath.Join(home, "Library", "Application Support")
		}
	} else if runtime.GOOS == "windows" {
		if base == "" {
			base = os.Getenv("APPDATA")
		}
		if base == "" {
			base = filepath.Join(home, "AppData", "Roaming")
		}
	} else if base == "" {
		base = filepath.Join(home, ".config")
	}
	return filepath.Abs(filepath.Join(base, configDirName))
}

func resolveBinary(candidate string) (string, error) {
	if candidate == "" {
		return "", errors.New("sing-box path is not configured; run `singbox-switch init` then `singbox-switch config binary-path <path>`")
	}
	if !filepath.IsAbs(candidate) {
		return "", fmt.Errorf("sing-box path must be absolute: %q", candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve sing-box path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("sing-box binary %q: %w", abs, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("sing-box binary %q is a directory", abs)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		return "", fmt.Errorf("sing-box binary %q is not executable", abs)
	}
	return abs, nil
}
