//go:build !windows

package app

import "os"

func replaceFile(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
