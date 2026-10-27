package gui

import "github.com/egoist/mygo/ui"

// segmented 画本应用安静的分段控件：槽是淡底，选中的一项是主区底色，悬停的一项有淡底。
// width 不为 0 时每一项定宽，并在悬停时给出名称，用于只有图标的项；为 0 时项按内容撑开。
// content 画第 i 项的内容，color 是它这时该用的文字色；name 是第 i 项的名称。
func segmented(
	c *ui.Context, t *ui.Theme, key, label string, selected *int, n int, width float32,
	onChange func(), content func(i int, color ui.Color), name func(i int) string,
) {
	seg := ui.SegmentedBase(c.Key(key), selected, n)
	seg.Track.Gap(2).Padding(2).Radius(controlRound + 2).Background(rowFill(t)).Label(label).
		OnChange(onChange)
	seg.Track.Children(func() {
		for i := range n {
			item := seg.Segment(i).Radius(controlRound).Label(name(i))
			if width > 0 {
				item.Size(width, 24).Tooltip(name(i))
			} else {
				item.Height(24).Padding(0, 12)
			}
			color := t.TextMuted
			switch {
			case i == *selected:
				item.Background(t.Background)
				color = t.Text
			case item.Hovered():
				item.Background(buttonFill(t))
			}
			item.Children(func() { content(i, color) })
		}
	})
}
