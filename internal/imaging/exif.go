package imaging

import (
	"bytes"
	"encoding/binary"
	"image"
)

// exifOrientation 返回 JPEG 里 EXIF 记的拍摄方向（1–8，1 为正）；不是 JPEG 或没有记录时为 1。
func exifOrientation(data []byte) int {
	if !bytes.HasPrefix(data, []byte{0xFF, 0xD8}) {
		return 1
	}
	// 逐段往后找 APP1 里的 Exif，扫描数据（SOS）开始后不会再有。
	for i := 2; i+4 <= len(data) && data[i] == 0xFF && data[i+1] != 0xDA; {
		size := int(binary.BigEndian.Uint16(data[i+2:]))
		end := min(i+2+size, len(data))
		if data[i+1] == 0xE1 {
			if tiff, ok := bytes.CutPrefix(data[min(i+4, end):end], []byte("Exif\x00\x00")); ok {
				return tiffOrientation(tiff)
			}
		}
		i += 2 + size
	}
	return 1
}

// tiffOrientation 在 TIFF 结构的第一个目录里找方向标签（0x0112）。
func tiffOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	dir := int(order.Uint32(tiff[4:]))
	if dir+2 > len(tiff) {
		return 1
	}
	for i := range int(order.Uint16(tiff[dir:])) {
		entry := dir + 2 + 12*i
		if entry+12 > len(tiff) {
			break
		}
		if order.Uint16(tiff[entry:]) == 0x0112 {
			if v := int(order.Uint16(tiff[entry+8:])); v >= 1 && v <= 8 {
				return v
			}
			break
		}
	}
	return 1
}

// orient 按 EXIF 方向把图转正：2–4 是镜像与旋转 180°，5–8 要转 90°，宽高对调。
func orient(img image.Image, orientation int) image.Image {
	if orientation <= 1 || orientation > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	size := image.Rect(0, 0, w, h)
	if orientation >= 5 {
		size = image.Rect(0, 0, h, w)
	}
	dst := image.NewRGBA(size)
	for y := range h {
		for x := range w {
			var dx, dy int
			switch orientation {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
