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

// fileRasterExts 是文件面板按位图读取的图片扩展名，与 imaging 解得了的格式一致。
// 附件的 imageExts（attach.go）是另一回事，那是能发给模型的图片。
var fileRasterExts = []string{"png", "jpg", "jpeg", "webp", "gif"}

// isRaster 报告路径是不是要按位图读取的图片。SVG 是可编辑的文本，走文本那条路，
// 预览另算（parseSVG）。
func isRaster(p string) bool {
	return slices.Contains(fileRasterExts, strings.TrimPrefix(extOf(p), "."))
}

// isSVG 报告路径是不是 SVG。
func isSVG(p string) bool { return extOf(p) == ".svg" }

// svgImage 是把 SVG 文本解析成图的结果：src 不为空时画它，note 不为空时是解析失败的原因。
type svgImage struct {
	src  ui.ImageSource
	note string
}

// parseSVG 把 SVG 文本解析成可以画的图。预览跟着缓冲区走，所以按文本解析，而不是按文件。
func parseSVG(text string) svgImage {
	img, err := decodePreviewImage([]byte(text), true)
	if err != nil {
		return svgImage{note: err.Error()}
	}
	return svgImage{src: img}
}

// readImage 将图片读取结果整理成文件标签所需的数据与错误提示。
func readImage(p string) tabContent {
	img, stat, err := readImageFile(p)
	content := tabContent{img: img, stat: stat}
	switch {
	case errors.Is(err, errImageTooBig):
		content.note = fmt.Sprintf("%s 文件大小超过 %d MiB 限制",
			filepath.Base(p), imageMaxBytes>>20)
	case err != nil:
		content.note = noteOf(p, err)
	}
	return content
}

// readImageFile 只读普通文件。位图按 EXIF 方向转正，长边缩到 imageEdge 以内。
func readImageFile(p string) (ui.ImageSource, fileStat, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, fileStat{}, err
	}
	stat := fileStat{mtime: info.ModTime(), size: info.Size()}
	if !info.Mode().IsRegular() {
		return nil, stat, fmt.Errorf("%s 不是普通文件", filepath.Base(p))
	}
	file, err := os.Open(p)
	if err != nil {
		return nil, stat, err
	}
	defer file.Close()
	data, err := readImageData(file)
	if err != nil {
		return nil, stat, err
	}
	img, err := decodePreviewImage(data, isSVG(p))
	return img, stat, err
}

func decodePreviewImage(data []byte, svg bool) (ui.ImageSource, error) {
	if svg {
		img, err := ui.ParseSVG(data)
		if err != nil {
			return nil, fmt.Errorf("无法解析 SVG：%w", err)
		}
		return img, nil
	}
	img, err := imaging.Decode(data, imageEdge)
	if err != nil {
		return nil, err
	}
	return ui.NewBitmap(img), nil
}

// readImageData 限制实际读取的大小，文件大小和响应头都可能与内容不符。
func readImageData(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, imageMaxBytes+1))
	if err == nil && len(data) > imageMaxBytes {
		return nil, errImageTooBig
	}
	return data, err
}

// noteOf 把错误里的完整路径换成文件名，内容区里放不下完整路径。
func noteOf(p string, err error) string {
	return strings.ReplaceAll(err.Error(), p, filepath.Base(p))
}
