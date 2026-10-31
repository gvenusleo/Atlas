package gui

import (
	"cmp"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
)

// CSV 解析的上限：再大就不是用来查看的了，源码视图仍然完整。
const (
	csvMaxRows = 2000
	csvMaxCols = 64
)

// csvTable 是一个 CSV/TSV 文件解析出来的表格：head 是第一行，其后是数据行；
// 解析失败或截断时 note 说明原因。
type csvTable struct {
	head    []string
	rows    [][]string
	err     string
	rowsCut bool // 行数超过上限，只取了一部分
	colsCut bool // 列数超过上限，每行只取了一部分
}

// parseCSV 把文本解析成表格：第一行作表头，行宽不齐也照收，引号不严也照收。
// 解析出错时保留出错前读到的部分。
func parseCSV(text string, comma rune) csvTable {
	var table csvTable
	// Excel 导出的 CSV 常带 BOM，不去掉的话第一个表头会多一个看不见的字符。
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\ufeff")))
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
		table.colsCut = table.colsCut || cut
		if table.head == nil {
			table.head = record
			continue
		}
		if len(table.rows) == csvMaxRows {
			table.rowsCut = true
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

// width 是表格的列数：取表头与最宽的数据行里大的那个，行比表头宽时不丢数据。
func (t csvTable) width() int {
	n := len(t.head)
	for _, row := range t.rows {
		n = max(n, len(row))
	}
	return n
}

// cell 返回第 row 行第 col 列的文字，行不够长时为空。
func (t csvTable) cell(row, col int) string {
	if row < 0 || row >= len(t.rows) || col < 0 || col >= len(t.rows[row]) {
		return ""
	}
	return t.rows[row][col]
}

// order 返回要显示的行号：col 为负时就是原文件的顺序，否则按该列排，相同的值保持原文件的
// 顺序。一列里只要有一个非空的值不是数字，整列就按文字比，免得同一列里两种比法打架。
func (t csvTable) order(col int, descending bool) []int {
	order := make([]int, len(t.rows))
	for i := range order {
		order[i] = i
	}
	if col < 0 {
		return order
	}
	numeric := t.numbers(col)
	slices.SortStableFunc(order, func(a, b int) int {
		n := t.compare(a, b, col, numeric)
		if descending {
			n = -n
		}
		return n
	})
	return order
}

// compare 比较两行在第 col 列上的值：numeric 为真时按数字比，否则按文字比。
func (t csvTable) compare(a, b, col int, numeric bool) int {
	x, y := t.cell(a, col), t.cell(b, col)
	if !numeric {
		return strings.Compare(x, y)
	}
	return cmp.Compare(number(x), number(y))
}

// numbers 报告第 col 列是不是数字列：非空的单元格都能读成数字。
func (t csvTable) numbers(col int) bool {
	for i := range t.rows {
		s := strings.TrimSpace(t.cell(i, col))
		if s == "" {
			continue
		}
		if _, err := strconv.ParseFloat(s, 64); err != nil {
			return false
		}
	}
	return true
}

// number 把单元格读成数字，读不成的当作负无穷：空单元格因此排在数字前面。
func number(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return math.Inf(-1)
	}
	return v
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

// note 返回表格没能完整显示的原因。
func (t csvTable) note() string {
	switch {
	case t.err != "":
		return "无法解析为表格：" + t.err + "，可以查看源码。"
	case t.rowsCut && t.colsCut:
		return fmt.Sprintf("表格只显示前 %d 行、每行前 %d 列。", csvMaxRows, csvMaxCols)
	case t.rowsCut:
		return fmt.Sprintf("表格只显示前 %d 行。", csvMaxRows)
	case t.colsCut:
		return fmt.Sprintf("每行只显示前 %d 列。", csvMaxCols)
	}
	return ""
}
