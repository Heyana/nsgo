//go:build darwin && !ios

package apple

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/videoalpha/clips"
)

// TestVideoAlphaLayerAndDecodedPixels verifies that AVPlayerLayer is non-opaque with no background color and that AVFoundation decodes HEVC with alpha preserving transparent corners and an opaque center.
func TestVideoAlphaLayerAndDecodedPixels(t *testing.T) {
	hevc := filepath.Join(t.TempDir(), "alpha.mov")
	if err := os.WriteFile(hevc, clips.HEVCAlpha, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(holdMainLoop())

	var mu sync.Mutex
	var loaded driver.Size
	var loadErr string
	var video *Video
	var holder objc.ID
	var layerOpaque bool
	var layerBG objc.ID

	onMain(func() {
		loadAVFoundation()
		holder = objc.ID(Class("NSView")).Send(sel("alloc")).Send(sel("initWithFrame:"), Rect(0, 0, 64, 64))
		holder.Send(sel("setWantsLayer:"), true)
		video = NewVideo(holder)
		video.Dispatch = func(ev driver.Event) {
			mu.Lock()
			defer mu.Unlock()
			switch ev.Kind {
			case driver.EventLoad:
				loaded = ev.Size
			case driver.EventError:
				loadErr = ev.Text
			}
		}
		video.Configure(&driver.VideoConfig{
			Source:           hevc,
			Muted:            true,
			Repeat:           true,
			Rate:             1,
			ResizeMode:       driver.Contain,
			ProgressInterval: 30 * time.Millisecond,
		})
		if video.layer != 0 {
			layerOpaque = objc.Send[bool](video.layer, sel("isOpaque"))
			layerBG = video.layer.Send(sel("backgroundColor"))
		}
	})
	defer onMain(func() {
		video.Close()
		holder.Send(sel("release"))
	})

	if video.layer == 0 {
		t.Fatal("expected an AVPlayerLayer for uncontrolled video")
	}
	if layerOpaque {
		t.Error("AVPlayerLayer.isOpaque = true, want false")
	}
	if layerBG != 0 {
		t.Errorf("AVPlayerLayer.backgroundColor = %#x, want 0 (clear)", layerBG)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		sz, errText := loaded, loadErr
		mu.Unlock()
		if errText != "" {
			t.Fatalf("HEVC alpha failed to load: %s", errText)
		}
		if sz.Width > 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	mu.Lock()
	gotSize := loaded
	mu.Unlock()
	if gotSize != (driver.Size{Width: 64, Height: 64}) {
		t.Fatalf("HEVC alpha loaded size = %v, want 64x64", gotSize)
	}

	var sampled bool
	var width, height int
	var corner1, corner2, center byte
	for sampleDeadline := time.Now().Add(5 * time.Second); time.Now().Before(sampleDeadline); time.Sleep(25 * time.Millisecond) {
		onMain(func() {
			if video.SampleHitTestAlpha() && len(video.hitTest.pixels) > 0 {
				sampled = true
				width, height = video.hitTest.width, video.hitTest.height
				corner1 = video.hitTest.pixels[4*width+4]
				corner2 = video.hitTest.pixels[(height-5)*width+(width-5)]
				center = video.hitTest.pixels[(height/2)*width+(width/2)]
			}
		})
		if sampled {
			break
		}
	}
	if !sampled {
		t.Fatal("did not decode a BGRA frame from HEVC with alpha")
	}
	if width != 64 || height != 64 {
		t.Fatalf("decoded frame size = %dx%d, want 64x64", width, height)
	}
	if corner1 > 10 || corner2 > 10 || center < 245 {
		t.Fatalf("decoded alpha corners=(%d, %d) center=%d, want transparent corners (<=10) and opaque center (>=245)", corner1, corner2, center)
	}
}

// TestVideoRefusesWebMAlpha verifies that local VP8 and VP9 WebM files declaring alpha are refused with VideoErrorAlphaUnsupported before AVPlayer opens them.
func TestVideoRefusesWebMAlpha(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		data []byte
	}{
		{"vp8a.webm", clips.VP8Alpha},
		{"vp9a.webm", clips.VP9Alpha},
	}
	t.Cleanup(holdMainLoop())

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name)
			if err := os.WriteFile(path, tc.data, 0o600); err != nil {
				t.Fatal(err)
			}
			var got driver.Event
			var player objc.ID
			onMain(func() {
				loadAVFoundation()
				holder := objc.ID(Class("NSView")).Send(sel("alloc")).Send(sel("initWithFrame:"), Rect(0, 0, 64, 64))
				defer holder.Send(sel("release"))
				video := NewVideo(holder)
				defer video.Close()
				video.Dispatch = func(ev driver.Event) { got = ev }
				video.Configure(&driver.VideoConfig{Source: path, Muted: true, Rate: 1, ProgressInterval: 100 * time.Millisecond})
				player = video.player
			})
			if got.Kind != driver.EventError || got.Index != driver.VideoErrorAlphaUnsupported {
				t.Fatalf("got event %+v, want EventError with VideoErrorAlphaUnsupported", got)
			}
			if player != 0 {
				t.Fatal("AVPlayer was created for a refused WebM alpha clip")
			}
		})
	}
}
