package core

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The layout audit follows mygo-layout-audit (https://github.com/knothhe/mygo-layout-audit, MIT): its colors, its grid and its ruler.
var (
	auditMargin  = RGBA(0xf97316, 0.8)
	auditContent = RGBA(0x22c55e, 0.8)
	auditBorder  = RGBA(0x3b82f6, 0.9)
	auditGrid    = RGBA(0x269182, 0.16)
	auditRuler   = RGB(0xd8508a)
	auditText    = RGB(0xf2f2f2)
)

// auditKey shows and hides a layout audit that started showing: a chord apps seldom bind, as the app's newest binding of a key wins over it, unlike Mod+Shift+L, which NSGo's editor and gallery bind.
const auditKey = "mod+alt+shift+l"

const (
	// defaultAuditGrid is the grid's step in points when nothing set one, and minAuditGrid the smallest it takes.
	defaultAuditGrid Points = 8
	minAuditGrid     Points = 4
)

// LayoutAudit is what DevTools' layout audit shows as it starts (UpdateHighlights.Layout): every view's border, padding and margin boxes, and a grid of Grid points, at least 4, or none for 0. One that shows anything, as nsgo dev -layout's does, binds auditKey to show and hide the audit; the zero LayoutAudit shows nothing and binds no key, so the audit shows only when DevTools show it, with the boxes.
type LayoutAudit struct {
	Bounds bool
	Grid   Points
}

// layoutAudit is DevTools' layout audit, on their overlay (UpdateHighlights). It draws every view's border, padding and margin boxes as DevTools read them (devBox), each clipped where its scroll views and clipping views cut it, a grid and a two-point ruler, with a readout in the bottom start corner inside the safe area: buttons for the boxes, the grid and the ruler, the surface's size and the measurement. nsgo dev -layout shows it, auditKey shows and hides it then, and DevTools do (devtools.Surface.Layout). Its boxes and grid take no pointer; the ruler takes the presses while it measures, so none reach the app, and Escape and back leave it before the app hears them. Resizing the surface clears the ruler's points. It is hidden from assistive technology and out of the Tab order, and says no words: only numbers in points and icons.
type layoutAudit struct {
	owner                *Scope
	shown, bounds, ruler *Signal[bool]
	// grid is the grid's step, 0 for none, and step the last it had.
	grid *Signal[Points]
	step Points
	m    *Signal[measurement]
	// frames changes after each commit that changed the boxes, while a drawing reads it (bumpAudit).
	frames *Signal[uint64]
	// watch hears whether the audit shows, for DevTools (devtools.Surface.WatchLayout).
	watch func(on bool)
	// stopTick cancels the pending display frame callback while animations move boxes.
	stopTick func()
	boxes    []auditBox
}

// newLayoutAudit starts the audit as l says, under DevTools' overlay's scope cx, binding auditKey when it shows.
func newLayoutAudit(cx *Scope, l LayoutAudit) *layoutAudit {
	grid := Points(0)
	if l.Grid > 0 {
		grid = max(l.Grid, minAuditGrid)
	}
	a := &layoutAudit{owner: cx, shown: NewSignal(cx, l.Bounds || grid > 0), bounds: NewSignal(cx, l.Bounds), ruler: NewSignal(cx, false),
		grid: NewSignal(cx, grid), step: defaultAuditGrid, m: NewSignal(cx, measurement{}), frames: NewSignal(cx, uint64(0))}
	if grid > 0 {
		a.step = grid
	}
	if a.shown.GetUntracked() {
		cx.OnKey(auditKey, func() { a.show(!a.shown.GetUntracked()) })
	}
	return a
}

// show shows the audit or hides it, leaving the ruler. Shown with neither its boxes nor its grid on, it turns the boxes on.
func (a *layoutAudit) show(on bool) {
	if on == a.shown.GetUntracked() {
		return
	}
	a.owner.Batch(func() {
		if on && !a.bounds.GetUntracked() && a.grid.GetUntracked() == 0 {
			a.bounds.Set(true)
		}
		a.shown.Set(on)
		a.leaveRuler()
	})
	if !on {
		a.stopTicking()
	}
	if a.watch != nil {
		a.watch(on)
	}
}

// stopTicking cancels any pending animation frame callback for the audit.
func (a *layoutAudit) stopTicking() {
	if a.stopTick != nil {
		stop := a.stopTick
		a.stopTick = nil
		stop()
	}
}

// keepTicking schedules display frames while the audit's boxes are drawn and an
// animation, transition, layout animation, or keyframe is moving views.
func (a *layoutAudit) keepTicking(t *Tree) {
	if a.stopTick != nil || t.auditRead == nil || !t.auditRead() {
		return
	}
	if t.devDrawn == nil || !t.devDrawn.active(t) {
		return
	}
	a.stopTick = a.owner.RequestAnimationFrame(func(at time.Time) {
		a.stopTick = nil
		if t.auditRead == nil || !t.auditRead() {
			return
		}
		if t.devDrawn != nil && t.devDrawn.step(t, at) {
			a.frames.Set(a.frames.GetUntracked() + 1)
		}
		a.keepTicking(t)
	})
}

// keepAudit has t keep a's boxes up to date while a drawing reads them: auditRead reports whether one does, so a scroll commits, bumpAudit, after each round's OnLayout handlers, has a draw them again when a frame, a scroll or a deletion changed them since the last round looked, which the next round sends within the commit, and tickAudit keeps them redrawing while animations or transitions run. DevTools' overlay calls it as it mounts, so release builds link none of them.
func (t *Tree) keepAudit(a *layoutAudit) {
	t.auditRead = func() bool { return len(a.frames.subs.list) > 0 }
	t.bumpAudit = func() {
		if !t.framesMoved {
			return
		}
		t.framesMoved = false
		if t.auditRead() {
			t.within(t.root, func() { a.frames.Set(a.frames.GetUntracked() + 1) })
		}
	}
	t.tickAudit = func() { a.keepTicking(t) }
}

// auditBox is a view's box as the layout audit draws it: its drawn border box and edges, the part of the surface it shows in, and the transform mapping its local border box to the surface.
type auditBox struct {
	bounds, clip Bounds
	box          devtools.Box
	matrix       driver.Matrix3D
	size         Size
}

// auditBoxes appends the box of each view the tree shows, from the root view, as DevTools list them (devChildren). It leaves out hidden views and what they hold (Display None, Visibility Hidden and attachments a text's line limit cuts off), the views the core makes for itself, such as scroll bars' hosts, what the platform shows apart from the surface (an input's accessory, a screen header's views and a sheet it presents) and modals that went.
func (t *Tree) auditBoxes(out []auditBox) []auditBox {
	t.stepDevDrawn()
	surface := Bounds{Size: t.view.frame.Size}
	var visit func(n, parent *node, parentContentM driver.Matrix3D, clip Bounds, invisible bool)
	visit = func(n, parent *node, parentContentM driver.Matrix3D, clip Bounds, invisible bool) {
		if !audited(n) {
			return
		}
		// An overlay or a modal is its host's native child, outside what clips and hides the views around it.
		if n.hoist != nil {
			clip, invisible = surface, false
		}
		if np := n.nativeParent(); np != parent {
			parent = np
			if np == nil {
				parentContentM = driver.Identity3D
			} else {
				pm, _ := t.drawnMatrix(np)
				parentContentM = pm.Mul(scrollContentMatrix(np))
			}
		}
		// Visibility Hidden hides what a view holds too, as the native view does and as hit testing reads it.
		invisible = invisible || n.style.Visibility == VisibilityHidden
		m, size := t.drawnMatrixWith(n, parent, parentContentM)
		b := transformBounds(m, Bounds{Size: size})
		box := devBoxEdges(n)
		if !invisible {
			out = append(out, auditBox{bounds: b, clip: clip, box: box, matrix: m, size: size})
		}
		x, y := n.clips()
		inner := clipTo(clip, b, x, y)
		contentM := m.Mul(scrollContentMatrix(n))
		t.devChildren(n, func(c *node) { visit(c, n, contentM, inner, invisible) })
	}
	visit(t.view, nil, driver.Identity3D, surface, false)
	return out
}

// audited reports whether the layout audit may outline n and what it holds.
func audited(n *node) bool {
	switch {
	case n.hidden || n.style.Display == DisplayNone || n.attach != nil && n.attach.cut || n.accessory != nil || n.headerView:
		return false
	case n.parent != nil && n.parent.scroll != nil && n.parent.scroll.barHost == n:
		return false
	case n.hoist != nil && n.hoist.modal != nil && (n.hoist.dismissed || n.hoist.reported.GetUntracked() != nil):
		return false
	}
	return true
}

// clips reports on which axes n clips what it holds: a scroll view on both, else as its Overflow says.
func (n *node) clips() (x, y bool) {
	if n.scroll != nil {
		return true, true
	}
	s := &n.style
	clipping := func(o Overflow) bool { return o == Hidden || o == OverflowScroll }
	return clipping(cmpOr(s.OverflowX, s.Overflow)), clipping(cmpOr(s.OverflowY, s.Overflow))
}

// clipTo cuts clip to b on the axes x and y say.
func clipTo(clip, b Bounds, x, y bool) Bounds {
	cut := func(at, length, lo, size Points) (Points, Points) {
		start, end := max(at, lo), min(at+length, lo+size)
		return start, max(end-start, 0)
	}
	if x {
		clip.Origin.X, clip.Size.Width = cut(clip.Origin.X, clip.Size.Width, b.Origin.X, b.Size.Width)
	}
	if y {
		clip.Origin.Y, clip.Size.Height = cut(clip.Origin.Y, clip.Size.Height, b.Origin.Y, b.Size.Height)
	}
	return clip
}

// measurement is what the ruler picked: points counts them, 0 to 2, from start to end.
type measurement struct {
	start, end Point
	points     int
}

// horizontal and vertical are the distances between its points, never negative.
func (m measurement) horizontal() Points { return Points(math.Abs(float64(m.end.X - m.start.X))) }

func (m measurement) vertical() Points { return Points(math.Abs(float64(m.end.Y - m.start.Y))) }

// pick adds p, in surface points, to m, as upstream's ruler does: a first point starts a measurement, a second ends it and a third starts anew. A point that is not finite or lies outside viewport is ignored.
func (m measurement) pick(p Point, viewport Size) measurement {
	finite := func(v Points) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }
	if !finite(p.X) || !finite(p.Y) || p.X < 0 || p.Y < 0 || p.X > viewport.Width || p.Y > viewport.Height {
		return m
	}
	if m.points == 1 {
		return measurement{start: m.start, end: p, points: 2}
	}
	return measurement{start: p, end: p, points: 1}
}

