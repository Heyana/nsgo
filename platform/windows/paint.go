//go:build windows

package windows

import (
	"encoding/binary"
	"math"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
)

var (
	setGraphicsMode   = gdi32.NewProc("SetGraphicsMode")
	setWorldTransform = gdi32.NewProc("SetWorldTransform")
	setViewportOrgEx  = gdi32.NewProc("SetViewportOrgEx")
	beginPath         = gdi32.NewProc("BeginPath")
	endPath           = gdi32.NewProc("EndPath")
	selectClipPath    = gdi32.NewProc("SelectClipPath")
	rectangle         = gdi32.NewProc("Rectangle")
	getClipBox        = gdi32.NewProc("GetClipBox")
	getClipRgn        = gdi32.NewProc("GetClipRgn")
	getRegionData     = gdi32.NewProc("GetRegionData")
	gdiFlush          = gdi32.NewProc("GdiFlush")
)

const (
	gmCompatible = 1
	gmAdvanced   = 2
	rgnDiff      = 4
)

// xform is GDI's XFORM.
type xform struct{ M11, M12, M21, M22, Dx, Dy float32 }

// matrix is an affine transform: x' = a x + c y + e, y' = b x + d y + f.
type matrix struct{ a, b, c, d, e, f float64 }

var identity = matrix{a: 1, d: 1}

func translation(x, y float64) matrix { return matrix{a: 1, d: 1, e: x, f: y} }

func scaling(x, y float64) matrix { return matrix{a: x, d: y} }

func (m matrix) apply(x, y float64) (float64, float64) {
	return m.a*x + m.c*y + m.e, m.b*x + m.d*y + m.f
}

// then returns m followed by n.
func (m matrix) then(n matrix) matrix {
	return matrix{
		a: m.a*n.a + m.b*n.c, b: m.a*n.b + m.b*n.d,
		c: m.c*n.a + m.d*n.c, d: m.c*n.b + m.d*n.d,
		e: m.e*n.a + m.f*n.c + n.e, f: m.e*n.b + m.f*n.d + n.f,
	}
}

func (m matrix) invert() matrix {
	det := m.a*m.d - m.b*m.c
	if det == 0 {
		return identity
	}
	return matrix{
		a: m.d / det, b: -m.b / det, c: -m.c / det, d: m.a / det,
		e: (m.c*m.f - m.d*m.e) / det, f: (m.b*m.e - m.a*m.f) / det,
	}
}

// axisAligned reports whether m only moves and grows, keeping rectangles
// rectangles the right way round.
func (m matrix) axisAligned() bool { return m.b == 0 && m.c == 0 && m.a > 0 && m.d > 0 }

// stretch returns how much m scales lengths, on average.
func (m matrix) stretch() float64 { return math.Sqrt(math.Abs(m.a*m.d - m.b*m.c)) }

func (m matrix) xform() xform {
	return xform{float32(m.a), float32(m.b), float32(m.c), float32(m.d), float32(m.e), float32(m.f)}
}

