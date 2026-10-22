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
			in2 := in
			p.clipRound(p.m.bound(r), r, float64(diameter)/2, func(q *painter) {
				q.drawImage(v, s.TintColor, in2)
			})
		} else {
			p.drawImage(v, s.TintColor, in)
		}
	case core.KindCanvas:
		if v.drawing != nil {
			p.canvas(v, r)
		}
	}
	if v.text != nil {
		v.text.draw(p, w.px(x+v.content.Origin.X), w.px(y+v.content.Origin.Y))
	}
	if len(v.children) == 0 {
		return
	}
	draw := func(q *painter) {
		for _, c := range v.children {
			q.draw(c, x-v.offset.X, y-v.offset.Y)
		}
		if v.kind == core.KindScroll && !v.scroll.HideIndicator {
			q.drawIndicator(v, x, y)
		}
	}
	switch {
	case !s.Clip && v.kind != core.KindScroll:
		draw(p)
	case diameter > 0 && p.m.axisAligned():
		p.clipRound(p.m.bound(r), r, float64(diameter)/2, draw)
	default:
		saved, _, _ := saveDC.Call(p.dc)
		p.clip(r, diameter, rgnAnd)
		defer p.restore(saved)
		draw(p)
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
	l, lp, ok := p.newLayer(box)
	if !ok {
		fn(p)
		return
	}
	defer l.free()
	fn(lp)
	// GDI maps a bit block through the source DC's viewport too, which
	// moved to the layer's place in the window.
	setViewportOrgEx.Call(l.dc, 0, 0, 0)
	alpha := uintptr(math.Round(float64(opacity)*255)) << 16 // BLENDFUNCTION{AC_SRC_OVER, 0, alpha, 0}
	p.device(func() {
		alphaBlend.Call(p.dc, uintptr(box.Left), uintptr(box.Top), uintptr(l.width), uintptr(l.height),
			l.dc, 0, 0, uintptr(l.width), uintptr(l.height), alpha)
	})
}

// newLayer copies what is behind box, in the target's pixels, into a
// bitmap of its own, and returns it with a painter drawing into it.
func (p *painter) newLayer(box rect) (*bitmap, *painter, bool) {
	l, ok := newBitmap(p.dc, box.Right-box.Left, box.Bottom-box.Top)
	if !ok {
		return nil, nil, false
	}
	p.device(func() {
		bitBlt.Call(l.dc, 0, 0, uintptr(l.width), uintptr(l.height),
			p.dc, uintptr(box.Left), uintptr(box.Top), srcCopy)
	})
	lp := &painter{w: p.w, target: l, dc: l.dc, ox: p.ox + box.Left, oy: p.oy + box.Top, m: p.m, solid: p.solid}
	setBkMode.Call(l.dc, transparent)
	setViewportOrgEx.Call(l.dc, uintptr(-lp.ox), uintptr(-lp.oy), 0)
	if lp.m != identity {
		setGraphicsMode.Call(l.dc, gmAdvanced)
		x := lp.m.xform()
		setWorldTransform.Call(l.dc, uintptr(unsafe.Pointer(&x)))
	}
	return l, lp, true
}

// clipRound draws what fn paints through r's rounded corners, so that a
// clipped view's edge blends into what lies outside it instead of
// stepping a whole pixel.
//
// The subtree draws inside r as usual, clipped to a rectangle. What the
// corner squares held is kept first, and blended with what the subtree
// drew there afterwards: the corners are the only pixels whose coverage
// is fractional, so they are the only ones that need it.
func (p *painter) clipRound(box rect, r rect, radius float64, fn func(*painter)) {
	box = box.offset(-p.ox, -p.oy).intersect(p.clipBox())
	if radius <= 0 || !p.m.axisAligned() || box.empty() {
		p.clipped(r, radius, fn)
		return
	}
	corners, _ := parts(r, radius)
	var squares [4]rect
	var behind [4]*bitmap
	defer func() {
		for _, b := range behind {
			if b != nil {
				b.free()
			}
		}
	}()
	for i, c := range corners {
		sq := p.m.bound(c).offset(-p.ox, -p.oy).intersect(box)
		if sq.empty() {
			continue
		}
		b, ok := newBitmap(p.dc, sq.Right-sq.Left, sq.Bottom-sq.Top)
		if !ok {
			p.clipped(r, radius, fn)
			return
		}
		squares[i], behind[i] = sq, b
		p.device(func() {
			bitBlt.Call(b.dc, 0, 0, uintptr(b.width), uintptr(b.height),
				p.dc, uintptr(sq.Left), uintptr(sq.Top), srcCopy)
		})
	}
	saved, _, _ := saveDC.Call(p.dc)
	p.clip(r, 0, rgnAnd)
	fn(p)
	p.restore(saved)
	p.repairCorners(squares, behind, shape{r: r, radius: radius})
}

