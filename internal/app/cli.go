package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	service "github.com/kardianos/service"
)

const usage = `Usage:
  singbox-switch list
  singbox-switch current
  singbox-switch status
  singbox-switch check <name>
  singbox-switch use <name>
  singbox-switch <name>
  singbox-switch init
  singbox-switch config -h
  singbox-switch config binary-path <PATH>
  singbox-switch config config-path <PATH>
  singbox-switch logs [-f|--follow]
  singbox-switch srv <install|uninstall|start|stop|restart|run>`

func Run(args []string, stdout, stderr io.Writer) error {
	opts, command, err := parseArgs(args)
	if err != nil {
		return err
	}
	if len(command) == 0 || command[0] == "help" {
		fmt.Fprintln(stdout, usage)
		return nil
	}
	if opts.Follow && command[0] != "logs" {
		return errors.New("-f/--follow is only valid with logs")
	}
	dir, err := resolveConfigDir()
	if err != nil {
		return err
	}
	switch command[0] {
	case "config":
		return runConfigCommand(dir, command[1:], opts.Help, stdout)
	case "init":
		if len(command) != 1 {
			return errors.New("init takes no arguments")
		}
		return runInit(dir, stdout)
	case "logs":
		if len(command) != 1 {
			return errors.New("logs takes no positional arguments")
		}
		return runLogs(opts.Follow, stdout)
	case "list":
		if len(command) != 1 {
			return errors.New("list takes no arguments")
		}
		return runList(dir, stdout)
	case "current":
		if len(command) != 1 {
			return errors.New("current takes no arguments")
		}
		return runCurrent(dir, stdout)
	case "status":
		if len(command) != 1 {
			return errors.New("status takes no arguments")
		}
		return runStatus(dir, stdout, stderr)
	case "check", "use":
		if len(command) != 2 {
			return fmt.Errorf("%s requires exactly one configuration name", command[0])
		}
		if command[0] == "check" {
			return runCheck(dir, command[1], stdout)
		}
		return runUse(dir, command[1], stdout)
	case "srv":
		if len(command) != 2 {
			return errors.New("srv requires one of install, uninstall, start, stop, restart, run")
		}
		return runServiceCommand(dir, command[1], stdout, stderr)
	default:
		if len(command) == 1 {
			return runUse(dir, command[0], stdout)
		}
		return fmt.Errorf("unknown command %q", strings.Join(command, " "))
	}
}

func runList(dir string, out io.Writer) error {
	state, err := readState(dir)
	if err != nil {
		return err
	}
	configs, err := scanConfigs(state.ConfigPath)
	if err != nil {
		return err
	}
	current := state.ConfigFile
	for _, config := range configs {
		marker := "  "
		if config.Filename == current {
			marker = "* "
		}
		fmt.Fprintln(out, marker+config.Name)
	}
	return nil
}

func runCurrent(dir string, out io.Writer) error {
	state, err := readState(dir)
	if err != nil {
		return fmt.Errorf("read current configuration: %w", err)
	}
	config, err := configFromFilename(state.ConfigPath, state.ConfigFile)
	if err != nil {
		return err
	}
	path, err := filepath.Abs(config.Path)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s\n%s\n", config.Name, path)
	return nil
}

func binaryForCommand(dir string) (string, error) {
	state, err := readState(dir)
	if err != nil {
		return "", err
	}
	return resolveBinary(state.BinaryPath)
}

