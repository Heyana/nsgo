//go:build js && wasm

// Package web is NSGo's browser backend: Go compiled to WebAssembly, one
// absolutely positioned element per view. Text is measured with the
// browser's own layout in a hidden element styled as the one that draws
// it, so measuring and drawing agree.
//
// In WebAssembly the engine goroutine shares the page's only thread, so
// text is measured and batches are applied right where the engine asks.
package web

import (
	"fmt"
	"strconv"
	"strings"
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

// longPressDelay is how long a press is held to be a long press, in
// milliseconds.
const longPressDelay = 500

type surface struct {
	doc       js.Value
	host      js.Value
	measure   js.Value      // a hidden element text is measured in
	measuring *preparedText // the text measure holds
	engine    *engine.Engine
	views     []*view // index is NodeID
	press     press
	timer     js.Value // the pending long press
	longPress js.Func
}

// view is the element of one NodeID.
type view struct {
	id      core.NodeID
	kind    core.ViewKind
	el      js.Value // framed and styled by the view's items
	content js.Value // where children go: el, or a scroll view's content
	inner   js.Value // a text view's text or a text field's <input>
	events  core.EventMask
	edits   uint32 // the user's edits of a text field
	border  core.Points
	scroll  core.Size // a scroll view's content size
	layout  *textLayout
	hover   bool // pointerenter and pointerleave are listened to
	funcs   []js.Func
}

// press is what the pointer last pressed in the page.
type press struct {
	active  bool
	pointer int
	view    *view // the pressable view it landed in, if any
	done    bool  // the pointer went up or the press was canceled
	cancel  bool  // the press was canceled or held as a long press
}

// Run fills the page with app. Call it from main; it does not return.
func Run(app engine.App, opts Options) {
	doc := js.Global().Get("document")
	host := doc.Get("body")
	if opts.Mount != "" {
		host = doc.Call("querySelector", opts.Mount)
	}
	setStyle(host, map[string]string{"position": "relative", "overflow": "hidden"})
	s := &surface{doc: doc, host: host}
	// Inside the host, measured text inherits what drawn text does.
	s.measure = doc.Call("createElement", "div")
	setStyle(s.measure, textBase)
	setStyle(s.measure, map[string]string{
		"position": "absolute", "visibility": "hidden", "left": "-10000px", "top": "0",
		"width": "max-content",
	})
	host.Call("appendChild", s.measure)
	s.listen()

	s.engine = engine.Start(s, app, engine.Options{Size: s.size()})
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

func (s *surface) view(id core.NodeID) *view {
	if int(id) < len(s.views) {
		return s.views[id]
	}
	return nil
}

// send reports ev for v if v is still mounted and asked for ev's kind.
func (s *surface) send(v *view, ev core.Event) {
	if s.view(v.id) != v || !v.events.Has(ev.Kind) {
		return
	}
	ev.ID = v.id
	s.engine.Dispatch(ev)
}

func setStyle(el js.Value, props map[string]string) {
	style := el.Get("style")
	for k, v := range props {
		style.Call("setProperty", k, v)
	}
}

// setAttr sets an attribute, or removes it when value is empty.
func setAttr(el js.Value, name, value string) {
	if value == "" {
		el.Call("removeAttribute", name)
	} else {
		el.Call("setAttribute", name, value)
	}
}

func px(p core.Points) string { return fmt.Sprintf("%gpx", float32(p)) }

func cssColor(c core.Color) string {
	r, g, b, a := c.Components()
	return fmt.Sprintf("rgba(%d,%d,%d,%g)", int(r*255+0.5), int(g*255+0.5), int(b*255+0.5), a)
}

func frameStyle(f core.Bounds) map[string]string {
	return map[string]string{
		"left": px(f.Origin.X), "top": px(f.Origin.Y),
		"width": px(f.Size.Width), "height": px(f.Size.Height),
	}
}

// Text.

// textBase is how both the measuring element and drawn text break lines.
var textBase = map[string]string{"white-space": "pre-wrap", "overflow-wrap": "break-word"}

// fontStyle is the CSS of a resolved text style.
func fontStyle(t core.TextStyle) map[string]string {
	family := fontFamily
	if t.Font.Family != "" {
		family = strconv.Quote(t.Font.Family) + ", " + fontFamily
	}
	italic := "normal"
	if t.Font.Italic {
		italic = "italic"
	}
	lineHeight := "normal"
	if t.LineHeight > 0 {
		lineHeight = px(t.LineHeight)
	}
	align := "start"
	switch t.Align {
	case core.TextAlignCenter:
		align = "center"
	case core.TextAlignEnd:
		align = "end"
	}
	var lines []string
	if t.Decoration&core.TextDecorationUnderline != 0 {
		lines = append(lines, "underline")
	}
	if t.Decoration&core.TextDecorationLineThrough != 0 {
		lines = append(lines, "line-through")
	}
	decoration := "none"
	if len(lines) > 0 {
		decoration = strings.Join(lines, " ")
	}
	return map[string]string{
		"font-family": family, "font-size": px(t.Font.Size), "font-weight": fmt.Sprint(t.Font.Weight),
		"font-style": italic, "line-height": lineHeight, "letter-spacing": px(t.LetterSpacing),
		"color": cssColor(t.Color), "text-align": align, "text-decoration-line": decoration,
	}
}

// preparedText is text styled for the browser to lay out.
type preparedText struct {
	s     *surface
	text  string
	style map[string]string
}

// textLayout is what the element that draws text needs; the browser lays
// it out at the size the engine measured.
type textLayout struct {
	text  string
	style map[string]string
}

// Release implements core.TextLayout.
func (*textLayout) Release() {}

// Prepare implements core.TextEngine.
func (s *surface) Prepare(t *core.AttributedText) core.PreparedText {
	style := fontStyle(t.Style)
	// Every property is set, so the measuring element keeps none of the
	// last text's.
	style["display"], style["-webkit-box-orient"], style["-webkit-line-clamp"], style["overflow"] =
		"block", "", "", "visible"
	if t.MaxLines > 0 {
		style["display"], style["-webkit-box-orient"], style["-webkit-line-clamp"], style["overflow"] =
			"-webkit-box", "vertical", strconv.Itoa(t.MaxLines), "hidden"
	}
	return &preparedText{s: s, text: t.Text, style: style}
}

// Measure implements core.PreparedText with the browser's layout.
func (p *preparedText) Measure(maxWidth core.Points) core.Size {
	m := p.s.measure
	if p.s.measuring != p {
		p.s.measuring = p
		setStyle(m, p.style)
		text := p.text
		if text == "" {
			text = "​" // an empty text keeps a line's height
		}
		m.Set("textContent", text)
	}
	limit := "none"
	if !maxWidth.IsAuto() {
		limit = px(maxWidth)
	}
	m.Get("style").Call("setProperty", "max-width", limit)
	r := m.Call("getBoundingClientRect")
	w := r.Get("width").Float()
	if p.text == "" {
		w = 0
	}
	return core.Size{Width: core.Points(w), Height: core.Points(r.Get("height").Float())}
}

// Layout implements core.PreparedText.
func (p *preparedText) Layout(core.Size) core.TextLayout {
	return &textLayout{text: p.text, style: p.style}
}

// Release implements core.PreparedText.
func (p *preparedText) Release() {
	if p.s.measuring == p {
		p.s.measuring = nil
	}
}

// Mounting.

// Mount implements engine.Platform; the page has one thread, so it
// applies the batch right away.
func (s *surface) Mount(b *core.MountBatch) {
	for i := range b.Items {
		it := &b.Items[i]
		if it.Kind == core.ItemCreate {
			v := s.create(it.ID, it.View)
			if it.ID == b.Root {
				s.host.Call("appendChild", v.el)
			}
			continue
		}
		v := s.view(it.ID)
		if v == nil {
			continue
		}
		switch it.Kind {
		case core.ItemInsert:
			parent := s.view(it.Parent)
			if parent == nil {
				continue
			}
			if before := s.view(it.Before); before != nil {
				parent.content.Call("insertBefore", v.el, before.el)
			} else {
				parent.content.Call("appendChild", v.el)
			}
		case core.ItemRemove:
			v.el.Call("remove")
		case core.ItemDelete:
			// Forgotten first, so events its removal fires are not sent.
			s.views[it.ID] = nil
			v.el.Call("remove")
			if v.layout != nil {
				v.layout.Release()
			}
			for _, f := range v.funcs {
				f.Release()
			}
		case core.ItemFrame:
			setStyle(v.el, frameStyle(it.Frame))
		case core.ItemViewStyle:
			s.setViewStyle(v, it.ViewStyle)
		case core.ItemText:
			s.setText(v, it.Text)
		case core.ItemInput:
			s.setInput(v, it.Input)
		case core.ItemInputValue:
			// Setting the text the field shows would move the caret.
			if v.kind == core.KindInput && v.edits <= it.Count && v.inner.Get("value").String() != it.Value {
				v.inner.Set("value", it.Value)
			}
		case core.ItemEvents:
			s.setEvents(v, it.Events)
		case core.ItemViewProps:
			setViewProps(v, it.ViewProps)
		case core.ItemScrollConfig:
			x, y := "hidden", "auto"
			if it.Scroll.Horizontal {
				x, y = y, x
			}
			bar := "auto"
			if it.Scroll.HideIndicator {
				bar = "none"
			}
			setStyle(v.el, map[string]string{"overflow-x": x, "overflow-y": y, "scrollbar-width": bar})
		case core.ItemScrollContent:
			v.scroll = it.Size
			v.placeContent()
		case core.ItemFocus:
			if v.kind == core.KindInput {
				v.inner.Call("focus")
			}
		case core.ItemBlur:
			if v.kind == core.KindInput {
				v.inner.Call("blur")
			}
		}
	}
}

func (s *surface) element(tag string) js.Value {
	el := s.doc.Call("createElement", tag)
	setStyle(el, map[string]string{
		"position": "absolute", "box-sizing": "border-box", "margin": "0", "left": "0", "top": "0",
	})
	return el
}

func (s *surface) create(id core.NodeID, kind core.ViewKind) *view {
	for len(s.views) <= int(id) {
		s.views = append(s.views, nil)
	}
	v := &view{id: id, kind: kind, el: s.element("div")}
	v.el.Set("nsgoID", int(id))
	v.content = v.el
	switch kind {
	case core.KindText:
		v.inner = s.element("div")
		setStyle(v.inner, textBase)
		v.el.Call("appendChild", v.inner)
	case core.KindInput:
		s.createInput(v)
	case core.KindScroll:
		setStyle(v.el, map[string]string{"overflow": "auto"})
		v.content = s.element("div")
		v.el.Call("appendChild", v.content)
		v.on(v.el, "scroll", func(js.Value) {
			s.send(v, core.Event{Kind: core.EventScroll, Point: core.Point{
				X: core.Points(v.el.Get("scrollLeft").Float()),
				Y: core.Points(v.el.Get("scrollTop").Float()),
			}})
		})
	}
	s.views[id] = v
	return v
}

// on listens to name on target until v is deleted.
func (v *view) on(target js.Value, name string, fn func(e js.Value)) {
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		fn(args[0])
		return nil
	})
	v.funcs = append(v.funcs, f)
	target.Call("addEventListener", name, f)
}

