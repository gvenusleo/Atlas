package gui

import (
	"errors"
	"fmt"
	"io"
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

// errImageTooBig 说明图片文件超过了 imageMaxBytes，读的时候才发现。
var errImageTooBig = errors.New("图片太大")

// fileImageExts 是文件面板能查看的图片扩展名：imaging 解得了的位图格式，加上 SVG。
// 附件的 imageExts（attach.go）是另一回事，那是能发给模型的图片。
var fileImageExts = []string{"png", "jpg", "jpeg", "webp", "gif", "svg"}

// isImage 报告路径是不是文件面板能查看的图片。
func isImage(p string) bool {
	return slices.Contains(fileImageExts, strings.TrimPrefix(extOf(p), "."))
}

// isSVG 报告路径是不是 SVG。
func isSVG(p string) bool { return extOf(p) == ".svg" }

// readImage 读取并解码一个图片文件：位图按 EXIF 转正、长边缩到 imageEdge 以内，
// SVG 解析成矢量。解码不便宜，调用方要放在后台。
func readImage(p string) tabContent {
	info, err := os.Stat(p)
	if err != nil {
		return tabContent{note: noteOf(p, err)}
	}
	content := tabContent{stat: fileStat{mtime: info.ModTime(), size: info.Size()}}
	if !info.Mode().IsRegular() {
		content.note = filepath.Base(p) + " 不是普通文件"
		return content
	}
	data, err := readImageData(p)
	switch {
	case errors.Is(err, errImageTooBig):
		content.note = fmt.Sprintf("%s 文件大小超过 %d MiB 限制",
			filepath.Base(p), imageMaxBytes>>20)
		return content
	case err != nil:
		content.note = noteOf(p, err)
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

// readImageData 读取图片文件。文件可能在 Stat 之后长大，所以读取本身也要有上限。
func readImageData(p string) ([]byte, error) {
	file, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, imageMaxBytes+1))
	if err == nil && len(data) > imageMaxBytes {
		return nil, errImageTooBig
	}
	return data, err
}

// noteOf 把错误里的完整路径换成文件名，内容区里放不下完整路径。
func noteOf(p string, err error) string {
	return strings.ReplaceAll(err.Error(), p, filepath.Base(p))
}
