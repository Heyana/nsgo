// Package diagram draws Mermaid diagrams natively, with limu (github.com/firede/limu), which reads every diagram type Pele, the Obsidian team's Mermaid renderer, draws and lays it out: flowcharts, sequence, class, state and entity relationship diagrams, Gantt charts, pies, mind maps, timelines and more. It draws the laid-out scene on a canvas in the platform's font, which it measures the labels in too, so text fills the room the layout gave it. textview.TextView draws its mermaid code blocks so with RenderDiagram: diagram.RenderDiagram.
package diagram

import (
	"errors"
	"sync"

	"github.com/firede/limu"
	ldiagram "github.com/firede/limu/diagram"
	"github.com/firede/limu/layout"
	"github.com/firede/limu/scene"
	"github.com/firede/limu/style"

	"github.com/nsgo-dev/nsgo/base/diagram/locales"
	"github.com/nsgo-dev/nsgo/base/internal/diagramlayout"
	"github.com/nsgo-dev/nsgo/core"
)

// View draws a Mermaid diagram laid out to the width its container gives it, turning a wide flowchart to run down where it fits better. Labels use the platform's font at FontSize, scaled by the user's text size setting as text is. It colors itself with the appearance's theme, gray with a blue accent in light and in dark, and with what Theme sets over it. A diagram that does not parse, or cannot be laid out, shows its source as text in ErrorClass and ErrorStyle. Assistive technology reads it as an image, named by Label, else by the diagram's accTitle and accDescr, else as a diagram, in the language the app shows.
type View struct {
	// Source is the diagram's Mermaid text, as a mermaid code block holds it; SourceFunc follows what it reads.
	Source     string
	SourceFunc func() string
	// FontSize is the size of labels with no size of their own; 0 is 16.
	FontSize core.Points
	// Theme colors the diagram over the appearance's theme: Pele's light theme in light, and its gray counterpart in dark, with the same blue accent as NSGo's own look.
	Theme *Theme
	// ThemeFunc follows the colors it reads. Returning nil keeps the appearance's defaults. Theme and ThemeFunc are mutually exclusive.
	ThemeFunc func() *Theme
	// Label is what assistive technology reads in place of the diagram's title and description.
	Label     string
	Class     string
	Style     core.Style
	StyleFunc func() core.Style
	// ErrorClass and ErrorStyle style the source a diagram that fails shows instead.
	ErrorClass string
	ErrorStyle core.Style
	TestID     string
}

// Theme is a diagram's colors, as limu's themes name them, and its corners' radius and lines' width, in points. What it leaves zero keeps the appearance's theme, so a theme may change only the colors it names.
type Theme struct {
	Background core.Color // behind the diagram, and behind labels on lines
	Text       core.Color
	Muted      core.Color // secondary text, as the titles of groups
	Line       core.Color // edges and other lines
	Surface    core.Color // the inside of shapes
	SurfaceAlt core.Color // the inside of groups, and of shapes set apart
	Border     core.Color // the outlines of shapes
	Accent     core.Color // what a diagram marks out
	Critical   core.Color // what a diagram marks as critical
	// Series are the colors of a chart's series, as a pie's slices, in turn; a series left zero keeps the appearance's.
	Series      [8]core.Color
	Radius      float32
	StrokeWidth float32
}

// look is the limu look a diagram paints with: the appearance's theme, with what t sets over it.
func (t *Theme) look(dark bool) *style.Look {
	th := defaultTheme(dark)
	if t == nil {
		return &style.Look{Theme: th}
	}
	set := func(dst *style.Color, c core.Color) {
		if c != 0 {
			*dst = limuColor(c)
		}
	}
	set(&th.Bg, t.Background)
	set(&th.Fg, t.Text)
	set(&th.Muted, t.Muted)
	set(&th.Line, t.Line)
	set(&th.Surface, t.Surface)
	set(&th.SurfaceAlt, t.SurfaceAlt)
	set(&th.Border, t.Border)
	set(&th.Accent, t.Accent)
	set(&th.Critical, t.Critical)
	for i, c := range t.Series {
		set(&th.Series[i], c)
	}
	if t.Radius > 0 {
		th.Radius = float64(t.Radius)
	}
	if t.StrokeWidth > 0 {
		th.StrokeWidth = float64(t.StrokeWidth)
	}
	return &style.Look{Theme: th}
}