// bound returns the box around r mapped by m, rounded out.
func (m matrix) bound(r rect) rect {
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, c := range [4][2]int32{{r.Left, r.Top}, {r.Right, r.Top}, {r.Left, r.Bottom}, {r.Right, r.Bottom}} {
		x, y := m.apply(float64(c[0]), float64(c[1]))
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	return rect{int32(math.Floor(x0)), int32(math.Floor(y0)), int32(math.Ceil(x1)), int32(math.Ceil(y1))}
}

func (r rect) union(o rect) rect {
	switch {
	case r.empty():
		return o
	case o.empty():
		return r
	}
	return rect{min(r.Left, o.Left), min(r.Top, o.Top), max(r.Right, o.Right), max(r.Bottom, o.Bottom)}
}

func (r rect) offset(dx, dy int32) rect {
	return rect{r.Left + dx, r.Top + dy, r.Right + dx, r.Bottom + dy}
}

func (r rect) inset(d int32) rect { return rect{r.Left + d, r.Top + d, r.Right - d, r.Bottom - d} }

// transformOf returns t in the window's pixels for a view drawn in r:
// around its center, translated, then rotated, then scaled.
func (w *window) transformOf(t core.Transform, r rect) matrix {
	if t == (core.Transform{}) {
		return identity
	}
	sx, sy := t.Scales()
	cx, cy := float64(r.Left+r.Right)/2, float64(r.Top+r.Bottom)/2
	sin, cos := math.Sincos(float64(t.Rotate) * math.Pi / 180)
	m := translation(-cx, -cy).
		then(scaling(float64(sx), float64(sy))).
		then(matrix{a: cos, b: sin, c: -sin, d: cos}).
		then(translation(cx+float64(t.TranslateX)*w.scale, cy+float64(t.TranslateY)*w.scale))
	if math.Abs(m.a-1) < 1e-9 && math.Abs(m.d-1) < 1e-9 && math.Abs(m.b) < 1e-9 && math.Abs(m.c) < 1e-9 &&
		math.Abs(m.e) < 1e-9 && math.Abs(m.f) < 1e-9 {
		return identity
	}
	return m
}

// bitmap is a 32-bit top-down DIB section selected into a DC of its own.
type bitmap struct {
	dc, handle, old uintptr
	bits            unsafe.Pointer
	width, height   int32
}

func newDIB(width, height int32) (uintptr, unsafe.Pointer) {
	bi := bitmapInfo{Width: width, Height: -height, Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Offsetof(bi.Colors))
	var bits unsafe.Pointer
	h, _, _ := createDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if h == 0 || bits == nil {
		return 0, nil
	}
	return h, bits
}

func newBitmap(like uintptr, width, height int32) (*bitmap, bool) {
	h, bits := newDIB(width, height)
	if h == 0 {
		return nil, false
	}
	b := &bitmap{handle: h, bits: bits, width: width, height: height}
	b.dc, _, _ = createCompatibleDC.Call(like)
	b.old, _, _ = selectObject.Call(b.dc, h)
	return b, true
}

func (b *bitmap) free() {
	selectObject.Call(b.dc, b.old)
	deleteDC.Call(b.dc)
	deleteObject.Call(b.handle)
}

// painter draws views into a target: the window's back buffer, or a
// layer a translucent subtree draws into first. It draws in the window's
// pixels; the DC maps them to the target's through m, a world transform
// views' transforms make, and the target's origin.
type painter struct {
	w      *window
	target *bitmap
	dc     uintptr
	ox, oy int32 // the target's origin in the window, in pixels
	m      matrix
	solid  *bitmap // 1×1, blended over clipped areas to fill them translucently
}

func (w *window) paint() {
	var ps paintStruct
	hdc, _, _ := beginPaint.Call(w.hwnd, uintptr(unsafe.Pointer(&ps)))
	defer endPaint.Call(w.hwnd, uintptr(unsafe.Pointer(&ps)))
	var client rect
	getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))
	if client.empty() {
		return
	}
	// Draw into a bitmap first so the window never shows a half-drawn frame.
	back, ok := newBitmap(hdc, client.Right, client.Bottom)
	if !ok {
		return
	}
	defer back.free()
	solid, ok := newBitmap(hdc, 1, 1)
	if !ok {
		return
	}
	defer solid.free()
	p := &painter{w: w, target: back, dc: back.dc, m: identity, solid: solid}
	white, _, _ := getStockObject.Call(whiteBrush)
	fillRect.Call(back.dc, uintptr(unsafe.Pointer(&client)), white)
	setBkMode.Call(back.dc, transparent)
	if root := w.view(w.root); root != nil {
		p.draw(root, 0, 0)
	}
	p.drawCaptionButtons(client.Right)
	bitBlt.Call(hdc, 0, 0, uintptr(client.Right), uintptr(client.Bottom), back.dc, 0, 0, srcCopy)
}

// draw paints v and its children; x and y are the origin of v's parent's
// content in the window.
func (p *painter) draw(v *view, x, y core.Points) {
	s := p.w.styleOf(v)
	if s.Hidden || s.Opacity <= 0 {
		return
	}
	if s.Opacity < 1 {
		box := p.w.extent(v, x, y, p.m).inset(-2) // text may overhang its frame
		p.layer(box, s.Opacity, func(lp *painter) { lp.drawView(v, s, x, y) })
		return
	}
	p.drawView(v, s, x, y)
}

