package windows

import "github.com/nsgo-dev/nsgo/internal/driver"

// limitedMeasureInput carries the measurements of a layout whose wrapped
// lines exceed MaxLines: preWidth is the widest kept line before height
// trimming, trimmedWidth and trimmedLines come from GetMetrics after
// SetMaxHeight trims the layout at the kept lines' height, and softWrap
// reports whether the last kept line wrapped rather than ending at a hard
// break.
type limitedMeasureInput struct {
	preWidth     float32
	trimmedWidth float32
	trimmedLines int
	shown        int
	limit        float32
	bounded      bool
	softWrap     bool
	ellipsize    driver.EllipsizeMode
	useCut       bool
}

// limitedMeasureWidth returns the width Measure reports for a layout whose
// wrapped lines exceed MaxLines.
func limitedMeasureWidth(in limitedMeasureInput) float32 {
	clamp := func(w float32) float32 {
		if in.bounded {
			return min(w, in.limit)
		}
		return w
	}
	if in.ellipsize == driver.EllipsizeClip {
		return clamp(in.preWidth)
	}
	if in.useCut {
		if in.softWrap && in.bounded {
			return in.limit
		}
		if in.trimmedLines > 0 && in.trimmedLines <= in.shown {
			return clamp(max(in.preWidth, in.trimmedWidth))
		}
		return clamp(in.preWidth)
	}
	if in.trimmedLines > 0 && in.trimmedLines <= in.shown {
		return clamp(max(in.preWidth, in.trimmedWidth))
	}
	if in.softWrap && in.bounded {
		return in.limit
	}
	return clamp(in.preWidth)
}

// singlePassMeasureWidth returns the width Measure reports when every wrapped
// line fits within MaxLines (including a single non-wrapping MaxLines:1 line
// trimmed horizontally by DirectWrite). When Layout will replace the line with
// a head or middle cut (useCut), a trimmed single line keeps the full offered
// width so CutToFit can fill it instead of DirectWrite's tail-trimmed width.
func singlePassMeasureWidth(metricsWidth, limit float32, bounded, useCut, firstLineTrimmed bool) float32 {
	if useCut && firstLineTrimmed && bounded {
		return limit
	}
	if bounded {
		return min(metricsWidth, limit)
	}
	return metricsWidth
}
