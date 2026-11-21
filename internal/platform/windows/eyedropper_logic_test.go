package windows

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestColorFromColorRef(t *testing.T) {
	cases := []struct {
		name string
		cr   uint32
		want driver.Color
		ok   bool
	}{
		{name: "black", cr: 0x00000000, want: 0x000000ff, ok: true},
		{name: "white", cr: 0x00ffffff, want: 0xffffffff, ok: true},
		{name: "red", cr: 0x000000ff, want: 0xff0000ff, ok: true},
		{name: "green", cr: 0x0000ff00, want: 0x00ff00ff, ok: true},
		{name: "blue", cr: 0x00ff0000, want: 0x0000ffff, ok: true},
		{name: "mixed bgr", cr: 0x00563412, want: 0x123456ff, ok: true},
		{name: "invalid", cr: colorRefInvalid, want: 0, ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := colorFromColorRef(tc.cr)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("colorFromColorRef(0x%08x) = (0x%08x, %v), want (0x%08x, %v)", tc.cr, uint32(got), ok, uint32(tc.want), tc.ok)
			}
		})
	}
}

func TestVirtualScreenRectMultiMonitor(t *testing.T) {
	r := virtualScreenRect{x: -1920, y: -200, width: 3840, height: 1280}
	if !r.valid() {
		t.Fatal("expected multi-monitor virtual screen rect to be valid")
	}
	if !r.contains(-1920, -200) || !r.contains(1919, 1079) || !r.contains(0, 0) {
		t.Fatal("expected corners and primary origin to be contained")
	}
	if r.contains(-1921, 0) || r.contains(1920, 0) || r.contains(0, -201) || r.contains(0, 1080) {
		t.Fatal("expected outside points not to be contained")
	}
	if x, y := r.clamp(-3000, -500); x != -1920 || y != -200 {
		t.Fatalf("clamp low = (%d, %d), want (-1920, -200)", x, y)
	}
	if x, y := r.clamp(5000, 3000); x != 1919 || y != 1079 {
		t.Fatalf("clamp high = (%d, %d), want (1919, 1079)", x, y)
	}
	if x, y := r.clamp(-400, 300); x != -400 || y != 300 {
		t.Fatalf("clamp interior = (%d, %d), want (-400, 300)", x, y)
	}

	empty := virtualScreenRect{x: 0, y: 0, width: 0, height: 1080}
	if empty.valid() || empty.contains(0, 0) {
		t.Fatal("expected zero-width virtual screen rect to be invalid")
	}
}

func TestColorPickerStateTransitions(t *testing.T) {
	s := colorPickerState{starting: true}
	if got := s.handle(colorPickCaptureLost); got != colorPickContinue {
		t.Fatalf("capture lost during startup = %v, want continue", got)
	}
	if got := s.handle(colorPickDeactivate); got != colorPickContinue {
		t.Fatalf("deactivate during startup = %v, want continue", got)
	}
	s.starting = false

	if got := s.handle(colorPickOtherKey); got != colorPickContinue {
		t.Fatalf("other key = %v, want continue", got)
	}
	if got := s.handle(colorPickLeftDown); got != colorPickSample {
		t.Fatalf("left down = %v, want sample", got)
	}
	for _, ev := range []colorPickerEvent{
		colorPickRightDown,
		colorPickMiddleDown,
		colorPickEscape,
		colorPickCaptureLost,
		colorPickDeactivate,
		colorPickCancelMode,
		colorPickDestroy,
	} {
		if got := s.handle(ev); got != colorPickCancel {
			t.Fatalf("event %v = %v, want cancel", ev, got)
		}
	}

	c, ok, first := s.finish(0x112233ff, true)
	if !first || !ok || c != 0x112233ff {
		t.Fatalf("first finish = (0x%08x, %v, %v), want (0x112233ff, true, true)", uint32(c), ok, first)
	}
	if _, _, second := s.finish(0x445566ff, true); second {
		t.Fatal("second finish must be ignored")
	}
	if got := s.handle(colorPickLeftDown); got != colorPickContinue {
		t.Fatalf("event after done = %v, want continue", got)
	}
}

func TestColorPickerResultEvent(t *testing.T) {
	picked := colorPickerResultEvent(7, 0x336699ff, true)
	if picked.Kind != driver.EventFiles || picked.Count != 7 || !picked.Finished || picked.Data != driver.Color(0x336699ff) {
		t.Fatalf("picked event = %+v", picked)
	}
	cancelled := colorPickerResultEvent(9, 0x336699ff, false)
	if cancelled.Kind != driver.EventFiles || cancelled.Count != 9 || cancelled.Finished || cancelled.Data != nil {
		t.Fatalf("cancelled event = %+v", cancelled)
	}
}
