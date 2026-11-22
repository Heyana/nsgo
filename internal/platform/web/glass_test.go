//go:build js && wasm

package web

import (
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func glassTestSurface(t *testing.T, w, h driver.Points, ratio float32) *surface {
	t.Helper()
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("run with nsgo test -target web")
	}
	if doc.Call("getElementById", "nsgo-base").IsNull() {
		base := doc.Call("createElement", "style")
		base.Set("id", "nsgo-base")
		base.Set("textContent", baseCSS)
		doc.Get("head").Call("appendChild", base)
	}
	host := doc.Call("createElement", "div")
	setStyle(host, map[string]string{
		"position": "fixed", "left": "0", "top": "0",
		"width": "300px", "height": "200px", "overflow": "hidden",
	})
	if w > 0 {
		host.Get("style").Set("width", js.ValueOf(int(w)).String()+"px")
	}
	if h > 0 {
		host.Get("style").Set("height", js.ValueOf(int(h)).String()+"px")
	}
	doc.Get("body").Call("appendChild", host)
	s := &surface{
		doc:      doc,
		host:     host,
		animator: driver.NewAnimator(nil),
		dom:      newDOM(),
		effects:  map[*view]bool{},
		env:      driver.Environment{PixelRatio: ratio, ReduceMotion: true},
	}
	s.parked = s.element("div")
	setStyle(s.parked, map[string]string{
		"position": "absolute", "visibility": "hidden", "left": "-10000px", "top": "0",
		"width": "0", "height": "0", "overflow": "hidden",
	})
	host.Call("appendChild", s.parked)
	t.Cleanup(func() {
		s.teardownGlass()
		for _, v := range s.views {
			if v == nil {
				continue
			}
			if v.gl != nil {
				v.gl.release()
			}
			v.listeners.release()
		}
		host.Call("remove")
	})
	return s
}

func waitGlassPaint(t *testing.T, s *surface, after int) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if s.glass != nil && s.glass.paintCount > after {
			return
		}
		if time.Now().After(deadline) {
			got := 0
			if s.glass != nil {
				got = s.glass.paintCount
			}
			t.Fatalf("timed out waiting for glass stage paint (paintCount=%d, want >%d, active=%v)",
				got, after, s.glass != nil && s.glass.active)
		}
	}
}

func stagePixelAt(s *surface, cssX, cssY float64) [4]byte {
	dpr := s.pixelRatio()
	x := int(cssX * dpr)
	y := int(cssY * dpr)
	data := s.glass.ctx.Call("getImageData", x, y, 1, 1).Get("data")
	return [4]byte{
		byte(data.Index(0).Int()),
		byte(data.Index(1).Int()),
		byte(data.Index(2).Int()),
		byte(data.Index(3).Int()),
	}
}

func TestGlassCapabilityDetection(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("run with nsgo test -target web")
	}
	// Verify detection does not panic and agrees with HTMLCanvasElement / CanvasRenderingContext2D prototypes in this browser.
	got := htmlInCanvasSupported()
	cp := js.Global().Get("HTMLCanvasElement").Get("prototype")
	p2d := js.Global().Get("CanvasRenderingContext2D").Get("prototype")
	hasAPI := hasProp(cp, "content") && hasProp(cp, "requestPaint") &&
		hasProp(cp, "getElementImageDefaultSize") && hasProp(p2d, "drawElementImage")
	if hasAPI && !got {
		t.Fatal("browser exposes HTML-in-Canvas APIs, but htmlInCanvasSupported() returned false")
	}
	if !hasAPI && got {
		t.Fatal("htmlInCanvasSupported() returned true without HTML-in-Canvas APIs")
	}
}

