//go:build windows && nsgoautomation

package windows

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func TestLiveSurfaceTracking(t *testing.T) {
	root := &window{primary: true, closed: make(chan struct{}), opts: Options{Hidden: true}}
	child := &window{surfaceID: 1, hwnd: 1, closed: make(chan struct{})}
	other := &window{surfaceID: 1, hwnd: 2, closed: make(chan struct{})}

	t.Cleanup(func() {
		untrackLiveWindow(0, root)
		untrackLiveWindow(1, child)
		untrackLiveWindow(1, other)
	})

	if s := testhost.Live(1, 0); s != nil {
		t.Fatalf("unexpected live surface 0 before tracking: %v", s)
	}
	trackLiveWindow(0, root)
	trackLiveWindow(1, child)

	s0, ok := testhost.Live(1, 0).(*testSurface)
	if !ok || s0.w != root || s0.test != nil {
		t.Fatalf("live surface 0 = %#v, want root live testSurface", s0)
	}
	s1, ok := s0.WindowSurface(1).(*testSurface)
	if !ok || s1.w != child || s1.test != nil {
		t.Fatalf("WindowSurface(1) = %#v, want child live testSurface", s1)
	}
	if err := s0.SetEnvironment(driver.Environment{}); err == nil || !strings.Contains(err.Error(), "system's settings") {
		t.Errorf("SetEnvironment on live surface = %v", err)
	}
	if err := s0.RequestClose(); err == nil || !strings.Contains(err.Error(), "no window to close") {
		t.Errorf("RequestClose on hidden root = %v", err)
	}

	// Untracking a different window with the same ID must leave child tracked.
	untrackLiveWindow(1, other)
	if testhost.Live(1, 1) == nil {
		t.Fatal("untracking another window dropped live surface 1")
	}

	// Closing a window's channel hides it from liveSurface even before untracking.
	close(child.closed)
	if s := testhost.Live(1, 1); s != nil {
		t.Fatalf("closed window still returned by Live(1): %v", s)
	}

	untrackLiveWindow(0, root)
	if s := testhost.Live(1, 0); s != nil {
		t.Fatalf("untracked root still returned by Live(0): %v", s)
	}
}
