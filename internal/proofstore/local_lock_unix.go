//go:build !windows

package proofstore

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockLocalNamespaceFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func unlockLocalNamespaceFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}

func validateLocalRootPermissions(info os.FileInfo) bool {
	return info.Mode().Perm()&0o022 == 0
}

func validateLocalLockPermissions(info os.FileInfo) bool {
	return info.Mode().Perm() == 0o600
}
