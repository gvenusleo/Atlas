package protocol

// 工具调用的 RawOutput：Atlas 的工具随结果附带的结构化数据，供界面渲染，不发给模型。

// ReadDetails 是 read 读取文本时的 RawOutput：实际返回的窗口，而不是请求的，
// 默认值与返回预算都已计入。
type ReadDetails struct {
	// Start 与 End 是实际返回的第一行与最后一行的行号（从 1 起）；空文件时 End 为 0。
	Start int `json:"start"`
	End   int `json:"end"`
	// Total 是文件的总行数。
	Total int `json:"total"`
}

// WriteDetails 是 write 的 RawOutput。
type WriteDetails struct {
	// Overwrote 为真表示覆盖了已有的文件，否则是新建。
	Overwrote bool `json:"overwrote"`
	// Old 是被覆盖前的内容，界面靠它显示对比；文件太大、不是有效的 UTF-8 文本或读不出来时
	// 为空，界面只显示新内容。
	Old *string `json:"old,omitzero"`
}

// EditDetails 是 edit 的 RawOutput。
type EditDetails struct {
	// Starts 是各处替换在原文件中的起始行（从 1 起），顺序与 edits 一致。
	Starts []int `json:"starts"`
}
