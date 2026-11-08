package receiptimg

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"testing"

	"github.com/gen2brain/webp"
)

// photo is a w×h image, red in its top-left quarter, white elsewhere.
func photo(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{255, 255, 255, 255}
			if x < w/2 && y < h/2 {
				c = color.RGBA{255, 0, 0, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

// withOrientation puts an EXIF APP1 with the orientation tag right after
// the JPEG's SOI.
func withOrientation(jpg []byte, o byte) []byte {
	tiff := []byte{
		'M', 'M', 0, 42, 0, 0, 0, 8, // big endian, IFD at 8
		0, 1, // one entry
		0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, o, 0, 0, // orientation, SHORT, 1
		0, 0, 0, 0, // no next IFD
	}
	seg := append([]byte("Exif\x00\x00"), tiff...)
	size := len(seg) + 2
	app1 := append([]byte{0xFF, 0xE1, byte(size >> 8), byte(size)}, seg...)
	out := append([]byte{}, jpg[:2]...)
	out = append(out, app1...)
	return append(out, jpg[2:]...)
}

func decodeWebP(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a webp: %v", err)
	}
	return img
}

func isRed(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r > 0xC000 && g < 0x4000 && b < 0x4000
}

func TestNormalize(t *testing.T) {
	t.Run("a big png is shrunk into a webp", func(t *testing.T) {
		var buf bytes.Buffer
		if err := png.Encode(&buf, photo(3200, 2000)); err != nil {
			t.Fatal(err)
		}
		out, err := Normalize(buf.Bytes(), "image/png")
		if err != nil {
			t.Fatal(err)
		}
		if b := decodeWebP(t, out).Bounds(); b.Dx() != 1600 || b.Dy() != 1000 {
			t.Errorf("got %v, want 1600×1000", b.Size())
		}
	})

	t.Run("a jpeg is turned upright", func(t *testing.T) {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, photo(400, 200), &jpeg.Options{Quality: 95}); err != nil {
			t.Fatal(err)
		}
		// 6: shot turned left, shown turned right: the red top-left corner
		// ends top-right.
		out, err := Normalize(withOrientation(buf.Bytes(), 6), "image/jpeg")
		if err != nil {
			t.Fatal(err)
		}
		img := decodeWebP(t, out)
		if b := img.Bounds(); b.Dx() != 200 || b.Dy() != 400 {
			t.Fatalf("got %v, want 200×400", b.Size())
		}
		if !isRed(img.At(150, 50)) || isRed(img.At(50, 50)) {
			t.Error("not turned right")
		}
	})

	t.Run("a small webp is kept as is", func(t *testing.T) {
		var buf bytes.Buffer
		if err := webp.Encode(&buf, photo(800, 600), webp.Options{Quality: Quality}); err != nil {
			t.Fatal(err)
		}
		out, err := Normalize(buf.Bytes(), ContentType)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out, buf.Bytes()) {
			t.Error("re-encoded")
		}
	})

	t.Run("heic", func(t *testing.T) {
		data, err := os.ReadFile("testdata/receipt.heic")
		if err != nil {
			t.Fatal(err)
		}
		out, err := Normalize(data, "image/heic")
		if err != nil {
			t.Fatal(err)
		}
		if b := decodeWebP(t, out).Bounds(); b.Empty() || max(b.Dx(), b.Dy()) > MaxSide {
			t.Errorf("got %v", b.Size())
		}
	})

	t.Run("not an image", func(t *testing.T) {
		if _, err := Normalize([]byte("nope"), "image/jpeg"); err == nil {
			t.Error("no error")
		}
		if _, err := Normalize([]byte("nope"), "image/gif"); err != ErrUnsupported {
			t.Errorf("got %v, want ErrUnsupported", err)
		}
	})
}

func TestJPEGOrientationWithoutExif(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, photo(10, 10), nil); err != nil {
		t.Fatal(err)
	}
	if o := jpegOrientation(buf.Bytes()); o != 1 {
		t.Errorf("got %d, want 1", o)
	}
}
