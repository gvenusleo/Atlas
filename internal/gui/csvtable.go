package gui

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// CSV 解析的上限：再大就不是用来看的了，源码视图仍然完整。
const (
	csvMaxRows = 2000
	csvMaxCols = 64
)

// csvTable 是一个 CSV/TSV 文件解析出来的表格：head 是第一行，其后是数据行；
// 解析失败或截断时 note 说明原因。
type csvTable struct {
	head      []string
	rows      [][]string
	err       string
	truncated bool
}

// parseCSV 把文本解析成表格：第一行作表头，行宽不齐也照收，引号不严也照收。
// 解析出错时保留出错前读到的部分。
func parseCSV(text string, comma rune) csvTable {
	var table csvTable
	r := csv.NewReader(strings.NewReader(text))
	r.Comma, r.FieldsPerRecord, r.LazyQuotes = comma, -1, true
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			return table
		}
		if err != nil {
			table.err = err.Error()
			return table
		}
		record, cut := cutRow(record)
		table.truncated = table.truncated || cut
		if table.head == nil {
			table.head = record
			continue
		}
		if len(table.rows) == csvMaxRows {
			table.truncated = true
			return table
		}
		table.rows = append(table.rows, record)
	}
}

// cutRow 把一行裁到列数上限，报告是否裁过。
func cutRow(record []string) ([]string, bool) {
	if len(record) <= csvMaxCols {
		return record, false
	}
	return record[:csvMaxCols], true
}

// isCSV 报告路径是不是 CSV 或 TSV。
func isCSV(p string) bool {
	ext := extOf(p)
	return ext == ".csv" || ext == ".tsv"
}

// csvComma 是 CSV/TSV 的分隔符。
func csvComma(p string) rune {
	if extOf(p) == ".tsv" {
		return '\t'
	}
	return ','
}

// extOf 是路径的小写扩展名，带点。
func extOf(p string) string { return strings.ToLower(filepath.Ext(p)) }

// columns 把表头变成表格的列：标识用序号，表头可能是空的或重复的。
func (t csvTable) columns() []ui.TableColumn {
	cols := make([]ui.TableColumn, len(t.head))
	for i, title := range t.head {
		cols[i] = ui.TableColumn{Title: title, ID: strconv.Itoa(i)}
	}
	return cols
}

// note 返回表格没能完整显示的原因。
func (t csvTable) note() string {
	switch {
	case t.err != "":
		return "无法解析为表格：" + t.err + "，可以查看源码。"
	case t.truncated:
		return fmt.Sprintf("表格只显示前 %d 行、每行前 %d 列。", csvMaxRows, csvMaxCols)
	}
	return ""
}