var auditCover = Style{Position: Absolute, Top: Pt(0), Left: Pt(0), Width: Pct(100), Height: Pct(100)}

// auditHidden hides a view and what it holds from assistive technology.
func auditHidden() bool { return true }

// auditLayer is the audit on DevTools' overlay while it shows: a canvas with the drawing, which takes no pointer, and the readout, or while the ruler measures, an overlay above it taking every press.
func (t *Tree) auditLayer(a *layoutAudit) Node {
	return Show{When: a.shown.Get, Then: func(cx *Scope) Node {
		var seen Size
		cx.Effect(func() {
			size := cx.Env().Window()
			if seen != (Size{}) && size != seen {
				a.m.Set(measurement{})
			}
			seen = size
		})
		draw := func(d *Drawing, size Size) {
			a.boxes = a.boxes[:0]
			if a.bounds.Get() {
				a.frames.Get()
				a.boxes = t.auditBoxes(a.boxes)
				a.keepTicking(t)
			} else {
				a.stopTicking()
			}
			paintAudit(d, a.boxes, size, cx.Env().PixelRatio(), a.grid.Get(), a.m.Get())
		}
		return View{TestID: "nsgo-layout-audit", PointerEvents: PointerEventsBoxNone, AccessibilityElementsHidden: auditHidden, Style: auditCover, Children: []Node{
			Canvas{TestID: "nsgo-layout-canvas", Style: Style{Position: Absolute, Top: Pt(0), Left: Pt(0), Width: Pct(100), Height: Pct(100),
				PointerEvents: PointerEventsNone}, Draw: draw},
			Show{When: a.ruler.Get, Then: a.measuring, Else: a.readout},
		}}
	}}
}

