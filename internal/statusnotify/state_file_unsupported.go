//go:build !unix && !windows

package statusnotify

import (
	"errors"
	"os"
)

func replaceStatusStateFile(source, target string) error {
	return os.Rename(source, target)
}

func syncStatusStateDirectory(string) error {
	return errors.ErrUnsupported
}
