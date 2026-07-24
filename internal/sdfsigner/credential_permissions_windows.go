//go:build windows

package sdfsigner

import "os"

// Windows credential loading fails closed until an owner-only DACL policy is
// continuously runtime-qualified on every supported filesystem.
func credentialFilePermissionsSafe(os.FileInfo) bool {
	return false
}
