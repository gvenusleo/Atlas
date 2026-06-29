//go:build !windows

package weixin

import "os"

// writeSecureFile writes sensitive state using the current platform's private file permissions.
func writeSecureFile(path string, content []byte) error {
	return os.WriteFile(path, content, 0o600)
}

// secureStorePath sets private access permissions on sensitive state directories.
func secureStorePath(path string, _ bool) error {
	return os.Chmod(path, 0o700)
}
