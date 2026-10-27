package schedule

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// File 是定时任务的文件名，在配置目录下。
const File = "schedule.json"

// backup 是损坏的文件被改成的名字。
const backup = File + ".bad"

// document 是 schedule.json 的内容。
type document struct {
	Tasks []Task `json:"tasks"`
}

// Load 读取 dir 下的定时任务。文件不存在时没有任务；读不了（权限、临时不可读）时返回读取的
// 错误，文件原样留着；内容损坏时把它改名为 schedule.json.bad，免得之后的保存覆盖掉用户写的
// 内容，同样当作没有任务，并返回说明原因的错误。出错都不阻止启动。
func Load(dir string) ([]Task, error) {
	path := filepath.Join(dir, File)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败：%w", File, err)
	}
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		if renameErr := os.Rename(path, filepath.Join(dir, backup)); renameErr != nil {
			return nil, fmt.Errorf("%s 已损坏，改名为 %s 也失败了：%w", File, backup, err)
		}
		return nil, fmt.Errorf("%s 已损坏，原文件改名为 %s：%w", File, backup, err)
	}
	return doc.Tasks, nil
}

// Save 把定时任务写进 dir：先写同目录的临时文件再改名，读者不会看到写了一半的文件。
func Save(dir string, tasks []Task) error {
	data, err := json.Marshal(document{Tasks: tasks})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".schedule-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // 改名成功之后它已经不在了
	_, err = tmp.Write(data)
	if err = errors.Join(err, tmp.Close()); err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, File))
	}
	return err
}
