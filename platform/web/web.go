//go:build js && wasm

// Package web is NSGo's browser backend: Go compiled to WebAssembly, one
// absolutely positioned element per node, text measured with the
// browser's own layout so measuring and drawing agree.
package web

import (
	"fmt"
	"syscall/js"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/engine"
)

// Options configure the page.
type Options struct {
	// Mount is the CSS selector of the element the app fills; the body
	// when empty.
	Mount string
}

const fontFamily = `system-ui, -apple-system, "Segoe UI", Roboto, sans-serif`

type surface struct {
	doc     js.Value
	host    js.Value
	measure js.Value // a hidden element text is measured in
	engine  *engine.Engine
	nodes   []js.Value // index is NodeID
	inputs  map[core.NodeID]js.Value
	root    core.NodeID
}

// Run fills the page with app. Call it from main; it does not return.
func Run(app engine.App, opts Options) {
	doc := js.Global().Get("document")
	host := doc.Get("body")
	if opts.Mount != "" {
		host = doc.Call("querySelector", opts.Mount)
	}
	setStyle(host, map[string]string{"position": "relative", "overflow": "hidden"})
	s := &surface{doc: doc, host: host, inputs: map[core.NodeID]js.Value{}}
	s.measure = doc.Call("createElement", "div")
	setStyle(s.measure, map[string]string{
		"position": "absolute", "visibility": "hidden", "left": "-10000px", "top": "0",
		"white-space": "pre-wrap", "overflow-wrap": "break-word", "font-family": fontFamily,
	})
	doc.Get("body").Call("appendChild", s.measure)

	s.engine = engine.Start(s, app, s.size())
	// The host can change size without the window doing so.
	observer := js.Global().Get("ResizeObserver").New(js.FuncOf(func(js.Value, []js.Value) any {
		s.engine.Resize(s.size())
		return nil
	}))
	observer.Call("observe", host)
	select {}
}

func (s *surface) size() core.Size {
	return core.Size{
		Width:  core.Points(s.host.Get("clientWidth").Float()),
		Height: core.Points(s.host.Get("clientHeight").Float()),
	}
}

func setStyle(el js.Value, props map[string]string) {
	style := el.Get("style")
	for k, v := range props {
		style.Call("setProperty", k, v)
	}
}

func px(p core.Points) string { return fmt.Sprintf("%gpx", float32(p)) }

func cssColor(c core.Color) string {
	r, g, b, a := c.Components()
	return fmt.Sprintf("rgba(%d,%d,%d,%g)", int(r*255+0.5), int(g*255+0.5), int(b*255+0.5), a)
}

func fontStyle(f core.Font) map[string]string {
	return map[string]string{
		"font-size":   px(f.Size),
		"font-weight": fmt.Sprint(f.Weight),
		"line-height": "normal",
	}
}

// Measure implements core.TextEngine with the browser's layout. In
// WebAssembly the engine goroutine shares the page's only thread.
func (s *surface) Measure(text string, font core.Font, maxWidth core.Points) core.Size {
	style := fontStyle(font)
	if maxWidth.IsAuto() {
		style["width"] = "max-content"
		style["max-width"] = "none"
	} else {
		style["width"] = "max-content"
		style["max-width"] = px(maxWidth)
	}
	setStyle(s.measure, style)
	if text == "" {
		text = "​" // an empty line keeps its height
	}
	s.measure.Set("textContent", text)
	r := s.measure.Call("getBoundingClientRect")
	w := r.Get("width").Float()
	if text == "​" {
		w = 0
	}
	return core.Size{Width: core.Points(ceil(w)), Height: core.Points(ceil(r.Get("height").Float()))}
}

func ceil(f float64) float64 {
	i := float64(int64(f))
	if i < f {
		return i + 1
	}
	return i
}

