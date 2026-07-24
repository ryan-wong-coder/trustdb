//go:build !windows

package sdfsigner

import "os"

func credentialFilePermissionsSafe(info os.FileInfo) bool {
	return info.Mode().Perm()&0o077 == 0
}
