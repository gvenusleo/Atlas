package mcp

import (
	"cmp"
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gvenusleo/atlas/internal/agent"
)

// maxResultBytes 是交给模型的文本的上限，与 shell 的输出上限一致。
const maxResultBytes = 50 << 10

// imageTypes 是模型能看的图片格式，与 read 一致。
var imageTypes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true,
}

// toResult 把 MCP 的调用结果转成 agent 的工具结果：文本合在一起，图片单独交给模型，
// 音频、资源链接与二进制资源换成一行说明。没有内容时用 structuredContent 的 JSON。
func toResult(res *sdk.CallToolResult) agent.ToolResult {
	var texts []string
	var images []agent.Content
	for _, block := range res.Content {
		switch c := block.(type) {
		case *sdk.TextContent:
			texts = append(texts, c.Text)
		case *sdk.ImageContent:
			if imageTypes[c.MIMEType] {
				images = append(images, agent.ImageContent{
					Data: base64.StdEncoding.EncodeToString(c.Data), MIMEType: c.MIMEType,
				})
			} else {
				texts = append(texts, fmt.Sprintf("[图片 %s，不支持显示]", c.MIMEType))
			}
		case *sdk.AudioContent:
			texts = append(texts,
				fmt.Sprintf("[音频 %s，%d 字节，不支持播放]", c.MIMEType, len(c.Data)))
		case *sdk.ResourceLink:
			texts = append(texts, fmt.Sprintf("[资源链接 %s %s]", cmp.Or(c.Name, c.URI), c.URI))
		case *sdk.EmbeddedResource:
			if text := resourceText(c.Resource); text != "" {
				texts = append(texts, text)
			}
		}
	}
	if len(texts) == 0 && len(images) == 0 && res.StructuredContent != nil {
		if data, err := json.Marshal(res.StructuredContent, json.Deterministic(true)); err == nil {
			texts = append(texts, string(data))
		}
	}
	content := images
	if text := strings.Join(texts, "\n\n"); text != "" || len(images) == 0 {
		text = cmp.Or(clip(text), "无输出")
		content = append([]agent.Content{agent.TextContent{Text: text}}, images...)
	}
	return agent.ToolResult{Content: content, IsError: res.IsError}
}

// resourceText 是内嵌资源给模型看的文字：文本资源是它的内容，其余是一行说明。
func resourceText(r *sdk.ResourceContents) string {
	switch {
	case r == nil:
		return ""
	case r.Text != "":
		return r.Text
	}
	return fmt.Sprintf("[二进制资源 %s，%s，%d 字节，不支持显示]", r.URI, r.MIMEType, len(r.Blob))
}

// lineSlack 是截断点为了落在行界上，最多可以挪动的字节数。
const lineSlack = 2 << 10

// clip 把超长的文本去掉中间，只留首尾；完整的文本存进临时文件，路径写在省略处，
// 模型可以用 read 读取。
func clip(text string) string {
	if len(text) <= maxResultBytes {
		return text
	}
	head := headEnd(text, maxResultBytes/2)
	tail := tailStart(text, len(text)-maxResultBytes/2)
	note := fmt.Sprintf("[结果过长，已省略中间 %d 字节", tail-head)
	if file, err := saveText(text); err == nil {
		note += "，完整内容在 " + file + "，可用 read 读取"
	}
	return text[:head] + "\n\n" + note + "]\n\n" + text[tail:]
}

// headEnd 返回开头保留部分的结尾：n 之前 lineSlack 之内有换行就停在那一行末尾，
// 否则退到字符边界。
func headEnd(text string, n int) int {
	if i := strings.LastIndexByte(text[:n], '\n'); i >= 0 && n-i <= lineSlack {
		return i + 1
	}
	for n > 0 && !utf8.RuneStart(text[n]) {
		n--
	}
	return n
}

// tailStart 返回末尾保留部分的开头：n 之后 lineSlack 之内有换行就从下一行开始，
// 否则进到字符边界。
func tailStart(text string, n int) int {
	if i := strings.IndexByte(text[n:], '\n'); i >= 0 && i < lineSlack {
		return n + i + 1
	}
	for n < len(text) && !utf8.RuneStart(text[n]) {
		n++
	}
	return n
}

// saveText 把文本存进临时文件，返回路径。
func saveText(text string) (string, error) {
	f, err := os.CreateTemp("", "atlas-mcp-*.txt")
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(text)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