// placeContent sizes a scroll view's content. Children are framed from
// the view's origin, but the browser scrolls inside its border.
func (v *view) placeContent() {
	b := v.border
	setStyle(v.content, map[string]string{
		"left": px(-b), "top": px(-b), "width": px(v.scroll.Width - b), "height": px(v.scroll.Height - b),
	})
}

func (s *surface) setViewStyle(v *view, vs *core.ViewStyle) {
	style := map[string]string{
		"background-color": cssColor(vs.Background),
		"border-radius":    px(vs.CornerRadius),
		"border":           px(vs.BorderWidth) + " solid " + cssColor(vs.BorderColor),
		"opacity":          fmt.Sprint(vs.Opacity),
		"box-shadow":       "none",
		"display":          "block",
	}
	if vs.ShadowColor != 0 {
		style["box-shadow"] = fmt.Sprintf("%s %s %s %s",
			px(vs.ShadowOffset.X), px(vs.ShadowOffset.Y), px(vs.ShadowRadius), cssColor(vs.ShadowColor))
	}
	if vs.Hidden {
		style["display"] = "none"
	}
	if v.kind == core.KindScroll {
		if vs.BorderWidth != v.border {
			v.border = vs.BorderWidth
			v.placeContent()
		}
	} else {
		style["overflow"] = "visible"
		if vs.Clip {
			style["overflow"] = "hidden"
		}
	}
	setStyle(v.el, style)
}

