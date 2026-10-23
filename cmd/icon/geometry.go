package main

import "math"

// pt 是画布上的一点，坐标单位为像素，原点在左上角。
type pt struct{ x, y float64 }

// poly 是一个闭合的多边形。
type poly []pt

// area 返回多边形带符号的面积，屏幕坐标系里顺时针为正。
func area(p poly) float64 {
	var s float64
	for i, v := range p {
		w := p[(i+1)%len(p)]
		s += v.x*w.y - v.y*w.x
	}
	return s / 2
}

// shift 返回平移后的多边形。
func shift(p poly, dx, dy float64) poly {
	q := make(poly, len(p))
	for i, v := range p {
		q[i] = pt{v.x + dx, v.y + dy}
	}
	return q
}

// squircle 是以 (cx, cy) 为中心、半边长 r 的连续圆角方块（超椭圆，指数 5），
// 与 macOS 图标的轮廓相近。
func squircle(cx, cy, r float64) poly {
	const steps = 1440
	p := make(poly, steps)
	for i := range p {
		s, c := math.Sincos(2 * math.Pi * float64(i) / steps)
		p[i] = pt{
			cx + r*math.Copysign(math.Pow(math.Abs(c), 0.4), c),
			cy + r*math.Copysign(math.Pow(math.Abs(s), 0.4), s),
		}
	}
	return p
}

// disc 是圆心 (cx, cy)、半径 r 的圆。
func disc(cx, cy, r float64) poly {
	const steps = 360
	p := make(poly, steps)
	for i := range p {
		s, c := math.Sincos(2 * math.Pi * float64(i) / steps)
		p[i] = pt{cx + r*c, cy + r*s}
	}
	return p
}

// 图标是三个由外向内逐个收小的圆，在底部相切，像一层层叠起的球。
// floor 是相切处的高度，也是最大的圆的底；radii 是三个圆的半径，由外向内。
const floor = 842.0

var radii = []float64{330, 215, 110}
