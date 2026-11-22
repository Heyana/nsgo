package ios

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// titleWrapExpandedWidth asks UINavigationBar to expand a filling titleView
// across all room between its leading and trailing items before the bar's
// width is known, matching UIView.layoutFittingExpandedSize.
const titleWrapExpandedWidth = 10000.0

// titleWrapIntrinsicSize is the intrinsic content size NsgoTitleView reports
// to UIKit's UINavigationBar: expanded across the bar when fills is set,
// otherwise the core's laid-out frame size.
func titleWrapIntrinsicSize(frame driver.Size, barWidth float64, fills bool) (width, height float64) {
	height = float64(frame.Height)
	if fills {
		if barWidth > 0 {
			return barWidth, height
		}
		return titleWrapExpandedWidth, height
	}
	return float64(frame.Width), height
}

// titleWrapFitSize answers UIView.sizeThatFits: for NsgoTitleView.
func titleWrapFitSize(frame driver.Size, fitWidth, barWidth float64, fills bool) (width, height float64) {
	w, h := titleWrapIntrinsicSize(frame, barWidth, fills)
	if fills && fitWidth > 0 && fitWidth < w {
		w = fitWidth
	}
	return w, h
}

// titleWrapPriorities returns horizontal content hugging and compression
// resistance priorities for NsgoTitleView. A filling title view uses low
// priorities so UIKit's back button and bar buttons keep their full widths
// while the title view expands into whatever room remains between them.
func titleWrapPriorities(fills bool) (hugging, compression float32) {
	if fills {
		return 50, 50
	}
	return 750, 250
}

// reportedTitleWidth returns the available TitleView width to report to the
// engine from NsgoTitleView's laid-out bounds width. Filling title views
// always report their laid-out width; content-sized title views only report
// when UIKit compressed them narrower than their core frame so their natural
// width is never mistakenly saved as the bar's capacity.
func reportedTitleWidth(boundsWidth float64, frameWidth driver.Points, ratio float32, fills bool) (driver.Points, bool) {
	if boundsWidth <= 0 {
		return 0, false
	}
	if ratio <= 0 {
		ratio = 1
	}
	w := driver.Points(math.Round(boundsWidth*float64(ratio)) / float64(ratio))
	if w <= 0 {
		return 0, false
	}
	if fills {
		return w, true
	}
	if float64(w) < float64(frameWidth)-0.5 {
		return w, true
	}
	return 0, false
}

// titleWrapNeedsRelayout reports whether updating a title view's core frame
// should invalidate its intrinsic content size and relayout the navigation
// bar. Filling title views take their width from UIKit, so only height
// changes require another UIKit pass.
func titleWrapNeedsRelayout(last, cur driver.Size, fills bool) bool {
	if fills {
		return cur.Height != last.Height
	}
	return cur != last
}
