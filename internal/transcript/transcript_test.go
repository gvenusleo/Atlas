package transcript

import (
	"testing"

	"github.com/liuyuxin/atlas/internal/model"
)

func TestTranscriptMessages(t *testing.T) {
	tr := New()

	tr.Append(model.Message{Role: model.RoleUser, Content: "hello"})
	tr.Append(model.Message{Role: model.RoleAssistant, Content: "hi"})

	messages := tr.Messages()
	if len(messages) != 2 {
		t.Fatalf("len(Messages()) = %d, want 2", len(messages))
	}
	if messages[0].Role != model.RoleUser || messages[0].Content != "hello" {
		t.Fatalf("messages[0] = %#v", messages[0])
	}
	if messages[1].Role != model.RoleAssistant || messages[1].Content != "hi" {
		t.Fatalf("messages[1] = %#v", messages[1])
	}
}

func TestTranscriptMessagesReturnsCopy(t *testing.T) {
	tr := New()
	tr.Append(model.Message{Role: model.RoleUser, Content: "hello"})

	messages := tr.Messages()
	messages[0].Content = "changed"
	messages = append(messages, model.Message{Role: model.RoleAssistant, Content: "extra"})

	got := tr.Messages()
	if len(got) != 1 {
		t.Fatalf("len(Messages()) = %d, want 1", len(got))
	}
	if got[0].Content != "hello" {
		t.Fatalf("got content %q, want %q", got[0].Content, "hello")
	}
}
