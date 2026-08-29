package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	logDirName        = "singbox-switch"
	logFileName       = "singbox-switch.log"
	maxLogSize  int64 = 10 * 1024 * 1024
)

func logDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory for logs: %w", err)
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Logs", logDirName), nil
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base, err = os.UserCacheDir()
			if err != nil {
				return "", fmt.Errorf("locate local application data for logs: %w", err)
			}
		}
		return filepath.Join(base, logDirName, "logs"), nil
	default:
		base := os.Getenv("XDG_STATE_HOME")
		if base == "" {
			base = filepath.Join(home, ".local", "state")
		}
		return filepath.Join(base, logDirName, "logs"), nil
	}
}

func logPath() (string, error) {
	dir, err := logDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, logFileName), nil
}

// logWriter keeps one fixed log file. Once it reaches maxLogSize it is
// truncated in place instead of creating rotated historical files.
type logWriter struct {
	mu   sync.Mutex
	file *os.File
	path string
}

func openLogWriter() (*logWriter, error) {
	path, err := logPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	writer := &logWriter{file: file, path: path}
	if err := writer.cleanupIfNeeded(); err != nil {
		_ = file.Close()
		return nil, err
	}
	return writer, nil
}

func (w *logWriter) cleanupIfNeeded() error {
	info, err := w.file.Stat()
	if err != nil {
		return err
	}
	if info.Size() <= maxLogSize {
		return nil
	}
	if err := w.file.Truncate(0); err != nil {
		return fmt.Errorf("clear oversized log: %w", err)
	}
	_, err = w.file.Seek(0, io.SeekEnd)
	return err
}

func (w *logWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	info, err := w.file.Stat()
	if err != nil {
		return 0, err
	}
	if info.Size()+int64(len(data)) > maxLogSize {
		if err := w.file.Truncate(0); err != nil {
			return 0, fmt.Errorf("clear oversized log: %w", err)
		}
		if _, err := w.file.Seek(0, io.SeekEnd); err != nil {
			return 0, err
		}
		marker := fmt.Sprintf("[%s] log cleared after exceeding %d bytes\n", time.Now().Format(time.RFC3339), maxLogSize)
		if _, err := io.WriteString(w.file, marker); err != nil {
			return 0, err
		}
		available := maxLogSize - int64(len(marker))
		if available < 0 {
			available = 0
		}
		if int64(len(data)) > available {
			data = data[len(data)-int(available):]
		}
	}
	return w.file.Write(data)
}

func (w *logWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

func (w *logWriter) Path() string { return w.path }

func runLogs(follow bool, out io.Writer) error {
	path, err := logPath()
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no logs found at %s", path)
	}
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()
	if !follow {
		return writeLastLines(file, out, 200)
	}
	if err := writeLastLines(file, out, 200); err != nil {
		return err
	}
	position, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	buffer := make([]byte, 32*1024)
	for {
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		info, err := file.Stat()
		if err != nil {
			return err
		}
		if info.Size() < position {
			position = 0
		}
		if _, err := file.Seek(position, io.SeekStart); err != nil {
			return err
		}
		for {
			n, readErr := file.Read(buffer)
			if n > 0 {
				if _, err := out.Write(buffer[:n]); err != nil {
					return err
				}
				position += int64(n)
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				return readErr
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func writeLastLines(file *os.File, out io.Writer, count int) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	if len(lines) == 0 {
		return nil
	}
	_, err = io.WriteString(out, strings.Join(lines, "\n")+"\n")
	return err
}
