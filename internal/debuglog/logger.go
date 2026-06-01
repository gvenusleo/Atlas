package debuglog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Logger writes per-session debug events as JSONL when debug mode is enabled.
type Logger struct {
	enabled bool
	dir     string
	mu      sync.Mutex
}

// Entry is one durable debug record for a session.
type Entry struct {
	Time      time.Time `json:"time"`
	SessionID string    `json:"session_id"`
	Event     string    `json:"event"`
	Payload   any       `json:"payload,omitempty"`
}

// New creates a best-effort local debug logger.
func New(enabled bool, dir string) *Logger {
	return &Logger{enabled: enabled, dir: dir}
}

// Enabled reports whether debug logging has enough configuration to write.
func (l *Logger) Enabled() bool {
	return l != nil && l.enabled && strings.TrimSpace(l.dir) != ""
}

// Write appends one JSONL event to the session log.
func (l *Logger) Write(sessionID string, event string, payload any) {
	if !l.Enabled() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return
	}
	file, err := os.OpenFile(filepath.Join(l.dir, safeFileName(sessionID)+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer file.Close()

	_ = json.NewEncoder(file).Encode(Entry{
		Time:      time.Now().UTC(),
		SessionID: sessionID,
		Event:     event,
		Payload:   payload,
	})
}

func safeFileName(value string) string {
	var out strings.Builder
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			out.WriteRune(ch)
		} else {
			out.WriteByte('_')
		}
	}
	if out.Len() == 0 {
		return "session"
	}
	return out.String()
}
