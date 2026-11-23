//go:build js && wasm && nsgoautomation

package miniprogram

import (
	"encoding/base64"
	"errors"
	"math"
	"strings"
	"sync"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvas2d"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

var (
	liveMu      sync.Mutex
	liveCurrent *surface
)

func init() { testhost.RegisterLive(liveSurfaceFor) }

func (s *surface) registerAutomation() {
	liveMu.Lock()
	liveCurrent = s
	liveMu.Unlock()
}

// liveSurfaceFor returns the running mini program's live surface for surface 0 of its one graph, or nil for another graph or surface or before Run has started the engine.
func liveSurfaceFor(graph int, id uint32) testhost.Surface {
	if id != 0 {
		return nil
	}
	liveMu.Lock()
	s := liveCurrent
	liveMu.Unlock()
	if s == nil || s.engine == nil || s.engine.Graph() != graph {
		return nil
	}
	return &liveSurface{
		m: s.m,
		run: func(fn func()) {
			s.mu.Lock()
			defer s.mu.Unlock()
			fn()
		},
		syncFn:     func() { s.engine.Sync() },
		screenshot: s.capturePNG,
	}
}

// capturePNG renders the mounted view tree onto an offscreen 2D canvas (drawing backgrounds, borders, clipping, text runs, inputs, canvases and modals) and decodes its PNG data URL.
func (s *surface) capturePNG() (data []byte, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			data, err = nil, errors.New("offscreen screenshot failed")
		}
	}()
	if !s.api.Truthy() || s.api.Get("createOffscreenCanvas").Type() != js.TypeFunction {
		return nil, errors.New("no offscreen canvas")
	}
	size := s.m.viewport
	if root := s.m.view(s.m.root); root != nil && (size.Width <= 0 || size.Height <= 0) {
		size = root.frame.Size
	}
	if size.Width <= 0 || size.Height <= 0 {
		return nil, errors.New("nothing shows")
	}
	ratio := float64(s.env.PixelRatio)
	if ratio <= 0 {
		ratio = 2
	}
	pw := max(1, int(math.Ceil(float64(size.Width)*ratio)))
	ph := max(1, int(math.Ceil(float64(size.Height)*ratio)))
	c := s.api.Call("createOffscreenCanvas", map[string]any{"type": "2d", "width": pw, "height": ph})
	ctx := c.Call("getContext", "2d")
	ctx.Call("scale", ratio, ratio)
	bg := "#ffffff"
	if s.dark() {
		bg = "#000000"
	}
	ctx.Set("fillStyle", bg)
	ctx.Call("fillRect", 0, 0, float64(size.Width), float64(size.Height))
	if root := s.m.view(s.m.root); root != nil {
		s.drawViewToCanvas(ctx, root, ratio)
	}
	for _, modal := range s.m.modals {
		s.drawViewToCanvas(ctx, modal, ratio)
	}
	url := c.Call("toDataURL", "image/png").String()
	const prefix = "data:image/png;base64,"
	raw, ok := strings.CutPrefix(url, prefix)
	if !ok {
		return nil, errors.New("canvas did not return a PNG data URL")
	}
	return base64.StdEncoding.DecodeString(raw)
}

func (s *surface) drawViewToCanvas(ctx js.Value, v *view, ratio float64) {
	if v == nil || v.style.Hidden {
		return
	}
	f := s.m.shownFrame(v)
	w, h := float64(f.Size.Width), float64(f.Size.Height)
	ctx.Call("save")
	ctx.Call("translate", float64(f.Origin.X), float64(f.Origin.Y))
	if v.style.Opacity >= 0 && v.style.Opacity < 1 {
		ctx.Set("globalAlpha", ctx.Get("globalAlpha").Float()*float64(v.style.Opacity))
	}
	if fill := webstyle.Fill(v.style.Background); fill != "" {
		ctx.Set("fillStyle", fill)
		ctx.Call("fillRect", 0, 0, w, h)
	}
	if b := v.border(); b != (driver.Edges{}) && v.style.BorderColor != 0 {
		ctx.Set("strokeStyle", webstyle.Color(v.style.BorderColor))
		bw := float64(max(b.Top, max(b.Right, max(b.Bottom, b.Left))))
		if bw > 0 {
			ctx.Set("lineWidth", bw)
			ctx.Call("strokeRect", bw/2, bw/2, max(0, w-bw), max(0, h-bw))
		}
	}
	if v.style.Clip || v.kind == driver.KindScroll {
		ctx.Call("beginPath")
		ctx.Call("rect", 0, 0, w, h)
		ctx.Call("clip")
	}
	switch v.kind {
	case driver.KindText:
		if v.layout != nil && v.text != nil {
			ctx.Set("textBaseline", "top")
			ox, oy := float64(v.text.Content.Origin.X), float64(v.text.Content.Origin.Y)
			for _, run := range v.layout.runs {
				if run.text == "" {
					continue
				}
				font, col := textRunStyle(v.text, run)
				ctx.Set("font", font)
				ctx.Set("fillStyle", col)
				ctx.Call("fillText", run.text, ox+float64(run.x), oy+float64(run.y))
			}
		}
	case driver.KindInput:
		if v.inputConfig != nil {
			c := v.inputConfig
			text := v.value
			col := c.Style.Color
			if text == "" {
				text = c.Placeholder
				col = c.PlaceholderColor
				if col == 0 {
					col = c.Style.Color&0xffffff00 | (c.Style.Color&0xff)/2
				}
			}
			if text != "" {
				ctx.Set("textBaseline", "top")
				ctx.Set("font", canvasFont(c.Style.Font))
				ctx.Set("fillStyle", webstyle.Color(col))
				ctx.Call("fillText", text, float64(c.Content.Origin.X), float64(c.Content.Origin.Y))
			}
		}
	case driver.KindCanvas:
		if v.drawing != nil && w > 0 && h > 0 {
			p := canvas2d.NewPlayer(ctx, ratio, math.Hypot(w, h))
			p.Font = canvasFont
			ops := s.m.animator.CanvasOps(v.id, v.drawing)
			for i := range ops {
				p.Play(&ops[i])
			}
		}
	}
	if v.kind == driver.KindScroll {
		ctx.Call("translate", -float64(v.offset.X), -float64(v.offset.Y))
	}
	for _, child := range v.children {
		if child.kind == driver.KindModal {
			continue
		}
		s.drawViewToCanvas(ctx, child, ratio)
	}
	ctx.Call("restore")
}

func textRunStyle(tc *driver.TextContent, r run) (font, color string) {
	st := tc.Style
	if r.style > 0 && r.style-1 < len(tc.Spans) {
		sp := tc.Spans[r.style-1].Style
		if sp.Font != (driver.Font{}) {
			st.Font = sp.Font
		}
		if sp.Color != 0 {
			st.Color = sp.Color
		}
	}
	return canvasFont(st.Font), webstyle.Color(st.Color)
}
