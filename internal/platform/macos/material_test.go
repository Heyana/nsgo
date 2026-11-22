//go:build darwin && !ios

package macos

import (
	"image/color"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestMaterialCode(t *testing.T) {
	cases := map[driver.WindowMaterial]int{
		driver.WindowMaterialWindow:       12,
		driver.WindowMaterialContent:      18,
		driver.WindowMaterialSidebar:      7,
		driver.WindowMaterialHeader:       10,
		driver.WindowMaterialTitlebar:     3,
		driver.WindowMaterialUnderWindow:  21,
		driver.WindowMaterialUnderPage:    22,
		driver.WindowMaterialMenu:         5,
		driver.WindowMaterialPopover:      6,
		driver.WindowMaterialHUD:          13,
		driver.WindowMaterialSheet:        11,
		driver.WindowMaterialTooltip:      17,
		driver.WindowMaterialSelection:    4,
		driver.WindowMaterialFullScreenUI: 15,
		driver.WindowMaterialMica:         21,
		driver.WindowMaterialAcrylic:      13,
		driver.WindowMaterialTabbed:       21,
	}
	for m, want := range cases {
		if got, ok := materialCode(m); !ok || got != want {
			t.Errorf("materialCode(%d) = %d, %v; want %d, true", m, got, ok, want)
		}
	}
	for _, m := range []driver.WindowMaterial{driver.WindowMaterialNone, driver.WindowMaterial(100)} {
		if code, ok := materialCode(m); ok {
			t.Errorf("materialCode(%d) = %d, true; want no material", m, code)
		}
	}
}

func TestViewsClipAsTheirStyleSays(t *testing.T) {
	if clipsBounds(nil, 1) {
		t.Fatal("a view not yet registered clips, masking its layer for good")
	}
	plain := &view{native: 1, content: 1}
	if clipsBounds(plain, 1) {
		t.Fatal("a view whose style does not clip clips")
	}
	plain.style.Clip = true
	if !clipsBounds(plain, 1) {
		t.Fatal("a view whose style clips does not")
	}
	// A holder clips the children of a view with a shadow, whose own layer would clip the shadow.
	plain.content, plain.clipHeld = 2, true
	if clipsBounds(plain, 1) {
		t.Fatal("a view whose holder clips its children clips its own shadow")
	}
	if !clipsBounds(plain, 2) {
		t.Fatal("the holder of a view that clips does not")
	}
}

// TestApplyAfterClose ignores the apply message of a surface closed while
// it waited on the main thread.
func TestApplyAfterClose(t *testing.T) {
	surfaceApply(0, 0, 0)
}

// A view clips what it holds without its own layer only where that layer would clip its shadow or outline with it, and only where a holder or its image layer clips all it holds.
func TestViewsClipInAHolderToKeepTheirShadow(t *testing.T) {
	shadow := &driver.Effects{Shadows: []driver.BoxShadow{{Blur: 24, Color: driver.RGBA(0, 0x59)}}}
	card := driver.ViewStyle{Clip: true, CornerRadius: 16, Effects: shadow}
	shaped := card
	shaped.Corners = driver.Corners{TopLeft: 16, TopRight: 4, BottomRight: 4, BottomLeft: 4}
	for _, c := range []struct {
		name string
		v    *view
		vs   driver.ViewStyle
		want bool
	}{
		{"a card", &view{kind: driver.KindView}, card, true},
		{"a card of corners that differ", &view{kind: driver.KindView}, shaped, true},
		{"a card that does not clip", &view{kind: driver.KindView}, driver.ViewStyle{CornerRadius: 16, Effects: shadow}, false},
		{"a clip with no shadow", &view{kind: driver.KindView}, driver.ViewStyle{Clip: true, CornerRadius: 16}, false},
		{"a text", &view{kind: driver.KindText}, card, true},
		{"a canvas", &view{kind: driver.KindCanvas}, card, true},
		{"a shader", &view{kind: driver.KindShader}, card, true},
		{"an input", &view{kind: driver.KindInput}, shaped, true},
		{"a web view", &view{kind: driver.KindWeb}, shaped, true},
		{"a video", &view{kind: driver.KindVideo}, shaped, true},
		{"a native component", &view{kind: driver.KindNative}, shaped, true},
		{"an image, which clips to one radius itself", &view{kind: driver.KindImage}, card, true},
		{"an image of corners that differ", &view{kind: driver.KindImage}, shaped, true},
		{"a scroll view", &view{kind: driver.KindScroll, sc: &scrollState{}}, card, true},
		{"a scroll view of corners that differ", &view{kind: driver.KindScroll, sc: &scrollState{}}, shaped, true},
	} {
		if got := c.v.clipsInHolder(&c.vs); got != c.want {
			t.Errorf("%s: clipsInHolder = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestABareBackdropBlurFollowsItsRadius draws a blur with no material named as CSS does: a Gaussian blur of the style's radius, which the pointer goes through, with the view's background drawn once, over it, and not again in the view's own layer under the blur. A named material keeps AppKit's.
func TestABareBackdropBlurFollowsItsRadius(t *testing.T) {
	onMain(func() { aBareBackdropBlurFollowsItsRadius(t) })
}

func aBareBackdropBlurFollowsItsRadius(t *testing.T) {
	loadAppKit()
	native := objc.ID(classView).Send(selNew)
	defer native.Send(selRelease)
	native.Send(selSetWantsLayer, true)
	native.Send(selSetFrame, apple.Rect(0, 0, 200, 120))
	v := &view{kind: driver.KindView, native: native, content: native, surface: &surface{}}
	views[native] = v
	defer delete(views, native)
	defer v.setBackdrop(&driver.ViewStyle{})

	apply := func(blur float32, m driver.BackdropMaterial) {
		vs := driver.ViewStyle{Background: driver.RGBA(0, 0.4), CornerRadius: 12, Material: m,
			Effects: &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBlur, Amount: blur}}}}
		v.style = vs
		v.setBackdrop(&vs)
		v.styleLayer(&vs)
	}
	isA := func(o objc.ID, class objc.Class) bool {
		return o != 0 && objc.Send[bool](o, sel("isKindOfClass:"), class)
	}
	radius := func() float64 {
		filters := v.backdropLayer.Send(sel("filters"))
		if apple.Msg(filters, selCount) != 1 {
			return -1
		}
		f := filters.Send(selObjectAtIndex, uint(0))
		return objc.Send[float64](f.Send(sel("valueForKey:"), apple.NSString("inputRadius")), sel("doubleValue"))
	}
	ownBackground := func() bool { return native.Send(selLayer).Send(sel("backgroundColor")) != 0 }

	apply(6, driver.BackdropUltraThin)
	if !isA(v.backdrop, classOverlay) || !isA(v.backdropLayer, apple.Class("CABackdropLayer")) {
		t.Fatalf("a bare blur's backdrop is %v holding %v, want a holder the pointer goes through holding a CABackdropLayer", v.backdrop, v.backdropLayer)
	}
	if got := radius(); got != 6 {
		t.Fatalf("a blur of 6 blurs by %v", got)
	}
	if got := v.backdrop.Send(sel("hitTest:"), apple.CGPoint{X: 100, Y: 60}); got != 0 {
		t.Fatalf("the blur took the pointer: %v", got)
	}
	if ownBackground() {
		t.Fatal("the view's own layer draws its background under the blur, which blurs it and shows it twice")
	}
	if v.tint == 0 || v.tint.Send(selLayer).Send(sel("backgroundColor")) == 0 {
		t.Fatal("the background is not drawn over the blur")
	}
	if got, want := objc.Send[apple.CGRect](v.backdropLayer, selFrame), objc.Send[apple.CGRect](v.backdrop, selBounds); got != want {
		t.Fatalf("the blur's layer is at %v, want the holder's bounds %v", got, want)
	}

	held := v.backdrop
	apply(30, driver.BackdropUltraThin)
	if v.backdrop != held {
		t.Fatal("a new radius made a new backdrop")
	}
	if got := radius(); got != 30 {
		t.Fatalf("a blur of 30 blurs by %v", got)
	}

	apply(30, driver.BackdropThin)
	if !isA(v.backdrop, classMaterial) || v.backdropLayer != 0 {
		t.Fatalf("a named material's backdrop is %v, want AppKit's material", v.backdrop)
	}
	if !ownBackground() {
		t.Fatal("the view's own layer gave up its background under a material, which hides it")
	}

	apply(8, driver.BackdropUltraThin)
	if !isA(v.backdrop, classOverlay) || radius() != 8 || ownBackground() {
		t.Fatal("going back from a material does not blur by the radius again")
	}
	apply(0, driver.BackdropUltraThin)
	if v.backdrop != 0 || v.backdropLayer != 0 || v.tint != 0 || !ownBackground() {
		t.Fatal("taking the blur away left its backdrop or kept the background off the view's layer")
	}
}

// TestBackdropColorFiltersChangeWhatIsBehind puts a backdrop with filters over the right half of a red window and reads the window as WindowServer composites it, as CSS's backdrop-filter draws them: a brightness of a half darkens the red behind with no blur, an invert turns it cyan, a blur with saturate(0) greys it, and taking the filters away leaves the red. A named material does not stand in for color filters without a blur.
func TestBackdropColorFiltersChangeWhatIsBehind(t *testing.T) {
	onMain(func() { backdropColorFiltersChangeWhatIsBehind(t) })
}

func backdropColorFiltersChangeWhatIsBehind(t *testing.T) {
	loadAppKit()
	pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
	defer pool.Send(sel("drain"))
	window := objc.ID(classWindow).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"), apple.Rect(200, 200, 200, 100), uint(0), uint(2), false)
	window.Send(sel("setReleasedWhenClosed:"), false)
	defer func() {
		window.Send(sel("close"))
		window.Send(selRelease)
	}()
	root := window.Send(sel("contentView"))
	root.Send(selSetWantsLayer, true)
	red := apple.CGColor(driver.RGB(0xff0000))
	defer apple.Release(red)
	root.Send(selLayer).Send(sel("setBackgroundColor:"), red)
	native := objc.ID(classView).Send(selNew)
	native.Send(selSetWantsLayer, true)
	native.Send(selSetFrame, apple.Rect(100, 0, 100, 100))
	root.Send(selAddSubview, native)
	native.Send(selRelease)
	v := &view{kind: driver.KindView, native: native, content: native, surface: &surface{window: window}}
	views[native] = v
	defer delete(views, native)
	defer v.setBackdrop(&driver.ViewStyle{})
	window.Send(sel("orderFrontRegardless"))

	// shown applies filters and returns the colors composited behind the plain left half and the filtered right half, ok false when WindowServer gives no picture of the window.
	shown := func(m driver.BackdropMaterial, filters ...driver.Filter) (left, right color.RGBA, ok bool) {
		vs := driver.ViewStyle{Opacity: 1, Material: m, Effects: &driver.Effects{BackdropFilters: filters}}
		v.style = vs
		v.setBackdrop(&vs)
		v.styleLayer(&vs)
		for range 20 {
			objc.ID(apple.Class("NSRunLoop")).Send(sel("mainRunLoop")).Send(sel("runUntilDate:"), objc.ID(apple.Class("NSDate")).Send(sel("dateWithTimeIntervalSinceNow:"), 0.05))
		}
		img := captureView(root)
		if img == nil {
			return left, right, false
		}
		b := img.Bounds()
		at := func(fx float64) color.RGBA {
			return color.RGBAModel.Convert(img.At(b.Min.X+int(fx*float64(b.Dx())), b.Min.Y+b.Dy()/2)).(color.RGBA)
		}
		return at(0.25), at(0.75), true
	}

	left, right, ok := shown(driver.BackdropUltraThin, driver.Filter{Kind: driver.FilterBrightness, Amount: 0.5})
	if !ok {
		t.Skip("WindowServer gives no picture of the window here, so the composited pixels are not checked")
	}
	if v.backdropLayer == 0 || int(right.R) > int(left.R)*3/4 || right.R < 40 {
		t.Errorf("brightness(0.5) over red shows %v beside the red %v, want a darker red", right, left)
	}
	if _, right, _ := shown(driver.BackdropUltraThin, driver.Filter{Kind: driver.FilterInvert, Amount: 1}); right.R > 100 || right.G < 150 || right.B < 150 {
		t.Errorf("invert(1) over red shows %v, want cyan", right)
	}
	if _, right, _ := shown(driver.BackdropUltraThin, driver.Filter{Kind: driver.FilterBlur, Amount: 8}, driver.Filter{Kind: driver.FilterSaturate, Amount: 0}); absDiff(right.R, right.G) > 30 || absDiff(right.G, right.B) > 30 {
		t.Errorf("blur(8) saturate(0) over red shows %v, want a grey", right)
	}
	if _, right, _ := shown(driver.BackdropThin, driver.Filter{Kind: driver.FilterBrightness, Amount: 0.5}); !objc.Send[bool](v.backdrop, sel("isKindOfClass:"), classOverlay) || int(right.R) > int(left.R)*3/4 {
		t.Errorf("brightness(0.5) with a named material shows %v in %v, want the darker red of a CABackdropLayer", right, v.backdrop)
	}
	if left, right, _ := shown(driver.BackdropUltraThin); v.backdrop != 0 || absDiff(left.R, right.R) > 10 || absDiff(left.G, right.G) > 10 {
		t.Errorf("with no backdrop filters the right half shows %v beside %v, want the same red (backdrop %v)", right, left, v.backdrop)
	}
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
