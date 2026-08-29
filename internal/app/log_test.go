package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLogWriterUsesPlatformPathAndClearsAtLimit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	writer, err := openLogWriter()
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(dir, logDirName, "logs")
	if filepath.Dir(writer.Path()) != wantDir {
		t.Fatalf("log path = %s, want directory %s", writer.Path(), wantDir)
	}
	if _, err := writer.Write(bytes.Repeat([]byte("a"), int(maxLogSize)-1024)); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(bytes.Repeat([]byte("b"), 2048)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(wantDir, logFileName))
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(data)) > maxLogSize {
		t.Fatalf("log size = %d, exceeds limit %d", len(data), maxLogSize)
	}
	if !bytes.Contains(data, []byte("log cleared after exceeding")) {
		t.Fatal("oversized log did not contain cleanup marker")
	}
}

func TestRunLogsPrintsLastTwoHundredLines(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	writer, err := openLogWriter()
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 250; i++ {
		if _, err := writer.Write([]byte("line-" + strconv.Itoa(i) + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runLogs(false, &out); err != nil {
		t.Fatal(err)
	}
	result := out.String()
	if strings.Contains(result, "line-50\n") || !strings.Contains(result, "line-51\n") || !strings.Contains(result, "line-250\n") {
		t.Fatalf("unexpected tail output: first bytes %q", result[:min(100, len(result))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
