//go:build !windows

package wal

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockNamespaceFile(file *os.File, exclusive bool) error {
	mode := unix.LOCK_SH | unix.LOCK_NB
	if exclusive {
		mode = unix.LOCK_EX | unix.LOCK_NB
	}
	return unix.Flock(int(file.Fd()), mode)
}

func unlockNamespaceFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}

func validateNamespaceRootPermissions(info os.FileInfo) bool {
	return info.Mode().Perm()&0o022 == 0
}

func validateNamespaceLockPermissions(info os.FileInfo) bool {
	return info.Mode().Perm() == 0o600
}
