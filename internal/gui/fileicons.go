package gui

import (
	"cmp"
	"path"
	"strings"

	"github.com/egoist/mygo/ui"
)

// fileIconNames 把小写的文件名或扩展名（带点）对应到 icons/material 里的图标名；
// 图标取自 Material Icon Theme，对应关系也参照它。先按文件名、再按扩展名找。
var fileIconNames = map[string]string{
	"go.mod": "go-mod", "go.sum": "go-mod", "dockerfile": "docker", "license": "license",
	"readme.md": "readme", "package.json": "nodejs", "tsconfig.json": "tsconfig",
	"makefile": "makefile", ".gitignore": "git", ".env": "tune",

	".go": "go", ".md": "markdown", ".yaml": "yaml", ".yml": "yaml", ".json": "json",
	".toml": "toml", ".ts": "typescript", ".tsx": "react_ts", ".js": "javascript",
	".mjs": "javascript", ".cjs": "javascript", ".jsx": "react", ".py": "python", ".rs": "rust",
	".sh": "console", ".zsh": "console", ".bash": "console", ".html": "html", ".css": "css",
	".sql": "database", ".c": "c", ".h": "h", ".cpp": "cpp", ".cc": "cpp", ".cxx": "cpp",
	".java": "java", ".swift": "swift", ".png": "image", ".jpg": "image", ".jpeg": "image",
	".gif": "image", ".webp": "image", ".ico": "image", ".svg": "svg", ".pdf": "pdf",
	".csv": "table", ".tsv": "table", ".xlsx": "table", ".lock": "lock", ".xml": "xml",
	".plist": "xml", ".ini": "settings", ".conf": "settings", ".mk": "makefile",
	".zip": "zip", ".tar": "zip", ".gz": "zip",
}

// fileIcons 是用到的文件类型图标，按图标名；启动时就全部解析，图标名写错在这时就暴露。
var fileIcons = func() map[string]*ui.SVG {
	icons := map[string]*ui.SVG{"document": icon("material/document")}
	for _, name := range fileIconNames {
		if icons[name] == nil {
			icons[name] = icon("material/" + name)
		}
	}
	return icons
}()

// fileIcon 返回路径对应的图标：目录是界面里通用的文件夹，文件按名称或扩展名找，
// 都不认识就是一页文档。
func fileIcon(p string) *ui.SVG {
	if strings.HasSuffix(p, "/") {
		return folderIcon
	}
	base := strings.ToLower(path.Base(p))
	name := cmp.Or(fileIconNames[base], fileIconNames[path.Ext(base)], "document")
	return fileIcons[name]
}
