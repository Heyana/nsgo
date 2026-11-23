//go:build darwin && !ios

package macos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	alphaclips "github.com/nsgo-dev/nsgo/internal/platform/videoalpha/clips"
)

// A window that takes no pointer ignores AppKit's mouse events, and one that takes it where it draws finds a painted view under a point and nothing beside it; switching back gives it the pointer again. Run owns AppKit's process lifetime, so this test gets a process of its own.
func TestWindowPointerEventsFollowTheMode(t *testing.T) {
	if os.Getenv("NSGO_TEST_POINTER_PROCESS") != t.Name() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_POINTER_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	const title = "nsgo-window-pointer"
	pixels := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		a := uint8(255)
		if y >= 2 {
			a = 128
		}
		for x := 2; x < 4; x++ {
			pixels.SetNRGBA(x, y, color.NRGBA{R: 255, A: a})
		}
	}
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, pixels); err != nil {
		t.Fatal(err)
	}
	var loaded atomic.Int32
	done := make(chan error, 1)
	go func() {
		// main finds the window and its pet once laid out.
		main := func() (s *surface, pet, alpha, defaultImage *view) {
			for s := range surfaces {
				if s.window == 0 || apple.GoString(s.window.Send(sel("title"))) != title {
					continue
				}
				for _, v := range s.views {
					if v == nil || !v.framed {
						continue
					}
					switch v.props.TestID {
					case "pet":
						pet = v
					case "alpha":
						alpha = v
					case "default":
						defaultImage = v
					}
				}
				if pet != nil && alpha != nil && defaultImage != nil && loaded.Load() >= 2 {
					return s, pet, alpha, defaultImage
				}
			}
			return nil, nil, nil, nil
		}
		check := func() error {
			deadline := time.Now().Add(5 * time.Second)
			for {
				var ready bool
				apple.OnMain(func() { s, _, _, _ := main(); ready = s != nil })
				barrier := make(chan struct{})
				apple.OnMain(func() { close(barrier) })
				<-barrier
				if ready {
					break
				}
				if time.Now().After(deadline) {
					return errors.New("the window never laid out the pet and loaded its pictures")
				}
				time.Sleep(10 * time.Millisecond)
			}
			var err error
			barrier := make(chan struct{})
			apple.OnMain(func() {
				defer close(barrier)
				s, pet, alpha, defaultImage := main()
				ignores := func() bool { return objc.Send[bool](s.window, sel("ignoresMouseEvents")) }
				s.setPointerEvents(driver.WindowPointerNone)
				if !ignores() {
					err = errors.New("a window that takes no pointer takes AppKit's mouse events")
					return
				}
				s.setPointerEvents(driver.WindowPointerPainted)
				if s.pointer.timer == 0 {
					err = errors.New("a painted window does not follow the pointer")
					return
				}
				toWindow := sel("convertPoint:toView:")
				onPet := objc.Send[apple.CGPoint](pet.native, toWindow, apple.CGPoint{X: 100, Y: 100}, objc.ID(0))
				beside := objc.Send[apple.CGPoint](s.native, toWindow, apple.CGPoint{X: 500, Y: 300}, objc.ID(0))
				if !s.paintedHit(s.hitAt(onPet)) {
					err = errors.New("a painted window finds nothing painted over its pet")
					return
				}
				if s.paintedHit(s.hitAt(beside)) {
					err = errors.New("a painted window finds something painted where nothing draws")
					return
				}
				at := func(v *view, x, y float64) apple.CGPoint {
					return objc.Send[apple.CGPoint](v.native, toWindow, apple.CGPoint{X: x, Y: y}, objc.ID(0))
				}
				transparent := at(alpha, 25, 25)
				if s.paintedHit(s.hitAt(transparent)) || s.paintedHit(s.hitAt(at(alpha, 75, 75))) {
					err = errors.New("a painted window took the pointer below the image threshold")
					return
				}
				if !s.paintedHit(s.hitAt(at(alpha, 75, 25))) || !s.paintedHit(s.hitAt(at(defaultImage, 25, 25))) {
					err = errors.New("a painted window let the pointer through an opaque or default image")
					return
				}
				s.setPointerEvents(driver.WindowPointerAuto)
				if s.hitAt(transparent) != alpha.native {
					err = errors.New("an ordinary window filtered the image's alpha")
					return
				}
				if ignores() || s.pointer.timer != 0 {
					err = errors.New("a window given the pointer back still lets it through")
				}
			})
			<-barrier
			return err
		}
		done <- check()
		apple.OnMain(finish)
	}()
	onMain(func() {
		Run(core.AppRoot(func(cx *core.Scope) {
			cx.SetMainView(func(cx *core.Scope) core.Node {
				desktop.SetTitle(cx, title)
				picture := core.Image{Source: core.ImageSource{Data: pngData.Bytes()}, TestID: "default",
					Style:  core.Style{Width: core.Pt(100), Height: core.Pt(100)},
					OnLoad: func(core.Size) { loaded.Add(1) }, OnPress: func(core.PressEvent) {}}
				alpha := picture
				alpha.TestID, alpha.HitTestAlpha = "alpha", 0.75
				return core.View{Style: core.Style{Width: core.Pt(600), Height: core.Pt(400)}, Children: []core.Node{
					core.View{TestID: "pet", Style: core.Style{Width: core.Pt(200), Height: core.Pt(200), Background: core.RGB(0xcc6600)}},
					alpha, picture,
				}}
			})
		}), Options{Title: title, Width: 600, Height: 400, Transparent: true})
	})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// TestWindowPointerUsesVideoAlpha checks the same AppKit hit path as the