// clipped draws through fn with r's rounded region as the clipping, which
// GDI can only do a whole pixel at a time.
func (p *painter) clipped(r rect, radius float64, fn func(*painter)) {
	saved, _, _ := saveDC.Call(p.dc)
	p.clip(r, int32(math.Round(2*radius)), rgnAnd)
	fn(p)
	p.restore(saved)
}

// repairCorners blends the corner squares' pixels from what they held
// before the subtree drew there and what it drew, by how much of each
// pixel the shape covers.
func (p *painter) repairCorners(squares [4]rect, behind [4]*bitmap, s shape) {
	gdiFlush.Call() // the pixels may still be in a batched GDI call
	smp := p.sampler()
	clips := p.clipRects()
	target := unsafe.Slice((*uint32)(p.target.bits), int(p.target.width)*int(p.target.height))
	for i, sq := range squares {
		b := behind[i]
		if b == nil {
			continue
		}
		was := unsafe.Slice((*uint32)(b.bits), int(b.width)*int(b.height))
		for _, clip := range clips {
			r := sq.intersect(clip)
			for y := r.Top; y < r.Bottom; y++ {
				row := target[int(y)*int(p.target.width):]
				for x := r.Left; x < r.Right; x++ {
					if cov := smp.cover(s, x, y); cov < 255 {
						row[x] = mixPixels(was[int(y-sq.Top)*int(b.width)+int(x-sq.Left)], row[x], uint32(cov))
					}
				}
			}
		}
	}
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
	radius := float64(diameter) / 2
	if c := s.Background; c&0xff != 0 {
		p.paintShape(r, radius, c)
	}
	b := p.w.borderWidths(s)
	if b == (rect{}) {
		return
	}
	colors := [4]core.Color{s.BorderColor, s.BorderColor, s.BorderColor, s.BorderColor}
	if s.Borders != (core.Edges{}) {
		c := s.BorderColors
		colors = [4]core.Color{c.Top, c.Right, c.Bottom, c.Left}
	}
	p.paintRing(r, radius, b, colors)
}

// paintShape fills r and its rounded corners with c.
func (p *painter) paintShape(r rect, radius float64, c core.Color) {
	if radius <= 0 {
		p.fillRects([]rect{r}, c)
		return
	}
	if p.m != identity {
		// A transform leaves no straight edge in the target's pixels, so
		// every pixel is measured.
		p.paintCoverage([]rect{r}, shape{r: r, radius: radius}, c)
		return
	}
	corners, middle := parts(r, radius)
	p.paintCoverage(corners[:], shape{r: r, radius: radius}, c)
	p.fillRects(middle[:], c)
}