func (s *surface) setText(v *view, t *core.TextContent) {
	if v.kind != core.KindText {
		return
	}
	l, _ := t.Layout.(*textLayout)
	if v.layout != nil && v.layout != l {
		v.layout.Release()
	}
	v.layout = l
	if l == nil {
		v.inner.Set("textContent", "")
		return
	}
	setStyle(v.inner, l.style)
	setStyle(v.inner, frameStyle(t.Content))
	v.inner.Set("textContent", l.text)
}

// Text fields.

var inputModes = map[core.KeyboardType]string{
	core.KeyboardEmail:   "email",
	core.KeyboardNumber:  "numeric",
	core.KeyboardDecimal: "decimal",
	core.KeyboardPhone:   "tel",
	core.KeyboardURL:     "url",
}

// createInput puts an <input> in v's element, which draws the field's
// background and border.
func (s *surface) createInput(v *view) {
	in := s.element("input")
	in.Set("type", "text")
	setStyle(in, map[string]string{
		"padding": "0", "border": "0", "outline": "none", "background": "transparent",
	})
	v.inner = in
	v.el.Call("appendChild", in)
	v.on(in, "input", func(js.Value) {
		v.edits++
		s.send(v, core.Event{Kind: core.EventChangeText, Text: in.Get("value").String(), Count: v.edits})
	})
	v.on(in, "keydown", func(e js.Value) {
		if e.Get("key").String() == "Enter" && !e.Get("isComposing").Bool() {
			s.send(v, core.Event{Kind: core.EventSubmitEditing, Text: in.Get("value").String()})
		}
	})
	v.on(in, "focus", func(js.Value) { s.send(v, core.Event{Kind: core.EventFocus}) })
	v.on(in, "blur", func(js.Value) { s.send(v, core.Event{Kind: core.EventBlur}) })
}

