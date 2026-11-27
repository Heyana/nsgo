//go:build ios && nsgotest

package ios

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	alphaclips "github.com/nsgo-dev/nsgo/internal/platform/videoalpha/clips"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestIOSVideoAlphaLayerAndRefusesWebM verifies that HEVC with alpha loads on a non-opaque AVPlayerLayer with clear background on iOS, that Controls uses a non-opaque AVPlayerViewController view, and that local WebM with alpha reports MediaErrorAlphaUnsupported.
func TestIOSVideoAlphaLayerAndRefusesWebM(t *testing.T) {
	dir := t.TempDir()
	hevc, webm := filepath.Join(dir, "alpha.mov"), filepath.Join(dir, "alpha.webm")
	for path, data := range map[string][]byte{hevc: alphaclips.HEVCAlpha, webm: alphaclips.VP9Alpha} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var plainLoaded, controlsLoaded atomic.Pointer[core.VideoInfo]
	var webmReason atomic.Int32
	var webmErr atomic.Pointer[string]
	var unexpectedErr atomic.Pointer[string]

	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		box := core.Style{Width: core.Pt(64), Height: core.Pt(64)}
		return core.View{Style: core.Style{Direction: core.Row, Background: core.RGB(0xff0000)}, Children: []core.Node{
			core.Video{Source: hevc, Muted: true, Repeat: true, Style: box,
				OnLoad:  func(info core.VideoInfo) { plainLoaded.Store(&info) },
				OnError: func(err error) { s := err.Error(); unexpectedErr.Store(&s) }},
			core.Video{Source: hevc, Muted: true, Repeat: true, Controls: true, Style: box,
				OnLoad:  func(info core.VideoInfo) { controlsLoaded.Store(&info) },
				OnError: func(err error) { s := err.Error(); unexpectedErr.Store(&s) }},
			core.Video{Source: webm, Muted: true, Style: box,
				OnLoad: func(core.VideoInfo) { s := "WebM unexpectedly loaded"; unexpectedErr.Store(&s) },
				OnError: func(err error) {
					if me, ok := err.(*core.MediaError); ok {
						webmReason.Store(int32(me.Reason))
					}
					s := err.Error()
					webmErr.Store(&s)
				}},
		}}
	})

	app.WaitUntil(10*time.Second, func() bool {
		return unexpectedErr.Load() != nil || (plainLoaded.Load() != nil && controlsLoaded.Load() != nil && webmErr.Load() != nil)
	})
	if s := unexpectedErr.Load(); s != nil {
		t.Fatal(*s)
	}
	if info := plainLoaded.Load(); info == nil || info.Size != (core.Size{Width: 64, Height: 64}) {
		t.Fatalf("plain HEVC alpha = %+v, want 64x64", info)
	}
	if info := controlsLoaded.Load(); info == nil || info.Size != (core.Size{Width: 64, Height: 64}) {
		t.Fatalf("controlled HEVC alpha = %+v, want 64x64", info)
	}
	if core.MediaErrorReason(webmReason.Load()) != core.MediaErrorAlphaUnsupported {
		t.Fatalf("WebM alpha reason = %v (%q), want alpha-unsupported", core.MediaErrorReason(webmReason.Load()), *webmErr.Load())
	}

	var foundLayer, layerOpaque bool
	var layerBG objc.ID
	var foundControllerView, controllerOpaque bool
	onMainSync(func() {
		avPlayerLayer := objc.ID(apple.Class("AVPlayerLayer"))
		for _, v := range runner.views {
			if v == nil || v.video == nil {
				continue
			}
			if layers := v.native.Send(sel("layer")).Send(sel("sublayers")); layers != 0 {
				for i := 0; i < objc.Send[int](layers, sel("count")); i++ {
					layer := layers.Send(sel("objectAtIndex:"), i)
					if objc.Send[bool](layer, sel("isKindOfClass:"), avPlayerLayer) {
						foundLayer = true
						layerOpaque = objc.Send[bool](layer, sel("isOpaque"))
						layerBG = layer.Send(sel("backgroundColor"))
					}
				}
			}
			if subviews := v.native.Send(sel("subviews")); subviews != 0 && objc.Send[int](subviews, sel("count")) > 0 {
				sub := subviews.Send(sel("objectAtIndex:"), 0)
				foundControllerView = true
				controllerOpaque = objc.Send[bool](sub, sel("isOpaque"))
			}
		}
	})
	if !foundLayer {
		t.Fatal("did not find AVPlayerLayer on iOS video")
	}
	if layerOpaque || layerBG != 0 {
		t.Fatalf("AVPlayerLayer opaque=%v bg=%#x, want false and 0", layerOpaque, layerBG)
	}
	if !foundControllerView {
		t.Fatal("did not find AVPlayerViewController subview for Controls video")
	}
	if controllerOpaque {
		t.Fatal("AVPlayerViewController view isOpaque = true, want false")
	}
}
