// Package qrcode rasterizes a QR code to a PNG image, for endpoints
// that need to hand a student a scannable code (as opposed to
// internal/certificate, which draws QR modules directly onto a PDF
// page and never needs a standalone image file).
package qrcode

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"rsc.io/qr"
)

// modulePx is the pixel size of one QR module — large enough to stay
// scannable from a phone screen or a small kiosk print without the
// image getting unreasonably large.
const modulePx = 8

// DataURL encodes data as a QR code and returns it as a
// "data:image/png;base64,..." string, ready to drop straight into an
// <img src="..."> on the frontend.
func DataURL(data string) (string, error) {
	code, err := qr.Encode(data, qr.M)
	if err != nil {
		return "", fmt.Errorf("encode qr: %w", err)
	}

	size := code.Size * modulePx
	img := image.NewGray(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if code.Black(x/modulePx, y/modulePx) {
				img.SetGray(x, y, color.Gray{Y: 0})
			} else {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("encode png: %w", err)
	}

	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
