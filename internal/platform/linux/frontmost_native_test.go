//go:build linux && !android

package linux

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// This opts into an isolated Sway compositor, never the user's Wayland socket. Focus commands target only the two fixture application IDs.
func TestWaylandFrontmostTracksCompositor(t *testing.T) {
	if os.Getenv("NSGO_FRONTMOST_WAYLAND_NATIVE") != "1" {
		t.Skip("needs an owned Sway session and NSGO_FRONTMOST_WAYLAND_NATIVE=1")
	}
	const firstID = "dev.nsgo.frontmost1393.first"
	const secondID = "dev.nsgo.frontmost1393.second"
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME"} {
		t.Setenv(key, t.TempDir())
	}
	if err := os.MkdirAll(filepath.Join(data, "applications"), 0700); err != nil {
		t.Fatal(err)
	}
	for id, name := range map[string]string{firstID: "NSGo Frontmost First", secondID: "NSGo Frontmost Second"} {
		if err := os.WriteFile(filepath.Join(data, "applications", id+".desktop"), []byte("[Desktop Entry]\nType=Application\nName="+name+"\nExec=true\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	clip := filepath.Join(t.TempDir(), "frontmost-player.webm")
	if err := os.WriteFile(clip, videoRateClip, 0600); err != nil {
		t.Fatal(err)
	}
	var played atomic.Bool
	errors := make(chan error, 1)
	var latest core.FrontmostAppInfo
	var latestMu sync.Mutex
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		cx.Effect(func() { app := cx.FrontmostApp(); latestMu.Lock(); latest = app; latestMu.Unlock() })
		return core.Video{Source: clip, Muted: true, Repeat: true, Controls: true, Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
			OnProgress: func(position, _ time.Duration) {
				if position > 0 {
					played.Store(true)
				}
			},
			OnError: func(err error) {
				select {
				case errors <- err:
				default:
				}
			},
		}
	}, Options{ID: firstID, Title: "NSGo 1393 First Player", Width: 480, Height: 300})
	wait := func(what string, condition func() bool) {
		t.Helper()
		for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			select {
			case err := <-errors:
				t.Fatalf("native player: %v", err)
			default:
			}
			if condition() {
				return
			}
		}
		latestMu.Lock()
		last := latest
		latestMu.Unlock()
		t.Fatalf("timed out waiting for %s; owned observation: %+v", what, last)
	}
	seen := func(id, name string, full bool) bool {
		latestMu.Lock()
		defer latestMu.Unlock()
		return latest.ID == id && latest.Name == name && latest.PID == 0 && latest.FullScreen == full
	}
	supported := false
	w.onLoop(t, func() { _, supported = frontmostApp() })
	if !supported {
		t.Fatal("the owned compositor did not expose the foreign-toplevel protocol")
	}
	wait("the active player", func() bool { return seen(firstID, "NSGo Frontmost First", false) })
	wait("actual media playback", played.Load)
	w.onLoop(t, func() { gtkWindowMaximize(w.win) })
	time.Sleep(100 * time.Millisecond)
	var maximized bool
	w.onLoop(t, func() { maximized = gtkWindowIsMaximized(w.win) != 0 })
	wait("a maximize request that does not become fullscreen", func() bool { return seen(firstID, "NSGo Frontmost First", false) })
	t.Logf("compositor honored maximize request: %v", maximized)
	w.onLoop(t, func() { gtkWindowFullscreen(w.win) })
	wait("the player's compositor fullscreen state", func() bool { return seen(firstID, "NSGo Frontmost First", true) })
	w.onLoop(t, func() { gtkWindowUnfullscreen(w.win) })
	wait("the player leaving fullscreen", func() bool { return seen(firstID, "NSGo Frontmost First", false) })
	var other uintptr
	w.onLoop(t, func() {
		other = gtkWindowNew()
		gtkWindowSetTitle(other, cstr("NSGo 1393 Second Fixture"))
		gtkWindowSetDefaultSize(other, 200, 150)
		var realize func(uintptr)
		var setID func(uintptr, *byte)
		purego.RegisterLibFunc(&realize, gtkLib, "gtk_widget_realize")
		purego.RegisterLibFunc(&setID, gtkLib, "gdk_wayland_toplevel_set_application_id")
		realize(other)
		gtkWindowPresent(other)
		setID(gtkNativeGetSurface(other), cstr(secondID))
	})
	t.Cleanup(func() {
		if other != 0 {
			w.onLoop(t, func() { gtkWindowDestroy(other) })
		}
	})
	focus := func(id string) {
		t.Helper()
		if out, err := exec.Command("swaymsg", "[app_id=\""+id+"\"] focus").CombinedOutput(); err != nil {
			t.Fatalf("focus owned %s: %v: %s", id, err, out)
		}
	}
	wait("the second fixture mapping", func() bool {
		ready := false
		w.onLoop(t, func() {
			if s := frontmostWayland; s != nil {
				for _, item := range s.windows {
					if item.app.ID == secondID {
						ready = true
					}
				}
			}
		})
		return ready
	})
	focus(secondID)
	wait("the second fixture's application identity", func() bool { return seen(secondID, "NSGo Frontmost Second", false) })
	focus(firstID)
	wait("the player regaining focus without reactivation", func() bool { return seen(firstID, "NSGo Frontmost First", false) })
	w.onLoop(t, func() { gtkWindowDestroy(other); other = 0 })
	wait("the surviving player", func() bool { return seen(firstID, "NSGo Frontmost First", false) })
	t.Log("real Wayland focus, application IDs/names, playing video fullscreen, non-fullscreen maximize request, and close transitions passed; PID is unavailable by protocol")
}

