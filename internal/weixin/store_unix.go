//go:build !windows

package weixin

import "os"

// writeSecureFile 用当前平台的私有文件权限写入敏感状态。
func writeSecureFile(path string, content []byte) error {
	return os.WriteFile(path, content, 0o600)
}

// secureStorePath 固定敏感状态目录的私有访问权限。
func secureStorePath(path string, _ bool) error {
	return os.Chmod(path, 0o700)
}
