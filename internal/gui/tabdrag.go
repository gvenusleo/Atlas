package gui

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
)

// tabDragValue 带上标签栏本身，避免会话切换后把拖动投给同编号的标签。
type tabDragValue struct {
	bar *tabBar
	id  int
}

type tabDrag struct {
	id        int
	x         float32
	direction float32
	dropped   bool
}

func trackTabDrag(c *ui.Context, strip ui.Element, bar *tabBar, items []tabItem) tabDrag {
	// 采用松手时的位置，避免构建下一帧前移动指针改变落点。
	release := ui.Local(strip, "tab-release-x", func() float32 { return 0 })
	strip.ObserveInput(func(ev ui.InputEvent) {
		if ev.Kind == ui.InputPointerUp && ev.Button == 0 && !ev.Canceled {
			*release = ev.X
		}
	})
	area := strip.LayoutBounds()
	dir := float32(1)
	if c.Direction() == ui.RightToLeft {
		dir = -1
	}
	next := ui.Local(strip, "tab-drag-scroll", func() time.Time { return time.Time{} })
	if value, ok := ui.Drop[tabDragValue](strip); ok {
		*next = time.Time{}
		if value.bar == bar {
			return tabDrag{value.id, area.X + *release, dir, true}
		}
		return tabDrag{}
	}
	if value, ok := ui.DragOver[tabDragValue](strip); ok && value.bar == bar &&
		slices.ContainsFunc(items, func(it tabItem) bool { return it.id == value.id }) {
		x, _, inside := strip.PointerPosition()
		if inside {
			distance := x
			if dir < 0 {
				distance = area.W - x
			}
			oldX := bar.scroll.X
			scrollTabsOnDrag(c, &bar.scroll, distance, area.W, next)
			return tabDrag{value.id, area.X + x + dir*(bar.scroll.X-oldX), dir, false}
		}
	}
	*next = time.Time{}
	return tabDrag{}
}

func scrollTabsOnDrag(c *ui.Context, s *ui.ScrollState, x, width float32, next *time.Time) {
	const step = 12
	edge := min(float32(tabFade), width/2)
	if edge <= 0 {
		return
	}
	inner := min(max(x, edge), width-edge)
	offset := min(max(s.X+(x-inner)*step/edge, 0), s.MaxX)
	if offset == s.X {
		*next = time.Time{}
		return
	}
	now := c.Now()
	if !now.Before(*next) {
		s.X = offset
		*next = now.Add(time.Second / 60)
	}
	c.After(next.Sub(now))
}

func (d tabDrag) applyTabDrag(rows []ui.Element, items []tabItem, move func(from, gap int)) {
	from := slices.IndexFunc(items, func(it tabItem) bool { return it.id == d.id })
	if from < 0 {
		return
	}
	gap := len(rows)
	for i, row := range rows {
		// 使用让位前的中点，避免标签一移动就让插入位置跳回去。
		r := row.LayoutBounds()
		if r.W > 0 && d.direction*(d.x-r.X-r.W/2) < 0 {
			gap = i
			break
		}
	}
	if d.dropped {
		move(from, gap)
		return
	}
	source := rows[from].LayoutBounds()
	shift := d.direction * (source.W + tabGap)
	switch {
	case gap < from:
		for _, row := range rows[gap:from] {
			row.Translate(shift, 0)
		}
		target := rows[gap].LayoutBounds()
		rows[from].Translate(tabEdge(target, d.direction)-tabEdge(source, d.direction), 0)
	case gap > from+1:
		for _, row := range rows[from+1 : gap] {
			row.Translate(-shift, 0)
		}
		target := rows[gap-1].LayoutBounds()
		rows[from].Translate(tabEdge(target, -d.direction)-tabEdge(source, -d.direction), 0)
	}
}

func tabEdge(bounds ui.Rect, direction float32) float32 {
	if direction < 0 {
		return bounds.X + bounds.W
	}
	return bounds.X
}