func TestWaylandFrontmostCommitsCompleteStates(t *testing.T) {
	s := &wlFrontmostServer{windows: map[uintptr]*wlFrontmostWindow{}}
	first := &wlFrontmostWindow{server: s, app: driver.FrontmostAppInfo{ID: "first"}, active: true, serial: 1}
	next := &wlFrontmostWindow{server: s, pending: driver.FrontmostAppInfo{ID: "second", FullScreen: true}, pendingActive: true}
	s.windows[1], s.windows[2] = first, next
	wlFrontmostWindows[2] = next
	defer delete(wlFrontmostWindows, 2)
	if got := s.current(); got.ID != "first" {
		t.Fatalf("uncommitted state displaced the active window: %+v", got)
	}
	s.serial = 1
	wlFrontmostDone(0, 2)
	if got := s.current(); got.ID != "second" || !got.FullScreen {
		t.Fatalf("the complete activation was not committed: %+v", got)
	}
	next.pendingActive = false
	wlFrontmostDone(0, 2)
	if got := s.current(); got.ID != "first" {
		t.Fatalf("inactive state remained selected: %+v", got)
	}
	first.active = false
	if got := s.current(); got != (driver.FrontmostAppInfo{}) {
		t.Fatalf("no activation retained an app: %+v", got)
	}
}

// An isolated X11 window manager must acknowledge maximization and fullscreen independently while a real player is running.
func TestX11FrontmostTracksPlayerState(t *testing.T) {
	if os.Getenv("NSGO_FRONTMOST_X11_NATIVE") != "1" {
		t.Skip("needs an owned X11 window manager and NSGO_FRONTMOST_X11_NATIVE=1")
	}
	const id = "dev.nsgo.frontmost1393.x11"
	for _, key := range []string{"XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME"} {
		t.Setenv(key, t.TempDir())
	}
	clip := filepath.Join(t.TempDir(), "frontmost-player.webm")
	if err := os.WriteFile(clip, videoRateClip, 0600); err != nil {
		t.Fatal(err)
	}
	var played atomic.Bool
	failures := make(chan error, 1)
	var mu sync.Mutex
	var latest core.FrontmostAppInfo
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		cx.Effect(func() { app := cx.FrontmostApp(); mu.Lock(); latest = app; mu.Unlock() })
		return core.Video{Source: clip, Muted: true, Repeat: true, Controls: true, Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
			OnProgress: func(p, _ time.Duration) {
				if p > 0 {
					played.Store(true)
				}
			},
			OnError: func(err error) {
				select {
				case failures <- err:
				default:
				}
			},
		}
	}, Options{ID: id, Title: "NSGo 1393 X11 Player", Width: 480, Height: 300})
	wait := func(what string, condition func() bool) {
		t.Helper()
		for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			select {
			case err := <-failures:
				t.Fatalf("owned X11 player: %v", err)
			default:
			}
			if condition() {
				return
			}
		}
		mu.Lock()
		last := latest
		mu.Unlock()
		t.Fatalf("the owned X11 player did not reach %s; latest=%+v", what, last)
	}
	seen := func(full bool) bool {
		mu.Lock()
		defer mu.Unlock()
		return latest.ID == id && latest.Name == id && latest.PID == os.Getpid() && latest.FullScreen == full
	}
	maximized := func() bool {
		value := false
		w.onLoop(t, func() { value = gtkWindowIsMaximized(w.win) != 0 })
		return value
	}
	wait("the active application identity", func() bool { return seen(false) })
	wait("actual media playback", played.Load)
	w.onLoop(t, func() { gtkWindowMaximize(w.win) })
	wait("actual maximized state without fullscreen", func() bool { return maximized() && seen(false) })
	w.onLoop(t, func() { gtkWindowFullscreen(w.win) })
	wait("fullscreen from maximized", func() bool { return seen(true) })
	w.onLoop(t, func() { gtkWindowUnfullscreen(w.win) })
	wait("restored maximized state", func() bool { return maximized() && seen(false) })
	w.onLoop(t, func() { gtkWindowUnmaximize(w.win) })
	wait("restored ordinary state", func() bool { return !maximized() && seen(false) })
	t.Log("actual X11 identity/PID, video playback, maximize/fullscreen/restore passed")
}
