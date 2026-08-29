package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func fakeSingBox(t *testing.T, dir string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("process integration helper is a POSIX shell script")
	}
	path := filepath.Join(dir, "fake-sing-box")
	script := `#!/bin/sh
mode="$1"
config="$3"
content="$(sed -n '1p' "$config")"
if [ "$mode" = "check" ]; then
  case "$content" in
    *check-fail*|*\{broken*) echo "invalid configuration" >&2; exit 20 ;;
  esac
  exit 0
fi
if [ -n "$FAKE_SINGBOX_LOG" ]; then
  echo "start $(basename "$config") $$" >> "$FAKE_SINGBOX_LOG"
fi
case "$content" in
  *start-fail*) echo "bind failed" >&2; exit 21 ;;
  *crash*) sleep 0.12; exit 22 ;;
  *cache-test*) : > cache.db || exit 30 ;;
esac
trap 'exit 0' TERM INT
while :; do sleep 1; done
`
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}
