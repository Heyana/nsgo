//go:build android && nsgotest

package android

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// The generated NSGo video-controls fixture, shared with Windows:
// ffmpeg -f lavfi -i testsrc2=size=32x24:rate=12:duration=8 -an -c:v libx264 -pix_fmt yuv420p -movflags +faststart testdata/pip.mp4
//
//go:embed testdata/pip.mp4
var pipClip []byte

// Uses the actual Activity, MediaPlayer, TextureView and PiP callbacks,
// including Android's task expansion on Stop; no callback is injected.
func TestVideoPictureInPictureNativeLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pip.mp4")
	if err := os.WriteFile(path, pipClip, 0o600); err != nil {
		t.Fatal(err)
	}
	var scope *core.Scope
	var source *core.Signal[string]
	var visible *core.Signal[bool]
	var ref core.Ref
	var state atomic.Pointer[core.PictureInPictureState]
	var startBounds atomic.Pointer[core.Bounds]
	var progress, progressCount atomic.Int64
	var pauseReason atomic.Int32
	pauseReason.Store(-1)
	var loads, starts, restores atomic.Int32
	var mediaError, pipError atomic.Pointer[string]
	surface, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		scope = cx
		source = core.NewSignal(cx, "")
		visible = core.NewSignal(cx, true)
		return core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), Background: core.RGB(0xffffff)}, Children: []core.Node{
			core.View{TestID: "start-pip", Style: core.Style{Width: core.Pct(100), Height: core.Pt(64), Background: core.RGB(0x3366ee)},
				Children: []core.Node{core.Text{Content: "Start Picture in Picture"}},
				OnLayout: func(bounds core.Bounds) { startBounds.Store(&bounds) },
				OnPress:  func(core.PressEvent) { ref.StartPictureInPicture(); ref.StartPictureInPicture() }},
			core.Show{When: visible.Get, Then: func(*core.Scope) core.Node {
				return core.Video{SourceFunc: source.Get, PictureInPicture: true, Ref: &ref, Muted: true, Repeat: true,
					Style:      core.Style{Width: core.Pt(240), Height: core.Pt(180)},
					OnLoad:     func(core.VideoInfo) { loads.Add(1) },
					OnProgress: func(at, _ time.Duration) { progress.Store(int64(at)); progressCount.Add(1) },
					OnPause:    func(reason core.VideoPauseReason) { pauseReason.Store(int32(reason)) },
					OnError:    func(err error) { text := err.Error(); mediaError.Store(&text) },
					OnPictureInPicture: func(next core.PictureInPictureState) {
						if next.Error != "" {
							text := next.Error
							pipError.Store(&text)
						}
						previous := state.Swap(&next)
						if next.Active && (previous == nil || !previous.Active) {
							starts.Add(1)
						}
					},
					OnPictureInPictureRestore: func() { restores.Add(1) },
				}
			}},
		}}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := surface.(*testSurface)
	s.Sync()
	wait := func(why string, condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for !condition() {
			if err := pipError.Load(); err != nil {
				t.Fatal(*err)
			}
			if err := mediaError.Load(); err != nil {
				t.Fatal(*err)
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: state %+v, loads %d, starts %d, restores %d, pause reason %d", why, state.Load(), loads.Load(), starts.Load(), restores.Load(), pauseReason.Load())
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	wait("initial PiP support report", func() bool { return state.Load() != nil })
	if !state.Load().Supported {
		t.Skip("the Activity or device does not support native video PiP")
	}
	if state.Load().Possible {
		t.Fatal("an empty video was ready for PiP")
	}
	wait("start control laid out", func() bool { return startBounds.Load() != nil })
	pressStart := func() {
		t.Helper()
		bounds := startBounds.Load()
		if err := s.Press(driver.Point{X: bounds.Origin.X + bounds.Size.Width/2, Y: bounds.Origin.Y + bounds.Size.Height/2}, 0, driver.Modifiers{}); err != nil {
			t.Fatal(err)
		}
	}
	pressStart()
	wait("start before readiness fails explicitly", func() bool { return state.Load().Error != "" })
	pipError.Store(nil)
	scope.Post(func() { source.Set(path) })
	wait("prepared video becomes possible", func() bool { return state.Load().Possible && state.Load().Error == "" && loads.Load() == 1 })
	pressStart()
	wait("Activity enters PiP", func() bool { return state.Load().Active })
	if starts.Load() != 1 {
		t.Fatalf("duplicate start entered %d sessions", starts.Load())
	}
	before, beforeCount := progress.Load(), progressCount.Load()
	wait("video keeps progressing in PiP", func() bool { return progress.Load() != before && progressCount.Load() >= beforeCount+3 })
	frame := func() image.Image {
		t.Helper()
		data, err := s.Screenshot()
		if err != nil {
			t.Fatal(err)
		}
		image, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		return image
	}
	var first image.Image
	wait("PiP window shows video rather than the inline interface", func() bool {
		candidate := frame()
		bounds := candidate.Bounds()
		ratio := float64(bounds.Dx()) / float64(bounds.Dy())
		if ratio < 1.2 || ratio > 1.5 {
			return false
		}
		red, blue := 0, 0
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, _ := candidate.At(x, y).RGBA()
				if r > g*2+10000 && r > b*2+10000 {
					red++
				}
				if b > g*2+10000 && b > r*2+10000 {
					blue++
				}
			}
		}
		if red < bounds.Dx()*bounds.Dy()/25 || blue < bounds.Dx()*bounds.Dy()/25 {
			return false
		}
		first = candidate
		return true
	})
	fingerprint := func(frame image.Image) [32]byte {
		bounds := frame.Bounds()
		h := sha256.New()
		for y := bounds.Min.Y + bounds.Dy()/10; y < bounds.Max.Y-bounds.Dy()/10; y++ {
			for x := bounds.Min.X + bounds.Dx()/10; x < bounds.Max.X-bounds.Dx()/10; x++ {
				r, g, b, _ := frame.At(x, y).RGBA()
				h.Write([]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8)})
			}
		}
		var sum [32]byte
		copy(sum[:], h.Sum(nil))
		return sum
	}
	firstHash := fingerprint(first)
	wait("PiP presents successive decoded video frames", func() bool {
		next := frame()
		return next.Bounds() == first.Bounds() && fingerprint(next) != firstHash
	})
	scope.Post(func() { ref.StopPictureInPicture(); ref.StopPictureInPicture() })
	wait("Activity expands and restores its inline video", func() bool { return !state.Load().Active && state.Load().Possible && restores.Load() == 1 })
	if loads.Load() != 1 {
		t.Fatalf("restoration reopened the player %d times", loads.Load())
	}
	before, beforeCount = progress.Load(), progressCount.Load()
	wait("restored video keeps progressing", func() bool { return progress.Load() != before && progressCount.Load() >= beforeCount+3 })
	pressStart()
	wait("second session enters PiP", func() bool { return state.Load().Active && starts.Load() == 2 })
	scope.Post(func() { visible.Set(false) })
	s.Sync()
	// Deleting the owner must not restore a page that no longer owns it.
	time.Sleep(500 * time.Millisecond)
	if restores.Load() != 1 {
		t.Fatalf("deleted PiP owner restored %d times", restores.Load())
	}
}
