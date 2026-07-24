//go:build windows

package statusnotify

import (
	"errors"

	"golang.org/x/sys/windows"
)

func replaceStatusStateFile(source, target string) error {
	sourcePath, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	targetPath, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(
		sourcePath,
		targetPath,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}

func syncStatusStateDirectory(path string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return err
	}
	flushErr := windows.FlushFileBuffers(handle)
	closeErr := windows.CloseHandle(handle)
	return errors.Join(flushErr, closeErr)
}
