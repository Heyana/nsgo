//go:build ios

package ios

import (
	"strconv"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func (s *surface) displaySnapshot() ([]driver.DisplayInfo, string) {
	class := objc.ID(apple.Class("UIScreen"))
	main := class.Send(sel("mainScreen"))
	window := s.uiWindow()
	current := window.Send(sel("windowScene")).Send(sel("screen"))
	if current == 0 {
		current = window.Send(sel("screen"))
	}
	if current == 0 {
		current = main
	}
	screens := class.Send(sel("screens"))
	count := objc.Send[uint](screens, sel("count"))
	infos := make([]driver.DisplayInfo, 0, count)
	currentID := ""
	for i := uint(0); i < count; i++ {
		screen := screens.Send(sel("objectAtIndex:"), i)
		id := strconv.FormatUint(uint64(i), 10)
		frame := objc.Send[apple.CGRect](screen, sel("bounds"))
		bounds := driver.Bounds{Size: driver.Size{Width: driver.Points(frame.Size.Width), Height: driver.Points(frame.Size.Height)}}
		rate := float64(max(0, objc.Send[int](screen, sel("maximumFramesPerSecond"))))
		infos = append(infos, driver.DisplayInfo{ID: id, Frame: bounds, VisibleFrame: bounds, Main: screen == main,
			Scale: float32(objc.Send[float64](screen, sel("scale"))), RefreshRate: rate, MaxRefreshRate: rate})
		if screen == current {
			currentID = id
		}
	}
	return infos, currentID
}