func (p *painter) drawView(v *view, s *core.ViewStyle, x, y core.Points) {
	w := p.w
	x, y = x+v.frame.Origin.X, y+v.frame.Origin.Y
	r := w.rect(x, y, v.frame.Size)
	if t := w.transformOf(s.Transform, r); t != identity {
		saved, old := p.push(t.then(p.m))
		defer p.pop(saved, old)
	}
	diameter := min(2*w.px(s.CornerRadius), r.Right-r.Left, r.Bottom-r.Top)
	p.fill(r, diameter, s)
	switch v.kind {
	case core.KindImage:
		// Inside the border, and inside the corners when clipping.
		in := r
		if b := w.borderWidths(s); b != (rect{}) {
			in = rect{r.Left + b.Left, r.Top + b.Top, r.Right - b.Right, r.Bottom - b.Bottom}
		}
		if s.Clip && diameter > 0 {
			saved, _, _ := saveDC.Call(p.dc)
			p.clip(r, diameter, rgnAnd)
			p.drawImage(v, s.TintColor, in)
			p.restore(saved)
		} else {
			p.drawImage(v, s.TintColor, in)
		}
	case core.KindCanvas:
		if v.drawing != nil {
			p.canvas(v, r)
		}
	}
	if v.text != nil {
		v.text.draw(p.dc, w.px(x+v.content.Origin.X), w.px(y+v.content.Origin.Y))
	}
	if len(v.children) == 0 {
		return
	}
	if s.Clip || v.kind == core.KindScroll {
		saved, _, _ := saveDC.Call(p.dc)
		p.clip(r, diameter, rgnAnd)
		defer p.restore(saved)
	}
	for _, c := range v.children {
		p.draw(c, x-v.offset.X, y-v.offset.Y)
	}
	if v.kind == core.KindScroll && !v.scroll.HideIndicator {
		p.drawIndicator(v, x, y)
	}
}

// extent returns the window's pixels v's subtree may draw on through m:
// its frames, transformed, cut by the views that clip.
func (w *window) extent(v *view, x, y core.Points, m matrix) rect {
	s := w.styleOf(v)
	if s.Hidden || s.Opacity <= 0 {
		return rect{}
	}
	x, y = x+v.frame.Origin.X, y+v.frame.Origin.Y
	r := w.rect(x, y, v.frame.Size)
	m = w.transformOf(s.Transform, r).then(m)
	box := m.bound(r)
	if s.Clip || v.kind == core.KindScroll {
		return box
	}
	for _, c := range v.children {
		box = box.union(w.extent(c, x-v.offset.X, y-v.offset.Y, m))
	}
	return box
}

// push applies m as the world transform until pop.
func (p *painter) push(m matrix) (saved uintptr, old matrix) {
	saved, _, _ = saveDC.Call(p.dc)
	old, p.m = p.m, m
	setGraphicsMode.Call(p.dc, gmAdvanced)
	x := m.xform()
	setWorldTransform.Call(p.dc, uintptr(unsafe.Pointer(&x)))
	return saved, old
}

func (p *painter) pop(saved uintptr, old matrix) {
	p.m = old
	p.restore(saved)
}

// restore restores the DC's state, back in the compatible graphics mode
// views without transforms draw in.
func (p *painter) restore(saved uintptr) {
	restoreDC.Call(p.dc, saved)
	if p.m == identity {
		x := identity.xform()
		setWorldTransform.Call(p.dc, uintptr(unsafe.Pointer(&x)))
		setGraphicsMode.Call(p.dc, gmCompatible)
	}
}

// device runs fn with the DC's coordinates the target's pixels.
func (p *painter) device(fn func()) {
	saved, _, _ := saveDC.Call(p.dc)
	x := identity.xform() // fails harmlessly in the compatible mode
	setWorldTransform.Call(p.dc, uintptr(unsafe.Pointer(&x)))
	setViewportOrgEx.Call(p.dc, 0, 0, 0)
	fn()
	restoreDC.Call(p.dc, saved)
}

// clipBox returns the box the clipping lets through, in the target's
// pixels.
func (p *painter) clipBox() rect {
	var box rect
	p.device(func() { getClipBox.Call(p.dc, uintptr(unsafe.Pointer(&box))) })
	return box
}

