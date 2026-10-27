// Command icon 画出 Atlas 的应用图标并写成 PNG：三个在底部相切的圆，由外向内逐个收小，
// 像一层层叠起的球，层与层之间用软影分开，放在淡色的底板上。生成两张：正式版 icon.png，
// 与开发版 icon-dev.png，后者换一个主题色，在 Dock 里与正式版区分。另有托盘用的模板图标
// tray.png（见 tray.go），放在 gui 包里内嵌。
//
//	go run ./cmd/icon [-o resources] [-tray internal/gui/tray.png]
package main

import (
	"flag"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// style 是一张图标的配色。
type style struct {
	plate  [2]color.RGBA    // 底板渐变的两端，左上到右下
	balls  [3][2]color.RGBA // 三个圆由外向内，各自从上到下的渐变
	shadow color.NRGBA      // 层与层之间的软影
}

func rgb(v uint32) color.RGBA { return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} }

// themed 以主题色配出一套图标：最里一层就是主题色，中间一层是它的浅色，最外一层与
// 底板是更淡的底，层间的软影取自它的深色。
func themed(theme uint32) style {
	c, white, black := rgb(theme), rgb(0xFFFFFF), rgb(0x000000)
	tint := func(t float64) color.RGBA { return lerp(white, c, t) } // t 越大越接近主题色
	darken := func(x color.RGBA, t float64) color.RGBA { return lerp(x, black, t) }
	// 一个圆上浅下深的渐变，以 x 为中点：圆心的高度上正是 x。
	ball := func(x color.RGBA) [2]color.RGBA {
		top := lerp(x, white, 0.03)
		mirror := func(m, t uint8) uint8 { return uint8(max(0, 2*int(m)-int(t))) }
		return [2]color.RGBA{top, {mirror(x.R, top.R), mirror(x.G, top.G), mirror(x.B, top.B), 255}}
	}
	s := darken(c, 0.7)
	return style{
		plate:  [2]color.RGBA{tint(0.03), tint(0.12)},
		balls:  [3][2]color.RGBA{ball(tint(0.22)), ball(tint(0.55)), ball(c)},
		shadow: color.NRGBA{s.R, s.G, s.B, 75},
	}
}

var styles = map[string]style{
	"icon.png":     themed(0x2983BB), // 正式版
	"icon-dev.png": themed(0x806D9E), // 开发版：换一个色相，在 Dock 里与正式版区分
}

func main() {
	dir := flag.String("o", "resources", "输出目录")
	tray := flag.String("tray", "internal/gui/tray.png", "托盘图标的路径")
	flag.Parse()
	if err := writeTray(*tray); err != nil {
		fmt.Fprintln(os.Stderr, "icon:", err)
		os.Exit(1)
	}
	for name, st := range styles {
		if err := write(filepath.Join(*dir, name), st); err != nil {
			fmt.Fprintln(os.Stderr, "icon:", err)
			os.Exit(1)
		}
	}
}

// write 按配色画出图标，写到 path。
func write(path string, st style) error {
	c := newCanvas()
	c.plate(st.plate[0], st.plate[1])
	c.draw(st)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(f, c.img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// draw 由外向内画三个圆：每个圆先把软影投在包着它的各层上，再画自己。
func (c *canvas) draw(st style) {
	var below []poly
	for i, r := range radii {
		ball := disc(size/2, floor-r, r)
		if len(below) > 0 {
			c.shade(st.shadow, 4, 14, 12, c.mask(below...), ball)
		}
		c.fill(ramp(floor-2*r, floor, st.balls[i][0], st.balls[i][1]), []poly{ball})
		below = append(below, ball)
	}
}
