package debuglog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoggerWritesSessionJSONL(t *testing.T) {
	dir := t.TempDir()
	logger := New(true, dir)

	logger.Write("ses/test", "turn_started", map[string]string{"input": "hello"})

	data, err := os.ReadFile(filepath.Join(dir, "ses_test.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatal(err)
	}
	if entry.SessionID != "ses/test" || entry.Event != "turn_started" {
		t.Fatalf("unexpected entry: %#v", entry)
	}
}

func TestDisabledLoggerDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	logger := New(false, dir)

	logger.Write("ses_1", "turn_started", nil)

	if _, err := os.Stat(filepath.Join(dir, "ses_1.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("disabled logger should not create a file, err=%v", err)
	}
}
