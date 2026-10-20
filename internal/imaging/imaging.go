// Package imaging 整理要交给模型的图片：限制格式与大小，太大的缩小，按 EXIF 方向转正；
// 也给界面解码预览用的图。
package imaging

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // image.Decode 识别 GIF
	"image/jpeg"
	"image/png"
	"io"
	"slices"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // image.Decode 识别 WebP

	"github.com/gvenusleo/atlas/internal/agent"
)

// 图片的限制。每次请求都会把历史里的图片重传一遍，所以进来时就要缩小。
const (
	maxSource   = 20 << 20   // 原始数据的大小上限
	maxPixels   = 40_000_000 // 解码前按尺寸拦下像素太多的图，免得一张压得很小的巨幅图占满内存
	maxEdge     = 2048       // 交给模型的图片的长边上限（像素）
	keepBytes   = 1 << 20    // 长边不超限、不超过这个大小、方向正确的图原样保留
	jpegQuality = 85
)

// formats 是接受的格式，取 image.DecodeConfig 报的格式名。
var formats = []string{"png", "jpeg", "webp", "gif"}

// ErrUnsupported 表示数据不是接受的图片格式。
var ErrUnsupported = errors.New("不支持的图片格式，只收 PNG、JPEG、WebP 与 GIF")

// Prepare 把用户给的图片整理成发给模型的图片块：只收 PNG、JPEG、WebP 与 GIF。
// 太大的图缩到长边 maxEdge 以内，不透明的编成 JPEG，有透明度的编成 PNG；带旋转标记的
// JPEG 转正；GIF 只取首帧。不需要处理的图原样保留。
func Prepare(r io.Reader) (agent.ImageContent, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxSource+1))
	if err != nil {
		return agent.ImageContent{}, fmt.Errorf("读取图片失败: %w", err)
	}
	if len(data) > maxSource {
		return agent.ImageContent{}, fmt.Errorf("图片超过 %d MB", maxSource>>20)
	}
	size, format, err := image.DecodeConfig(bytes.NewReader(data))
	if errors.Is(err, image.ErrFormat) {
		return agent.ImageContent{}, ErrUnsupported
	}
	if err != nil {
		return agent.ImageContent{}, fmt.Errorf("无法读取图片: %w", err)
	}
	if !slices.Contains(formats, format) {
		return agent.ImageContent{}, ErrUnsupported
	}
	if format != "gif" && len(data) <= keepBytes && max(size.Width, size.Height) <= maxEdge &&
		exifOrientation(data) == 1 {
		return block(data, "image/"+format), nil
	}
	img, err := Decode(data, maxEdge)
	if err != nil {
		return agent.ImageContent{}, err
	}
	out, mime, err := reencode(img)
	if err != nil {
		return agent.ImageContent{}, fmt.Errorf("编码图片失败: %w", err)
	}
	return block(out, mime), nil
}

// Decode 解码图片：按 EXIF 方向转正，长边缩到 edge 以内，不放大。
func Decode(data []byte, edge int) (image.Image, error) {
	size, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("无法读取图片: %w", err)
	}
	if size.Width*size.Height > maxPixels {
		return nil, fmt.Errorf("图片 %d×%d 太大", size.Width, size.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("无法读取图片: %w", err)
	}
	// 缩小之后再转，要处理的像素少得多。
	return orient(fit(img, edge), exifOrientation(data)), nil
}

// block 把图片数据包成图片块。
func block(data []byte, mime string) agent.ImageContent {
	return agent.ImageContent{Data: base64.StdEncoding.EncodeToString(data), MIMEType: mime}
}

// reencode 重新编码图片：不透明的用 JPEG，有透明度的用 PNG。
func reencode(img image.Image) (data []byte, mime string, err error) {
	var out bytes.Buffer
	if opaque, ok := img.(interface{ Opaque() bool }); ok && opaque.Opaque() {
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: jpegQuality})
		return out.Bytes(), "image/jpeg", err
	}
	err = png.Encode(&out, img)
	return out.Bytes(), "image/png", err
}

// fit 把长边缩到 edge 以内，不放大。
func fit(img image.Image, edge int) image.Image {
	b := img.Bounds()
	long := max(b.Dx(), b.Dy())
	if long <= edge {
		return img
	}
	w, h := max(1, b.Dx()*edge/long), max(1, b.Dy()*edge/long)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}
