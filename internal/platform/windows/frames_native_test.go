//go:build windows

package windows

import (
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// An app that scrolls a list a row further every display frame, as the
// benchmark's does, wakes the engine once a frame: the scroll each batch
// reports rides with the next frame rather than waking it for a pass of
// its own.
func TestAListScrolledEveryFrameWakesTheEngineOnceAFrame(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	title := "nsgo-native-" + t.Name()
	var frames atomic.Int64
	app := func(cx *core.Scope) core.Node {
		list := &core.Ref{}
		top := 0
		var frame func(time.Time)
		frame = func(time.Time) {
			frames.Add(1)
			top = (top + 1) % 900
			list.ScrollToIndex(top, false)
			cx.RequestAnimationFrame(frame)
		}
		cx.RequestAnimationFrame(frame)
		return core.List{Ref: list, Count: func() int { return 1000 }, EstimatedSize: 20,
			Style: core.Style{Width: driver.Pt(200), Height: driver.Pt(200)},
			Row: func(cx *core.Scope, r core.ListRow) core.Node {
				return core.Text{ContentFunc: func() string { return strconv.Itoa(r.Index()) }, Style: core.Style{Height: driver.Pt(20)}}
			}}
	}
	hwnd := startAppWith(t, app, Options{Title: title, Width: 200, Height: 200, Perf: true})
	time.Sleep(500 * time.Millisecond)
	from := frames.Load()
	time.Sleep(time.Second)
	var passes float32
	onMain(t, hwnd, func(w *window) { passes = w.hud.monitor.Snapshot().Passes })
	n := frames.Load() - from
	if n < 20 && !sessionActive() {
		t.Skipf("the session is not active, where DWM composed only %d frames in a second", n)
	}
	if n < 20 || passes > float32(n+n/10+2) {
		t.Errorf("%d frames took %.0f engine passes, want about one a frame", n, passes)
	}
}

// sessionActive reports whether the process's session is connected (WTSActive). DWM composes a disconnected session's desktop a few times a second, so DwmFlush paces the frame clock that slowly there.
func sessionActive() bool {
	query := syscall.NewLazyDLL("wtsapi32.dll").NewProc("WTSQuerySessionInformationW")
	free := syscall.NewLazyDLL("wtsapi32.dll").NewProc("WTSFreeMemory")
	const currentServer, currentSession, connectState, active = 0, ^uintptr(0), 8, 0
	var buf *int32
	var size uint32
	if ok, _, _ := query.Call(currentServer, currentSession, connectState, uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&size))); ok == 0 || buf == nil {
		return true
	}
	defer free.Call(uintptr(unsafe.Pointer(buf)))
	return *buf == active
}
