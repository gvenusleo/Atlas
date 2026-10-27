package gui

import (
	"encoding/json/v2"
	"errors"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// 这个文件只管偏好的内容与它在磁盘上的格式；偏好怎样生效、怎样改，见 settings.go。

// prefsFile 是界面偏好的文件名，在配置目录下。
const prefsFile = "gui.json"

// prefs 是界面偏好，只有 gui 读写；空串表示默认：主题跟随系统，界面字体用系统的，
// 代码字体用 monoFamily，菜单栏里显示托盘图标（所以记的是隐藏，默认值才是零值）。
type prefs struct {
	Theme    mygo.ThemeSource `json:"theme,omitzero"`
	UIFont   string           `json:"ui_font,omitzero"`
	CodeFont string           `json:"code_font,omitzero"`
	HideTray bool             `json:"hide_tray,omitzero"`
}

// loadPrefs 读取 dir 下的偏好。文件缺失、损坏或值不认识时用默认值，不阻止启动。
func loadPrefs(dir string) prefs {
	var p prefs
	data, err := os.ReadFile(filepath.Join(dir, prefsFile))
	if err != nil || json.Unmarshal(data, &p) != nil {
		return prefs{}
	}
	if p.Theme != mygo.ThemeLight && p.Theme != mygo.ThemeDark {
		p.Theme = ""
	}
	return p
}

// savePrefs 把偏好写进 dir：先写同目录的临时文件再改名，读者不会看到写了一半的文件。
func savePrefs(dir string, p prefs) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".gui-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // 改名成功之后它已经不在了
	_, err = tmp.Write(data)
	if err = errors.Join(err, tmp.Close()); err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, prefsFile))
	}
	return err
}

// uiFamily 是界面用的字体族：选了字体就用它，缺的字形退回系统界面字体；空表示系统的。
func (p prefs) uiFamily() string {
	if p.UIFont == "" {
		return ""
	}
	return p.UIFont + ", system-ui"
}

// codeFamily 是代码用的字体族：选了字体就用它，缺的字形退回系统等宽字体。
func (p prefs) codeFamily() string {
	if p.CodeFont == "" {
		return monoFamily
	}
	return p.CodeFont + ", monospace"
}
