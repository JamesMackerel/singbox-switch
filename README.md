# singbox-switch

`singbox-switch` is a cross-platform sing-box configuration switcher. It scans a configuration directory for JSON files, records the selected configuration in a state file, and uses a background service to keep exactly one sing-box child process running.

## Build

Go 1.23 or later is required:

```sh
make build

# Build for macOS Intel
make build-macos-amd64

# Build for all supported platforms
make build-all
```

Build artifacts are written to `dist/`. You can also build directly with `go build -o singbox-switch ./cmd/singbox-switch`.

## Usage

```text
singbox-switch list
singbox-switch current
singbox-switch status
singbox-switch logs
singbox-switch logs -f
singbox-switch init
singbox-switch config -h
singbox-switch config binary-path /absolute/path/to/sing-box
singbox-switch config config-path /absolute/path/to/sing-box/configs
singbox-switch check <name>
singbox-switch use <name>
singbox-switch <name>
singbox-switch srv install
singbox-switch srv uninstall
singbox-switch srv start|stop|restart
```

The switcher's configuration directory follows platform conventions. It can be explicitly set with `SINGBOX_CONFIG_DIR`. The defaults are `$XDG_CONFIG_HOME/singbox-switch` or `$HOME/.config/singbox-switch` on Linux, `~/Library/Application Support/singbox-switch` on macOS, and `%APPDATA%/singbox-switch` on Windows. `singbox-switch init` creates the `config.toml` file there. sing-box's own JSON configurations live in a separate directory configured with `config config-path`. The executable is configured with `config binary-path`; the program does not search `PATH` automatically. Only the `SINGBOX_CONFIG_DIR` environment variable is supported for overriding the switcher's configuration directory.

Logs are stored by default at `~/Library/Logs/singbox-switch/singbox-switch.log` on macOS, `${XDG_STATE_HOME:-$HOME/.local/state}/singbox-switch/logs/singbox-switch.log` on Linux, and `%LOCALAPPDATA%/singbox-switch/logs/singbox-switch.log` on Windows. `logs` shows the last 200 lines, while `logs -f` follows the log continuously. When a log exceeds 10 MiB, the current file is cleared directly; no rotated file is created.

`srv install` installs and starts the service. macOS and Linux systems using systemd install a per-user service by default; Windows installs an automatic startup service. Installation does not stop, remove, or modify any other existing LaunchAgents or services. The regular `use` command only validates the configuration and atomically updates the state file; it does not call the service manager.

The sing-box child process always uses the selected configuration directory as its working directory. Relative paths in the configuration, such as `cache.db`, are therefore created in that directory rather than depending on a service manager's default working directory.

## Tests

```sh
go test -race ./...
go vet ./...
```