func runInit(dir string, out io.Writer) error {
	if _, err := os.Stat(statePath(dir)); err == nil {
		return errors.New("config.toml already exists; refusing to overwrite it")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	state := State{}
	if err := writeState(dir, state); err != nil {
		return err
	}
	fmt.Fprintln(out, "initialized config.toml; set paths with `singbox-switch config ...`")
	return nil
}

const configHelp = `Configuration items:

  binary-path <PATH>
      Absolute path to the sing-box executable (required).

  config-path <PATH>
      Absolute directory containing sing-box JSON configurations (required).

  config-file
      Current JSON configuration filename; set with use <name> (required).
`

func runConfigCommand(dir string, args []string, help bool, out io.Writer) error {
	if help || len(args) == 0 {
		fmt.Fprint(out, configHelp)
		if state, err := readState(dir); err == nil {
			fmt.Fprintf(out, "Current values:\n  binary-path: %s\n  config-path: %s\n  config-file: %s\n", state.BinaryPath, state.ConfigPath, state.ConfigFile)
		}
		return nil
	}
	if len(args) > 2 {
		return errors.New("config command accepts one item and one value")
	}
	state, err := readState(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("config.toml does not exist; run `singbox-switch init` first")
		}
		return err
	}
	if len(args) == 1 {
		switch args[0] {
		case "binary-path":
			fmt.Fprintln(out, state.BinaryPath)
		case "config-path":
			fmt.Fprintln(out, state.ConfigPath)
		case "config-file":
			fmt.Fprintln(out, state.ConfigFile)
		default:
			return fmt.Errorf("unknown config item %q", args[0])
		}
		return nil
	}
	switch args[0] {
	case "binary-path":
		binary, err := resolveBinary(args[1])
		if err != nil {
			return err
		}
		state.BinaryPath = binary
	case "config-path":
		path, err := filepath.Abs(args[1])
		if err != nil {
			return err
		}
		if !filepath.IsAbs(args[1]) {
			return errors.New("config-path must be absolute")
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("config-path: %w", err)
		}
		if !info.IsDir() {
			return errors.New("config-path must be a directory")
		}
		state.ConfigPath = path
	case "config-file":
		return errors.New("config-file is managed by `use <name>`")
	default:
		return fmt.Errorf("unknown config item %q", args[0])
	}
	if err := writeState(dir, state); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s saved\n", args[0])
	return nil
}

func runCheck(dir, name string, out io.Writer) error {
	state, err := readState(dir)
	if err != nil {
		return err
	}
	config, err := findConfig(state.ConfigPath, name)
	if err != nil {
		return err
	}
	binary, err := binaryForCommand(dir)
	if err != nil {
		return err
	}
	if err := checkConfig(binary, config); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s: valid\n", name)
	return nil
}

