//go:build darwin || linux

package tools

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func hasOtherLinks(_ string, info fs.FileInfo) (bool, error) {
	return info.Sys().(*syscall.Stat_t).Nlink > 1, nil
}

func copyExtendedAttributes(original, replacement *os.File) error {
	src, dst := int(original.Fd()), int(replacement.Fd())
	n, err := unix.Flistxattr(src, nil)
	if errors.Is(err, unix.ENOTSUP) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取扩展属性列表失败: %w", err)
	}
	if n == 0 {
		return nil
	}
	names := make([]byte, n)
	n, err = unix.Flistxattr(src, names)
	if err != nil {
		return fmt.Errorf("读取扩展属性列表失败: %w", err)
	}
	for raw := range bytes.SplitSeq(names[:n], []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		name := string(raw)
		if err := copyExtendedAttribute(src, dst, name); err != nil {
			return fmt.Errorf("复制扩展属性 %q 失败: %w", name, err)
		}
	}
	return nil
}

func copyExtendedAttribute(src, dst int, name string) error {
	n, err := unix.Fgetxattr(src, name, nil)
	if err != nil {
		return err
	}
	// 至少传入一个字节，空属性在两次调用之间增长时也会被实际读取。
	value := make([]byte, max(n, 1))
	n, err = unix.Fgetxattr(src, name, value)
	if err != nil {
		return err
	}
	return unix.Fsetxattr(dst, name, value[:n], 0)
}
