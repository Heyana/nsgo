package imagedecode

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// RasterizeSVG draws a parsed *driver.SVG into a width×height premultiplied bitmap in layout (BGRA or RGBA), painting currentColor with color (black when zero), fitted and centered while preserving aspect ratio.
func RasterizeSVG(s *driver.SVG, width, height int, color driver.Color, layout Layout) (*Image, error) {
	if s == nil || width <= 0 || height <= 0 || width > MaxSide || height > MaxSide || width*height > MaxPixels {
		return nil, errors.New("imagedecode: invalid SVG raster size")
	}
	d := &driver.Drawing{}
	s.Draw(d, driver.Size{Width: driver.Points(width), Height: driver.Points(height)}, color)
	pix, err := rasterize(d, width, height)
	if err != nil {
		return nil, err
	}
	if layout == BGRA {
		swapRB(pix)
	}
	pw, ph := s.Size()
	return &Image{
		Format: SVG,
		Width:  width,
		Height: height,
		Size:   driver.Size{Width: pw, Height: ph},
		Frames: []Frame{{Pix: pix}},
	}, nil
}

// FitPremultiplied fits a premultiplied 4-byte-per-pixel image (RGBA or BGRA) into a dstW×dstH buffer, centered with its aspect ratio preserved. Area averaging is used when downscaling and bilinear interpolation when upscaling.
func FitPremultiplied(pix []byte, srcW, srcH, dstW, dstH int) []byte {
	if srcW <= 0 || srcH <= 0 || dstW <= 0 || dstH <= 0 || len(pix) < srcW*srcH*4 {
		return nil
	}
	if srcW == dstW && srcH == dstH {
		return bytes.Clone(pix[:srcW*srcH*4])
	}
	scale := min(float64(dstW)/float64(srcW), float64(dstH)/float64(srcH))
	fitW := max(1, int(math.Round(float64(srcW)*scale)))
	fitH := max(1, int(math.Round(float64(srcH)*scale)))
	fitW = min(fitW, dstW)
	fitH = min(fitH, dstH)
	ox := (dstW - fitW) / 2
	oy := (dstH - fitH) / 2

	out := make([]byte, dstW*dstH*4)
	downscale := fitW < srcW || fitH < srcH
	for y := range fitH {
		dy := oy + y
		if downscale {
			sy0 := float64(y) * float64(srcH) / float64(fitH)
			sy1 := float64(y+1) * float64(srcH) / float64(fitH)
			iy0 := int(math.Floor(sy0))
			iy1 := min(srcH, int(math.Ceil(sy1)))
			for x := range fitW {
				sx0 := float64(x) * float64(srcW) / float64(fitW)
				sx1 := float64(x+1) * float64(srcW) / float64(fitW)
				ix0 := int(math.Floor(sx0))
				ix1 := min(srcW, int(math.Ceil(sx1)))
				var c0, c1, c2, c3, area float64
				for sy := iy0; sy < iy1; sy++ {
					wy := min(float64(sy+1), sy1) - max(float64(sy), sy0)
					if wy <= 0 {
						continue
					}
					row := sy * srcW * 4
					for sx := ix0; sx < ix1; sx++ {
						wx := min(float64(sx+1), sx1) - max(float64(sx), sx0)
						if wx <= 0 {
							continue
						}
						w := wx * wy
						i := row + sx*4
						c0 += float64(pix[i]) * w
						c1 += float64(pix[i+1]) * w
						c2 += float64(pix[i+2]) * w
						c3 += float64(pix[i+3]) * w
						area += w
					}
				}
				if area > 0 {
					di := (dy*dstW + ox + x) * 4
					out[di] = clampByte(c0 / area)
					out[di+1] = clampByte(c1 / area)
					out[di+2] = clampByte(c2 / area)
					out[di+3] = clampByte(c3 / area)
				}
			}
		} else {
			sy := (float64(y)+0.5)*float64(srcH)/float64(fitH) - 0.5
			y0 := max(0, min(srcH-1, int(math.Floor(sy))))
			y1 := max(0, min(srcH-1, y0+1))
			fy := clampUnit(sy - float64(y0))
			for x := range fitW {
				sx := (float64(x)+0.5)*float64(srcW)/float64(fitW) - 0.5
				x0 := max(0, min(srcW-1, int(math.Floor(sx))))
				x1 := max(0, min(srcW-1, x0+1))
				fx := clampUnit(sx - float64(x0))
				i00 := (y0*srcW + x0) * 4
				i10 := (y0*srcW + x1) * 4
				i01 := (y1*srcW + x0) * 4
				i11 := (y1*srcW + x1) * 4
				w00 := (1 - fx) * (1 - fy)
				w10 := fx * (1 - fy)
				w01 := (1 - fx) * fy
				w11 := fx * fy
				di := (dy*dstW + ox + x) * 4
				for c := range 4 {
					v := float64(pix[i00+c])*w00 + float64(pix[i10+c])*w10 + float64(pix[i01+c])*w01 + float64(pix[i11+c])*w11
					out[di+c] = clampByte(v)
				}
			}
		}
	}
	return out
}

func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampByte(v float64) byte {
	n := int(math.Round(v))
	if n <= 0 {
		return 0
	}
	if n >= 255 {
		return 255
	}
	return byte(n)
}

// TintPremultiplied replaces the RGB channels of a premultiplied bitmap with color scaled by each pixel's alpha, preserving alpha.
func TintPremultiplied(pix []byte, layout Layout, color driver.Color) {
	r := uint32((color >> 24) & 0xff)
	g := uint32((color >> 16) & 0xff)
	b := uint32((color >> 8) & 0xff)
	for i := 0; i+3 < len(pix); i += 4 {
		a := uint32(pix[i+3])
		if a == 0 {
			pix[i], pix[i+1], pix[i+2] = 0, 0, 0
			continue
		}
		pr := byte((r*a + 127) / 255)
		pg := byte((g*a + 127) / 255)
		pb := byte((b*a + 127) / 255)
		if layout == BGRA {
			pix[i], pix[i+1], pix[i+2] = pb, pg, pr
		} else {
			pix[i], pix[i+1], pix[i+2] = pr, pg, pb
		}
	}
}

// EncodePNG encodes a premultiplied RGBA or BGRA pixel buffer as a straight-alpha PNG.
func EncodePNG(pix []byte, width, height int, layout Layout) ([]byte, error) {
	if width <= 0 || height <= 0 || len(pix) < width*height*4 {
		return nil, errors.New("imagedecode: invalid bitmap for PNG")
	}
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < width*height*4; i += 4 {
		a := pix[i+3]
		if a == 0 {
			continue
		}
		var r, g, b byte
		if layout == BGRA {
			b, g, r = pix[i], pix[i+1], pix[i+2]
		} else {
			r, g, b = pix[i], pix[i+1], pix[i+2]
		}
		if a < 255 {
			r = byte(min(255, (uint32(r)*255+uint32(a)/2)/uint32(a)))
			g = byte(min(255, (uint32(g)*255+uint32(a)/2)/uint32(a)))
			b = byte(min(255, (uint32(b)*255+uint32(a)/2)/uint32(a)))
		}
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = r, g, b, a
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