// measuring is the ruler's overlay: what takes the presses that pick its points, under the readout. Escape and back leave it.
func (a *layoutAudit) measuring(cx *Scope) Node {
	cx.OnKeyWith("escape", KeyOptions{When: KeyAlways}, a.leaveRuler)
	return Overlay{Topmost: true, OnCancel: a.leaveRuler, Style: auditCover, Children: []Node{
		View{AccessibilityElementsHidden: auditHidden, Style: auditCover, Children: []Node{
			View{TestID: "nsgo-layout-ruler", NoFocus: true, Style: Style{Position: Absolute, Top: Pt(0), Left: Pt(0),
				Width: Pct(100), Height: Pct(100), Cursor: CursorCrosshair},
				OnPress: func(ev PressEvent) {
					viewport := Untracked(cx, cx.Env().Window)
					a.m.Set(a.m.GetUntracked().pick(ev.PagePosition, viewport))
				}},
			a.readout(cx),
		}},
	}}
}

// leaveRuler stops measuring and drops its points.
func (a *layoutAudit) leaveRuler() {
	a.ruler.Set(false)
	a.m.Set(measurement{})
}

// readout is the corner's chip: the boxes', the grid's and the ruler's buttons, the surface's size and the measurement.
func (a *layoutAudit) readout(cx *Scope) Node {
	text := Style{Color: auditText, FontSize: 11, FontVariant: FontTabularNums}
	return View{TestID: "nsgo-layout-readout",
		StyleFunc: func() Style {
			safe := cx.Env().SafeArea()
			return Style{Position: Absolute, Left: Pt(safe.Left + 8), Bottom: Pt(safe.Bottom + 8), Direction: Row,
				AlignItems: AlignCenter, Gap: 6, Padding: Symmetric(3, 6), CornerRadius: 6, Background: RGBA(0x111111, 0.8)}
		},
		Children: []Node{
			auditToggle("nsgo-layout-bounds", a.bounds.Get, func() { a.bounds.Set(!a.bounds.GetUntracked()) }, drawBoundsIcon),
			auditToggle("nsgo-layout-grid", func() bool { return a.grid.Get() > 0 }, a.toggleGrid, drawGridIcon),
			auditToggle("nsgo-layout-measure", a.ruler.Get, a.toggleRuler, drawRulerIcon),
			Text{Style: text, ContentFunc: func() string {
				w := cx.Env().Window()
				return auditNumber(w.Width) + " × " + auditNumber(w.Height)
			}},
			Show{When: func() bool { return a.m.Get().points == 2 }, Then: func(cx *Scope) Node {
				return Text{TestID: "nsgo-layout-measurement", Style: text.Merge(Style{Color: auditRuler}), ContentFunc: func() string {
					m := a.m.Get()
					return "↔ " + auditNumber(m.horizontal()) + "  ↕ " + auditNumber(m.vertical())
				}}
			}},
		}}
}