func TestGlassFallbackWhenUnsupported(t *testing.T) {
	orig := glassSupportedFunc
	glassSupportedFunc = func() bool { return false }
	t.Cleanup(func() { glassSupportedFunc = orig })

	s := glassTestSurface(t, 300, 200, 2)
	rootStyle := driver.DefaultViewStyle
	rootStyle.Background = driver.RGB(0x204080)
	bareGlass := driver.DefaultViewStyle
	bareGlass.Glass = driver.GlassRegular
	bareGlass.CornerRadius = 16
	explicitGlass := driver.DefaultViewStyle
	explicitGlass.Glass = driver.GlassRegular
	explicitGlass.Background = driver.RGBA(0xffffff, 0.6)
	explicitGlass.Effects = &driver.Effects{BackdropFilters: []driver.Filter{{Kind: driver.FilterBlur, Amount: 24}}}

	s.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemFrame, ID: 1, Frame: driver.Bounds{Size: driver.Size{Width: 300, Height: 200}}},
		{Kind: driver.ItemViewStyle, ID: 1, ViewStyle: &rootStyle},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemFrame, ID: 2, Frame: driver.Bounds{Origin: driver.Point{X: 20, Y: 20}, Size: driver.Size{Width: 100, Height: 60}}},
		{Kind: driver.ItemViewStyle, ID: 2, ViewStyle: &bareGlass},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 3, Parent: 1},
		{Kind: driver.ItemFrame, ID: 3, Frame: driver.Bounds{Origin: driver.Point{X: 140, Y: 20}, Size: driver.Size{Width: 100, Height: 60}}},
		{Kind: driver.ItemViewStyle, ID: 3, ViewStyle: &explicitGlass},
	}})

	if s.glass != nil && s.glass.active {
		t.Fatal("glass stage activated when HTML-in-Canvas is unsupported")
	}
	if !s.host.Call("querySelector", "canvas.nsgo-glass-stage").IsNull() {
		t.Fatal("unexpected stage canvas mounted in fallback mode")
	}
	v2, v3 := s.view(2), s.view(3)
	if v2.glassActive || v3.glassActive {
		t.Fatal("glass views marked shader-active in fallback mode")
	}
	if v2.el.Call("hasAttribute", "drawable").Bool() || s.root.el.Call("hasAttribute", "drawable").Bool() {
		t.Fatal("drawable attribute set in fallback mode")
	}
	if !strings.Contains(v2.look.css, "backdrop-filter:blur(") {
		t.Errorf("bare glass fallback missing backdrop-filter: %q", v2.look.css)
	}
	if !strings.Contains(v2.look.css, "background-color:rgba(") {
		t.Errorf("bare glass fallback missing tint background-color: %q", v2.look.css)
	}
	if !strings.Contains(v2.look.css, "box-shadow:inset 0 0 0 1px") {
		t.Errorf("bare glass fallback missing inset rim: %q", v2.look.css)
	}
	if !strings.Contains(v3.look.css, "backdrop-filter:blur(24px)") {
		t.Errorf("explicit glass fallback lost explicit blur: %q", v3.look.css)
	}

	// Dark mode updates fallback tint/rim cleanly.
	s.setAppearance(driver.AppearanceDark)
	if !strings.Contains(v2.look.css, "rgba(30,30,34,0.45)") {
		t.Errorf("dark fallback tint not applied: %q", v2.look.css)
	}

	// Teardown via delete leaves no stray state.
	s.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemDelete, ID: 2},
		{Kind: driver.ItemDelete, ID: 3},
	}})
	if s.glass != nil && len(s.glass.views) != 0 {
		t.Errorf("deleted glass views still tracked: %d", len(s.glass.views))
	}
}

