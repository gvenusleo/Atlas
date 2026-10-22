package tools

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gvenusleo/atlas/internal/protocol"
)

// edit 的结果附带各处替换在原文件中的起始行（按 edits 的顺序），界面靠它显示真实行号。
func TestEditDetails(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("a\nb\nc\nd\ne\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	params := jsontext.Value(`{"path":"a.txt","edits":[` +
		`{"oldText":"d","newText":"D1\nD2"},{"oldText":"b\nc","newText":"x"}]}`)
	result, err := editFile(t.Context(), dir, params)
	if err != nil {
		t.Fatal(err)
	}
	var details protocol.EditDetails
	if err := json.Unmarshal(result.Details, &details); err != nil {
		t.Fatal(err)
	}
	if want := []int{4, 2}; !slices.Equal(details.Starts, want) {
		t.Errorf("起始行 = %v，期望 %v（按 edits 的顺序）", details.Starts, want)
	}
}
