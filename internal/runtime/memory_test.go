package runtime

import (
	"strings"
	"testing"

	"github.com/liuyuxin/atlas/internal/model"
)

func TestFormatMemoryTranscriptKeepsNewest(t *testing.T) {
	messages := []model.Message{
		{Role: model.RoleUser, Content: "old message"},
		{Role: model.RoleAssistant, Content: "old reply"},
		{Role: model.RoleUser, Content: "new message"},
		{Role: model.RoleAssistant, Content: "new reply"},
	}
	// maxRunes=40 fits only the last two messages (18+21=39 runes).
	result := formatMemoryTranscript(messages, 40)
	if !strings.Contains(result, "new reply") {
		t.Fatalf("expected newest message to be kept, got: %s", result)
	}
	if strings.Contains(result, "old message") {
		t.Fatalf("expected oldest message to be dropped, got: %s", result)
	}
	// Chronological order: new message before new reply.
	newMsgIdx := strings.Index(result, "new message")
	newReplyIdx := strings.Index(result, "new reply")
	if newMsgIdx < 0 || newReplyIdx < 0 || newMsgIdx > newReplyIdx {
		t.Fatalf("expected chronological order, got: %s", result)
	}
}

func TestFormatMemoryTranscriptKeepsAllWhenFits(t *testing.T) {
	messages := []model.Message{
		{Role: model.RoleUser, Content: "first"},
		{Role: model.RoleAssistant, Content: "second"},
	}
	result := formatMemoryTranscript(messages, 1000)
	if !strings.Contains(result, "first") || !strings.Contains(result, "second") {
		t.Fatalf("expected all messages to be kept, got: %s", result)
	}
}
