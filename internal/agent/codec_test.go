package agent

import (
	"encoding/json/jsontext"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestMessageRoundTrip(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	messages := []Message{
		SystemMessage{Type: SystemMessageTypePrompt, Content: "提示词", Timestamp: ts},
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

func TestUnmarshalMessageRejects(t *testing.T) {
	const assistant = `{"role":"assistant","model":"m","stop_reason":"stop","content":[%s]}`
	for name, data := range map[string]string{
		"未知字段":    `{"role":"user","content":[],"extra":1}`,
		"未知内容类型":  `{"role":"user","content":[{"type":"nope"}]}`,
		"助手含图片":   fmt.Sprintf(assistant, `{"type":"image","data":"x","mime_type":"y"}`),
		"参数非字符串":  fmt.Sprintf(assistant, `{"type":"tool_call","id":"a","name":"b","arguments":{}}`),
		"系统消息为数组": `{"role":"system","content":[]}`,
	} {
		if _, err := UnmarshalMessage([]byte(data)); err == nil {
			t.Errorf("%s: 应当报错", name)
		}
	}
}