// layer draws through fn into a bitmap holding what is behind box (in
// the window's pixels), then blends it back at opacity: the subtree
// fades as one, as group opacity does.
func (p *painter) layer(box rect, opacity float32, fn func(*painter)) {
	box = box.offset(-p.ox, -p.oy).intersect(p.clipBox())
	if box.empty() {
		return
	}
	width, height := box.Right-box.Left, box.Bottom-box.Top
	l, ok := newBitmap(p.dc, width, height)
	if !ok {
		fn(p)
		return
	}
	defer l.free()
	p.device(func() {
		bitBlt.Call(l.dc, 0, 0, uintptr(width), uintptr(height), p.dc, uintptr(box.Left), uintptr(box.Top), srcCopy)
	})
	lp := &painter{w: p.w, target: l, dc: l.dc, ox: p.ox + box.Left, oy: p.oy + box.Top, m: p.m, solid: p.solid}
	setBkMode.Call(l.dc, transparent)
	setViewportOrgEx.Call(l.dc, uintptr(-lp.ox), uintptr(-lp.oy), 0)
	if lp.m != identity {
		setGraphicsMode.Call(l.dc, gmAdvanced)
		x := lp.m.xform()
		setWorldTransform.Call(l.dc, uintptr(unsafe.Pointer(&x)))
	}
	fn(lp)
	alpha := uintptr(math.Round(float64(opacity)*255)) << 16 // BLENDFUNCTION{AC_SRC_OVER, 0, alpha, 0}
	p.device(func() {
		alphaBlend.Call(p.dc, uintptr(box.Left), uintptr(box.Top), uintptr(width), uintptr(height),
			l.dc, 0, 0, uintptr(width), uintptr(height), alpha)
	})
}

// shape adds r with its corners to the DC's path.
func (p *painter) shape(r rect, diameter int32) {
	if diameter > 0 {
		roundRect.Call(p.dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom),
			uintptr(diameter), uintptr(diameter))
	} else {
		rectangle.Call(p.dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom))
	}
}

// clip combines the clipping with r and its corners by mode: through a
// path under a transform, else exactly as a region.
func (p *painter) clip(r rect, diameter int32, mode uintptr) {
	if p.m != identity {
		beginPath.Call(p.dc)
		p.shape(r, diameter)
		endPath.Call(p.dc)
		selectClipPath.Call(p.dc, mode)
		return
	}
	r = r.offset(-p.ox, -p.oy) // regions are in the target's pixels
	var rgn uintptr
	if diameter > 0 {
		rgn, _, _ = createRoundRectRgn.Call(uintptr(r.Left), uintptr(r.Top), uintptr(r.Right+1), uintptr(r.Bottom+1),
			uintptr(diameter), uintptr(diameter))
	} else {
		rgn, _, _ = createRectRgn.Call(uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom))
	}
	extSelectClipRgn.Call(p.dc, rgn, mode) // copies the region
	deleteObject.Call(rgn)
}

// fill paints a view's background and its border inside r; translucent
// colors blend with what is behind.
func (p *painter) fill(r rect, diameter int32, s *core.ViewStyle) {
	switch a := s.Background & 0xff; {
	case a == 0xff:
		brush, _, _ := createSolidBrush.Call(colorRef(s.Background))
		pen, _, _ := getStockObject.Call(nullPen)
		oldBrush, _, _ := selectObject.Call(p.dc, brush)
		oldPen, _, _ := selectObject.Call(p.dc, pen)
		// A null pen leaves out the right and bottom edges.
		roundRect.Call(p.dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right+1), uintptr(r.Bottom+1),
			uintptr(diameter), uintptr(diameter))
		selectObject.Call(p.dc, oldBrush)
		selectObject.Call(p.dc, oldPen)
		deleteObject.Call(brush)
	case a != 0:
		p.blend(r, diameter, rect{}, 0, s.Background)
	}
	if s.Borders != (core.Edges{}) {
		p.sides(r, diameter, s)
		return
	}
	if s.BorderWidth <= 0 || s.BorderColor&0xff == 0 {
		return
	}
	width := max(p.w.px(s.BorderWidth), 1)
	if s.BorderColor&0xff != 0xff {
		p.blend(r, diameter, r.inset(width), max(diameter-2*width, 0), s.BorderColor)
		return
	}
	pen, _, _ := createPen.Call(psInsideFrame, uintptr(width), colorRef(s.BorderColor))
	brush, _, _ := getStockObject.Call(nullBrush)
	oldBrush, _, _ := selectObject.Call(p.dc, brush)
	oldPen, _, _ := selectObject.Call(p.dc, pen)
	roundRect.Call(p.dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(diameter), uintptr(diameter))
	selectObject.Call(p.dc, oldBrush)
	selectObject.Call(p.dc, oldPen)
	deleteObject.Call(pen)
}

