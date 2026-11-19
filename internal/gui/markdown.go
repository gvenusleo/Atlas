package gui

import (
	"errors"
	"fmt"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// markdownImages 的缓存由主线程管理，后台任务只向自己的通道返回图片。
type markdownImages struct {
	base    string
	entries map[string]*markdownImage
}

type markdownImage struct {
	src     ui.ImageSource
	result  <-chan ui.ImageSource
	retryAt time.Time
}

// getImage 只为同一地址启动一次加载。失败后留出间隔，避免每次重建视图都重新请求。
func (m *markdownImages) getImage(c *ui.Context, dest string) ui.ImageSource {
	if m.entries == nil {
		m.entries = make(map[string]*markdownImage)
	}
	entry := m.entries[dest]
	if entry == nil {
		entry = &markdownImage{}
		m.entries[dest] = entry
	}
	now := c.Now()
	if entry.result != nil {
		select {
		case entry.src = <-entry.result:
			entry.result = nil
			if entry.src == nil {
				entry.retryAt = now.Add(10 * time.Second)
			}
		default:
		}
	}
	if entry.src == nil && entry.result == nil && !now.Before(entry.retryAt) {
		result := make(chan ui.ImageSource, 1)
		entry.result = result
		base, services := m.base, c.Services()
		go func() {
			src, _ := loadMarkdownImage(base, dest)
			result <- src
			services.Invalidate()
		}()
	}
	return entry.src
}

// forgetLocalImages 随文档重读丢弃旧的本地位图和在途结果，保留已下载的网络图片。
func (m *markdownImages) forgetLocalImages() {
	maps.DeleteFunc(m.entries, func(dest string, _ *markdownImage) bool {
		u, err := url.Parse(dest)
		return err != nil || !isWebImage(u)
	})
}

func isWebImage(u *url.URL) bool { return u.Scheme == "http" || u.Scheme == "https" }

func loadMarkdownImage(base, dest string) (ui.ImageSource, error) {
	// Windows 盘符会被 URL 解析器认成协议。
	if vol := filepath.VolumeName(dest); len(vol) == 2 && filepath.IsAbs(dest) {
		p, err := url.PathUnescape(dest)
		if err != nil {
			return nil, err
		}
		src, _, err := readImageFile(p)
		return src, err
	}
	u, err := url.Parse(dest)
	if err != nil {
		return nil, err
	}
	if isWebImage(u) {
		return fetchMarkdownImage(u)
	}
	switch u.Scheme {
	case "file":
		if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
			return nil, errors.New("图片文件地址须指向本机")
		}
		// file:///C:/… 比 Windows 的绝对路径多一个斜杠。
		if p, ok := strings.CutPrefix(u.Path, "/"); ok && filepath.VolumeName(p) != "" {
			u.Path = p
		}
		if !filepath.IsAbs(filepath.FromSlash(u.Path)) {
			return nil, errors.New("图片文件地址缺少绝对路径")
		}
	case "":
		if u.Host != "" || u.Path == "" {
			return nil, errors.New("图片地址缺少本地路径或 HTTP(S) 协议")
		}
	default:
		return nil, fmt.Errorf("不支持的图片地址协议：%s", u.Scheme)
	}
	p := filepath.FromSlash(u.Path)
	if !filepath.IsAbs(p) {
		p = filepath.Join(base, p)
	}
	src, _, err := readImageFile(p)
	return src, err
}

var markdownImageClient = &http.Client{Timeout: 10 * time.Second}

func fetchMarkdownImage(u *url.URL) (ui.ImageSource, error) {
	resp, err := markdownImageClient.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("下载图片失败，HTTP 状态码 %d", resp.StatusCode)
	}
	data, err := readImageData(resp.Body)
	if err != nil {
		return nil, err
	}
	kind, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return decodePreviewImage(data, kind == "image/svg+xml" || isSVG(resp.Request.URL.Path))
}