func (s *surface) setInput(v *view, c *core.InputConfig) {
	if v.kind != core.KindInput {
		return
	}
	in := v.inner
	setStyle(in, fontStyle(c.Style))
	setStyle(in, frameStyle(c.Content))
	kind := "text"
	if c.Masked {
		kind = "password"
	}
	if in.Get("type").String() != kind {
		in.Set("type", kind)
	}
	in.Set("placeholder", c.Placeholder)
	in.Set("readOnly", c.Readonly)
	in.Set("disabled", c.Disabled)
	setAttr(in, "inputmode", inputModes[c.KeyboardType])
}

// Events and accessibility.

var roles = map[core.Role]string{
	core.RoleButton:   "button",
	core.RoleLink:     "link",
	core.RoleHeader:   "heading",
	core.RoleImage:    "img",
	core.RoleCheckbox: "checkbox",
	core.RoleSwitch:   "switch",
	core.RoleRadio:    "radio",
	core.RoleTab:      "tab",
}

// setViewProps maps props to ARIA. A text field's go on its <input>,
// whose role is its own.
func setViewProps(v *view, p *core.ViewProps) {
	el := v.el
	if v.kind == core.KindInput {
		el = v.inner
	}
	flag := func(on bool) string {
		if on {
			return "true"
		}
		return ""
	}
	checked := flag(p.Checked)
	switch p.Role {
	case core.RoleCheckbox, core.RoleSwitch, core.RoleRadio:
		checked = strconv.FormatBool(p.Checked)
	}
	setAttr(el, "role", roles[p.Role])
	setAttr(el, "aria-label", p.Label)
	setAttr(el, "aria-disabled", flag(p.Disabled))
	setAttr(el, "aria-selected", flag(p.Selected))
	setAttr(el, "aria-checked", checked)
	setAttr(el, "data-testid", p.TestID)
}

// pressable reports whether v captures presses on its descendants.
func (v *view) pressable() bool {
	m := v.events
	return m.Has(core.EventPress) || m.Has(core.EventLongPress) ||
		m.Has(core.EventPressDown) || m.Has(core.EventPressUp)
}