// defaultFontSize is limu's size of labels with no size of their own.
const defaultFontSize = 16

// laidOut is a diagram laid out, or why it could not be.
type laidOut struct {
	scene *scene.Scene
	err   error
}

// layoutKey is what a diagram's layout depends on.
type layoutKey struct {
	source string
	size   float64
	width  float64
}

// layouts keeps diagrams laid out by source, font size and available width, so a diagram a list shows again, or one each copy of a chat shows, is laid out once at each width. A layout measures with the platform's text engine, which every diagram of the process shares.
var layouts = struct {
	sync.Mutex
	m map[layoutKey]laidOut
}{m: map[layoutKey]laidOut{}}

// maxLayouts is how many layouts layouts keeps before it lets them all go.
const maxLayouts = 256

// lay parses and lays out source at size within width, measuring labels with m; zero width leaves it unbounded.
func lay(source string, size, width float64, m *measurer) laidOut {
	key := layoutKey{source: source, size: size, width: width}
	layouts.Lock()
	l, ok := layouts.m[key]
	layouts.Unlock()
	if ok {
		return l
	}
	d, err := limu.Parse(source, ldiagram.Options{})
	if err == nil {
		l.scene, err = d.Layout(&layout.Options{Measurer: m, FontSize: size, MaxWidth: width})
	}
	l.err = err
	layouts.Lock()
	if len(layouts.m) >= maxLayouts {
		clear(layouts.m)
	}
	layouts.m[key] = l
	layouts.Unlock()
	return l
}

// errEmpty is the error of a diagram with no source.
var errEmpty = errors.New("diagram: no source")