// blend fills r and its corners, less hole and its corners, with c.
func (p *painter) blend(r rect, diameter int32, hole rect, holeDiameter int32, c core.Color) {
	saved, _, _ := saveDC.Call(p.dc)
	p.clip(r, diameter, rgnAnd)
	if !hole.empty() {
		p.clip(hole, holeDiameter, rgnDiff)
	}
	p.fillClip(c)
	p.restore(saved)
}

// fillClip blends c over everything the clipping lets through: a 1×1
// bitmap of c stretched over the clipped box.
func (p *painter) fillClip(c core.Color) {
	if c&0xff == 0 {
		return
	}
	gdiFlush.Call() // the pixel may still be in use by a batched blend
	*(*uint32)(p.solid.bits) = premultiply(c)
	p.device(func() {
		var box rect
		getClipBox.Call(p.dc, uintptr(unsafe.Pointer(&box)))
		if box.empty() {
			return
		}
		alphaBlend.Call(p.dc, uintptr(box.Left), uintptr(box.Top), uintptr(box.Right-box.Left), uintptr(box.Bottom-box.Top),
			p.solid.dc, 0, 0, 1, 1, blendPremultiplied)
	})
}

// premultiply returns c, 0xRRGGBBAA, as a premultiplied BGRA pixel.
func premultiply(c core.Color) uint32 {
	a := uint32(c & 0xff)
	r, g, b := uint32(c>>24&0xff)*a/255, uint32(c>>16&0xff)*a/255, uint32(c>>8&0xff)*a/255
	return a<<24 | r<<16 | g<<8 | b
}

// drawIndicator paints a scroll bar's thumb along v's scrolling edge;
// x and y are v's origin in the window.
func (p *painter) drawIndicator(v *view, x, y core.Points) {
	const thickness, inset, minLength = 3, 2, 20
	frame, content, offset := v.frame.Size.Height, v.size.Height, v.offset.Y
	if v.scroll.Horizontal {
		frame, content, offset = v.frame.Size.Width, v.size.Width, v.offset.X
	}
	if content <= frame || frame <= 0 {
		return
	}
	length := min(max(frame*frame/content, minLength), frame)
	at := offset / (content - frame) * (frame - length)
	var bar rect
	if v.scroll.Horizontal {
		bar = p.w.rect(x+at, y+v.frame.Size.Height-inset-thickness, core.Size{Width: length, Height: thickness})
	} else {
		bar = p.w.rect(x+v.frame.Size.Width-inset-thickness, y+at, core.Size{Width: thickness, Height: length})
	}
	brush, _, _ := createSolidBrush.Call(colorRef(core.RGB(0x8c8c8c)))
	fillRect.Call(p.dc, uintptr(unsafe.Pointer(&bar)), brush)
	deleteObject.Call(brush)
}

