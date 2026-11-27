//go:build darwin && !ios && nsgotest

package macos

import (
	"bytes"
	"image/png"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestBitmapRepIsAllZero(t *testing.T) {
	onMain(func() {
		loadAppKit()
		rep := objc.ID(apple.Class("NSBitmapImageRep")).Send(selAlloc).Send(
			sel("initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:hasAlpha:isPlanar:colorSpaceName:bytesPerRow:bitsPerPixel:"),
			uintptr(0), 4, 4, 8, 4, true, false, apple.NSString("NSCalibratedRGBColorSpace"), 16, 32,
		)
		if rep == 0 {
			t.Fatal("failed to allocate NSBitmapImageRep")
		}
		defer rep.Send(selRelease)
		ptr := objc.Send[*byte](rep, sel("bitmapData"))
		buf := unsafe.Slice(ptr, 4*16)
		clear(buf)
		if !bitmapRepIsAllZero(rep) {
			t.Fatal("zeroed alpha bitmap reported non-zero")
		}
		// Pure opaque black (RGBA = 0,0,0,255) must not be treated as empty.
		buf[3] = 0xff
		if bitmapRepIsAllZero(rep) {
			t.Fatal("opaque black pixel reported all-zero")
		}
	})
}

func TestHiddenWindowSkipsCompositeAndUsesFallback(t *testing.T) {
	onMain(func() {
		loadAppKit()
		window := objc.ID(classWindow).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(0, 0, 120, 80), uint(styleTitled), uint(2), false)
		window.Send(sel("setReleasedWhenClosed:"), false)
		s := newSurface(Options{Width: 120, Height: 80}, false)
		s.window = window
		window.Send(sel("setContentView:"), s.native)
		defer func() {
			s.close()
			window.Send(sel("close"))
			window.Send(selRelease)
		}()

		// An ordered-out window is not visible, so WindowServer cannot composite it.
		ts := &testSurface{s: s, own: s, fixed: true}
		bounds := objc.Send[apple.CGRect](s.native, selBounds)
		if data, retry := ts.compositedAttempt(bounds); data != nil || retry {
			t.Fatalf("hidden window compositedAttempt = (len=%d, retry=%v), want (nil, false)", len(data), retry)
		}
		fallbackPNG := ts.fallbackScreenshot()
		cfg, err := png.DecodeConfig(bytes.NewReader(fallbackPNG))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
			t.Fatalf("fallback PNG invalid: cfg=%+v err=%v", cfg, err)
		}
	})
}
