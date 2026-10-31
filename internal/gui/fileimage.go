package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/imaging"
)

// 图片的限制。位图解码后长边缩到 imageEdge 以内，高分屏也够清楚。
const (
	imageMaxBytes = 20 << 20
	imageEdge     = 2048
)

// isImage 报告路径是不是文件面板能查看的图片：attach 认的位图格式，加上 SVG。
func isImage(p string) bool {
	if isSVG(p) {
		return true
	}
	return slices.Contains(imageExts, strings.TrimPrefix(extOf(p), "."))
}

// isSVG 报告路径是不是 SVG。
func isSVG(p string) bool { return extOf(p) == ".svg" }

// readImage 读取并解码一个图片文件：位图按 EXIF 转正、长边缩到 imageEdge 以内，
// SVG 解析成矢量。解码不便宜，调用方要放在后台。
func readImage(p string) tabContent {
	info, err := os.Stat(p)
	if err != nil {
		return tabContent{note: fileName(p, err)}
	}
	content := tabContent{st: fileStat{mtime: info.ModTime(), size: info.Size()}}
	switch {
	case !info.Mode().IsRegular():
		content.note = filepath.Base(p) + " 不是普通文件"
		return content
	case info.Size() > imageMaxBytes:
		content.note = fmt.Sprintf("%s 文件大小超过 %d MiB 限制",
			filepath.Base(p), imageMaxBytes>>20)
		return content
	}
	data, err := os.ReadFile(p)
	if err != nil {
		content.note = fileName(p, err)
		return content
	}
	if isSVG(p) {
		svg, err := ui.ParseSVG(data)
		if err != nil {
			content.note = "无法解析 SVG：" + err.Error()
			return content
		}
		content.img = svg
		return content
	}
	img, err := imaging.Decode(data, imageEdge)
	if err != nil {
		content.note = err.Error()
		return content
	}
	content.img = ui.NewBitmap(img)
	return content
}

// fileName 把错误里的完整路径换成文件名，内容区里放不下完整路径。
func fileName(p string, err error) string {
	return strings.ReplaceAll(err.Error(), p, filepath.Base(p))
}