func runUse(dir, name string, out io.Writer) error {
	state, err := readState(dir)
	if err != nil {
		return err
	}
	config, err := findConfig(state.ConfigPath, name)
	if err != nil {
		return err
	}
	binary, err := binaryForCommand(dir)
	if err != nil {
		return err
	}
	if err := checkConfig(binary, config); err != nil {
		return err
	}
	_, wasRunning := daemonRunning(dir)
	requestedAt := time.Now()
	if err := writeState(dir, State{ConfigFile: config.Filename, ConfigPath: state.ConfigPath, BinaryPath: binary}); err != nil {
		return err
	}
	if !wasRunning {
		fmt.Fprintf(out, "selected %s (service is not running)\n", name)
		return nil
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, running := daemonRunning(dir)
		if !running {
			return errors.New("configuration was saved, but the daemon stopped before applying it")
		}
		if status.UpdatedAt.After(requestedAt) && status.Active == config.Filename && status.ChildState == "running" {
			fmt.Fprintf(out, "selected and activated %s\n", name)
			return nil
		}
		state, stateErr := readState(dir)
		if stateErr == nil && state.ConfigFile != config.Filename && status.UpdatedAt.After(requestedAt) {
			if status.LastError != "" {
				return fmt.Errorf("configuration activation failed and was rolled back: %s", status.LastError)
			}
			return errors.New("configuration activation failed and was rolled back")
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("timed out waiting for the daemon to apply the configuration")
}

func runStatus(dir string, out, stderr io.Writer) error {
	currentName, currentPath := "none", "none"
	if state, err := readState(dir); err == nil {
		if config, configErr := configFromFilename(state.ConfigPath, state.ConfigFile); configErr == nil {
			currentName, currentPath = config.Name, config.Path
		} else {
			currentName = "invalid: " + configErr.Error()
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		currentName = "invalid: " + err.Error()
	}
	daemonState, running := daemonRunning(dir)
	daemonText, childText := "stopped", "stopped"
	if running {
		daemonText = fmt.Sprintf("running (pid %d)", daemonState.DaemonPID)
		childText = daemonState.ChildState
		if daemonState.ChildPID > 0 {
			childText += fmt.Sprintf(" (pid %d, config %s)", daemonState.ChildPID, daemonState.Active)
		}
		if daemonState.LastError != "" {
			childText += "; last error: " + daemonState.LastError
		}
	}
	serviceText := "unknown"
	if svc, _, err := makeService(dir, io.Discard, stderr); err == nil {
		if status, statusErr := svc.Status(); statusErr == nil {
			switch status {
			case service.StatusRunning:
				serviceText = "running"
			case service.StatusStopped:
				serviceText = "stopped"
			default:
				serviceText = "unknown"
			}
		} else {
			serviceText = "not installed"
		}
	}
	platform := service.Platform()
	if platform == "" {
		platform = runtime.GOOS
	}
	fmt.Fprintf(out, "current: %s\npath: %s\ndaemon: %s\nsing-box: %s\nservice: %s\nplatform: %s\n",
		currentName, currentPath, daemonText, childText, serviceText, platform)
	return nil
}

func runServiceCommand(dir, action string, out, stderr io.Writer) error {
	svc, _, err := makeService(dir, out, stderr)
	if err != nil {
		return err
	}
	switch action {
	case "run":
		return svc.Run()
	case "install":
		_, err := ensureInstallState(dir)
		if err != nil {
			return err
		}
		if logs, logErr := logDirectory(); logErr == nil {
			if err := os.MkdirAll(logs, 0700); err != nil {
				return fmt.Errorf("create log directory: %w", err)
			}
		}
		if err := svc.Install(); err != nil {
			return fmt.Errorf("install service: %w", err)
		}
		if err := svc.Start(); err != nil {
			return fmt.Errorf("service installed but could not be started: %w", err)
		}
		fmt.Fprintln(out, "service installed and started")
		return nil
	case "uninstall":
		if err := svc.Stop(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not running") {
			return fmt.Errorf("stop service before uninstall: %w", err)
		}
		if err := svc.Uninstall(); err != nil {
			return fmt.Errorf("uninstall service: %w", err)
		}
		fmt.Fprintln(out, "service stopped and uninstalled")
		return nil
	case "start":
		err = svc.Start()
	case "stop":
		err = svc.Stop()
	case "restart":
		err = svc.Restart()
	default:
		return fmt.Errorf("unknown srv command %q", action)
	}
	if err != nil {
		return fmt.Errorf("service %s: %w", action, err)
	}
	fmt.Fprintf(out, "service %sed\n", strings.TrimSuffix(action, "e"))
	return nil
}

func ensureInstallState(dir string) (string, error) {
	state, err := readState(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errors.New("no config.toml; run `singbox-switch init` first")
		}
		return "", err
	}
	if state.BinaryPath == "" {
		return "", errors.New("config.toml binary_path is required; run `singbox-switch config binary-path <path>`")
	}
	if state.ConfigPath == "" {
		return "", errors.New("config.toml config_path is required; run `singbox-switch config config-path <path>`")
	}
	if state.ConfigFile == "" {
		return "", errors.New("config.toml has no selected configuration; run `singbox-switch use <name>`")
	}
	binary, err := binaryForCommand(dir)
	if err != nil {
		return "", err
	}
	config, err := configFromFilename(state.ConfigPath, state.ConfigFile)
	if err != nil {
		return "", err
	}
	if err := checkConfig(binary, config); err != nil {
		return "", err
	}
	if err := writeState(dir, State{ConfigFile: config.Filename, ConfigPath: state.ConfigPath, BinaryPath: binary}); err != nil {
		return "", err
	}
	return binary, nil
}