func (s *surface) setEvents(v *view, m core.EventMask) {
	v.events = m
	if v.kind == core.KindInput {
		return
	}
	// Pressable views take keyboard focus and keep the page's own text
	// selection and tap highlight out of presses.
	style := map[string]string{
		"cursor": "", "user-select": "", "-webkit-user-select": "",
		"-webkit-touch-callout": "", "-webkit-tap-highlight-color": "",
	}
	tabIndex := ""
	if v.pressable() {
		style = map[string]string{
			"cursor": "pointer", "user-select": "none", "-webkit-user-select": "none",
			"-webkit-touch-callout": "none", "-webkit-tap-highlight-color": "transparent",
		}
		tabIndex = "0"
	}
	setStyle(v.el, style)
	setAttr(v.el, "tabindex", tabIndex)
	if !v.hover && (m.Has(core.EventHoverIn) || m.Has(core.EventHoverOut)) {
		v.hover = true
		v.on(v.el, "pointerenter", func(js.Value) { s.send(v, core.Event{Kind: core.EventHoverIn}) })
		v.on(v.el, "pointerleave", func(js.Value) { s.send(v, core.Event{Kind: core.EventHoverOut}) })
	}
}

// listen follows presses on the whole page: the nearest pressable view
// takes a press, unless a text field holds it first.
func (s *surface) listen() {
	on := func(name string, fn func(e js.Value)) {
		s.host.Call("addEventListener", name, js.FuncOf(func(_ js.Value, args []js.Value) any {
			fn(args[0])
			return nil
		}))
	}
	on("pointerdown", s.pointerDown)
	on("pointerup", s.pointerUp)
	on("pointercancel", s.pointerUp)
	on("click", s.click)
	on("keydown", s.keyDown)
	s.longPress = js.FuncOf(func(js.Value, []js.Value) any {
		s.timer = js.Undefined()
		if p := &s.press; p.view != nil && !p.done {
			p.cancel = true
			s.send(p.view, core.Event{Kind: core.EventLongPress})
		}
		return nil
	})
}

// target returns the view that takes a press on el: the nearest text
// field or pressable view holding it.
func (s *surface) target(el js.Value) *view {
	for ; el.Truthy() && !el.Equal(s.host); el = el.Get("parentElement") {
		id := el.Get("nsgoID")
		if id.IsUndefined() {
			continue
		}
		v := s.view(core.NodeID(id.Int()))
		if v != nil && v.el.Equal(el) && (v.kind == core.KindInput || v.pressable()) {
			return v
		}
	}
	return nil
}

func (s *surface) stopLongPress() {
	if s.timer.Truthy() {
		js.Global().Call("clearTimeout", s.timer)
		s.timer = js.Undefined()
	}
}

func (s *surface) pointerDown(e js.Value) {
	s.stopLongPress()
	if e.Get("button").Int() != 0 {
		s.press = press{}
		return
	}
	s.press = press{active: true, pointer: e.Get("pointerId").Int()}
	el := e.Get("target")
	v := s.target(el)
	if v == nil {
		return
	}
	if v.kind == core.KindInput {
		// The field's padding focuses it, as an input's own would.
		if !el.Equal(v.inner) {
			e.Call("preventDefault")
			v.inner.Call("focus")
		}
		return
	}
	s.press.view = v
	s.send(v, core.Event{Kind: core.EventPressDown})
	if v.events.Has(core.EventLongPress) {
		s.timer = js.Global().Call("setTimeout", s.longPress, longPressDelay)
	}
}

func (s *surface) pointerUp(e js.Value) {
	p := &s.press
	if p.view == nil || p.done || e.Get("pointerId").Int() != p.pointer {
		return
	}
	s.stopLongPress()
	p.done = true
	if e.Get("type").String() == "pointercancel" {
		p.cancel = true
	}
	s.send(p.view, core.Event{Kind: core.EventPressUp})
}

// click completes a press. Clicks also come from assistive technology,
// without a pointer.
func (s *surface) click(e js.Value) {
	p := s.press
	s.press = press{}
	v := s.target(e.Get("target"))
	if v == nil || v.kind == core.KindInput {
		return
	}
	// A pointer's click counts only on the view it went down in.
	if p.active && (p.view != v || p.cancel) {
		return
	}
	s.send(v, core.Event{Kind: core.EventPress})
}

// keyDown presses a focused pressable view with Enter or Space.
func (s *surface) keyDown(e js.Value) {
	if k := e.Get("key").String(); k != "Enter" && k != " " {
		return
	}
	el := e.Get("target")
	v := s.target(el)
	if v == nil || v.kind == core.KindInput || !v.el.Equal(el) {
		return
	}
	e.Call("preventDefault")
	s.send(v, core.Event{Kind: core.EventPress})
}
