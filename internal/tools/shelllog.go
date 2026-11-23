package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const shellLogRetention = 7 * 24 * time.Hour

var shellLogCleanup struct {
	sync.Mutex
	day string
}

func shellLogDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Atlas", "shell-logs"), nil
}

// PruneShellLogs 检查已完成的临时日志，每天至多扫描一次。
func PruneShellLogs(now time.Time) error {
	dir, err := shellLogDir()
	if err != nil {
		return err
	}
	shellLogCleanup.Lock()
	defer shellLogCleanup.Unlock()
	day := now.Format(time.DateOnly)
	if shellLogCleanup.day == day {
		return nil
	}
	err = pruneShellLogs(dir, now)
	if err == nil {
		shellLogCleanup.day = day
	}
	return err
}

func pruneShellLogs(dir string, now time.Time) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var failures error
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), "atlas-shell-") ||
			!strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err == nil && !info.ModTime().After(now.Add(-shellLogRetention)) {
			err = os.Remove(filepath.Join(dir, entry.Name()))
		}
		if !errors.Is(err, os.ErrNotExist) {
			failures = errors.Join(failures, err)
		}
	}
	return failures
}

func createShellLog(now time.Time) (*os.File, error) {
	dir, err := shellLogDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	// 清理失败时仍可执行命令，下一次调用再尝试清理。
	_ = PruneShellLogs(now)
	return os.CreateTemp(dir, "atlas-shell-*.running")
}

func finishShellOutput(output *os.File) (string, error) {
	text, truncated, readErr := shellOutput(output)
	if readErr == nil && !truncated {
		return text, nil
	}
	path, err := completeShellLog(output, time.Now())
	if err != nil {
		err = fmt.Errorf("保留完整日志失败: %w", err)
		if readErr != nil {
			return "", errors.Join(readErr, err)
		}
		return text + "\n\n[" + err.Error() + "]", nil
	}
	if readErr != nil {
		return "", fmt.Errorf("读取命令输出失败，原始日志: %s: %w", path, readErr)
	}
	return text + "\n完整输出: " + path, nil
}

func completeShellLog(file *os.File, now time.Time) (string, error) {
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Chtimes(file.Name(), now, now); err != nil {
		return "", err
	}
	path := strings.TrimSuffix(file.Name(), ".running") + ".log"
	return path, os.Rename(file.Name(), path)
}
