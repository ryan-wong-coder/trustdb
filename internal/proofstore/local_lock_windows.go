//go:build windows

package proofstore

import (
	"os"

	"golang.org/x/sys/windows"
)

func lockLocalNamespaceFile(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&overlapped,
	)
}

func unlockLocalNamespaceFile(file *os.File) error {
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
}

// Windows does not expose ACLs through FileMode permission bits. The lock file
// is still created with mode 0600 and opened without sharing its byte-range
// lock; ACL hardening belongs to deployment directory policy.
func validateLocalRootPermissions(os.FileInfo) bool {
	return true
}

func validateLocalLockPermissions(os.FileInfo) bool {
	return true
}
