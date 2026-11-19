package gui

import (
	"cmp"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func (a *App) useMarkdown(
	o *markdown.Options, images *markdownImages, anchor *string, base string,
) *markdown.Options {
	if images.base != base {
		*images = markdownImages{base: base}
	}
	// MyGo 在构建段落时调用 Block。图片在这里逐张处理，地址尚未写完的只显示占位。
	o.Block = func(c *ui.Context, d *parser.Document, i int) bool {
		if d.Block(i).Kind != parser.Paragraph {
			return false
		}
		in := d.Inlines(i)
		if !isImageParagraph(in) {
			return false
		}
		size := cmp.Or(c.Theme().FontSize, 14)
		if o.Theme != nil {
			size = cmp.Or(o.Theme.FontSize, size)
		}
		images.showParagraph(c, in, size)
		return true
	}
	return a.useLinks(o, anchor, base)
}

// isImageParagraph 沿用 MyGo 对图片段落的判定，混有文字的段落交给默认渲染器。
func isImageParagraph(in []parser.Inline) bool {
	hasImage := false
	for i := 0; i < len(in); i = in[i].Next {
		switch n := in[i]; n.Kind {
		case parser.Image:
			hasImage = true
		case parser.SoftBreak, parser.LineBreak:
		case parser.Text:
			if strings.TrimSpace(n.Text) != "" {
				return false
			}
		default:
			return false
		}
	}
	return hasImage
}

func (m *markdownImages) showParagraph(c *ui.Context, in []parser.Inline, size float32) {
	t := c.Theme()
	ui.Row(c).Wrap().Gap(size * 0.5).AlignSelf(ui.Stretch).Children(func() {
		for i := 0; i < len(in); i = in[i].Next {
			n := in[i]
			if n.Kind != parser.Image {
				continue
			}
			alt := cmp.Or(parser.PlainText(in[i+1:n.Next]), "图片")
			var src ui.ImageSource
			if !n.Pending && n.Dest != "" {
				src = m.getImage(c, n.Dest)
			}
			if src == nil {
				ui.Row(c.Key(i)).Padding(size*0.5, size*0.75).Radius(size * 0.3).
					Background(t.Text.Alpha(0.07)).Role(ui.RoleImage).Label(alt).Children(func() {
					ui.Text(c, alt).TextColor(t.TextMuted).FontSize(size * 0.875)
				})
				continue
			}
			img := ui.Image(c.Key(i), src).Radius(size * 0.3).MaxWidthPercent(100).Label(alt)
			if w, h := ui.ImageSize(src); w > 0 && h > 0 {
				img.AspectRatio(w / h)
			}
			if n.Title != "" {
				img.Tooltip(n.Title)
			}
		}
	})
}
