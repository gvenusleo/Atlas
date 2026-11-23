//go:build darwin || linux

package tools

import (
	"io/fs"
	"syscall"
)

func hasOtherLinks(_ string, info fs.FileInfo) (bool, error) {
	return info.Sys().(*syscall.Stat_t).Nlink > 1, nil
}
