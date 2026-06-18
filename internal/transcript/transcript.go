// Package transcript 保存当前 agent 实例中的模型消息序列。
package transcript

import "github.com/liuyuxin/atlas/internal/model"

// Transcript 保存一次会话中的模型消息序列。
type Transcript struct {
	messages []model.Message
}

// New 创建一个空的内存 transcript。
func New() *Transcript {
	return &Transcript{}
}

// Append 按调用顺序追加一条消息。
func (t *Transcript) Append(msg model.Message) {
	t.messages = append(t.messages, msg)
}

// Messages 返回当前消息快照。
// 返回值是副本，调用方修改它不会影响 Transcript 内部状态。
func (t *Transcript) Messages() []model.Message {
	return append([]model.Message(nil), t.messages...)
}