func TestGlassShaderRendering(t *testing.T) {
	if !htmlInCanvasSupported() {
		t.Skip("browser does not support HTML-in-Canvas")
	}
	s := glassTestSurface(t, 300, 200, 2)

	rootStyle := driver.DefaultViewStyle
	rootStyle.Background = driver.RGB(0x101010)
	redStyle := driver.DefaultViewStyle
	redStyle.Background = driver.RGB(0xff0000)
	blueStyle := driver.DefaultViewStyle
	blueStyle.Background = driver.RGB(0x0000ff)
	glassStyle := driver.DefaultViewStyle
	glassStyle.Glass = driver.GlassRegular
	glassStyle.CornerRadius = 16
	glassStyle.Background = driver.RGBA(0x0080ff, 0.15)
	childStyle := driver.DefaultViewStyle
	childStyle.Background = driver.RGB(0x00ff00)

	s.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemFrame, ID: 1, Frame: driver.Bounds{Size: driver.Size{Width: 300, Height: 200}}},
		{Kind: driver.ItemViewStyle, ID: 1, ViewStyle: &rootStyle},
		// Two-tone backdrop behind the glass panel (split at x=150).
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemFrame, ID: 2, Frame: driver.Bounds{Size: driver.Size{Width: 150, Height: 200}}},
		{Kind: driver.ItemViewStyle, ID: 2, ViewStyle: &redStyle},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 3, Parent: 1},
		{Kind: driver.ItemFrame, ID: 3, Frame: driver.Bounds{Origin: driver.Point{X: 150, Y: 0}, Size: driver.Size{Width: 150, Height: 200}}},
		{Kind: driver.ItemViewStyle, ID: 3, ViewStyle: &blueStyle},
		// Liquid Glass card straddling the red/blue split.
		{Kind: driver.ItemCreate, ID: 4, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 4, Parent: 1},
		{Kind: driver.ItemFrame, ID: 4, Frame: driver.Bounds{Origin: driver.Point{X: 50, Y: 40}, Size: driver.Size{Width: 200, Height: 120}}},
		{Kind: driver.ItemViewStyle, ID: 4, ViewStyle: &glassStyle},
		// Child inside the glass card.
		{Kind: driver.ItemCreate, ID: 5, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 5, Parent: 4},
		{Kind: driver.ItemFrame, ID: 5, Frame: driver.Bounds{Origin: driver.Point{X: 140, Y: 40}, Size: driver.Size{Width: 40, Height: 40}}},
		{Kind: driver.ItemViewStyle, ID: 5, ViewStyle: &childStyle},
	}})

	waitGlassPaint(t, s, 0)

	v4 := s.view(4)
	if !s.glass.active || !v4.glassActive {
		t.Fatalf("expected active glass stage and view, got active=%v glassActive=%v", s.glass.active, v4.glassActive)
	}
	if strings.Contains(v4.look.css, "backdrop-filter:") {
		t.Errorf("shader-active glass view should omit CSS backdrop-filter: %q", v4.look.css)
	}

	// Raw backdrop outside glass stays crisp red and blue.
	pxRed := stagePixelAt(s, 20, 20)
	pxBlue := stagePixelAt(s, 280, 20)
	if pxRed[0] < 240 || pxRed[2] > 15 {
		t.Errorf("backdrop left = %v, want red", pxRed)
	}
	if pxBlue[2] < 240 || pxBlue[0] > 15 {
		t.Errorf("backdrop right = %v, want blue", pxBlue)
	}

	// Inside the glass card on the red side (x=90, y=100): light regular glass lifts luminance well above raw red's green/blue = 0 (light tone curve + tint).
	lightGlass := stagePixelAt(s, 90, 100)
	if lightGlass[3] < 250 {
		t.Errorf("light glass alpha = %d, want opaque", lightGlass[3])
	}
	if lightGlass[1] < 40 || lightGlass[2] < 40 {
		t.Errorf("light glass over red = %v, expected lifted luminance/tint from glass shader", lightGlass)
	}

	// At the red/blue seam inside the glass (x=150, y=100): blur mixes red & blue.
	seam := stagePixelAt(s, 150, 100)
	if seam[0] < 60 || seam[2] < 60 {
		t.Errorf("glass seam pixel = %v, expected blurred mix of red and blue", seam)
	}

	// Child box inside the glass card (at x=50+140+20=210, y=40+40+20=100) draws on top.
	childPx := stagePixelAt(s, 210, 100)
	if childPx[1] < 240 || childPx[0] > 20 || childPx[2] > 20 {
		t.Errorf("glass child pixel = %v, want green on top of glass", childPx)
	}

	// Hit testing reaches the child inside the glass card.
	hit := s.doc.Call("elementFromPoint", 210, 100)
	if !hit.Equal(s.view(5).el) {
		t.Errorf("elementFromPoint(210,100) = %v, want glass child view 5", hit)
	}

	// Dark appearance switches to the darker glass tone curve.
	beforeDark := s.glass.paintCount
	s.setAppearance(driver.AppearanceDark)
	waitGlassPaint(t, s, beforeDark)
	darkGlass := stagePixelAt(s, 90, 100)
	if darkGlass[1] >= lightGlass[1] {
		t.Errorf("dark glass green=%d not darker than light glass green=%d (light=%v dark=%v)",
			darkGlass[1], lightGlass[1], lightGlass, darkGlass)
	}

	// GlassClear switches to the clear tone curve (less tone lift/blur).
	beforeClear := s.glass.paintCount
	clearStyle := glassStyle
	clearStyle.Glass = driver.GlassClear
	clearStyle.Background = 0
	s.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemViewStyle, ID: 4, ViewStyle: &clearStyle},
	}})
	waitGlassPaint(t, s, beforeClear)
	clearPx := stagePixelAt(s, 90, 100)
	if clearPx[0] < 180 {
		t.Errorf("clear glass over red = %v, expected high red transmission", clearPx)
	}
}

