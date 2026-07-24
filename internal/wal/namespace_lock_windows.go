//go:build windows

package wal

import (
	"os"

	"golang.org/x/sys/windows"
)

func lockNamespaceFile(file *os.File, exclusive bool) error {
	flags := uint32(windows.LOCKFILE_FAIL_IMMEDIATELY)
	if exclusive {
		flags |= windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	var overlapped windows.Overlapped
	return windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, 1, 0, &overlapped)
}

func unlockNamespaceFile(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
}

func validateNamespaceRootPermissions(os.FileInfo) bool {
	return true
}

func validateNamespaceLockPermissions(os.FileInfo) bool {
	return true
}
