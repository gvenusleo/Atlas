package storage

import "time"

// Session records durable state for one Atlas conversation.
type Session struct {
	ID        string
	Title     string
	Workdir   string
	Model     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Message records one persisted conversation item.
type Message struct {
	ID         int64
	SessionID  string
	Role       string
	Content    string
	ToolCallID string
	ToolCalls  string
	CreatedAt  time.Time
}

// Store is the durable session boundary used by the agent loop.
type Store interface {
	CreateSession(session Session) error
	GetSession(id string) (Session, error)
	ListSessions() ([]Session, error)
	AddMessage(message Message) error
	Messages(sessionID string, limit int) ([]Message, error)
	Close() error
}