// auditToggle is a readout's button: an icon, drawn by icon, on a lighter ground while on says so.
func auditToggle(testID string, on func() bool, press func(), icon func(d *Drawing, size Size)) Node {
	return View{TestID: testID, NoFocus: true, OnPress: func(PressEvent) { press() },
		StyleFunc: func() Style {
			s := Style{Width: Pt(20), Height: Pt(20), CornerRadius: 4, AlignItems: AlignCenter, JustifyContent: JustifyCenter}
			if on() {
				s.Background = RGBA(0xffffff, 0.22)
			}
			return s
		},
		Children: []Node{Canvas{Style: Style{Width: Pt(14), Height: Pt(14)}, Draw: icon}}}
}

// toggleGrid turns the grid off, or on at its last step.
func (a *layoutAudit) toggleGrid() {
	if g := a.grid.GetUntracked(); g > 0 {
		a.step = g
		a.grid.Set(0)
		return
	}
	a.grid.Set(a.step)
}

// toggleRuler starts or stops measuring.
func (a *layoutAudit) toggleRuler() {
	if a.ruler.GetUntracked() {
		a.leaveRuler()
		return
	}
	a.ruler.Set(true)
}

// auditNumber writes v in points with one decimal, none when it is whole.
func auditNumber(v Points) string {
	return strings.TrimSuffix(strconv.FormatFloat(float64(v), 'f', 1, 32), ".0")
}

