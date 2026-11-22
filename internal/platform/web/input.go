//go:build js && wasm

package web

import (
	"strconv"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/field"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

// Text fields follow NSH's web backend: an <input>, or a <textarea> when
// multiline, in the view's element, which draws the background and
// border. The browser keeps maxLength itself; Return submits unless it
// breaks the line (SubmitBehavior), and Shift or Alt with Return always
// break a textarea's. An input method's composition is followed with the
// composition events, and a value the app sets during one waits for its
// end, since setting the value would end the composition and commit the
// text as typed.

// fieldCSS colors placeholders that have a color and hides native scrollbars
// when HideIndicator is set.
const fieldCSS = `.nsgo-placeholder::placeholder { color: var(--nsgo-placeholder); opacity: 1; }
.nsgo-no-bar::-webkit-scrollbar { display: none; }`

var inputModes = map[driver.KeyboardType]string{
	driver.KeyboardEmail:   "email",
	driver.KeyboardNumber:  "numeric",
	driver.KeyboardDecimal: "decimal",
	driver.KeyboardPhone:   "tel",
	driver.KeyboardURL:     "url",
}

var enterHints = map[driver.ReturnKeyType]string{
	driver.ReturnKeyDone: "done", driver.ReturnKeyGo: "go", driver.ReturnKeyNext: "next",
	driver.ReturnKeySearch: "search", driver.ReturnKeySend: "send",
}

var capitalize = map[driver.AutoCapitalize]string{
	driver.AutoCapitalizeSentences: "sentences", driver.AutoCapitalizeNone: "off",
	driver.AutoCapitalizeWords: "words", driver.AutoCapitalizeCharacters: "characters",
}

// webInput is a text field's element and what it tracks.
type webInput struct {
	fs             field.State
	listeners      listeners
	props          *driver.ViewProps // the last, which a new element takes
	config         *driver.InputConfig
	look           classSlot // the field's class of its text style (classes.go)
	lang           string    // the lang attribute v.inner holds
	applied        uint32
	lastScroll     driver.Point
	lastScrollMax  driver.Points
	lastApplied    uint32
	scrollReported bool
}

// createInput puts an <input> in v's element.
func (s *surface) createInput(v *view) {
	v.input = &webInput{fs: field.State{ID: v.id, Dispatch: s.engine.Dispatch}}
	s.setFieldElement(v, "input")
}

// setFieldElement makes v's field an element of tag, keeping its text.
func (s *surface) setFieldElement(v *view, tag string) {
	in := v.input
	value := ""
	if !v.inner.IsUndefined() && !v.inner.IsNull() {
		value = v.inner.Get("value").String()
		in.listeners.release()
		v.inner.Call("remove")
	}
	el := s.element(tag)
	el.Get("classList").Call("add", "nsgo-field")
	if tag == "input" {
		el.Set("type", "text")
	}
	s.freeClass(&in.look)
	in.look = classSlot{} // the new element has none
	in.lang = ""
	in.applied = 0
	in.lastScroll = driver.Point{}
	in.lastScrollMax = 0
	in.lastApplied = 0
	in.scrollReported = false
	el.Set("value", value)
	v.inner = el
	// Keep the field beneath any overlay children such as a Scrollbar host.
	v.el.Call("insertBefore", el, v.el.Get("firstChild"))
	if v.border != (driver.Edges{}) {
		unborder(el, v.border)
	}
	if in.props != nil {
		setViewProps(v, in.props)
	}
	on := func(name string, fn func(e js.Value)) { in.listeners.on(el, name, fn) }
	on("input", func(js.Value) {
		in.fs.Edited(el.Get("value").String())
		s.fieldSelection(v)
	})
	on("compositionstart", func(e js.Value) { s.compose(v, true, eventData(e)) })
	on("compositionupdate", func(e js.Value) { s.compose(v, true, eventData(e)) })
	on("compositionend", func(js.Value) {
		// The committed text goes first; browsers differ in whether its
		// input event comes before or after this.
		in.fs.Edited(el.Get("value").String())
		s.compose(v, false, "")
	})
	on("keydown", func(e js.Value) {
		key := e.Get("key").String()
		composing := e.Get("isComposing").Bool() || e.Get("keyCode").Int() == 229
		switch {
		case key == "Enter":
			if composing {
				return
			}
			lineBreak := tag == "textarea" && (e.Get("shiftKey").Bool() || e.Get("altKey").Bool())
			insert, blur := in.fs.Return(el.Get("value").String(), lineBreak)
			if insert {
				in.fs.Key("Enter")
				return
			}
			e.Call("preventDefault")
			if blur {
				el.Call("blur")
			}
		case key == "Backspace" || key == "Escape" || len([]rune(key)) == 1 && !composing:
			in.fs.Key(key)
		}
	})
	for _, name := range []string{"select", "keyup", "pointerup"} {
		on(name, func(js.Value) { s.fieldSelection(v) })
	}
	for _, name := range []string{"select", "keyup", "pointerup", "input", "scroll", "focus"} {
		on(name, func(js.Value) { s.caretMoved(v) })
	}
	if tag == "textarea" {
		on("scroll", func(js.Value) { s.reportInputScroll(v, false) })
	}
	on("paste", func(e js.Value) { s.pasted(v, e) })
	on("focus", func(js.Value) { s.send(v, driver.Event{Kind: driver.EventFocus}) })
	on("blur", func(js.Value) {
		s.compose(v, false, "")
		s.send(v, driver.Event{Kind: driver.EventBlur})
	})
}

func eventData(e js.Value) string {
	if d := e.Get("data"); d.Type() == js.TypeString {
		return d.String()
	}
	return ""
}

func (s *surface) compose(v *view, composing bool, text string) {
	el := v.inner
	if value, apply := v.input.fs.Compose(composing, text, el.Get("value").String()); apply && el.Get("value").String() != value {
		el.Set("value", value)
	}
}

// fieldSelection reports the selection the user left.
func (s *surface) fieldSelection(v *view) {
	el := v.inner
	if !v.input.fs.Events.Has(driver.EventSelectionChange) || el.Get("selectionStart").IsNull() {
		return
	}
	v.input.fs.Selected(driver.TextRange{Start: el.Get("selectionStart").Int(), End: el.Get("selectionEnd").Int()})
}

// inputValue is ItemInputValue.
func (s *surface) inputValue(v *view, text string, count uint32) {
	// Setting the text the field shows would move the caret.
	if v.input.fs.Value(text, count) && v.inner.Get("value").String() != text {
		v.inner.Set("value", text)
	}
}

// selectInput is ItemSelection.
func (s *surface) selectInput(v *view, r driver.TextRange) {
	n := driver.UTF16Len(v.inner.Get("value").String())
	start := min(max(r.Start, 0), n)
	end := min(max(r.End, start), n)
	v.input.fs.SetSelection(driver.TextRange{Start: start, End: end})
	v.inner.Call("setSelectionRange", start, end)
}

// fieldBox returns the box the field element fills: the padding box for a
// multiline textarea so its scroll bar runs along the edges and its text
// scrolls under the padding, or the content box for a single-line input.
func fieldBox(c *driver.InputConfig) driver.Bounds {
	b := c.Content
	if !c.Multiline {
		return b
	}
	p := c.Padding
	return driver.Bounds{
		Origin: driver.Point{X: b.Origin.X - p.Left, Y: b.Origin.Y - p.Top},
		Size:   driver.Size{Width: b.Size.Width + p.Left + p.Right, Height: b.Size.Height + p.Top + p.Bottom},
	}
}

func (s *surface) setInput(v *view, c *driver.InputConfig) {
	if v.kind != driver.KindInput {
		return
	}
	v.input.config = c
	in := v.input
	tag := "input"
	if c.Multiline {
		tag = "textarea"
	}
	if v.inner.Get("localName").String() != tag {
		s.setFieldElement(v, tag)
	}
	in.fs.Config = *c
	el := v.inner
	setStyle(el, frameStyle(fieldBox(c)))
	if !c.Multiline {
		// A search field stays type=text with ARIA's searchbox role (setViewProps): type=search would add the browser's clear button and look, and clear the field on Escape.
		kind := "text"
		if c.Masked {
			kind = "password"
		}
		if el.Get("type").String() != kind {
			el.Set("type", kind)
		}
	}
	el.Set("placeholder", c.Placeholder)
	el.Set("readOnly", c.Readonly)
	el.Set("disabled", c.Disabled)
	setAttr(el, "inputmode", inputModes[c.KeyboardType])
	// A textarea that submits shows the return key's action too.
	hint := ""
	if c.Submit() != driver.SubmitBehaviorNewline {
		hint = enterHints[c.ReturnKey]
		if hint == "" {
			hint = "enter"
		}
	}
	setAttr(el, "enterkeyhint", hint)
	setAttr(el, "autocapitalize", capitalize[c.AutoCapitalize])
	el.Set("spellcheck", !c.NoAutoCorrect)
	autocorrect := ""
	if c.NoAutoCorrect {
		autocorrect = "off"
	}
	setAttr(el, "autocorrect", autocorrect)
	setAttr(el, "autocomplete", c.AutoComplete.String())
	maxLength := ""
	if c.MaxLength > 0 {
		maxLength = strconv.Itoa(c.MaxLength)
	}
	setAttr(el, "maxlength", maxLength)
	if in.lang != c.Style.Font.Language {
		in.lang = c.Style.Font.Language
		setAttr(el, "lang", in.lang)
	}
	css := fontStyle(c.Style)
	if c.Multiline && c.Padding != (driver.Edges{}) {
		pad := webstyle.Px(c.Padding.Top) + " " + webstyle.Px(c.Padding.Right) + " " + webstyle.Px(c.Padding.Bottom) + " " + webstyle.Px(c.Padding.Left)
		css["padding"] = pad
		css["scroll-padding"] = pad
	}
	switch {
	case c.HideCaret:
		css["caret-color"] = "transparent"
	case c.Caret() != 0:
		css["caret-color"] = webstyle.Color(c.Caret())
	}
	classes := el.Get("classList")
	if c.SelectionColor != 0 {
		s.selectionStyle()
		classes.Call("add", "nsgo-selection")
		css["--nsgo-selection"] = webstyle.Color(c.Highlight())
	} else {
		classes.Call("remove", "nsgo-selection")
	}
	if c.PlaceholderColor != 0 {
		s.fieldStyle()
		classes.Call("add", "nsgo-placeholder")
		css["--nsgo-placeholder"] = webstyle.Color(c.PlaceholderColor)
	} else {
		classes.Call("remove", "nsgo-placeholder")
	}
	if c.Multiline && c.HideIndicator {
		s.fieldStyle()
		classes.Call("add", "nsgo-no-bar")
		css["scrollbar-width"] = "none"
	} else {
		classes.Call("remove", "nsgo-no-bar")
	}
	s.setClass(el, &in.look, s.sortedCSS(css))
}

func (s *surface) inputScrollMetrics(v *view) (offset driver.Point, maxY driver.Points) {
	if v.input == nil || v.input.config == nil || !v.input.config.Multiline || v.inner.IsUndefined() || v.inner.IsNull() {
		return driver.Point{}, 0
	}
	el := v.inner
	offset = driver.Point{
		X: driver.Points(max(el.Get("scrollLeft").Float(), 0)),
		Y: driver.Points(max(el.Get("scrollTop").Float(), 0)),
	}
	if el.Get("value").String() == "" {
		return offset, 0
	}
	maxY = driver.Points(max(el.Get("scrollHeight").Float()-el.Get("clientHeight").Float(), 0))
	return offset, maxY
}

func (s *surface) reportInputScroll(v *view, force bool) {
	in := v.input
	if in == nil || in.config == nil || !in.config.Multiline {
		return
	}
	offset, maxY := s.inputScrollMetrics(v)
	if !force && in.scrollReported && offset == in.lastScroll && maxY == in.lastScrollMax && in.applied == in.lastApplied {
		return
	}
	in.scrollReported = true
	in.lastScroll = offset
	in.lastScrollMax = maxY
	in.lastApplied = in.applied
	s.send(v, driver.Event{
		Kind:  driver.EventScroll,
		Point: offset,
		Size:  driver.Size{Width: v.frame.Size.Width, Height: v.frame.Size.Height + maxY},
		Count: in.applied,
	})
}

func (s *surface) scrollInputTo(v *view, to driver.Point, animated bool) {
	in := v.input
	if in == nil || in.config == nil || !in.config.Multiline || v.inner.IsUndefined() || v.inner.IsNull() {
		return
	}
	el := v.inner
	_, maxY := s.inputScrollMetrics(v)
	y := min(max(float64(to.Y), 0), float64(maxY))
	if animated {
		opts := js.Global().Get("Object").New()
		opts.Set("top", y)
		opts.Set("behavior", "smooth")
		el.Call("scrollTo", opts)
	} else {
		el.Set("scrollTop", y)
	}
	s.reportInputScroll(v, true)
}

// fieldStyle adds fieldCSS to the page once.
func (s *surface) fieldStyle() {
	if s.fieldCSS {
		return
	}
	s.fieldCSS = true
	el := s.doc.Call("createElement", "style")
	el.Set("textContent", fieldCSS)
	s.doc.Get("head").Call("appendChild", el)
}