// paintRing paints a border inside r: the ring between r and the inner
// rectangle the sides' widths leave, in one color or in a color per side.
func (p *painter) paintRing(r rect, radius float64, b rect, colors [4]core.Color) {
	in := rect{
		min(r.Left+b.Left, r.Right), min(r.Top+b.Top, r.Bottom),
		max(r.Right-b.Right, r.Left), max(r.Bottom-b.Bottom, r.Top),
	}
	inRadius := max(radius-float64(b.Left+b.Top+b.Right+b.Bottom)/4, 0)
	uniform := colors[0] == colors[1] && colors[0] == colors[2] && colors[0] == colors[3]
	if radius <= 0 || (p.m != identity && !uniform) {
		// A rotated border with a color per side has no edge GDI can fill
		// exactly.
		p.ringGDI(r, radius, b, in, inRadius, colors, uniform)
		return
	}
	s := shape{r: r, radius: radius, hole: in, holeRadius: inRadius}
	if p.m != identity {
		// A transform leaves no straight edge in the target's pixels, so
		// every pixel is measured.
		p.paintCoverage([]rect{r}, s, colors[0])
		return
	}
	corners, middle := parts(r, radius)
	if uniform {
		p.paintCoverage(corners[:], s, colors[0])
	} else {
		// A corner belongs to the two sides that meet there, split down
		// the diagonal from the corner: the top left one to the top and
		// the left, the top right one to the top and the right, and so on.
		for i, pair := range [4][2]int{{1, 4}, {1, 2}, {3, 4}, {3, 2}} {
			ca, cb := colors[pair[0]-1], colors[pair[1]-1]
			if ca&0xff == 0 && cb&0xff == 0 || corners[i].empty() {
				continue
			}
			a, b := s, s
			a.side, b.side = uint8(pair[0]), uint8(pair[1])
			p.paintCorner(corners[i], a, b, ca, cb)
		}
	}
	// The straight parts, one fill per color.
	var bands [4][]rect
	for _, piece := range middle {
		for _, part := range sub(piece, in) {
			if i := sideOf(part, in) - 1; colors[i]&0xff != 0 {
				bands[i] = append(bands[i], part)
			}
		}
	}
	for i, rects := range bands {
		p.fillRects(rects, colors[i])
	}
}

// ringGDI paints a border through GDI, for the shapes coverage cannot
// draw: rotated ones, and rings with a color per side that a transform
// applies to.
func (p *painter) ringGDI(r rect, radius float64, b, in rect, inRadius float64, colors [4]core.Color, uniform bool) {
	diameter := int32(math.Round(2 * radius))
	if !uniform || b.Left != b.Top || b.Top != b.Right || b.Right != b.Bottom {
		p.sides(r, diameter, b, colors)
		return
	}
	c := colors[0]
	if c&0xff != 0xff {
		p.blend(r, diameter, in, int32(math.Round(2*inRadius)), c)
		return
	}
	width := max(b.Top, 1)
	pen, _, _ := createPen.Call(psInsideFrame, uintptr(width), colorRef(c))
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

// drawIndicator paints a scroll bar's thumb along v's scrolling edge
// while it shows; x and y are v's origin in the window.
func (p *painter) drawIndicator(v *view, x, y core.Points) {
	thumb, ok := v.bar().Thumb()
	if !ok || !p.w.showsBar(v) {
		return
	}
	bar := p.w.rect(x+thumb.Origin.X, y+thumb.Origin.Y, thumb.Size)
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
func (p *painter) sides(r rect, diameter int32, b rect, colors [4]core.Color) {
	in := rect{min(r.Left+b.Left, r.Right), min(r.Top+b.Top, r.Bottom), max(r.Right-b.Right, r.Left), max(r.Bottom-b.Bottom, r.Top)}
	inDiameter := max(diameter-(b.Left+b.Top+b.Right+b.Bottom)/2, 0)
	for _, side := range []struct {
		width int32
		color core.Color
		shape [4]point
	}{
		{b.Top, colors[0], [4]point{{r.Left, r.Top}, {r.Right, r.Top}, {in.Right, in.Top}, {in.Left, in.Top}}},
		{b.Right, colors[1], [4]point{{r.Right, r.Top}, {r.Right, r.Bottom}, {in.Right, in.Bottom}, {in.Right, in.Top}}},
		{b.Bottom, colors[2], [4]point{{r.Right, r.Bottom}, {r.Left, r.Bottom}, {in.Left, in.Bottom}, {in.Right, in.Bottom}}},
		{b.Left, colors[3], [4]point{{r.Left, r.Bottom}, {r.Left, r.Top}, {in.Left, in.Top}, {in.Left, in.Bottom}}},
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
