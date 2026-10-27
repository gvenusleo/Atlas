package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

// 托盘图标是 16 点见方、32 像素的模板图标：黑色加透明底，macOS 按菜单栏的明暗给它上色。
// 画的是应用图标里三个在底部相切的圆，只留轮廓，小尺寸下比实心的清楚。
const (
	traySize   = 32
	trayStroke = 1.7 // 轮廓的粗细，像素
	traySample = 4   // 每个像素每个方向取的采样数，用来抗锯齿
)

// trayCircles 是三个圆的圆心高度与半径，由外向内，都与同一条底边相切。
var trayCircles = [3]struct{ y, r float64 }{{15.5, 13.5}, {20, 9}, {24, 5}}

// writeTray 把托盘图标写成 PNG。
func writeTray(path string) error {
	img := image.NewNRGBA(image.Rect(0, 0, traySize, traySize))
	for y := range traySize {
		for x := range traySize {
			covered := 0
			for sy := range traySample {
				for sx := range traySample {
					px := float64(x) + (float64(sx)+0.5)/traySample
					py := float64(y) + (float64(sy)+0.5)/traySample
					if onOutline(px, py) {
						covered++
					}
				}
			}
			alpha := uint8(255 * covered / (traySample * traySample))
			img.SetNRGBA(x, y, color.NRGBA{A: alpha})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// onOutline 报告点 (x, y) 是否落在某个圆的轮廓上。
func onOutline(x, y float64) bool {
	for _, c := range trayCircles {
		if math.Abs(math.Hypot(x-traySize/2, y-c.y)-c.r) <= trayStroke/2 {
			return true
		}
	}
	return false
}
