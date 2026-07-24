//go:build cgo && windows

package csdk

/*
#include <windows.h>
#include <stddef.h>
#include "../../../bcos-c-sdk/bcos_sdk_c.h"

static int trustdb_loaded_library_path(wchar_t* output, size_t capacity)
{
    if (output == NULL || capacity < 2) { return -1; }
    HMODULE module = NULL;
    if (!GetModuleHandleExW(
            GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS |
                GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT,
            (LPCWSTR)&bcos_sdk_version,
            &module)) {
        return -1;
    }
    DWORD length = GetModuleFileNameW(module, output, (DWORD)capacity);
    if (length == 0 || length >= capacity) { return -2; }
    return (int)length;
}
*/
import "C"

import (
	"errors"
	"path/filepath"
	"syscall"
	"unsafe"
)

// LoadedLibraryPath returns the DLL that actually supplies
// bcos_sdk_version.
func LoadedLibraryPath() (string, error) {
	buffer := make([]uint16, 32768)
	length := int(C.trustdb_loaded_library_path(
		(*C.wchar_t)(unsafe.Pointer(&buffer[0])),
		C.size_t(len(buffer)),
	))
	if length <= 0 || length >= len(buffer) {
		return "", errors.New("cannot resolve loaded FISCO BCOS native SDK path")
	}
	path, err := filepath.Abs(syscall.UTF16ToString(buffer[:length]))
	if err != nil {
		return "", err
	}
	return filepath.Clean(path), nil
}
