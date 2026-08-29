package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	service "github.com/kardianos/service"
)

type serviceProgram struct {
	dir    string
	stdout io.Writer
	stderr io.Writer

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan error
}

func (p *serviceProgram) Start(service.Service) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.done = make(chan error, 1)
	logger, err := openLogWriter()
	if err != nil {
		p.cancel = nil
		cancel()
		return err
	}
	d := newDaemon(p.dir, logger, logger)
	go func() {
		err := d.run(ctx)
		_ = logger.Close()
		p.done <- err
	}()
	select {
	case err := <-p.done:
		p.cancel = nil
		return err
	case <-time.After(50 * time.Millisecond):
		return nil
	}
}

func (p *serviceProgram) Stop(service.Service) error {
	p.mu.Lock()
	if p.cancel == nil {
		p.mu.Unlock()
		return nil
	}
	cancel, done := p.cancel, p.done
	p.cancel = nil
	p.mu.Unlock()
	cancel()
	return <-done
}

func makeService(dir string, stdout, stderr io.Writer) (service.Service, *serviceProgram, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, nil, fmt.Errorf("locate singbox-switch executable: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return nil, nil, err
	}
	program := &serviceProgram{dir: dir, stdout: stdout, stderr: stderr}
	options := service.KeyValue{
		"KeepAlive": true,
		"RunAtLoad": true,
		"Restart":   "always",
	}
	if logs, err := logDirectory(); err == nil {
		options["LogDirectory"] = logs
	}
	if runtime.GOOS == "darwin" || (runtime.GOOS == "linux" && service.Platform() == "linux-systemd") {
		options["UserService"] = true
	}
	envVars := map[string]string{"SINGBOX_CONFIG_DIR": dir}
	config := &service.Config{
		Name:             "singbox-switch",
		DisplayName:      "singbox-switch",
		Description:      "Manage the active sing-box configuration and process",
		Executable:       executable,
		Arguments:        []string{"srv", "run"},
		WorkingDirectory: dir,
		Option:           options,
		EnvVars:          envVars,
	}
	if runtime.GOOS == "windows" {
		// An empty UserName asks the Windows SCM to use its service account;
		// StartType is interpreted by kardianos/service as automatic startup.
		config.Option["StartType"] = "automatic"
		config.Option["OnFailure"] = "restart"
	}
	svc, err := service.New(program, config)
	if err != nil {
		return nil, nil, err
	}
	return svc, program, nil
}
