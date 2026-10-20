package gui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/imaging"
)

const (
	maxImages = 6 // 一条消息最多带几张图片

	// noImagesHint 在当前模型不接受图片时提示怎么打开。
	noImagesHint = "当前模型没有声明图片输入：" +
		"在 settings.json 里该模型的 modalities.input 加上 \"image\""
)

// imageExts 是可以当作图片的文件的扩展名（小写，不带点）。
var imageExts = []string{"png", "jpg", "jpeg", "webp", "gif"}

// imageFiles 取出这些路径里按扩展名是图片的。
func imageFiles(paths []string) []string {
	return slices.DeleteFunc(slices.Clone(paths), func(path string) bool {
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		return !slices.Contains(imageExts, ext)
	})
}

// attachment 是输入框里待发送的一张图片。
type attachment struct {
	id     int
	name   string // 提示与悬停时显示的文件名
	image  agent.ImageContent
	bitmap *ui.Bitmap // 缩略图；解码失败时为空
}

// draftMessage 把输入框里的文字与图片排成一条用户消息：文字在前，图片在后。
func draftMessage(text string, attachments []*attachment) agent.UserMessage {
	var content []agent.Content
	if text != "" {
		content = append(content, agent.TextContent{Text: text})
	}
	for _, f := range attachments {
		content = append(content, f.image)
	}
	return agent.UserMessage{Content: content}
}

// acceptsImages 报告会话选定的模型是否接受图片输入。
func (a *App) acceptsImages(ch *chat) bool {
	info, ok := a.modelInfo(ch.model)
	return ok && info.Modalities.AcceptsImages()
}

// pickImages 让用户选图片文件，加进会话的待发送图片。
func (a *App) pickImages(ch *chat) {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:   a.win,
			Title:    "选择图片",
			Filters:  []mygo.FileFilter{{Name: "图片", Extensions: imageExts}},
			Multiple: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		a.win.Update(func() { a.addImageFiles(ch, paths) })
	}()
}

// addImageFiles 把这些文件当作图片加进会话的待发送图片。
func (a *App) addImageFiles(ch *chat, paths []string) {
	for _, path := range paths {
		a.addImage(ch, filepath.Base(path), func() (io.ReadCloser, error) { return os.Open(path) })
	}
}

// dropFiles 处理拖进输入框的文件：图片加进待发送图片，其余的忽略并提示。
func (a *App) dropFiles(ch *chat, paths []string) {
	images := imageFiles(paths)
	if len(images) < len(paths) {
		a.toast = "只能添加图片（" + strings.Join(imageExts, "、") + "）"
	}
	a.addImageFiles(ch, images)
}

// pasteImages 处理粘贴：剪贴板里有图片文件，或没有文字只有图片（如截图）时，把图片加进
// 待发送图片并返回 true，输入框不再粘贴；其余返回 false，照常粘贴文字。text 是剪贴板里的文字。
// 这里只看剪贴板有哪些格式，图片数据到后台再读，粘贴大截图不会卡住界面。
func (a *App) pasteImages(ch *chat, text string) bool {
	formats := mygo.Clipboard.Formats()
	if slices.Contains(formats, transfer.FileList) || slices.Contains(formats, transfer.URIList) {
		if files, err := mygo.Clipboard.ReadFiles(); err == nil {
			if images := imageFiles(files); len(images) > 0 {
				a.addImageFiles(ch, images)
				return true
			}
		}
	}
	if text != "" || !slices.Contains(formats, transfer.PNG) {
		return false
	}
	a.addImage(ch, "粘贴的图片", func() (io.ReadCloser, error) {
		data, err := mygo.Clipboard.ReadFormat(transfer.PNG)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(data)), nil
	})
	return true
}

// addImage 在后台读取并整理一张图片，成功后加进会话的待发送图片，失败时提示原因。
// 整理期间 preparing 不为零，输入框上方显示等待的方块，也不能发送。
func (a *App) addImage(ch *chat, name string, open func() (io.ReadCloser, error)) {
	switch {
	case !a.acceptsImages(ch):
		a.toast = noImagesHint
		return
	case len(ch.attachments)+ch.preparing >= maxImages:
		a.toast = fmt.Sprintf("一条消息最多带 %d 张图片", maxImages)
		return
	}
	ch.preparing++
	go func() {
		img, bitmap, err := loadImage(open)
		a.win.Update(func() {
			ch.preparing--
			if err != nil {
				a.toast = name + "：" + err.Error()
				return
			}
			ch.next++
			ch.attachments = append(ch.attachments,
				&attachment{id: ch.next, name: name, image: img, bitmap: bitmap})
		})
	}()
}

// loadImage 读取并整理图片，再解出缩略图；缩略图解不出来不影响发送。
func loadImage(open func() (io.ReadCloser, error)) (agent.ImageContent, *ui.Bitmap, error) {
	r, err := open()
	if err != nil {
		return agent.ImageContent{}, nil, err
	}
	defer r.Close()
	img, err := imaging.Prepare(r)
	if err != nil {
		return agent.ImageContent{}, nil, err
	}
	bitmap, _ := decodePicture(img, userPictureEdge)
	return img, bitmap, nil
}