// clipRects returns the rectangles the clipping lets through, in the
// target's pixels.
func (p *painter) clipRects() []rect {
	whole := rect{0, 0, p.target.width, p.target.height}
	rgn, _, _ := createRectRgn.Call(0, 0, 0, 0)
	defer deleteObject.Call(rgn)
	if r, _, _ := getClipRgn.Call(p.dc, rgn); int32(r) != 1 {
		return []rect{whole}
	}
	size, _, _ := getRegionData.Call(rgn, 0, 0)
	if size < 32 {
		return nil
	}
	buf := make([]byte, size)
	if n, _, _ := getRegionData.Call(rgn, size, uintptr(unsafe.Pointer(&buf[0]))); n == 0 {
		return nil
	}
	count := binary.LittleEndian.Uint32(buf[8:])
	header := binary.LittleEndian.Uint32(buf[0:])
	var out []rect
	for i := range count {
		at := header + i*16
		if int(at)+16 > len(buf) {
			break
		}
		r := rect{
			int32(binary.LittleEndian.Uint32(buf[at:])), int32(binary.LittleEndian.Uint32(buf[at+4:])),
			int32(binary.LittleEndian.Uint32(buf[at+8:])), int32(binary.LittleEndian.Uint32(buf[at+12:])),
		}
		out = append(out, r.intersect(whole))
	}
	return out
}

// borderWidths returns the width of each side's drawn border, in pixels:
// a side without a visible color has none.
func (w *window) borderWidths(s *core.ViewStyle) rect {
	side := func(width core.Points, c core.Color) int32 {
		if width <= 0 || c&0xff == 0 {
			return 0
		}
		return max(w.px(width), 1)
	}
	if s.Borders == (core.Edges{}) {
		d := side(s.BorderWidth, s.BorderColor)
		return rect{d, d, d, d}
	}
	b, c := s.Borders, s.BorderColors
	return rect{Left: side(b.Left, c.Left), Top: side(b.Top, c.Top), Right: side(b.Right, c.Right), Bottom: side(b.Bottom, c.Bottom)}
}

// sides paints a border with a width and a color per side inside r: each
// side is the ring between r and its inside, with their corners, cut to
// the trapezoid from its outer edge to its inner one, so the sides meet
// on the diagonals as CSS's do. The inner corners are approximate.
func (p *painter) sides(r rect, diameter int32, s *core.ViewStyle) {
	b := p.w.borderWidths(s)
	in := rect{min(r.Left+b.Left, r.Right), min(r.Top+b.Top, r.Bottom), max(r.Right-b.Right, r.Left), max(r.Bottom-b.Bottom, r.Top)}
	inDiameter := max(diameter-(b.Left+b.Top+b.Right+b.Bottom)/2, 0)
	c := s.BorderColors
	for _, side := range []struct {
		width int32
		color core.Color
		shape [4]point
	}{
		{b.Top, c.Top, [4]point{{r.Left, r.Top}, {r.Right, r.Top}, {in.Right, in.Top}, {in.Left, in.Top}}},
		{b.Right, c.Right, [4]point{{r.Right, r.Top}, {r.Right, r.Bottom}, {in.Right, in.Bottom}, {in.Right, in.Top}}},
		{b.Bottom, c.Bottom, [4]point{{r.Right, r.Bottom}, {r.Left, r.Bottom}, {in.Left, in.Bottom}, {in.Right, in.Bottom}}},
		{b.Left, c.Left, [4]point{{r.Left, r.Bottom}, {r.Left, r.Top}, {in.Left, in.Top}, {in.Left, in.Bottom}}},
	} {
		if side.width == 0 {
			continue
		}
		saved, _, _ := saveDC.Call(p.dc)
		p.clip(r, diameter, rgnAnd)
		if !in.empty() {
			p.clip(in, inDiameter, rgnDiff)
		}
		p.clipPolygon(side.shape[:], rgnAnd)
		p.fillClip(side.color)
		p.restore(saved)
	}
}

// clipPolygon combines the clipping with the polygon pts by mode, as
// clip does with a rectangle.
func (p *painter) clipPolygon(pts []point, mode uintptr) {
	const winding = 2
	if p.m != identity {
		beginPath.Call(p.dc)
		polygon.Call(p.dc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
		endPath.Call(p.dc)
		selectClipPath.Call(p.dc, mode)
		return
	}
	moved := make([]point, len(pts)) // regions are in the target's pixels
	for i, pt := range pts {
		moved[i] = point{pt.X - p.ox, pt.Y - p.oy}
	}
	rgn, _, _ := createPolygonRgn.Call(uintptr(unsafe.Pointer(&moved[0])), uintptr(len(moved)), winding)
	extSelectClipRgn.Call(p.dc, rgn, mode)
	deleteObject.Call(rgn)
}