// painted-window pointer timer, using an AVPlayer-decoded HEVC alpha frame.
func TestWindowPointerUsesVideoAlpha(t *testing.T) {
	if os.Getenv("NSGO_TEST_POINTER_PROCESS") != t.Name() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_POINTER_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	clip := filepath.Join(t.TempDir(), "alpha.mov")
	if err := os.WriteFile(clip, alphaclips.HEVCAlpha, 0o600); err != nil {
		t.Fatal(err)
	}
	const title = "nsgo-video-pointer-alpha"
	var loaded atomic.Int32
	done := make(chan error, 1)
	go func() {
		for deadline := time.Now().Add(8 * time.Second); ; time.Sleep(15 * time.Millisecond) {
			var checked, transparent, letterbox, opaque, unfiltered bool
			barrier := make(chan struct{})
			apple.OnMain(func() {
				defer close(barrier)
				if loaded.Load() < 2 {
					return
				}
				var alpha, defaultVideo *view
				for _, v := range views {
					switch v.props.TestID {
					case "alpha-video":
						alpha = v
					case "default-video":
						defaultVideo = v
					}
				}
				if alpha == nil || defaultVideo == nil {
					return
				}
				s := alpha.surface
				if s.pointer.mode != driver.WindowPointerPainted {
					return
				}
				point := func(v *view, x, y float64) apple.CGPoint {
					return objc.Send[apple.CGPoint](v.native, sel("convertPoint:toView:"),
						apple.CGPoint{X: x, Y: y}, objc.ID(0))
				}
				transparent = s.paintedHit(s.hitAt(point(alpha, 36, 12)))
				letterbox = s.paintedHit(s.hitAt(point(alpha, 12, 48)))
				// The clip's opaque square is in its middle, not at the top edge.
				opaque = s.paintedHit(s.hitAt(point(alpha, 72, 48)))
				unfiltered = s.paintedHit(s.hitAt(point(defaultVideo, 36, 12)))
				checked = true
			})
			<-barrier
			if checked && !transparent && !letterbox && opaque && unfiltered {
				done <- nil
				break
			}
			if time.Now().After(deadline) {
				done <- fmt.Errorf("video hit test after %d loads: checked=%t transparent=%t letterbox=%t opaque=%t opt-out=%t",
					loaded.Load(), checked, transparent, letterbox, opaque, unfiltered)
				break
			}
		}
		apple.OnMain(finish)
	}()

	onMain(func() {
		Run(core.AppRoot(func(cx *core.Scope) {
			cx.SetMainView(func(cx *core.Scope) core.Node {
				desktop.SetTitle(cx, title)
				desktop.SetWindowPointerEvents(cx, desktop.WindowPointerPainted)
				video := core.Video{
					Source: clip, TestID: "default-video", Paused: true, Muted: true,
					Style:   core.Style{Width: core.Pt(144), Height: core.Pt(96)},
					OnLoad:  func(core.VideoInfo) { loaded.Add(1) },
					OnError: func(err error) { t.Error(err) },
					OnPress: func(core.PressEvent) {},
				}
				alpha := video
				alpha.TestID, alpha.HitTestAlpha = "alpha-video", 0.5
				return core.View{Style: core.Style{Width: core.Pt(360), Height: core.Pt(120), Direction: core.Row},
					Children: []core.Node{alpha, video}}
			})
		}), Options{Title: title, Width: 360, Height: 120, Transparent: true})
	})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
