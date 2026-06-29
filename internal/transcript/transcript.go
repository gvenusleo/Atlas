// Package transcript holds the model message sequence for the current agent instance.
package transcript

import "github.com/liuyuxin/atlas/internal/model"

// Transcript holds the model message sequence for a conversation.
type Transcript struct {
	messages []model.Message
}

// New creates an empty in-memory transcript.
func New() *Transcript {
	return &Transcript{}
}

// Append appends a message in call order.
func (t *Transcript) Append(msg model.Message) {
	t.messages = append(t.messages, msg)
}

// Reset clears and replaces the message list with the given messages.
// Used during context-overflow recovery to rebuild the transcript with compacted messages.
func (t *Transcript) Reset(messages []model.Message) {
	t.messages = append([]model.Message(nil), messages...)
}

// Messages returns a snapshot of the current messages.
// The returned slice is a copy; modifying it does not affect the Transcript's internal state.
func (t *Transcript) Messages() []model.Message {
	messages := append([]model.Message(nil), t.messages...)
	for i := range messages {
		messages[i].Parts = append([]model.ContentPart(nil), messages[i].Parts...)
	}
	return messages
}
