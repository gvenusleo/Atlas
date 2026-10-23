package core

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/store"
)

// 记录的种类，也是 messages 表里 kind 列的取值。
const (
	kindUser         = "user"
	kindAssistant    = "assistant"
	kindToolResult   = "tool_result"
	kindSystemPrompt = "system_prompt"
	kindNotice       = "notice"
	kindCompaction   = "compaction"
)

// kindOf 给出消息的记录种类。
func kindOf(m agent.Message) string {
	switch m := m.(type) {
	case agent.UserMessage:
		return kindUser
	case agent.AssistantMessage:
		return kindAssistant
	case agent.ToolResultMessage:
		return kindToolResult
	case agent.SystemMessage:
		switch m.Type {
		case agent.SystemMessageTypeSystemPrompt:
			return kindSystemPrompt
		case agent.SystemMessageTypeNotice:
			return kindNotice
		case agent.SystemMessageTypeCompaction:
			return kindCompaction
		}
	}
	return ""
}

// encode 把消息编成要写入的记录：图片外置成 blob，记录里只留引用。
// 还没定稿的助手消息不能保存。
func encode(m agent.Message) (store.Entry, error) {
	if a, ok := m.(agent.AssistantMessage); ok && a.StopReason == agent.StopReasonPending {
		return store.Entry{}, errors.New("不能保存未定稿的助手消息")
	}
	var blobs []store.Blob
	var err error
	switch v := m.(type) {
	case agent.UserMessage:
		v.Content, blobs, err = externalize(v.Content)
		m = v
	case agent.ToolResultMessage:
		v.Content, blobs, err = externalize(v.Content)
		m = v
	}
	if err != nil {
		return store.Entry{}, err
	}
	body, err := agent.MarshalMessage(m)
	if err != nil {
		return store.Entry{}, fmt.Errorf("编码消息失败: %w", err)
	}
	return store.Entry{Kind: kindOf(m), Body: string(body), Blobs: blobs}, nil
}

// externalize 把内容里的图片数据换成对 blob 的引用，返回换好的内容与要写入的 blob。
// 消息是不可变的值，没有图片要换时原样返回，要换时另起一份。
func externalize(content []agent.Content) ([]agent.Content, []store.Blob, error) {
	var blobs []store.Blob
	out, cloned := content, false
	for i, block := range content {
		img, ok := block.(agent.ImageContent)
		if !ok || img.Data == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			return nil, nil, fmt.Errorf("图片数据不是合法的 base64: %w", err)
		}
		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])
		blobs = append(blobs, store.Blob{Hash: hash, MIME: img.MIMEType, Data: data})
		if !cloned {
			out, cloned = slices.Clone(content), true
		}
		out[i] = agent.ImageContent{Blob: hash, MIMEType: img.MIMEType}
	}
	return out, blobs, nil
}

// decode 读出记录里的消息；图片仍是对 blob 的引用。只需要文字或种类时用它，免得读 blob。
func decode(id string, r store.Record) (agent.Message, error) {
	m, err := agent.UnmarshalMessage([]byte(r.Body))
	if err != nil {
		return nil, fmt.Errorf("会话 %s 的记录 %d 解码失败: %w", id, r.Seq, err)
	}
	if kindOf(m) != r.Kind {
		return nil, fmt.Errorf("会话 %s 的记录 %d 种类与正文不一致", id, r.Seq)
	}
	return m, nil
}

// decodeWithImages 读出记录里的消息，并把对 blob 的引用还原成图片数据。
func (c *Core) decodeWithImages(
	ctx context.Context, id string, r store.Record,
) (agent.Message, error) {
	m, err := decode(id, r)
	if err != nil {
		return nil, err
	}
	switch v := m.(type) {
	case agent.UserMessage:
		v.Content, err = c.restoreImages(ctx, id, v.Content)
		m = v
	case agent.ToolResultMessage:
		v.Content, err = c.restoreImages(ctx, id, v.Content)
		m = v
	}
	return m, err
}

// restoreImages 把内容里对 blob 的引用还原成图片数据，另起一份内容。
func (c *Core) restoreImages(
	ctx context.Context, id string, content []agent.Content,
) ([]agent.Content, error) {
	out, cloned := content, false
	for i, block := range content {
		img, ok := block.(agent.ImageContent)
		if !ok || img.Blob == "" || img.Data != "" {
			continue
		}
		blob, err := c.db.Blob(ctx, id, img.Blob)
		if err != nil {
			return nil, fmt.Errorf("会话 %s 的图片 %s 读取失败: %w", id, img.Blob, err)
		}
		if !cloned {
			out, cloned = slices.Clone(content), true
		}
		out[i] = agent.ImageContent{
			Data: base64.StdEncoding.EncodeToString(blob.Data), MIMEType: img.MIMEType,
		}
	}
	return out, nil
}

// mapStoreError 把存储层的错误换成协议的错误。
func mapStoreError(err error) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return protocol.ErrNotFound
	case errors.Is(err, store.ErrConflict):
		return protocol.ErrConflict
	}
	return err
}
