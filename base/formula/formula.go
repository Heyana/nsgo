// Package formula typesets math, written in TeX or Typst, with Untie (github.com/caelyreth/untie), and draws it as shapes in the font size and the text color it inherits: inline, on its text's baseline, or as a display formula of its own. Its font is New Computer Modern Math Book, embedded and loaded off the engine's goroutine as a formula first shows, or the app's through NewRenderer. textview.TextView draws its formulas so with RenderMath: formula.RenderMath.
package formula

import (
	"context"

	"github.com/caelyreth/untie"
	"github.com/caelyreth/untie/display"

	"github.com/nsgo-dev/nsgo/base/formula/locales"
	"github.com/nsgo-dev/nsgo/core"
)

// Dialect is the language a formula is written in.
type Dialect uint8

const (
	// TeX is LaTeX's math, as $…$ holds it; the zero value.
	TeX Dialect = iota
	// Typst is Typst's math mode.
	Typst
)

// HitPhase identifies the pointer interaction reported by HitEvent.
type HitPhase uint8

const (
	// HitHover reports pointer movement over the formula.
	HitHover HitPhase = iota
	// HitPress reports the primary press that starts a click.
	HitPress
	// HitClick reports the completed primary click at its press point.
	HitClick
)

// HitEvent carries the Untie source part and nearest caret under a formula's
// pointer. Diagnostics contains supplied diagnostics whose ranges cover that
// part, so an editor can route presses, caret placement and diagnostics without
// reproducing Untie's geometry.
type HitEvent struct {
	Source      string
	Dialect     Dialect
	Phase       HitPhase
	Hit         untie.Hit
	Found       bool
	Point       untie.Point
	Caret       untie.CaretStop
	HasCaret    bool
	Diagnostics []untie.Diagnostic
	Position    core.Point
	Clicks      int
	Modifiers   core.Modifiers
}

// View draws a formula. It sizes itself in ems of the font size it inherits, following the user's text size as text does, and paints in the text color it inherits, keeping the colors the formula names (\color{red}). As a core.Span's attachment it sits on the line's baseline. With Display it is a formula on a line of its own: larger operators, and limits above and below. What failed to parse is marked in ErrorStyle's color, the text color without one; a formula past Untie's limits shows its source as text in ErrorClass and ErrorStyle. It keeps its size in a container that stretches its children, standing at the start of the cross axis unless its style sets AlignSelf. Until its renderer's font has loaded, it is an empty em square. Assistive technology reads it as an image labelled with what the formula says, in the language the app shows.
type View struct {
	// Source is the formula without its delimiters ($…$); SourceFunc follows what it reads.
	Source     string
	SourceFunc func() string
	Dialect    Dialect
	Display    bool
	// Renderer lays it out; nil is Default().
	Renderer *Renderer
	// Label is what assistive technology reads in place of what the formula says.
	Label     string
	Class     string
	Style     core.Style
	StyleFunc func() core.Style
	// ErrorStyle's color marks what failed to parse; ErrorClass and ErrorStyle style the source a formula past Untie's limits, or whose fonts did not load, shows instead.
	ErrorClass string
	ErrorStyle core.Style
	// Diagnostics are source ranges an editor wants associated with the formula.
	// HitEvent reports the entries whose ranges cover the hit part.
	Diagnostics     []untie.Diagnostic
	DiagnosticsFunc func() []untie.Diagnostic
	// OnHit receives hover, press and click events resolved through Untie's Box.
	// It runs on the engine goroutine.
	OnHit  func(HitEvent)
	TestID string
}

// placeholder is a formula's size until its renderer's font has loaded: an em square, on the baseline as text's descent puts it.
var placeholder = core.EmBox{Width: 1, Ascent: 0.8, Descent: 0.2}

