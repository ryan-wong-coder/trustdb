//go:build unix

package statusnotify

import "os"

func replaceStatusStateFile(source, target string) error {
	return os.Rename(source, target)
}

func syncStatusStateDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}
