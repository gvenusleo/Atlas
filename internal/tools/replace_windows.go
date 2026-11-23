package tools

import (
	"io/fs"
	"os"
	"syscall"
)

func hasOtherLinks(path string, _ fs.FileInfo) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	var info syscall.ByHandleFileInformation
	err = syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &info)
	return info.NumberOfLinks > 1, err
}