// paintAudit draws boxes, the grid of step grid (0 for none) and m over a surface of size at ratio device pixels a point. Boxes and the grid take one device pixel, inset half of one so it lands on the pixel grid.
func paintAudit(d *Drawing, boxes []auditBox, size Size, ratio float32, grid Points, m measurement) {
	if ratio <= 0 {
		ratio = 1
	}
	line := 1 / ratio
	d.SetLineWidth(line)
	surface := Bounds{Size: size}
	for _, b := range boxes {
		cut := b.clip != surface
		if cut {
			d.Save()
			d.BeginPath()
			d.Rect(float32(b.clip.Origin.X), float32(b.clip.Origin.Y), float32(b.clip.Size.Width), float32(b.clip.Size.Height))
			d.Clip()
		}
		base := Bounds{Size: b.size}
		if b.matrix == (driver.Matrix3D{}) {
			base = b.bounds
		}
		if b.box.Margin != [4]float32{} {
			strokeBox(d, b.matrix, growBounds(base, b.box.Margin, 1), auditMargin, line)
		}
		if b.box.Padding != [4]float32{} {
			var in [4]float32
			for i := range in {
				in[i] = b.box.Border[i] + b.box.Padding[i]
			}
			strokeBox(d, b.matrix, growBounds(base, in, -1), auditContent, line)
		}
		strokeBox(d, b.matrix, base, auditBorder, line)
		if cut {
			d.Restore()
		}
	}
	if grid > 0 {
		snap := func(v Points) float32 { return float32(math.Round(float64(v)*float64(ratio)))/ratio + line/2 }
		d.BeginPath()
		for x := grid; x < size.Width; x += grid {
			d.MoveTo(snap(x), 0)
			d.LineTo(snap(x), float32(size.Height))
		}
		for y := grid; y < size.Height; y += grid {
			d.MoveTo(0, snap(y))
			d.LineTo(float32(size.Width), snap(y))
		}
		d.SetStrokeColor(auditGrid)
		d.Stroke()
	}
	if m.points > 0 {
		paintRuler(d, m)
	}
}

// strokeBox strokes local mapped through m, using strokeInside for pure
// translations so untransformed boxes land on the pixel grid.
func strokeBox(d *Drawing, m driver.Matrix3D, local Bounds, color Color, line float32) {
	if isPureTranslation(m) {
		strokeInside(d, transformBounds(m, local), color, line)
		return
	}
	if local.Size.Width <= 0 || local.Size.Height <= 0 {
		return
	}
	x0, y0 := float64(local.Origin.X), float64(local.Origin.Y)
	x1, y1 := x0+float64(local.Size.Width), y0+float64(local.Size.Height)
	var pts [4][2]float64
	var minX, minY, maxX, maxY float64
	for i, c := range [4][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}} {
		px, py := m.Apply(c[0], c[1])
		if math.IsNaN(px) || math.IsNaN(py) || math.IsInf(px, 0) || math.IsInf(py, 0) {
			return
		}
		pts[i] = [2]float64{px, py}
		if i == 0 {
			minX, maxX, minY, maxY = px, px, py, py
		} else {
			minX, maxX = min(minX, px), max(maxX, px)
			minY, maxY = min(minY, py), max(maxY, py)
		}
	}
	if maxX-minX < float64(line) || maxY-minY < float64(line) {
		return
	}
	d.SetStrokeColor(color)
	d.BeginPath()
	d.MoveTo(float32(pts[0][0]), float32(pts[0][1]))
	d.LineTo(float32(pts[1][0]), float32(pts[1][1]))
	d.LineTo(float32(pts[2][0]), float32(pts[2][1]))
	d.LineTo(float32(pts[3][0]), float32(pts[3][1]))
	d.ClosePath()
	d.Stroke()
}

func isPureTranslation(m driver.Matrix3D) bool {
	if m == (driver.Matrix3D{}) || m == driver.Identity3D {
		return true
	}
	a, b, c, d, _, _, ok := m.Affine()
	return ok && a == 1 && b == 0 && c == 0 && d == 1
}

// strokeInside strokes the inside edge of b in color, lines wide.
func strokeInside(d *Drawing, b Bounds, color Color, line float32) {
	w, h := float32(b.Size.Width)-line, float32(b.Size.Height)-line
	if w < 0 || h < 0 {
		return
	}
	d.SetStrokeColor(color)
	d.StrokeRect(float32(b.Origin.X)+line/2, float32(b.Origin.Y)+line/2, w, h)
}

