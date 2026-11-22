package harmonyos

import (
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// rawMeasurer measures text in style wrapped at maxWidth (Auto leaves it
// unwrapped), matching the HarmonyOS host's MeasureText bridge.
type rawMeasurer func(text string, style driver.TextStyle, maxWidth driver.Points) driver.Size

// splitParagraphs splits text at explicit line breaks (\r\n, \n, \r).
func splitParagraphs(text string) []string {
	var out []string
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			out = append(out, text[start:i])
			start = i + 1
		} else if text[i] == '\r' {
			out = append(out, text[start:i])
			if i+1 < len(text) && text[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	out = append(out, text[start:])
	return out
}

// limitedHeight returns the host's height for maxLines lines in style by
// probing one- and two-line heights once (which the caller's measurer caches).
func limitedHeight(style driver.TextStyle, maxLines int, raw rawMeasurer) driver.Points {
	one := raw("A", style, driver.Auto).Height
	if maxLines <= 1 || one <= 0 {
		return one
	}
	two := raw("A\nA", style, driver.Auto).Height
	step := two - one
	if step <= 0 {
		step = one
	}
	return one + driver.Points(maxLines-1)*step
}

// measureLimited measures text with MaxLines applied on top of the host's
// line-unlimited MeasureText bridge. Paragraphs past maxLines are dropped
// before measuring so hidden paragraphs cannot widen or heighten the box;
// when the remaining text still wraps past maxLines lines, its height is
// capped to maxLines lines and an ellipsized soft wrap keeps the offered width.
func measureLimited(text string, style driver.TextStyle, maxLines int, ellipsize driver.EllipsizeMode, maxWidth driver.Points, raw rawMeasurer) driver.Size {
	if maxLines <= 0 {
		return raw(text, style, maxWidth)
	}
	bounded := !maxWidth.IsAuto() && maxWidth > 0
	paras := splitParagraphs(text)
	droppedParas := len(paras) > maxLines
	kept := text
	if droppedParas {
		kept = strings.Join(paras[:maxLines], "\n")
	}
	if !bounded {
		if droppedParas && ellipsize != driver.EllipsizeClip {
			return raw(kept+"…", style, maxWidth)
		}
		return raw(kept, style, maxWidth)
	}
	s := raw(kept, style, maxWidth)
	limitH := limitedHeight(style, maxLines, raw)
	if limitH > 0 && s.Height > limitH+0.5 {
		w := s.Width
		if ellipsize != driver.EllipsizeClip {
			w = maxWidth
		} else if w > maxWidth {
			w = maxWidth
		}
		return driver.Size{Width: w, Height: limitH}
	}
	if droppedParas && ellipsize != driver.EllipsizeClip {
		ell := raw(kept+"…", style, maxWidth)
		if limitH > 0 && ell.Height > limitH+0.5 {
			return driver.Size{Width: maxWidth, Height: limitH}
		}
		if ell.Width > maxWidth {
			ell.Width = maxWidth
		}
		return ell
	}
	if s.Width > maxWidth {
		s.Width = maxWidth
	}
	return s
}
