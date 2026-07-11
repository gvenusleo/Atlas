package tool

import (
	"os"
	"strconv"
	"testing"
)

func quoteJSON(value string) string {
	return strconv.Quote(value)
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("file content = %q, want %q", string(data), want)
	}
}
