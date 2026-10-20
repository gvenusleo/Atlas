package imaging

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"slices"
	"testing"
)

// solid 是不透明的白图。
func solid(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	return img
}

// jpegWithOrientation 编一张白色 JPEG，并在 SOI 之后插入记了方向的 EXIF 段。
func jpegWithOrientation(t *testing.T, w, h, orientation int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, solid(w, h), nil); err != nil {
		t.Fatal(err)
	}
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 0x01, 3, 0, 1, 0, 0, 0,
		byte(orientation), 0, 0, 0}
	payload := append([]byte("Exif\x00\x00"), tiff...)
	segment := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	data := buf.Bytes()
	return slices.Concat(data[:2], segment, payload, data[2:])
}

func TestPrepare(t *testing.T) {
	var wide bytes.Buffer
	if err := png.Encode(&wide, solid(3000, 1000)); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name          string
		data          []byte
		mime          string
		width, height int
		wantErr       bool
	}{
		{"大图缩小并转 JPEG", wide.Bytes(), "image/jpeg", 2048, 682, false},
		{"小图转正", jpegWithOrientation(t, 300, 100, 6), "image/jpeg", 100, 300, false},
		{"不是图片", []byte("hello"), "", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Prepare(bytes.NewReader(tt.data))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if err != nil {
				return
			}
			data, err := base64.StdEncoding.DecodeString(got.Data)
			if err != nil {
				t.Fatal(err)
			}
			size, _, err := image.DecodeConfig(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if got.MIMEType != tt.mime || size.Width != tt.width || size.Height != tt.height {
				t.Errorf("得到 %s %d×%d，想要 %s %d×%d",
					got.MIMEType, size.Width, size.Height, tt.mime, tt.width, tt.height)
			}
		})
	}
}