// Render implements core.Node.
func (v View) Render(cx *core.Scope) core.Node {
	r := v.Renderer
	if r == nil {
		r = Default()
	}
	source := func() string { return v.Source }
	if v.SourceFunc != nil {
		source = core.NewMemo(cx, v.SourceFunc).Get
	}
	// The font loads on another goroutine: parsing a large one takes long enough to drop frames.
	ready := func() (ok, failed bool) { return true, false }
	if !r.loaded.Load() {
		font := core.NewResource(cx, nil, func(context.Context, struct{}, core.ResourceInfo) (bool, error) {
			_, err := r.engine()
			return true, err
		})
		ready = func() (bool, bool) {
			_, ok := font.Get()
			return ok, font.Err() != nil
		}
	}
	lay := core.NewMemoFunc(cx, func() *layout {
		key := layoutKey{source: source(), dialect: v.Dialect, display: v.Display}
		ok, failed := ready()
		if failed {
			return &layout{key: key, failed: true}
		}
		return r.parsed(key, ok)
	}, func(a, b *layout) bool { return a == b })
	failed := core.NewMemo(cx, func() bool { return lay.Get().failed })
	msgs := func() locales.Messages { return locales.For(cx) }
	diagnostics := func() []untie.Diagnostic {
		if v.DiagnosticsFunc != nil {
			return v.DiagnosticsFunc()
		}
		return v.Diagnostics
	}
	// AlignSelf keeps a container's stretch from widening it, which its em box would follow in height too.
	base := core.Style{Fill: core.PaintCurrentColor, AlignSelf: core.AlignStart}.Merge(v.Style)
	return core.Show{
		When: failed.Get,
		Then: func(cx *core.Scope) core.Node {
			return core.Text{ContentFunc: source, Class: v.ErrorClass, Style: v.ErrorStyle, TestID: v.TestID}
		},
		Else: func(cx *core.Scope) core.Node {
			canvasSize := core.NewSignal(cx, core.Size{})
			var pressed core.Point
			pressedValid := false
			emit := func(phase HitPhase, point core.Point, clicks int, mods core.Modifiers) {
				if v.OnHit == nil {
					return
				}
				l := lay.Get()
				p, ok := formulaPoint(l.box, canvasSize.GetUntracked(), point)
				if !ok {
					return
				}
				hit, found := l.box.At(p)
				caret, hasCaret := l.box.NearestCaret(p)
				v.OnHit(HitEvent{Source: source(), Dialect: v.Dialect, Phase: phase, Hit: hit, Found: found,
					Point: p, Caret: caret, HasCaret: hasCaret, Diagnostics: diagnosticsAt(hit, found, diagnostics()),
					Position: point, Clicks: clicks, Modifiers: mods})
			}
			canvas := core.Canvas{
				Class:     v.Class,
				Style:     base,
				StyleFunc: v.StyleFunc,
				OnLayout:  func(b core.Bounds) { canvasSize.Set(b.Size) },
				TestID:    v.TestID,
				EmFunc: func() core.EmBox {
					if l := lay.Get(); l.drawn {
						return l.em
					}
					return placeholder
				},
				LabelFunc: func() string {
					if v.Label != "" {
						return v.Label
					}
					return speak(msgs(), lay.Get().description)
				},
				Draw: func(d *core.Drawing, size core.Size) {
					l := lay.Get()
					if !l.drawn || len(l.runs) == 0 {
						return
					}
					scale := float32(size.Height) / (l.em.Ascent + l.em.Descent)
					grid := display.PixelGrid{Scale: scale, PixelRatio: cx.Env().PixelRatio()}
					d.Scale(scale, scale)
					for _, run := range l.runs {
						fill(d, run.paint, v.ErrorStyle.Color)
						run.draw(d, grid)
					}
				},
			}
			if v.OnHit == nil {
				return canvas
			}
			return core.View{
				OnMouseMove: func(e core.MouseEvent) { emit(HitHover, e.Position, 0, e.Modifiers) },
				OnPressDown: func(e core.PressEvent) {
					pressed, pressedValid = e.Position, true
					emit(HitPress, e.Position, e.Clicks, e.Modifiers)
				},
				OnPress: func(e core.PressEvent) {
					point := e.Position
					if pressedValid {
						point = pressed
					}
					pressedValid = false
					emit(HitClick, point, e.Clicks, e.Modifiers)
				},
				Children: []core.Node{canvas},
			}
		},
	}
}

// fill sets the fill of what paint paints: an explicit color as it is, an error in errorColor, and the rest in the text color.
func fill(d *core.Drawing, paint display.Paint, errorColor core.Color) {
	switch {
	case paint.HasExplicit:
		c := paint.Explicit
		d.SetFillColor(core.RGBA(uint32(c.R)<<16|uint32(c.G)<<8|uint32(c.B), float32(c.A)/255))
	case paint.Role == display.RoleError && errorColor != 0 && errorColor != core.CurrentColor:
		d.SetFillColor(errorColor)
	default:
		d.FillFromView(core.PaintCurrentColor)
	}
}
