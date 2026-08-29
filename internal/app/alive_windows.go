//go:build windows

package app

import "os"

func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// FindProcess only succeeds for an existing process handle on Windows.
	_ = process.Release()
	return true
}
