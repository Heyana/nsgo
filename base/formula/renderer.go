package formula

import (
	"container/list"
	"context"
	_ "embed"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/caelyreth/untie"
	"github.com/caelyreth/untie/display"
	"github.com/caelyreth/untie/render"

	"github.com/nsgo-dev/nsgo/core"
)

// newCMMath is New Computer Modern Math Book, unmodified (fonts/LICENSE).
//
//go:embed fonts/NewCMMath-Book.otf
var newCMMath []byte

// Fonts are a renderer's fonts, OpenType files: Math, with a MATH table, lays the formulas out; Text, else Math, sets \text and \operatorname; and Fallbacks are tried in order for characters the others lack, such as a CJK font's. The app is responsible for their licenses.
type Fonts struct {
	Math      []byte
	Text      []byte
	Fallbacks [][]byte
}

// Renderer lays formulas out in one set of fonts and keeps the last layouts it made, as shapes ready to draw. It is safe for concurrent use, so the engines of a process share one.
type Renderer struct {
	fonts  Fonts
	engine func() (*untie.Engine, error)
	// loaded is set once engine has returned.
	loaded atomic.Bool

	mu      sync.Mutex
	entries map[layoutKey]*list.Element
	order   list.List // of *layout, the most recent first
}

// cacheSize is how many layouts a renderer keeps.
const cacheSize = 256

// referenceSize is the font size formulas are laid out at, which drawing scales to the font size a formula inherits: layout is linear in the size.
const referenceSize = 100

// NewRenderer prepares a renderer for fonts, parsing and validating them, which takes a while for a large font: about 120 ms for New Computer Modern Math on an M-series Mac, and about 0.7 s in WebAssembly. It fails when Math has no usable MATH table.
func NewRenderer(fonts Fonts) (*Renderer, error) {
	r := newRenderer(fonts)
	if _, err := r.engine(); err != nil {
		return nil, err
	}
	return r, nil
}

func newRenderer(fonts Fonts) *Renderer {
	r := &Renderer{fonts: fonts, entries: map[layoutKey]*list.Element{}}
	r.engine = sync.OnceValues(func() (*untie.Engine, error) {
		defer r.loaded.Store(true)
		if len(fonts.Math) == 0 {
			return nil, errors.New("formula: no math font")
		}
		config := untie.FontSetConfig{Math: untie.FontFace{Bytes: fonts.Math}}
		if len(fonts.Text) > 0 {
			config.Text = &untie.FontFace{Bytes: fonts.Text}
		}
		for _, f := range fonts.Fallbacks {
			config.Fallbacks = append(config.Fallbacks, untie.FontFace{Bytes: f})
		}
		set, err := untie.LoadFontSet(config)
		if err != nil {
			return nil, err
		}
		return untie.NewEngine(set, untie.Limits{})
	})
	return r
}

var defaultRenderer = sync.OnceValue(func() *Renderer { return newRenderer(Fonts{Math: newCMMath}) })

// Default returns the renderer of the embedded New Computer Modern Math Book, whose font loads on first use: a View waits for it off the engine's goroutine.
func Default() *Renderer { return defaultRenderer() }

// layoutKey is what a layout is made from.
type layoutKey struct {
	source  string
	dialect Dialect
	display bool
}

// layout is a formula laid out: its size in ems and its shapes, in ems from its top left corner, in runs of one paint.
type layout struct {
	key layoutKey
	em  core.EmBox
	box *untie.Box
	// description is what assistive technology reads, nil when the formula has none.
	description *display.AltDescription
	runs        []paintRun
	// failed is set when the formula could not be laid out at all, past its limits, or its renderer's fonts did not load: it shows its source.
	failed bool
	// drawn is set once runs and em are made, with the renderer's fonts.
	drawn bool
}

// paintRun is a glyph path or a rule, filled in one paint. Rules keep their
// snapping axis until drawing knows the final font size and display density.
type paintRun struct {
	paint display.Paint
	path  *core.Path
	rect  untie.Rect
	snap  display.SnapAxis
}

