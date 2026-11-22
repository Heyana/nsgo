package terminal

import (
	"math"

	"github.com/nsgo-dev/nsgo/core"
)

// Emoji are color glyphs. Their text paint is not their visible colors.
func emojiRune(r rune) bool {
	return r >= 0x1f000 && r <= 0x1faff || r >= 0x2600 && r <= 0x27bf
}

func luminance(c core.Color) float64 {
	linear := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(uint8(c>>24)) + 0.7152*linear(uint8(c>>16)) + 0.0722*linear(uint8(c>>8))
}

func contrastRatio(a, b core.Color) float64 {
	x, y := luminance(a), luminance(b)
	return (max(x, y) + 0.05) / (min(x, y) + 0.05)
}

// compositeColor resolves the opacity of text or a selection against the
// cell, so faint text is checked as it appears rather than before blending.
func compositeColor(fg, bg core.Color) core.Color {
	a := float64(uint8(fg)) / 255
	channel := func(shift uint) uint32 {
		return uint32(math.Round(float64(uint8(fg>>shift))*a + float64(uint8(bg>>shift))*(1-a)))
	}
	return core.RGB(channel(24)<<16 | channel(16)<<8 | channel(8))
}

// minimumContrast moves only as far towards white or black as needed. Its
// endpoint is whichever can contrast most with this cell; impossible ratios
// reach that endpoint. The zero value preserves the program's colors.
func minimumContrast(fg, bg core.Color, ratio float64) core.Color {
	if !(ratio > 1) {
		return fg
	}
	ratio = min(ratio, 21)
	visible := compositeColor(fg, bg)
	if contrastRatio(visible, bg) >= ratio {
		return fg
	}
	end := core.RGB(0)
	if contrastRatio(core.RGB(0xffffff), bg) > contrastRatio(end, bg) {
		end = core.RGB(0xffffff)
	}
	mix := func(t float64) core.Color {
		ch := func(shift uint) uint32 {
			a, b := float64(uint8(visible>>shift)), float64(uint8(end>>shift))
			return uint32(math.Round(a + (b-a)*t))
		}
		return core.RGB(ch(24)<<16 | ch(16)<<8 | ch(8))
	}
	lo, hi := 0.0, 1.0
	for range 12 {
		mid := (lo + hi) / 2
		if contrastRatio(mix(mid), bg) >= ratio {
			hi = mid
		} else {
			lo = mid
		}
	}
	return mix(hi)
}
