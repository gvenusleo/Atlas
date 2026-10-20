package gui

import (
	"encoding/base64"

	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/imaging"
)

// 图片预览的尺寸，单位为 DIP：pictureWidth、pictureHeight 是工具结果里预览的最大尺寸，
// userPictureWidth、userPictureHeight 是用户消息里的（小一些，几张并排），都按比例缩小，
// 不放大。解码时长边缩到显示宽度的两倍（像素），高分屏够用，位图也不会比显示的大太多。
const (
	pictureWidth      = 640
	pictureHeight     = 360
	userPictureWidth  = 240
	userPictureHeight = 180

	pictureEdge     = 2 * pictureWidth
	userPictureEdge = 2 * userPictureWidth
)

// picture 是消息或工具结果里的图片，以及它在后台解码的结果。
type picture struct {
	data    agent.ImageContent // 解码完成后清空，不再占着 base64
	edge    int                // 解码后长边的上限（像素）
	started bool               // 已经开始解码
	bitmap  *ui.Bitmap         // 解码成功时才有
	err     string             // 解码失败或图片太大时的原因
}

// picturesOf 取出内容里的全部图片，没有时返回 nil；edge 是它们解码后长边的上限。
func picturesOf(content []agent.Content, edge int) []*picture {
	var out []*picture
	for _, c := range content {
		if img, ok := c.(agent.ImageContent); ok {
			out = append(out, &picture{data: img, edge: edge})
		}
	}
	return out
}

// decodePicture 解码图片，长边缩到 edge 以内。在后台调用。
func decodePicture(img agent.ImageContent, edge int) (*ui.Bitmap, error) {
	data, err := base64.StdEncoding.DecodeString(img.Data)
	if err != nil {
		return nil, err
	}
	decoded, err := imaging.Decode(data, edge)
	if err != nil {
		return nil, err
	}
	return ui.NewBitmap(decoded), nil
}

// startDecode 在后台解码图片，只启动一次；完成后回到主线程记下结果。
func (a *App) startDecode(p *picture) {
	if p.started {
		return
	}
	p.started = true
	go func() {
		bitmap, err := decodePicture(p.data, p.edge)
		a.win.Update(func() {
			p.bitmap, p.data = bitmap, agent.ImageContent{}
			if err != nil {
				p.err = err.Error()
			}
		})
	}()
}

// userPicture 画用户消息里的图片：按比例缩到不超过 userPictureWidth×userPictureHeight，
// 不放大。解码完成前，或解码失败时，是一个小方块，失败时写明原因。
func (a *App) userPicture(c *ui.Context, t *ui.Theme, p *picture) {
	a.startDecode(p)
	if p.bitmap == nil {
		ui.Column(c).Size(userPictureWidth/2, userPictureHeight/2).Padding(8).Radius(boxRound).
			Background(rowFill(t)).Children(func() {
			if p.err != "" {
				note(c, "无法预览："+p.err, smallSize, t.TextMuted)
			}
		})
		return
	}
	pixelsW, pixelsH := p.bitmap.Size()
	scale := min(1, userPictureWidth/float32(pixelsW), userPictureHeight/float32(pixelsH))
	ui.Image(c, p.bitmap).Size(float32(pixelsW)*scale, float32(pixelsH)*scale).Radius(boxRound)
}

// picturePreview 画读取到的图片：第一次画时在后台解码，解码完成前显示工具给的说明；
// 不能预览时说明原因。
//
// 图片按比例缩到不超过 pictureWidth×pictureHeight，也不超过所在的宽度，不放大。所在的宽度
// 取外层上一帧的：外层的高度由缩放后的图片定，图片绝对定位在外层里，不撑宽外层，
// 所以这个宽度不会被图片自己撑大，窄窗口里也不会把详情区撑宽。
func (a *App) picturePreview(c *ui.Context, t *ui.Theme, it *item) {
	p := it.pictures[0]
	a.startDecode(p)
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
			note(c, "无法预览："+p.err, smallSize, t.TextMuted)
		default:
			note(c, it.result, smallSize, t.TextMuted)
		}
	})
}
