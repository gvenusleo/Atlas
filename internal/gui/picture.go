package gui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/agent"
)

// 图片预览的限制：pictureMaxPixels 是解码的图片最多有多少像素，更大的不预览，免得一张
// 压得很小的巨幅图占满内存；pictureWidth、pictureHeight 是预览的最大尺寸（DIP），
// 按比例缩小，不放大。
const (
	pictureMaxPixels = 40_000_000
	pictureWidth     = 640
	pictureHeight    = 360
)

// picture 是工具结果里的图片，以及它在后台解码的结果。
type picture struct {
	data    agent.ImageContent // 解码完成后清空，不再占着 base64
	started bool               // 已经开始解码
	bitmap  *ui.Bitmap         // 解码成功时才有
	err     string             // 解码失败或图片太大时的原因
}

// pictureOf 取出工具结果里的第一张图片，没有时返回 nil。
func pictureOf(content []agent.Content) *picture {
	for _, c := range content {
		if img, ok := c.(agent.ImageContent); ok {
			return &picture{data: img}
		}
	}
	return nil
}

// decodePicture 解码图片：先按尺寸拦下像素太多的，再解码。在后台调用。
func decodePicture(img agent.ImageContent) (*ui.Bitmap, error) {
	data, err := base64.StdEncoding.DecodeString(img.Data)
	if err != nil {
		return nil, err
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if config.Width*config.Height > pictureMaxPixels {
		return nil, fmt.Errorf("图片 %d×%d 太大", config.Width, config.Height)
	}
	return ui.DecodeBitmap(data)
}

// picturePreview 画读取到的图片：第一次画时在后台解码，解码完成前显示工具给的说明；
// 不能预览时说明原因。
//
// 图片按比例缩到不超过 pictureWidth×pictureHeight，也不超过所在的宽度，不放大。所在的宽度
// 取外层上一帧的：外层的高度由缩放后的图片定，图片绝对定位在外层里，不撑宽外层，
// 所以这个宽度不会被图片自己撑大，窄窗口里也不会把详情区撑宽。
func (a *App) picturePreview(c *ui.Context, t *ui.Theme, it *item) {
	p := it.picture
	if !p.started {
		p.started = true
		go func() {
			bitmap, err := decodePicture(p.data)
			a.win.Update(func() {
				p.bitmap, p.data = bitmap, agent.ImageContent{}
				if err != nil {
					p.err = err.Error()
				}
			})
		}()
	}
	box := ui.Column(c).ClipX()
	var w, h float32
	if p.bitmap != nil {
		pixelsW, pixelsH := p.bitmap.Size()
		maxW := float32(pictureWidth)
		if boxW := box.Bounds().W; boxW > 0 {
			maxW = min(maxW, boxW)
		}
		scale := min(1, maxW/float32(pixelsW), pictureHeight/float32(pixelsH))
		w, h = float32(pixelsW)*scale, float32(pixelsH)*scale
		box.Height(h)
	}
	box.Children(func() {
		switch {
		case p.bitmap != nil:
			ui.Image(c, p.bitmap).Absolute().Top(0).Left(0).Size(w, h).Radius(boxRound)
		case p.err != "":
			note(c, "无法预览："+p.err, 12, t.TextMuted)
		default:
			note(c, it.result, 12, t.TextMuted)
		}
	})
}
