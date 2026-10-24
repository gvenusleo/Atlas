package gui

// listNav 是可以用键盘与指针选中其中一行的列表的状态，补全列表与启动器共用。
type listNav struct {
	index   int        // 选中的行；输入变了回到第一行
	moved   bool       // 按键移动了选中行，下一帧要滚到可见
	pointer [2]float32 // 指针上一次在窗口里的位置，还没见过时是 noPointer
}

// row 返回共 n 行的列表里选中的行。
func (m *listNav) row(n int) int { return min(m.index, n-1) }

// noPointer 是还没见过指针的位置：列表刚出现时，指针可能正停在某一行上，不算它动过。
var noPointer = [2]float32{-1, -1}

// pointerTo 记下指针在窗口里的位置 at，它正在第 i 行上：指针动了才选中这一行。
// 指针没动、只是列表在它底下滚动换了行时，不抢走键盘选中的行。
func (m *listNav) pointerTo(at [2]float32, i int) {
	if at == m.pointer {
		return
	}
	if m.pointer != noPointer {
		m.index = i
	}
	m.pointer = at
}