// Render implements core.Node.
func (v View) Render(cx *core.Scope) core.Node {
	source := func() string { return v.Source }
	if v.SourceFunc != nil {
		source = core.NewMemo(cx, v.SourceFunc).Get
	}
	size := v.FontSize
	if size <= 0 {
		size = defaultFontSize
	}
	m := newMeasurer(cx)
	env := cx.Env()
	width := core.NewSignal(cx, core.Points(0))
	viewport := diagramlayout.Width.Use(cx)
	styles := func() core.Style {
		s := v.Style
		if v.StyleFunc != nil {
			s = s.Merge(v.StyleFunc())
		}
		return s
	}
	layoutAt := func(width core.Points) laidOut {
		src := source()
		if src == "" {
			return laidOut{err: errEmpty}
		}
		return lay(src, float64(size)*float64(env.FontScale()), float64(width), m)
	}
	equal := func(a, b laidOut) bool { return a.scene == b.scene && a.err == b.err }
	natural := core.NewMemoFunc(cx, func() laidOut { return layoutAt(0) }, equal)
	laid := core.NewMemoFunc(cx, func() laidOut {
		w := width.Get()
		if viewport != nil && viewport() > 0 {
			available := viewport()
			w = available
			// An attachment's containing block is unbounded, so resolve the app's width constraints against its actual viewport.
			s := styles()
			if !s.MaxWidth.IsAuto() {
				w = s.MaxWidth.Resolve(available)
			}
			if !s.Width.IsAuto() {
				w = min(w, s.Width.Resolve(available))
			}
			if !s.MinWidth.IsAuto() {
				w = max(w, s.MinWidth.Resolve(available))
			}
		}
		return layoutAt(max(0, w))
	}, equal)
	failed := core.NewMemo(cx, func() bool { return natural.Get().err != nil || laid.Get().err != nil })
	msgs := func() locales.Messages { return locales.For(cx) }
	return core.Show{
		When: failed.Get,
		Then: func(cx *core.Scope) core.Node {
			return core.Text{ContentFunc: source, Class: v.ErrorClass, Style: v.ErrorStyle, TestID: v.TestID}
		},
		Else: func(cx *core.Scope) core.Node {
			return core.Canvas{
				Class:    v.Class,
				OnLayout: func(b core.Bounds) { width.Set(b.Size.Width) },
				StyleFunc: func() core.Style {
					s := styles()
					// Keep the unbounded preferred width so widening a container can restore the original direction after a narrow layout.
					sc := laid.Get().scene
					most := core.Pct(100)
					preferred := core.Points(natural.Get().scene.Width)
					if viewport != nil && viewport() > 0 {
						// An attachment has an independent viewport signal, so its intrinsic width can follow the fitted scene. A scene limu cannot fit still has room to scroll.
						preferred = core.Points(sc.Width)
						most = core.Pt(max(viewport(), preferred))
						for _, length := range []*core.Length{&s.Width, &s.MinWidth, &s.MaxWidth} {
							if _, percent := length.Percent(); percent {
								*length = core.Pt(length.Resolve(viewport()))
							}
						}
					}
					return core.Style{Width: core.Pt(preferred), MaxWidth: most,
						Height: core.Pt(core.Points(sc.Height)), AlignSelf: core.AlignStart}.Merge(s)
				},
				TestID: v.TestID,
				LabelFunc: func() string {
					if v.Label != "" {
						return v.Label
					}
					return label(msgs(), laid.Get().scene)
				},
				Draw: func(d *core.Drawing, sz core.Size) {
					sc := laid.Get().scene
					if sc == nil || sc.Width <= 0 || sc.Height <= 0 {
						return
					}
					// Reflow keeps labels in their measured font size. Any room left after fitting centers the scene without stretching it.
					d.Translate(max(0, (float32(sz.Width)-float32(sc.Width))/2), 0)
					d.Scale(1, float32(sz.Height)/float32(sc.Height))
					theme := v.Theme
					if v.ThemeFunc != nil {
						theme = v.ThemeFunc()
					}
					paint(d, sc, theme.look(env.Dark()), m)
				},
			}
		},
	}
}

// label is what assistive technology reads for a scene: its accTitle and accDescr, else what a diagram is called.
func label(msgs locales.Messages, sc *scene.Scene) string {
	switch {
	case sc == nil || sc.Title == "" && sc.Desc == "":
		return msgs.Diagram()
	case sc.Desc == "":
		return sc.Title
	case sc.Title == "":
		return sc.Desc
	}
	return msgs.Described(sc.Title, sc.Desc)
}

// defaultTheme is the appearance's theme, a new one each call for look to change: Pele's light theme, limu's Default, in light, and in dark its gray counterpart, the same family with the same hues lightened to read on a dark ground, as NSGo's own look is gray with a blue accent. limu's KohuDark is another family, tinted teal with a pink accent.
func defaultTheme(dark bool) *style.Theme {
	if !dark {
		return style.Default()
	}
	return &style.Theme{
		Bg:         style.Hex("#1f1f1f"),
		Fg:         style.Hex("#ececec"),
		Muted:      style.Hex("#a0a0a0"),
		Line:       style.Hex("#d6d6d6"),
		Surface:    style.Hex("#2c2c2c"),
		SurfaceAlt: style.Hex("#383838"),
		Border:     style.Hex("#8f8f8f"),
		Accent:     style.Hex("#8aa4ea"),
		Critical:   style.Hex("#f07c7b"),
		Series: [8]style.Color{
			style.Hex("#6f9fd8"), style.Hex("#ffa04a"), style.Hex("#7cc66e"), style.Hex("#f07c7b"),
			style.Hex("#8fd3cd"), style.Hex("#f5d75c"), style.Hex("#cf9ac2"), style.Hex("#c09a83"),
		},
		Radius:      4,
		StrokeWidth: 1,
	}
}