func TestGlassWithNestedCanvasesNoHang(t *testing.T) {
	if !htmlInCanvasSupported() {
		t.Skip("browser does not support HTML-in-Canvas")
	}
	s := glassTestSurface(t, 300, 200, 2)

	rootStyle := driver.DefaultViewStyle
	rootStyle.Background = driver.RGB(0x101010)
	glassStyle := driver.DefaultViewStyle
	glassStyle.Glass = driver.GlassClear
	glassStyle.CornerRadius = 12

	// 100x100 drawing at DPR=2 produces a 200x200 bitmap (larger than Chrome's drawElementImage software canvas threshold, verifying manual canvas compositing).
	bgDrawing := driver.NewDrawing(nil)
	bgDrawing.SetFillColor(driver.RGB(0x00ffff)).FillRect(0, 0, 140, 140)

	fgDrawing := driver.NewDrawing(nil)
	fgDrawing.SetFillColor(driver.RGB(0xffff00)).FillRect(0, 0, 40, 40)

	s.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemFrame, ID: 1, Frame: driver.Bounds{Size: driver.Size{Width: 300, Height: 200}}},
		{Kind: driver.ItemViewStyle, ID: 1, ViewStyle: &rootStyle},
		// Backdrop 2D canvas (cyan), partially behind the glass view.
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindCanvas},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemFrame, ID: 2, Frame: driver.Bounds{Origin: driver.Point{X: 20, Y: 20}, Size: driver.Size{Width: 140, Height: 140}}},
		{Kind: driver.ItemCanvas, ID: 2, Drawing: bgDrawing},
		// Glass card overlapping the cyan canvas (x=80..240, y=40..160).
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 3, Parent: 1},
		{Kind: driver.ItemFrame, ID: 3, Frame: driver.Bounds{Origin: driver.Point{X: 80, Y: 40}, Size: driver.Size{Width: 160, Height: 120}}},
		{Kind: driver.ItemViewStyle, ID: 3, ViewStyle: &glassStyle},
		// Nested 2D canvas INSIDE the glass card (yellow icon).
		{Kind: driver.ItemCreate, ID: 4, View: driver.KindCanvas},
		{Kind: driver.ItemInsert, ID: 4, Parent: 3},
		{Kind: driver.ItemFrame, ID: 4, Frame: driver.Bounds{Origin: driver.Point{X: 100, Y: 40}, Size: driver.Size{Width: 40, Height: 40}}},
		{Kind: driver.ItemCanvas, ID: 4, Drawing: fgDrawing},
	}})

	waitGlassPaint(t, s, 0)

	// Uncovered part of the backdrop canvas (x=40, y=60) must be bright cyan.
	rawCanvas := stagePixelAt(s, 40, 60)
	if rawCanvas[1] < 240 || rawCanvas[2] < 240 || rawCanvas[0] > 20 {
		t.Errorf("backdrop canvas pixel = %v, want cyan (0,255,255)", rawCanvas)
	}

	// Glass over the cyan backdrop canvas (x=115, y=100) must transmit cyan, proving the backdrop <canvas> was captured into the glass shader!
	glassOverCanvas := stagePixelAt(s, 115, 100)
	if glassOverCanvas[1] < 150 || glassOverCanvas[2] < 150 {
		t.Errorf("glass over cyan canvas = %v, want cyan refracted through glass", glassOverCanvas)
	}

	// Child <canvas> inside the glass card (x=80+100+20=200, y=40+40+20=100) must render bright yellow on top of the glass.
	innerCanvas := stagePixelAt(s, 200, 100)
	if innerCanvas[0] < 240 || innerCanvas[1] < 240 || innerCanvas[2] > 20 {
		t.Errorf("child canvas inside glass = %v, want yellow (255,255,0)", innerCanvas)
	}

	// Also mount a WebGL 2 <canvas> behind the glass view to verify GPU canvases composite without dropping or hanging.
	glCanvas := s.doc.Call("createElement", "canvas")
	glCanvas.Set("width", 100)
	glCanvas.Set("height", 100)
	setStyle(glCanvas, map[string]string{
		"position": "absolute", "left": "160px", "top": "10px", "width": "60px", "height": "60px",
	})
	if glCtx := glCanvas.Call("getContext", "webgl2", map[string]any{"preserveDrawingBuffer": true}); glCtx.Truthy() {
		glCtx.Call("clearColor", 1, 0, 1, 1)
		glCtx.Call("clear", glCtx.Get("COLOR_BUFFER_BIT"))
		t.Cleanup(func() {
			if lose := glCtx.Call("getExtension", "WEBGL_lose_context"); lose.Truthy() {
				lose.Call("loseContext")
			}
		})
		beforeGL := s.glass.paintCount
		s.root.el.Call("insertBefore", glCanvas, s.view(3).el)
		s.requestGlassPaint()
		waitGlassPaint(t, s, beforeGL)
		glPx := stagePixelAt(s, 190, 20)
		if glPx[0] < 240 || glPx[1] > 20 || glPx[2] < 240 {
			t.Errorf("backdrop WebGL2 canvas pixel = %v, want magenta (255,0,255)", glPx)
		}
	}

	// Verify no infinite repaint recursion loop.
	countAfterFirst := s.glass.paintCount
	time.Sleep(150 * time.Millisecond)
	if s.glass.paintCount-countAfterFirst > 5 {
		t.Errorf("glass stage repainted %d times while idle (possible recursion loop)", s.glass.paintCount-countAfterFirst)
	}
}

