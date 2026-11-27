//go:build js && wasm

package web

import (
	"encoding/json"
	"slices"
	"strings"
	"syscall/js"
	"unicode/utf16"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

// Text selection is the browser's own. A text with SelectableOn is
// user-select text and one with SelectableOff none; SelectableAuto
// keeps the page's default, which selects text outside pressable views.
// A selection area is user-select text throughout, so the browser's
// selection, handles and long presses included, runs across its texts.
// Cmd or Ctrl+A selects the area or text the selection is in, Escape
// clears it, and ::selection takes each text's SelectionColor.

// selectionCSS styles the highlight of texts with a selection color.
const selectionCSS = `.nsgo-selection::selection, .nsgo-selection *::selection {
	background: var(--nsgo-selection);
}`

// setSelection applies a view's selection props.
func (s *surface) setSelection(v *view, p *driver.ViewProps) {
	if v.kind == driver.KindText {
		select_ := map[driver.Selectable]string{driver.SelectableOn: "text", driver.SelectableOff: "none"}[p.Selectable]
		v.textCSS.set1("user-select", select_)
		v.textCSS.set1("-webkit-user-select", select_)
		v.selectable = p.Selectable
		if p.SelectionColor != v.selColor {
			v.selColor = p.SelectionColor
			s.selectionStyle()
			if p.SelectionColor == 0 {
				v.inner.Get("classList").Call("remove", "nsgo-selection")
			} else {
				v.inner.Get("classList").Call("add", "nsgo-selection")
				v.textCSS.set1("--nsgo-selection", webstyle.Color(p.SelectionColor))
			}
		}
	}
	if p.SelectionArea != v.area {
		v.area = p.SelectionArea
		s.setFeel(v)
	}
}

// selectionStyle adds selectionCSS to the page once.
func (s *surface) selectionStyle() {
	if s.selectionCSS {
		return
	}
	s.selectionCSS = true
	el := s.doc.Call("createElement", "style")
	el.Set("textContent", selectionCSS)
	s.doc.Get("head").Call("appendChild", el)
}

// selectKey selects the whole area or text the selection is in on Cmd or
// Ctrl+A, rather than the page, and clears the selection on Escape. It
// reports whether it took the key.
func (s *surface) selectKey(e js.Value) bool {
	if e.Get("isComposing").Bool() || editing(s.doc.Get("activeElement")) {
		return false
	}
	sel := js.Global().Call("getSelection")
	if !sel.Truthy() || sel.Get("rangeCount").Int() == 0 {
		return false
	}
	node := sel.Get("anchorNode")
	if !node.Truthy() || !s.host.Call("contains", node).Bool() {
		return false
	}
	key := e.Get("key").String()
	switch {
	case key == "Escape" && !sel.Get("isCollapsed").Bool():
		sel.Call("removeAllRanges")
	case (key == "a" || key == "A") && (e.Get("metaKey").Bool() || e.Get("ctrlKey").Bool()) && !e.Get("altKey").Bool():
		scope := s.selectionScope(node)
		if !scope.Truthy() {
			return false
		}
		sel.Call("selectAllChildren", scope)
	default:
		return false
	}
	e.Call("preventDefault")
	return true
}

// selectionChanged reports the selection's text, per-text ranges and extent to the area it is in when any changed, and to the area it left that it has none.
func (s *surface) selectionChanged() {
	var area *view
	var box driver.Bounds
	var text string
	var ranges []driver.SelectedRange
	if sel := js.Global().Call("getSelection"); sel.Truthy() && sel.Get("rangeCount").Int() > 0 && !sel.Get("isCollapsed").Bool() {
		r := sel.Call("getRangeAt", 0)
		el := r.Get("commonAncestorContainer")
		if el.Get("nodeType").Int() != 1 {
			el = el.Get("parentElement")
		}
		for ; el.Truthy() && !el.Equal(s.host); el = el.Get("parentElement") {
			if v := s.viewOf(el); v != nil && v.area {
				area = v
			}
		}
		if area != nil {
			from, to := area.el.Call("getBoundingClientRect"), r.Call("getBoundingClientRect")
			box = driver.Bounds{
				Origin: driver.Point{X: driver.Points(to.Get("left").Float() - from.Get("left").Float()), Y: driver.Points(to.Get("top").Float() - from.Get("top").Float())},
				Size:   driver.Size{Width: driver.Points(to.Get("width").Float()), Height: driver.Points(to.Get("height").Float())},
			}
			text = sel.Call("toString").String()
			ranges = s.selectionRanges(area, r)
		}
	}
	if s.selectedArea != nil && s.selectedArea != area {
		s.send(s.selectedArea, driver.Event{Kind: driver.EventSelectionChange})
		s.selectedBox, s.selectedText, s.selectedRanges = driver.Bounds{}, "", nil
	}
	s.selectedArea = area
	if area != nil && (box != s.selectedBox || text != s.selectedText || !slices.Equal(ranges, s.selectedRanges)) {
		s.selectedBox, s.selectedText, s.selectedRanges = box, text, ranges
		s.send(area, driver.Event{Kind: driver.EventSelectionChange, Point: box.Origin, Size: box.Size, Text: text, Ranges: ranges})
	}
}

// selectionRanges returns what each text of area has selected in DOM range r, as UTF-16 offsets of its content, in document order.
func (s *surface) selectionRanges(area *view, r js.Value) []driver.SelectedRange {
	var out []driver.SelectedRange
	texts := area.el.Call("querySelectorAll", ".nsgo-text")
	for i := range texts.Length() {
		el := texts.Index(i)
		if !r.Call("intersectsNode", el).Bool() || !s.userSelects(el) || el.Call("closest", ".nsgo-attachment").Truthy() {
			continue
		}
		v := s.viewOf(el.Get("parentElement"))
		if v == nil || v.kind != driver.KindText {
			continue
		}
		part := s.doc.Call("createRange")
		part.Call("selectNodeContents", el)
		if r.Call("compareBoundaryPoints", 0, part).Int() > 0 { // START_TO_START
			part.Call("setStart", r.Get("startContainer"), r.Get("startOffset"))
		}
		if r.Call("compareBoundaryPoints", 2, part).Int() < 0 { // END_TO_END
			part.Call("setEnd", r.Get("endContainer"), r.Get("endOffset"))
		}
		pre := s.doc.Call("createRange")
		pre.Call("setStart", el, 0)
		pre.Call("setEnd", part.Get("startContainer"), part.Get("startOffset"))
		start := jsTextLen(pre.Call("cloneContents").Get("textContent"))
		selLen := jsTextLen(part.Call("cloneContents").Get("textContent"))
		if selLen > 0 {
			out = append(out, driver.SelectedRange{ID: v.id, Start: start, End: start + selLen})
		}
	}
	return out
}

func jsTextLen(v js.Value) int {
	if v.Type() != js.TypeString {
		return 0
	}
	return len(utf16.Encode([]rune(v.String())))
}

// selectionScope returns the element Select All selects for a selection
// in node: the outermost area around it, or its selectable text.
func (s *surface) selectionScope(node js.Value) js.Value {
	scope := js.Null()
	el := node
	if el.Get("nodeType").Int() != 1 {
		el = el.Get("parentElement")
	}
	for ; el.Truthy() && !el.Equal(s.host); el = el.Get("parentElement") {
		v := s.viewOf(el)
		switch {
		case v == nil:
		case v.area:
			scope = v.el
		case v.kind == driver.KindText && v.selectable == driver.SelectableOn && !scope.Truthy():
			scope = v.inner
		}
	}
	return scope
}

// copySelection writes a selection across texts as core.SelectionModel copies one: each text's selected part, hidden spans left out and an attachment as its copy text (data-copy) or else left out, texts on one row joined by a tab and the others by a line break. The browser would run the absolutely placed texts together, and leave attachments' copy texts out.
func (s *surface) copySelection(e js.Value) {
	sel := js.Global().Call("getSelection")
	if !sel.Truthy() || sel.Get("rangeCount").Int() == 0 || sel.Get("isCollapsed").Bool() {
		return
	}
	r := sel.Call("getRangeAt", 0)
	if !s.host.Call("contains", r.Get("commonAncestorContainer")).Bool() {
		return
	}
	var parts []string
	var rows []js.Value
	copied := false
	singleItem := ""
	multiItem := false
	texts := s.host.Call("querySelectorAll", ".nsgo-text")
	for i := range texts.Length() {
		el := texts.Index(i)
		if !r.Call("intersectsNode", el).Bool() || !s.userSelects(el) || el.Call("closest", ".nsgo-attachment").Truthy() {
			continue
		}
		part := s.doc.Call("createRange")
		part.Call("selectNodeContents", el)
		if r.Call("compareBoundaryPoints", 0, part).Int() > 0 { // START_TO_START
			part.Call("setStart", r.Get("startContainer"), r.Get("startOffset"))
		}
		if r.Call("compareBoundaryPoints", 2, part).Int() < 0 { // END_TO_END
			part.Call("setEnd", r.Get("endContainer"), r.Get("endOffset"))
		}
		// The selected nodes, hidden spans left out and each attachment's placeholder replaced by its copy text.
		frag := part.Call("cloneContents")
		hidden := frag.Call("querySelectorAll", "[data-hidden]")
		for j := range hidden.Length() {
			hidden.Index(j).Call("remove")
		}
		placeholders := frag.Call("querySelectorAll", "[data-copy]")
		itemAttr := ""
		if placeholders.Length() == 1 {
			ph := placeholders.Index(0)
			if v := ph.Call("getAttribute", "data-copy-item"); v.Type() == js.TypeString && v.String() != "" {
				// Check whether the fragment has any visible text outside this one placeholder.
				check := frag.Call("cloneNode", true)
				check.Call("querySelector", "[data-copy]").Call("remove")
				if strings.ReplaceAll(check.Get("textContent").String(), "￼", "") == "" {
					itemAttr = v.String()
				}
			}
		}
		for j := range placeholders.Length() {
			ph := placeholders.Index(j)
			ph.Call("replaceWith", ph.Call("getAttribute", "data-copy"))
			copied = true
		}
		text := strings.ReplaceAll(frag.Get("textContent").String(), "￼", "")
		if text == "" {
			continue
		}
		if itemAttr != "" && singleItem == "" && len(parts) == 0 && !multiItem {
			singleItem = itemAttr
		} else {
			singleItem, multiItem = "", true
		}
		parts = append(parts, text)
		rows = append(rows, el.Call("getBoundingClientRect"))
	}
	if len(parts) < 2 && !copied && singleItem == "" {
		return // the browser's copy is right
	}
	var out strings.Builder
	for i, p := range parts {
		if i > 0 {
			a, b := rows[i-1], rows[i]
			if a.Get("top").Float() < b.Get("bottom").Float() && b.Get("top").Float() < a.Get("bottom").Float() {
				out.WriteByte('\t')
			} else {
				out.WriteByte('\n')
			}
		}
		out.WriteString(p)
	}
	cd := e.Get("clipboardData")
	cd.Call("setData", "text/plain", out.String())
	if singleItem != "" {
		for _, rep := range decodeWebCopyItem(singleItem) {
			if rep.T != "" && rep.T != "text/plain" {
				cd.Call("setData", rep.T, rep.V)
			}
		}
	}
	e.Call("preventDefault")
}

type webCopyRep struct {
	T string `json:"t"`
	V string `json:"v"`
}

func encodeWebCopyItem(item *driver.ClipboardItem) string {
	var reps []webCopyRep
	for _, rep := range item.Representations {
		if rep.Type == "" || rep.Type == "text/plain" {
			continue
		}
		reps = append(reps, webCopyRep{T: rep.Type, V: string(rep.Data)})
	}
	if len(reps) == 0 {
		return ""
	}
	b, err := json.Marshal(reps)
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeWebCopyItem(raw string) []webCopyRep {
	var reps []webCopyRep
	if err := json.Unmarshal([]byte(raw), &reps); err != nil {
		return nil
	}
	return reps
}

// userSelects reports whether the browser selects text in el: the
// nearest element setting user-select decides.
func (s *surface) userSelects(el js.Value) bool {
	for ; el.Truthy() && !el.Equal(s.host); el = el.Get("parentElement") {
		switch el.Get("style").Call("getPropertyValue", "user-select").String() {
		case "none":
			return false
		case "text":
			return true
		}
	}
	return true
}