func (r paintRun) draw(d *core.Drawing, grid display.PixelGrid) {
	if r.path != nil {
		d.FillPath(r.path, core.NonZero)
		return
	}
	b := grid.SnapRect(r.rect, r.snap)
	d.FillRect(b.X, b.Y, b.Width, b.Height)
}

// parsed returns the layout of key with its description, its shapes made once the renderer's fonts have loaded and draw is set.
func (r *Renderer) parsed(key layoutKey, draw bool) *layout {
	r.mu.Lock()
	el, ok := r.entries[key]
	if ok {
		r.order.MoveToFront(el)
	}
	r.mu.Unlock()
	var l *layout
	if ok {
		l = el.Value.(*layout)
		if l.drawn || !draw {
			return l
		}
	}
	// Made outside the lock: another goroutine may make the same, and the last stays.
	l = r.make(key, draw)
	r.mu.Lock()
	defer r.mu.Unlock()
	if el, ok := r.entries[key]; ok {
		el.Value = l
		r.order.MoveToFront(el)
	} else {
		r.entries[key] = r.order.PushFront(l)
		for r.order.Len() > cacheSize {
			last := r.order.Back()
			delete(r.entries, last.Value.(*layout).key)
			r.order.Remove(last)
		}
	}
	return l
}

// make parses key's formula and, with draw, lays it out.
func (r *Renderer) make(key layoutKey, draw bool) *layout {
	dialect, mode := untie.TeX, untie.Inline
	if key.dialect == Typst {
		dialect = untie.Typst
	}
	if key.display {
		mode = untie.Display
	}
	expr := untie.Parse(key.source, dialect, mode, untie.ParseOptions{})
	l := &layout{key: key, description: expr.Alt().Description}
	if !draw {
		return l
	}
	l.drawn = true
	engine, err := r.engine()
	if err != nil {
		l.failed = true
		return l
	}
	box, err := engine.LayoutContextWithOptions(context.Background(), expr, untie.Style{Size: referenceSize},
		untie.LayoutOptions{Recovery: untie.RecoveryVisible})
	if err != nil || box == nil {
		l.failed = true
		return l
	}
	l.box = box
	// The canvas holds the ink that reaches past the box (Box.Overflow) too: canvases clip on the web and Linux, so a narrower canvas would cut it.
	b := box.Bounds()
	l.em = core.EmBox{Width: b.Width / referenceSize, Ascent: -b.Y / referenceSize, Descent: (b.Height + b.Y) / referenceSize}
	at := func(x, y float32) (float32, float32) { return (x - b.X) / referenceSize, (y - b.Y) / referenceSize }
	var run *paintRun
	err = render.Walk(box, func(c render.Command) error {
		if c.Kind == render.CommandRule {
			x, y := at(c.Rect.X, c.Rect.Y)
			l.runs = append(l.runs, paintRun{paint: c.Paint,
				rect: untie.Rect{X: x, Y: y, Width: c.Rect.Width / referenceSize, Height: c.Rect.Height / referenceSize}, snap: c.Snap})
			run = nil
			return nil
		}
		if run == nil || run.paint != c.Paint {
			l.runs = append(l.runs, paintRun{paint: c.Paint, path: new(core.Path)})
			run = &l.runs[len(l.runs)-1]
		}
		p := run.path
		point := func(q untie.Point) (float32, float32) { return at(c.At.X+q.X*c.Em, c.At.Y+q.Y*c.Em) }
		for _, s := range c.Path.Segments {
			switch s.Op {
			case untie.SegMoveTo:
				p.MoveTo(point(s.Points[0]))
			case untie.SegLineTo:
				p.LineTo(point(s.Points[0]))
			case untie.SegQuadTo:
				cx, cy := point(s.Points[0])
				x, y := point(s.Points[1])
				p.QuadTo(cx, cy, x, y)
			case untie.SegCubeTo:
				x1, y1 := point(s.Points[0])
				x2, y2 := point(s.Points[1])
				x, y := point(s.Points[2])
				p.CubicTo(x1, y1, x2, y2, x, y)
			case untie.SegClose:
				p.ClosePath()
			}
		}
		return nil
	})
	if err != nil {
		l.failed = true
	}
	return l
}