// Mount implements engine.Platform; the page has one thread, so it
// applies the batch right away.
func (s *surface) Mount(b *core.MountBatch) {
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case core.ItemCreate:
			for len(s.nodes) <= int(it.ID) {
				s.nodes = append(s.nodes, js.Undefined())
			}
			s.nodes[it.ID] = s.create(it.Tag, it.ID)
		case core.ItemProps:
			s.props(it.ID, &it.Props)
		case core.ItemInsert:
			parent, child := s.nodes[it.Parent], s.nodes[it.ID]
			children := parent.Get("children")
			if it.Index >= children.Length() {
				parent.Call("appendChild", child)
			} else {
				parent.Call("insertBefore", child, children.Index(it.Index))
			}
		case core.ItemFrame:
			f := it.Frame
			setStyle(s.nodes[it.ID], map[string]string{
				"left": px(f.Origin.X), "top": px(f.Origin.Y),
				"width": px(f.Size.Width), "height": px(f.Size.Height),
			})
		}
	}
	if b.Root != s.root {
		s.root = b.Root
		s.host.Call("appendChild", s.nodes[b.Root])
	}
}

func (s *surface) create(tag core.Tag, id core.NodeID) js.Value {
	el := s.doc.Call("createElement", "div")
	setStyle(el, map[string]string{"position": "absolute", "box-sizing": "border-box", "margin": "0"})
	switch tag {
	case core.TagText:
		setStyle(el, map[string]string{
			"white-space": "pre-wrap", "overflow-wrap": "break-word", "font-family": fontFamily,
			"pointer-events": "none",
		})
	case core.TagInput:
		input := s.doc.Call("createElement", "input")
		input.Set("type", "text")
		setStyle(input, map[string]string{
			"position": "absolute", "border": "0", "outline": "none", "padding": "0", "margin": "0",
			"background": "transparent", "font-family": fontFamily, "box-sizing": "border-box",
		})
		input.Call("addEventListener", "input", js.FuncOf(func(this js.Value, _ []js.Value) any {
			s.engine.Change(id, input.Get("value").String())
			return nil
		}))
		input.Call("addEventListener", "keydown", js.FuncOf(func(_ js.Value, args []js.Value) any {
			if args[0].Get("key").String() == "Enter" {
				s.engine.Submit(id, input.Get("value").String())
			}
			return nil
		}))
		el.Call("appendChild", input)
		s.inputs[id] = input
	}
	return el
}

func (s *surface) props(id core.NodeID, p *core.Props) {
	el := s.nodes[id]
	style := map[string]string{
		"background-color": cssColor(p.Background),
		"border-radius":    px(p.CornerRadius),
		"cursor":           "default",
	}
	if p.Pressable {
		style["cursor"] = "pointer"
		if el.Get("onclick").IsNull() {
			el.Set("onclick", js.FuncOf(func(js.Value, []js.Value) any {
				s.engine.Press(id)
				return nil
			}))
		}
	}
	if p.Role == core.RoleButton {
		el.Call("setAttribute", "role", "button")
		el.Call("setAttribute", "tabindex", "0")
	}
	if input, ok := s.inputs[id]; ok {
		in := fontStyle(p.Font)
		in["color"] = cssColor(p.Color)
		in["left"], in["top"] = px(p.Padding.Left), px(p.Padding.Top)
		in["width"] = fmt.Sprintf("calc(100%% - %s)", px(p.Padding.Left+p.Padding.Right))
		in["height"] = fmt.Sprintf("calc(100%% - %s)", px(p.Padding.Top+p.Padding.Bottom))
		setStyle(input, in)
		// Setting the text the user just typed would move the caret.
		if input.Get("value").String() != p.Text {
			input.Set("value", p.Text)
		}
		input.Set("placeholder", p.Placeholder)
	} else if p.Font.Size > 0 {
		for k, v := range fontStyle(p.Font) {
			style[k] = v
		}
		style["color"] = cssColor(p.Color)
		style["padding"] = fmt.Sprintf("%s %s %s %s", px(p.Padding.Top), px(p.Padding.Right),
			px(p.Padding.Bottom), px(p.Padding.Left))
		el.Set("textContent", p.Text)
	}
	setStyle(el, style)
}
