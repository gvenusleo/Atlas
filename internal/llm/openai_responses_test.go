package llm

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gvenusleo/atlas/internal/agent"
)

func TestResponsesStreamsToolArgumentsAndReplaysReasoning(t *testing.T) {
	type input struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		CallID    string `json:"call_id"`
		Arguments string `json:"arguments"`
		Encrypted string `json:"encrypted_content"`
	}
	requests := make(chan []input, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []input `json:"input"`
		}
		if err := json.UnmarshalRead(r.Body, &req); err != nil {
			http.Error(w, "请求内容无效", http.StatusBadRequest)
			return
		}
		select {
		case requests <- req.Input:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range responsesFixture {
			fmt.Fprint(w, "data: "+event+"\n\n")
		}
	}))
	t.Cleanup(server.Close)
	stream := OpenAIResponses(server.URL, "fixture", "a", false, 0)
	user := agent.UserMessage{Content: []agent.Content{agent.TextContent{Text: "问题"}}}
	req := agent.StreamRequest{Model: "p/a", Messages: []agent.Message{user}, ThinkingLevel: agent.ThinkingLow}
	var deltas []string
	final := stream(t.Context(), req, func(d agent.Delta) {
		if d.Type == agent.DeltaTypeToolCall && (d.ToolCall.ID != "call_1" || d.ToolCall.Name != "read") {
			t.Fatalf("工具参数关联到错误的调用：%+v", d)
		}
		deltas = append(deltas, d.Text)
	})
	if final.StopReason != agent.StopReasonToolUse ||
		!slices.Equal(deltas, []string{"思考", "正文", `{"path":`, `"a.go"}`}) {
		t.Fatalf("事件顺序或参数片段错误：%q，%s", deltas, final.ErrorMessage)
	}
	<-requests
	// 和 agent 运行循环一样，用完整模型引用保存定稿，供后续请求识别。
	final.Model = req.Model
	result := agent.ToolResultMessage{ToolCallID: "call_1", ToolName: "read",
		Content: []agent.Content{agent.TextContent{Text: "结果"}}}
	req.Messages = append(req.Messages, final, result)
	for _, model := range []string{"p/a", "p/b"} {
		req.Model = model
		response := stream(t.Context(), req, func(agent.Delta) {})
		if response.StopReason != agent.StopReasonToolUse {
			t.Fatalf("请求没有完成：%s", response.ErrorMessage)
		}
		items := <-requests
		var foundReasoning, foundCall bool
		for _, item := range items {
			foundReasoning = foundReasoning || item.Type == "reasoning" &&
				item.ID == "rs_1" && item.Encrypted == "encrypted-fixture"
			foundCall = foundCall || item.Type == "function_call" &&
				item.CallID == "call_1" && item.Arguments == `{"path":"a.go"}`
		}
		if foundReasoning != (model == "p/a") || !foundCall {
			t.Fatalf("%s 的回传项错误：推理=%t，工具调用=%t", model, foundReasoning, foundCall)
		}
	}
}

var responsesFixture = []string{
	`{"type":"response.reasoning_summary_text.delta","delta":"思考"}`,
	`{"type":"response.output_text.delta","delta":"正文"}`,
	`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read"}}`,
	`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"path\":"}`,
	`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"\"a.go\"}"}`,
	`{"type":"response.completed","response":{"status":"completed","output":[` +
		`{"type":"reasoning","id":"rs_1","encrypted_content":"encrypted-fixture","summary":[]},` +
		`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"正文"}]},` +
		`{"type":"function_call","id":"fc_1","call_id":"call_1","name":"read","arguments":"{\"path\":\"a.go\"}"}]}}`,
}
