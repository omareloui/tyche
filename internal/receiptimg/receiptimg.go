// Package receiptimg turns receipts' photos into what is stored and read: a
// WebP at most MaxSide px on its long edge, upright (EXIF orientation
// applied), whatever was uploaded (JPEG, PNG, WebP, HEIC).
package receiptimg

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	"github.com/gen2brain/heic"
	"github.com/gen2brain/webp"
	"golang.org/x/image/draw"
)

const (
	// MaxSide is the longest edge kept, enough to read a receipt's text.
	MaxSide = 1600
	// Quality is the WebP's, the browser's too (receipts.js).
	Quality = 75
	// ContentType is the stored images'.
	ContentType = "image/webp"
)

// ErrUnsupported is an image of a type it doesn't read.
var ErrUnsupported = errors.New("receiptimg: unsupported image type")

// sem bounds the conversions at once: a 12 MP photo decodes to ~50 MB.
var sem = make(chan struct{}, 2)

// Normalize is the image as a WebP within MaxSide. A WebP already within it
// is kept as is (the browser's, receipts.js).
func Normalize(data []byte, contentType string) ([]byte, error) {
	if contentType == ContentType {
		if cfg, err := webp.DecodeConfig(bytes.NewReader(data)); err == nil && max(cfg.Width, cfg.Height) <= MaxSide {
			return data, nil
		}
	}

	sem <- struct{}{}
	defer func() { <-sem }()

	img, err := decode(data, contentType)
	if err != nil {
		return nil, err
	}
	img = orient(shrink(img), orientation(data, contentType))
	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, webp.Options{Quality: Quality}); err != nil {
		return nil, fmt.Errorf("receiptimg: encoding webp: %w", err)
	}
	return buf.Bytes(), nil
}

func decode(data []byte, contentType string) (image.Image, error) {
	r := bytes.NewReader(data)
	var (
		img image.Image
		err error
	)
	switch contentType {
	case "image/jpeg":
		img, err = jpeg.Decode(r)
	case "image/png":
		img, err = png.Decode(r)
	case "image/webp":
		img, err = webp.Decode(r, webp.Options{AutoRotate: true})
	case "image/heic", "image/heif":
		// Its orientation (irot / imir) is applied by the decoder.
		img, err = heic.Decode(r)
	default:
		return nil, ErrUnsupported
	}
	if err != nil {
		return nil, fmt.Errorf("receiptimg: decoding %s: %w", contentType, err)
	}
	return img, nil
}

// shrink scales img down to MaxSide on its long edge, as is when within it.
func shrink(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	long := max(w, h)
	if long <= MaxSide {
		return img
	}
	w, h = max(w*MaxSide/long, 1), max(h*MaxSide/long, 1)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// orientation is the EXIF orientation (1-8) to apply to the decoded image,
// 1 when none: only JPEG's decoder leaves it to the viewer.
func orientation(data []byte, contentType string) int {
	if contentType == "image/jpeg" {
		return jpegOrientation(data)
	}
	return 1
}

// orient applies the EXIF orientation o to img.
func orient(img image.Image, o int) image.Image {
	if o < 2 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	// 5 to 8 turn it a quarter: width and height swap.
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range h {
		for x := range w {
			var dx, dy int
			switch o {
			case 2: // mirrored
				dx, dy = w-1-x, y
			case 3: // upside down
				dx, dy = w-1-x, h-1-y
			case 4: // upside down, mirrored
				dx, dy = x, h-1-y
			case 5: // transposed
				dx, dy = y, x
			case 6: // turned left: turn right
				dx, dy = h-1-y, x
			case 7: // transversed
				dx, dy = h-1-y, w-1-x
			case 8: // turned right: turn left
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// jpegOrientation reads the orientation tag of a JPEG's EXIF (APP1), 1 when
// none.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // image data starts: no EXIF
			return 1
		}
		size := int(data[i+2])<<8 | int(data[i+3])
		end := i + 2 + size
		if size < 2 || end > len(data) {
			return 1
		}
		if seg := data[i+4 : end]; marker == 0xE1 && len(seg) > 6 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i = end
	}
	return 1
}

// tiffOrientation is the orientation tag (0x0112) of the TIFF header's first
// IFD, 1 when none.
func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var u16 func([]byte) int
	var u32 func([]byte) int
	switch string(t[:2]) {
	case "II":
		u16 = func(b []byte) int { return int(b[0]) | int(b[1])<<8 }
		u32 = func(b []byte) int { return u16(b) | u16(b[2:])<<16 }
	case "MM":
		u16 = func(b []byte) int { return int(b[0])<<8 | int(b[1]) }
		u32 = func(b []byte) int { return u16(b)<<16 | u16(b[2:]) }
	default:
		return 1
	}
	ifd := u32(t[4:])
	if ifd < 8 || ifd+2 > len(t) {
		return 1
	}
	n := u16(t[ifd:])
	for k := range n {
		e := ifd + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if u16(t[e:]) == 0x0112 {
			if o := u16(t[e+8:]); o >= 1 && o <= 8 {
				return o
			}
			return 1
		}
	}
	return 1
}
