package tui

import (
	"slices"
	"strings"

	"github.com/clipperhouse/uax29/v2/graphemes"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func chordNames(name string, mods driver.Modifiers) []string {
	var prefix string
	if mods.Control {
		prefix += "ctrl+"
	}
	if mods.Alt {
		prefix += "alt+"
	}
	if mods.Shift {
		prefix += "shift+"
	}
	if mods.Platform {
		prefix += "meta+"
	}
	names := []string{driver.NormalizeKey(prefix + name)}
	// Control is mod on a terminal, including terminals hosted on a Mac:
	// Command is normally reserved by the terminal application itself.
	if mods.Control {
		names = append(names, driver.NormalizeKey("mod+"+strings.TrimPrefix(prefix, "ctrl+")+name))
	}
	return names
}

func (t *tree) tab(back bool) {
	var stops []*view
	var visit func(*view)
	visit = func(v *view) {
		if !visible(v) {
			return
		}
		if focusable(v) {
			stops = append(stops, v)
		}
		for _, c := range v.children {
			visit(c)
		}
	}
	visit(t.views[t.root])
	if len(stops) == 0 {
		return
	}
	i := slices.Index(stops, t.focus)
	if back {
		if i <= 0 {
			i = len(stops)
		}
		i--
	} else {
		i = (i + 1) % len(stops)
	}
	t.setFocus(stops[i])
}

func (t *tree) key(name, text string, mods driver.Modifiers, repeat bool) bool {
	v := t.focus
	use := driver.KeyUse(0)
	if v != nil && enabled(v) {
		use = driver.FocusKeyUse(v.kind, v.props.Role, v.events.Has(driver.EventPress))
	} else {
		v = nil
	}
	if id, taken := driver.FindKey(t.keys, chordNames(name, mods), use, repeat); taken {
		if id != 0 {
			t.emit(driver.Event{Kind: driver.EventKey, Count: id})
		}
		return true
	}
	if name == "tab" && !mods.Control && !mods.Alt && !mods.Platform {
		t.tab(mods.Shift)
		return true
	}
	if v != nil && v.kind == driver.KindInput {
		if t.editKey(v, name, text, mods) {
			return true
		}
	} else if v != nil && v.events.Has(driver.EventPress) && (name == "space" || name == "enter") && !mods.Control && !mods.Alt && !mods.Platform {
		t.emit(driver.Event{Kind: driver.EventPressDown, ID: v.id, Modifiers: mods, Count: 1})
		t.emit(driver.Event{Kind: driver.EventPressUp, ID: v.id, Modifiers: mods, Count: 1})
		t.emit(driver.Event{Kind: driver.EventPress, ID: v.id, Modifiers: mods, Count: 1})
		return true
	}
	if name == "escape" {
		t.emit(driver.Event{Kind: driver.EventBack})
		return true
	}
	return false
}

func (t *tree) selection(v *view, from, to int) {
	v.selection = driver.TextRange{Start: from, End: to}
	t.emit(driver.Event{Kind: driver.EventSelectionChange, ID: v.id, Index: from, Count: uint32(to)})
}

func (t *tree) replace(v *view, text string) {
	if v.input.Readonly || v.input.Disabled || v.props.Disabled {
		return
	}
	if !v.input.Multiline {
		text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", " "), "\n", " ")
		text = strings.ReplaceAll(text, "\r", " ")
	}
	start, end := min(v.selection.Start, v.selection.End), max(v.selection.Start, v.selection.End)
	insert, ok := driver.LimitEdit(v.value, end-start, text, v.input.MaxLength)
	if !ok {
		return
	}
	from, to := driver.UTF16Offset(v.value, start), driver.UTF16Offset(v.value, end)
	v.value = v.value[:from] + insert + v.value[to:]
	v.edits++
	t.emit(driver.Event{Kind: driver.EventChangeText, ID: v.id, Text: v.value, Count: v.edits})
	t.selection(v, start+driver.UTF16Len(insert), start+driver.UTF16Len(insert))
}

// adjacent moves by grapheme, keeping combining characters and joined emoji
// together. Selections and events still count UTF-16 as every backend does.
func adjacent(text string, at int, back bool) int {
	it := graphemes.FromString(text)
	units, previous := 0, 0
	for it.Next() {
		end := units + driver.UTF16Len(it.Value())
		if back && at <= end && at > units {
			return units
		}
		if !back && at < end {
			return end
		}
		previous, units = units, end
	}
	if back {
		return previous
	}
	return units
}

func (t *tree) editKey(v *view, name, text string, mods driver.Modifiers) bool {
	length := driver.UTF16Len(v.value)
	if mods.Control && !mods.Alt && !mods.Platform {
		switch name {
		case "a":
			t.selection(v, 0, length)
			return true
		case "c", "x":
			if v.input.Masked {
				return true
			}
			from, to := driver.UTF16Offset(v.value, min(v.selection.Start, v.selection.End)), driver.UTF16Offset(v.value, max(v.selection.Start, v.selection.End))
			t.clipboard.Take(&driver.MountItem{Kind: driver.ItemSetClipboard, Value: v.value[from:to]})
			if name == "x" {
				t.replace(v, "")
			}
			return true
		case "v":
			ev, _, _ := t.clipboard.Take(&driver.MountItem{Kind: driver.ItemReadClipboard})
			t.pasteText(v, ev.Text)
			return true
		}
	}
	if mods.Control || mods.Platform || mods.Alt {
		return false
	}
	switch name {
	case "up", "down":
		if !v.input.Multiline {
			return false
		}
		l := (textEngine{}).Prepare(&driver.AttributedText{Text: v.value, Style: v.input.Style}).Layout(v.input.Content.Size).(*textLayout)
		if box, ok := l.CaretRect(driver.Caret{Offset: v.selection.End}); ok {
			delta := box.Size.Height
			if name == "up" {
				delta = -delta
			}
			at := l.HitTest(driver.Point{X: box.Origin.X, Y: box.Origin.Y + delta + box.Size.Height/2}).Offset
			if mods.Shift {
				t.selection(v, v.selection.Start, at)
			} else {
				t.selection(v, at, at)
			}
		}
		return true
	case "left", "right", "home", "end":
		at := v.selection.End
		if name == "home" {
			at = 0
		} else if name == "end" {
			at = length
		} else {
			at = adjacent(v.value, at, name == "left")
		}
		if mods.Shift {
			t.selection(v, v.selection.Start, at)
		} else {
			t.selection(v, at, at)
		}
		return true
	case "backspace", "delete":
		if !v.input.Readonly {
			t.emit(driver.Event{Kind: driver.EventKeyPress, ID: v.id, Text: "Backspace"})
		}
		if v.input.Readonly {
			return true
		}
		if v.selection.Start == v.selection.End {
			at := adjacent(v.value, v.selection.End, name == "backspace")
			v.selection.Start = at
		}
		t.replace(v, "")
		return true
	case "enter":
		t.emit(driver.Event{Kind: driver.EventKeyPress, ID: v.id, Text: "Enter"})
		if v.input.Submit() == driver.SubmitBehaviorNewline || v.input.Multiline && mods.Shift {
			t.replace(v, "\n")
		} else {
			t.emit(driver.Event{Kind: driver.EventSubmitEditing, ID: v.id, Text: v.value})
			if v.input.Submit() == driver.SubmitBehaviorBlurAndSubmit {
				t.setFocus(nil)
			}
		}
		return true
	default:
		if text != "" {
			if !v.input.Readonly {
				t.emit(driver.Event{Kind: driver.EventKeyPress, ID: v.id, Text: driver.KeyOf(text)})
			}
			t.replace(v, text)
			return true
		}
	}
	return false
}

func (t *tree) local(v *view, p driver.Point) driver.Point {
	var chain []*view
	for n := v; n != nil; n = n.parent {
		chain = append(chain, n)
	}
	for _, n := range slices.Backward(chain) {
		q, ok := driver.IntoView(n.frame, n.style.Matrix(n.frame.Size), p)
		if !ok {
			return driver.Point{}
		}
		p = q
		if n != v {
			p.X += n.offset.X
			p.Y += n.offset.Y
		}
	}
	return p
}

func (t *tree) mouseDown(p driver.Point, mods driver.Modifiers) {
	v := t.hit(p, func(v *view) bool {
		return enabled(v) && (v.kind == driver.KindInput || v.events.Has(driver.EventPress) || v.events.Has(driver.EventPressDown) || v.events.Has(driver.EventPressUp))
	})
	t.pressed = v
	if v == nil {
		return
	}
	if v.kind == driver.KindInput {
		t.setFocus(v)
	} else if t.focus != nil && t.focus.kind != driver.KindInput {
		t.setFocus(nil)
	}
	if v.kind == driver.KindInput {
		local := t.local(v, p)
		local.X -= v.input.Content.Origin.X
		local.Y -= v.input.Content.Origin.Y
		layout := v.editLayout(v.value, v.input.Style)
		local.X += v.inputShift
		at := driver.HitTest(layout, local).Offset
		t.selection(v, at, at)
	}
	t.emit(driver.Event{Kind: driver.EventPressDown, ID: v.id, Page: p, Point: t.local(v, p), Modifiers: mods, Count: 1})
}

func (t *tree) mouseUp(p driver.Point, mods driver.Modifiers) {
	v := t.pressed
	t.pressed = nil
	if v == nil || !enabled(v) {
		return
	}
	t.emit(driver.Event{Kind: driver.EventPressUp, ID: v.id, Page: p, Point: t.local(v, p), Modifiers: mods, Count: 1})
	if hit := t.hit(p, func(n *view) bool { return n == v }); hit != nil {
		t.emit(driver.Event{Kind: driver.EventPress, ID: v.id, Page: p, Point: t.local(v, p), Modifiers: mods, Count: 1})
	}
}

func (t *tree) hover(p driver.Point, mods driver.Modifiers) {
	v := t.hit(p, func(v *view) bool {
		return v.events.Has(driver.EventMouseEnter) || v.events.Has(driver.EventMouseLeave) || v.events.Has(driver.EventMouseMove)
	})
	if v != t.hovered {
		if t.hovered != nil {
			t.emit(driver.Event{Kind: driver.EventMouseLeave, ID: t.hovered.id, Page: p, Point: t.local(t.hovered, p), Modifiers: mods})
		}
		t.hovered = v
		if v != nil {
			t.emit(driver.Event{Kind: driver.EventMouseEnter, ID: v.id, Page: p, Point: t.local(v, p), Modifiers: mods})
		}
	}
	if v != nil {
		t.emit(driver.Event{Kind: driver.EventMouseMove, ID: v.id, Page: p, Point: t.local(v, p), Modifiers: mods})
	}
}

func (t *tree) wheel(p, delta driver.Point) {
	v := t.hit(p, func(v *view) bool { return v.kind == driver.KindScroll && !v.scroll.Behavior.NoScroll })
	if v != nil {
		t.scrollTo(v, driver.Point{X: v.offset.X + delta.X, Y: v.offset.Y + delta.Y})
		t.emit(driver.Event{Kind: driver.EventScrollEnd, ID: v.id, Point: v.offset, Count: v.scrollCount, Value: 1})
	}
}

func (t *tree) contextMenu(p driver.Point, mods driver.Modifiers) {
	if v := t.hit(p, func(v *view) bool { return enabled(v) && v.events.Has(driver.EventContextMenu) }); v != nil {
		t.emit(driver.Event{Kind: driver.EventContextMenu, ID: v.id, Page: p, Point: t.local(v, p), Modifiers: mods})
	}
}

// editLayout retains the original UTF-16 ranges behind password bullets;
// replacing the source with bullets would corrupt selections of emoji.
func (v *view) editLayout(value string, style driver.TextStyle) *textLayout {
	p := (textEngine{}).Prepare(&driver.AttributedText{Text: value, Style: style}).(*preparedText)
	if v.input.Masked && v.value != "" {
		for i := range p.glyphs {
			if p.glyphs[i].text != "\n" {
				p.glyphs[i].text = "•"
				p.glyphs[i].width = 1
			}
		}
	}
	size := v.input.Content.Size
	if !v.input.Multiline {
		size.Width = driver.Auto
	}
	return p.Layout(size).(*textLayout)
}

func (t *tree) pasteText(v *view, text string) {
	if !enabled(v) || v.input.Readonly {
		return
	}
	if v.events.Has(driver.EventPaste) {
		t.emit(driver.Event{Kind: driver.EventPaste, ID: v.id, Data: &driver.PasteEvent{Text: text}})
	}
	if v.input.PasteBehavior != driver.PasteReport {
		t.replace(v, text)
	}
}