func TestGlassUnsafeAncestorFallsBackWithoutHang(t *testing.T) {
	if !htmlInCanvasSupported() {
		t.Skip("browser does not support HTML-in-Canvas")
	}
	s := glassTestSurface(t, 300, 200, 2)

	rootStyle := driver.DefaultViewStyle
	rootStyle.Background = driver.RGB(0x204060)

	// Ancestor combining BlendMode + Mask: marking a descendant drawable would hang Chrome's HTML-in-Canvas paint recorder.
	unsafeParent := driver.DefaultViewStyle
	unsafeParent.BlendMode = driver.BlendMultiply
	unsafeParent.Effects = &driver.Effects{
		Mask: &driver.GradientPaint{
			Kind: driver.GradientLinear,
			X0:   0, Y0: 0, X1: 0, Y1: 1,
			Stops: []driver.GradientStop{{Color: driver.RGB(0x000000), Offset: 0}, {Color: driver.RGBA(0x000000, 0), Offset: 1}},
		},
	}

	glassStyle := driver.DefaultViewStyle
	glassStyle.Glass = driver.GlassRegular
	glassStyle.CornerRadius = 12

	s.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemFrame, ID: 1, Frame: driver.Bounds{Size: driver.Size{Width: 300, Height: 200}}},
		{Kind: driver.ItemViewStyle, ID: 1, ViewStyle: &rootStyle},
		// Unsafe ancestor holding a glass view.
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemFrame, ID: 2, Frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 10}, Size: driver.Size{Width: 120, Height: 120}}},
		{Kind: driver.ItemViewStyle, ID: 2, ViewStyle: &unsafeParent},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 3, Parent: 2},
		{Kind: driver.ItemFrame, ID: 3, Frame: driver.Bounds{Origin: driver.Point{X: 10, Y: 10}, Size: driver.Size{Width: 80, Height: 80}}},
		{Kind: driver.ItemViewStyle, ID: 3, ViewStyle: &glassStyle},
		// Safe sibling glass view.
		{Kind: driver.ItemCreate, ID: 4, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 4, Parent: 1},
		{Kind: driver.ItemFrame, ID: 4, Frame: driver.Bounds{Origin: driver.Point{X: 160, Y: 20}, Size: driver.Size{Width: 110, Height: 100}}},
		{Kind: driver.ItemViewStyle, ID: 4, ViewStyle: &glassStyle},
	}})

	waitGlassPaint(t, s, 0)

	v3, v4 := s.view(3), s.view(4)
	if v3.glassActive {
		t.Fatal("glass view under blend+mask ancestor must fall back, not activate shader drawable")
	}
	if v3.el.Call("hasAttribute", "drawable").Bool() {
		t.Fatal("unsafe glass view must not carry drawable attribute")
	}
	if !strings.Contains(v3.look.css, "backdrop-filter:") {
		t.Errorf("unsafe glass view missing CSS fallback backdrop-filter: %q", v3.look.css)
	}
	if !v4.glassActive {
		t.Fatal("safe sibling glass view should remain shader-active")
	}
}