// growBounds moves b's edges out by sign times e, its top, right, bottom and left.
func growBounds(b Bounds, e [4]float32, sign Points) Bounds {
	top, right, bottom, left := sign*Points(e[0]), sign*Points(e[1]), sign*Points(e[2]), sign*Points(e[3])
	return Bounds{Origin: Point{X: b.Origin.X - left, Y: b.Origin.Y - top},
		Size: Size{Width: b.Size.Width + left + right, Height: b.Size.Height + top + bottom}}
}

// paintRuler draws m: a cross at each point, a line across then down between them, and the distances on pills beside the lines.
func paintRuler(d *Drawing, m measurement) {
	const arm = 5
	sx, sy := float32(m.start.X), float32(m.start.Y)
	ex, ey := float32(m.end.X), float32(m.end.Y)
	d.SetLineWidth(1)
	d.SetStrokeColor(auditRuler)
	d.BeginPath()
	cross := func(x, y float32) {
		d.MoveTo(x-arm, y)
		d.LineTo(x+arm, y)
		d.MoveTo(x, y-arm)
		d.LineTo(x, y+arm)
	}
	cross(sx, sy)
	if m.points == 2 {
		cross(ex, ey)
		d.MoveTo(sx, sy)
		d.LineTo(ex, sy)
		d.LineTo(ex, ey)
	}
	d.Stroke()
	if m.points < 2 {
		return
	}
	font := Font{Size: 11, Variant: FontTabularNums}
	// pill writes text on a pill whose middle is at y and whose left edge is at x, or whose middle is there too when centered.
	pill := func(text string, x, y float32, centered bool) {
		w := d.MeasureText(text, font) + 8
		if centered {
			x -= w / 2
		}
		d.SetFillColor(auditRuler)
		d.BeginPath()
		d.RoundRect(x, y-8, w, 16, 4)
		d.Fill()
		d.SetFillColor(RGB(0xffffff))
		d.SetTextAlign(TextAlignCenter)
		d.SetTextBaseline(BaselineMiddle)
		d.FillText(text, x+w/2, y, font)
	}
	if ex != sx {
		y := sy - 14
		if y < 8 {
			y = sy + 14
		}
		pill(auditNumber(m.horizontal()), (sx+ex)/2, y, true)
	}
	if ey != sy {
		pill(auditNumber(m.vertical()), ex+8, (sy+ey)/2, false)
	}
}

// drawBoundsIcon draws a box inside a box.
func drawBoundsIcon(d *Drawing, size Size) {
	w, h := float32(size.Width), float32(size.Height)
	d.SetLineWidth(1.25)
	d.SetStrokeColor(auditText)
	d.StrokeRect(0.75, 0.75, w-1.5, h-1.5)
	d.StrokeRect(4, 4, w-8, h-8)
}

// drawGridIcon draws a three-by-three grid.
func drawGridIcon(d *Drawing, size Size) {
	w, h := float32(size.Width), float32(size.Height)
	d.SetLineWidth(1.25)
	d.SetStrokeColor(auditText)
	d.BeginPath()
	for i := range 4 {
		x := 0.75 + (w-1.5)*float32(i)/3
		d.MoveTo(x, 0.75)
		d.LineTo(x, h-0.75)
		y := 0.75 + (h-1.5)*float32(i)/3
		d.MoveTo(0.75, y)
		d.LineTo(w-0.75, y)
	}
	d.Stroke()
}

// drawRulerIcon draws a measure across then down, with a tick at each end.
func drawRulerIcon(d *Drawing, size Size) {
	w, h := float32(size.Width), float32(size.Height)
	d.SetLineWidth(1.25)
	d.SetStrokeColor(auditText)
	d.BeginPath()
	d.MoveTo(1, 3)
	d.LineTo(w-3, 3)
	d.LineTo(w-3, h-1)
	d.MoveTo(1, 0.5)
	d.LineTo(1, 5.5)
	d.MoveTo(w-5.5, h-1)
	d.LineTo(w-0.5, h-1)
	d.Stroke()
}
