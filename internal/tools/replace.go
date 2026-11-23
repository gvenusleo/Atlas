package tools

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// fileTarget 记住解析后的目标及读文件前的状态，防止编辑期间覆盖别处的修改。
type fileTarget struct {
	path, resolved string
	info           fs.FileInfo
}

func inspectTarget(path string) (fileTarget, error) {
	target := fileTarget{path: path}
	resolved, err := filepath.EvalSymlinks(path)
	if errors.Is(err, fs.ErrNotExist) {
		// 已存在但无法解析的路径可能是悬空链接。
		if _, statErr := os.Lstat(path); errors.Is(statErr, fs.ErrNotExist) {
			target.resolved = path
			return target, nil
		}
	}
	if err != nil {
		return target, err
	}
	target.resolved = resolved
	target.info, err = os.Stat(resolved)
	if err != nil {
		return target, err
	}
	if !target.info.Mode().IsRegular() {
		return target, errors.New("不是普通文件")
	}
	linked, err := hasOtherLinks(resolved, target.info)
	if err != nil {
		return target, err
	}
	if linked {
		return target, errors.New("文件存在其他硬链接，无法替换")
	}
	return target, nil
}

// replace 先完整写出内容，再提交替换，失败时原件仍可读取。
func (t fileTarget) replace(ctx context.Context, write func(*os.File) error) error {
	parent := filepath.Dir(t.resolved)
	dir, err := os.MkdirTemp(parent, ".atlas-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(dir)
	path := filepath.Join(dir, "data")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	err = write(file)
	if err == nil && t.info != nil {
		err = t.copyMetadata(file)
	}
	if err = errors.Join(err, file.Close()); err != nil {
		return err
	}
	current, err := inspectTarget(t.path)
	if err != nil {
		return err
	}
	if !t.matches(current) {
		return fmt.Errorf("%s 在磁盘上已变更", t.path)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(path, t.resolved)
}

func (t fileTarget) copyMetadata(file *os.File) error {
	// 保留原地写入对目标写权限的检查，并通过打开的文件复制扩展属性。
	original, err := os.OpenFile(t.resolved, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer original.Close()
	if err := file.Chmod(t.info.Mode().Perm()); err != nil {
		return err
	}
	return copyExtendedAttributes(original, file)
}

func (t fileTarget) matches(current fileTarget) bool {
	if t.resolved != current.resolved {
		return false
	}
	if t.info == nil || current.info == nil {
		return t.info == nil && current.info == nil
	}
	return os.SameFile(t.info, current.info) && t.info.Size() == current.info.Size() &&
		t.info.ModTime().Equal(current.info.ModTime()) && t.info.Mode() == current.info.Mode()
}