func TestGlassOverlayOrderAndTeardown(t *testing.T) {
	if !htmlInCanvasSupported() {
		t.Skip("browser does not support HTML-in-Canvas")
	}
	s := glassTestSurface(t, 300, 200, 2)

	rootStyle := driver.DefaultViewStyle
	rootStyle.Background = driver.RGB(0x203040)
	glassStyle := driver.DefaultViewStyle
	glassStyle.Glass = driver.GlassRegular
	glassStyle.CornerRadius = 12
	overlayStyle := driver.DefaultViewStyle
	overlayStyle.Background = driver.RGB(0xff00ff)

	s.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemFrame, ID: 1, Frame: driver.Bounds{Size: driver.Size{Width: 300, Height: 200}}},
		{Kind: driver.ItemViewStyle, ID: 1, ViewStyle: &rootStyle},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemFrame, ID: 2, Frame: driver.Bounds{Origin: driver.Point{X: 40, Y: 40}, Size: driver.Size{Width: 160, Height: 120}}},
		{Kind: driver.ItemViewStyle, ID: 2, ViewStyle: &glassStyle},
		// Overlay sibling after glass view overlapping its right half.
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 3, Parent: 1},
		{Kind: driver.ItemFrame, ID: 3, Frame: driver.Bounds{Origin: driver.Point{X: 120, Y: 60}, Size: driver.Size{Width: 60, Height: 60}}},
		{Kind: driver.ItemViewStyle, ID: 3, ViewStyle: &overlayStyle},
	}})

	waitGlassPaint(t, s, 0)

	// Overlay sibling must paint ON TOP of the glass view in solid magenta.
	ovPx := stagePixelAt(s, 150, 90)
	if ovPx[0] < 240 || ovPx[1] > 20 || ovPx[2] < 240 {
		t.Errorf("overlay pixel above glass = %v, want magenta (255,0,255)", ovPx)
	}

	// Clearing GlassEffect tears down the stage and restores s.root.el cleanly.
	plain := driver.DefaultViewStyle
	s.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemViewStyle, ID: 2, ViewStyle: &plain},
	}})
	if s.glass.active {
		t.Fatal("glass stage still active after clearing last glass view")
	}
	if !s.root.el.Get("parentNode").Equal(s.host) {
		t.Fatal("s.root.el was not restored directly under s.host after teardown")
	}
	if !s.host.Call("querySelector", "canvas.nsgo-glass-stage").IsNull() {
		t.Fatal("stage canvas still in DOM after teardown")
	}
	if s.host.Call("querySelectorAll", "[drawable]").Length() != 0 {
		t.Fatal("leftover drawable attributes after teardown")
	}
	if s.glass.gl != nil {
		t.Fatal("WebGL glass renderer was not released on teardown")
	}
}
