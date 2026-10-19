package gui

import (
	"bytes"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
)

// walkLimit 是遍历非 git 目录时最多列出的文件数，免得在巨大的目录里列太久、占太多内存。
const walkLimit = 20000

// listPaths 列出工作目录下的文件与目录，供补全消息里的 @ 引用：路径相对工作目录、
// 用 / 分隔，目录以 / 结尾。git 仓库里按 git 的口径，列出跟踪的文件与没被 .gitignore
// 忽略的新文件；其余目录（或 git 出错时）自浅入深遍历，跳过 .git 与 node_modules，
// 超过 walkLimit 时只列到浅处。macOS 自动生成的 .DS_Store 不列出，不管仓库忽略没有。
// 出错时当作没有。
func listPaths(cwd string) []string {
	var paths []string
	if inGitRepo(cwd) {
		paths = gitPaths(cwd)
	}
	if paths == nil {
		paths = walkPaths(cwd)
	}
	paths = slices.DeleteFunc(paths, func(p string) bool { return path.Base(p) == ".DS_Store" })
	return withDirs(paths)
}

// inGitRepo 报告 dir 或它的上层目录里有没有 .git（工作树与子模块里它是文件）。
// 没有时不调用 git：macOS 上没装开发工具时，运行 git 会弹出安装提示。
func inGitRepo(dir string) bool {
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// gitPaths 用 git 列出 cwd 下的文件，出错时返回 nil。git 把已删除但还没提交的跟踪文件
// 也列出来，所以要再去掉工作区里已经不在的。
func gitPaths(cwd string) []string {
	out, err := exec.Command("git", "-C", cwd, "ls-files", "-co", "--exclude-standard", "-z").
		Output()
	if err != nil {
		return nil
	}
	var paths []string
	for p := range bytes.SplitSeq(out, []byte{0}) {
		if len(p) > 0 {
			paths = append(paths, string(p))
		}
	}
	return slices.DeleteFunc(paths, func(p string) bool {
		_, err := os.Lstat(filepath.Join(cwd, p))
		return err != nil
	})
}

// walkPaths 自浅入深遍历 cwd，列出其中的文件。
func walkPaths(cwd string) []string {
	var paths []string
	for dirs := []string{""}; len(dirs) > 0 && len(paths) < walkLimit; dirs = dirs[1:] {
		entries, _ := os.ReadDir(filepath.Join(cwd, dirs[0]))
		for _, e := range entries {
			rel := path.Join(dirs[0], e.Name())
			switch {
			case !e.IsDir():
				paths = append(paths, rel)
			case e.Name() != ".git" && e.Name() != "node_modules":
				dirs = append(dirs, rel)
			}
		}
	}
	return paths
}

// withDirs 在文件之外补上它们所在的各级目录。
func withDirs(files []string) []string {
	dirs := make(map[string]bool)
	for _, f := range files {
		for dir := path.Dir(f); dir != "." && !dirs[dir+"/"]; dir = path.Dir(dir) {
			dirs[dir+"/"] = true
		}
	}
	return slices.AppendSeq(files, maps.Keys(dirs))
}
