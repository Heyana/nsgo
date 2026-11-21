package scrollbar

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestRebaseOffsetCarriesAnimationAndGrabbedThumb(t *testing.T) {
	offset := driver.Point{X: 60, Y: 80}
	delta := driver.Point{X: 11, Y: -17}
	start := time.Unix(123, 0)

	for _, axis := range []struct {
		name       string
		horizontal bool
		thumb      driver.Point
	}{
		{name: "vertical", thumb: driver.Point{X: 60, Y: 63}},
		{name: "horizontal", horizontal: true, thumb: driver.Point{X: 71, Y: 80}},
	} {
		t.Run(axis.name, func(t *testing.T) {
			config := driver.ScrollConfig{Horizontal: axis.horizontal}
			bar := Bar{Horizontal: axis.horizontal, Frame: driver.Size{Width: 100, Height: 100}, Content: driver.Size{Width: 400, Height: 400}, Offset: offset}
			grab := driver.Point{X: 20, Y: 25}
			drag := bar.Press(grab)
			if got := drag.Offset(grab); got != offset {
				t.Fatalf("the press missed the thumb: offset = %v", got)
			}
			moving := &OffsetAnimation{
				From: driver.Point{X: 20, Y: 40}, To: driver.Point{X: 80, Y: 120},
				Start: start, Momentum: true,
			}
			if got := RebaseOffset(&config, offset, delta, moving, &drag); got != (driver.Point{X: 71, Y: 63}) {
				t.Fatalf("offset = %v, want {71 63}", got)
			}
			want := OffsetAnimation{From: driver.Point{X: 31, Y: 23}, To: driver.Point{X: 91, Y: 103}, Start: start, Momentum: true}
			if *moving != want {
				t.Fatalf("animation = %+v, want %+v", *moving, want)
			}
			if got := drag.Offset(grab); got != axis.thumb {
				t.Fatalf("thumb offset = %v, want %v", got, axis.thumb)
			}
		})
	}

	if got := RebaseOffset(&driver.ScrollConfig{}, offset, driver.Point{X: -100, Y: 500}, nil, nil); got != (driver.Point{X: -40, Y: 580}) {
		t.Fatalf("anchor clamped the offset: %v", got)
	}
}
