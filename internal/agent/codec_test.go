package agent

import (
	"encoding/json/jsontext"
	"reflect"
	"testing"
	"time"
)

// 存进数据库的消息 JSON 能无损还原。
func TestMessageRoundTrip(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	messages := []Message{
		SystemMessage{Type: SystemMessageTypeSystemPrompt, Content: "系统提示词", Timestamp: ts},
		// 压缩记录带着保留条数，重新打开会话时据此重建上下文。
		SystemMessage{Type: SystemMessageTypeCompaction, Content: "摘要", Kept: 3, Timestamp: ts},
		UserMessage{Content: []Content{
			TextContent{Text: "你好"},
			ImageContent{Data: "QQ==", MIMEType: "image/png"},
		}, Timestamp: ts},
		AssistantMessage{
			Model:      "m",
			StopReason: StopReasonToolUse,
			Content: []Content{
				ThinkingContent{Thinking: "想", Signature: "{}"},
				// 截断的 JSON 参数也要无损保存。
				ToolCall{ID: "c1", Name: "read", Arguments: []byte(`{"path": "a"`)},
			},
			Timestamp: ts,
		},
		ToolResultMessage{
			ToolCallID: "c1",
			ToolName:   "read",
			Content:    []Content{TextContent{Text: "错误"}},
			IsError:    true,
			Timestamp:  ts,
		},
		// 工具随结果附带的结构化数据也要原样保存。
		ToolResultMessage{
			ToolCallID: "c2",
			ToolName:   "edit",
			Content:    []Content{TextContent{Text: "已替换"}},
			Details:    jsontext.Value(`{"starts":[3,10]}`),
			Timestamp:  ts,
		},
	}
	for _, want := range messages {
		data, err := MarshalMessage(want)
		if err != nil {
			t.Fatal(err)
		}
		got, err := UnmarshalMessage(data)
		if err != nil {
			t.Fatalf("%s: %v", data, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("往返不一致:\n got %#v\nwant %#v", got, want)
		}
	}
}
