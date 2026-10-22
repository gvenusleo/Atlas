package tools

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gvenusleo/atlas/internal/protocol"
)

// write 覆盖已有文件时，结果附带被覆盖的内容，界面靠它画对比；空文件也是旧内容。
// 旧内容太大或不是有效的 UTF-8 时不附带，写入仍然成功（JSON 编码不接受非法的 UTF-8）。
func TestWriteDetailsOld(t *testing.T) {
	dir := t.TempDir()
	for _, tt := range []struct {
		file string
		old  []byte // nil 表示文件原本不存在
		want protocol.WriteDetails
	}{
		{"new.txt", nil, protocol.WriteDetails{}},
		{"text.txt", []byte("旧\n"), protocol.WriteDetails{Overwrote: true, Old: new("旧\n")}},
		{"empty.txt", []byte{}, protocol.WriteDetails{Overwrote: true, Old: new("")}},
		{"binary.bin", []byte{0xff, 0xfe}, protocol.WriteDetails{Overwrote: true}},
		{"big.txt", bytes.Repeat([]byte("x"), maxOldBytes+1), protocol.WriteDetails{Overwrote: true}},
	} {
		path := filepath.Join(dir, tt.file)
		if tt.old != nil {
			if err := os.WriteFile(path, tt.old, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		params := fmt.Sprintf(`{"path":%q,"content":"新\n"}`, path)
		result, err := writeFile(t.Context(), dir, jsontext.Value(params))
		if err != nil {
			t.Fatalf("%s: 写入失败: %v", tt.file, err)
		}
		var got protocol.WriteDetails
		if err := json.Unmarshal(result.Details, &got); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: 附带的数据 = %+v，期望 %+v", tt.file, got, tt.want)
		}
	}
}
