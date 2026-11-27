package miniprogram

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/textselect"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

// defaultSelectionColor is the highlight drawn behind selected text when a
// text view sets no SelectionColor of its own.
var defaultSelectionColor = driver.RGBA(0x007aff, 0.22)

// isSelectionTrigger reports whether v starts its own selection area: either a
// container with SelectionArea or a standalone selectable Text view.
func isSelectionTrigger(v *view) bool {
	return v != nil && (v.props.SelectionArea || (v.kind == driver.KindText && v.props.Selectable == driver.SelectableOn))
}

// selectionAreaAt returns the selection area a gesture on v at page point p
// belongs to, or nil when a control inside it keeps the gesture or no area
// encloses it.
func (m *mounter) selectionAreaAt(v *view, p driver.Point) *view {
	hit := m.hitTarget(v, p, false)
	if hit == nil {
		hit = v
	}
	var under []*view
	for cur := hit; cur != nil; cur = cur.parent {
		under = append(under, cur)
		if cur.kind == driver.KindModal {
			break
		}
	}
	area, ok := textselect.AreaAt(under, func(cur *view) textselect.Taker {
		switch {
		case cur.kind == driver.KindInput || cur.kind == driver.KindWeb || cur.kind == driver.KindVideo:
			return textselect.Keeps
		case cur.events.Has(driver.EventLongPress) || cur.events.Has(driver.EventPan) || m.animator.PanBound(cur.id):
			return textselect.Keeps
		case cur.props.SelectionArea:
			return textselect.Area
		case cur.kind == driver.KindText && cur.props.Selectable == driver.SelectableOn:
			return textselect.SelectableText
		}
		return textselect.Passes
	})
	if !ok {
		return nil
	}
	return area
}

// listSelection populates sl.Texts with the selectable text views inside its
// area in document order, with their content boxes in page coordinates.
func (m *mounter) listSelection(sl *textselect.Selection) {
	area := m.view(sl.Area)
	sl.Texts = sl.Texts[:0]
	if area == nil {
		return
	}
	var walk func(v *view)
	walk = func(v *view) {
		if v == nil || v.style.Hidden {
			return
		}
		if v.kind == driver.KindText {
			if v.layout != nil && v.text != nil && v.props.Selectable != driver.SelectableOff {
				o := m.origin(v)
				box := v.text.Content
				if box.Size.Width <= 0 && box.Size.Height <= 0 {
					b := v.border()
					box.Size = driver.Size{
						Width:  max(0, v.frame.Size.Width-b.Left-b.Right),
						Height: max(0, v.frame.Size.Height-b.Top-b.Bottom),
					}
				}
				box.Origin.X += o.X
				box.Origin.Y += o.Y
				sl.Texts = append(sl.Texts, textselect.Text{
					ID:      v.id,
					Content: v.text.Text,
					Spans:   v.text.Spans,
					Box:     box,
					Layout:  v.layout,
				})
			}
			return
		}
		for _, c := range v.children {
			if c.kind == driver.KindModal {
				continue
			}
			walk(c)
		}
	}
	walk(area)
}

// selectLongPress begins word selection at page point p inside an enclosing
// selection area. It reports whether a selection area claimed the long press.
func (m *mounter) selectLongPress(v *view, p driver.Point) bool {
	area := m.selectionAreaAt(v, p)
	if area == nil {
		return false
	}
	if m.selecting == nil || m.selecting.Area != area.id {
		m.clearSelection()
		m.selecting = &textselect.Selection{Area: area.id}
		if root := m.view(m.root); root != nil {
			m.sendEvents(root)
		}
	}
	sl := m.selecting
	m.listSelection(sl)
	if !sl.Model.SelectWordAt(sl, p) {
		m.clearSelection()
		return true
	}
	m.selDragging = true
	m.showSelection()
	m.flush()
	return true
}

// selectDrag extends the active long-press selection to page point p.
func (m *mounter) selectDrag(p driver.Point) {
	sl := m.selecting
	if sl == nil || !m.selDragging {
		return
	}
	m.listSelection(sl)
	sl.Model.Drag(sl, p)
	m.showSelection()
	m.flush()
}

// selectionHandle moves the start or end selection handle in response to
// touches on the handle elements in the render layer.
func (m *mounter) selectionHandle(areaID driver.NodeID, handle int, phase driver.GesturePhase, page driver.Point) {
	sl := m.selecting
	if sl == nil || sl.Area != areaID {
		return
	}
	h := driver.HandleStart
	if handle == 1 {
		h = driver.HandleEnd
	}
	switch phase {
	case driver.GestureBegan:
		m.listSelection(sl)
		cx, top, height, ok := m.selectionCaret(sl, h)
		if !ok {
			return
		}
		m.selHandle = h
		m.selHandleActive = true
		m.selGrab = driver.Point{X: page.X - cx, Y: page.Y - (top + height/2)}
	case driver.GestureChanged:
		if !m.selHandleActive {
			return
		}
		m.listSelection(sl)
		s0, e0, _ := sl.Model.Bounds()
		target := driver.Point{X: page.X - m.selGrab.X, Y: page.Y - m.selGrab.Y}
		sl.Model.MoveHandle(sl, m.selHandle, target)
		if s1, e1, ok := sl.Model.Bounds(); ok {
			swapped := m.selHandle == driver.HandleStart && s1 == e0 && e1 != e0 ||
				m.selHandle == driver.HandleEnd && e1 == s0 && s1 != s0
			if swapped {
				m.selHandle = 1 - m.selHandle
			}
		}
		m.showSelection()
		m.flush()
	case driver.GestureEnded, driver.GestureCancelled:
		m.selHandleActive = false
	}
}

// selectionCaret returns the caret position (x, top, height) in page
// coordinates for the start or end of sl's selection.
func (m *mounter) selectionCaret(sl *textselect.Selection, h driver.SelectionHandle) (x, top, height driver.Points, ok bool) {
	start, end, ok := sl.Model.Bounds()
	if !ok || start.Text >= len(sl.Texts) || end.Text >= len(sl.Texts) {
		return 0, 0, 0, false
	}
	pos := start
	if h == driver.HandleEnd {
		pos = end
	}
	t := sl.Texts[pos.Text]
	r, ok := t.Layout.CaretRect(driver.Caret{Offset: pos.Offset, Affinity: driver.Upstream * driver.Affinity(h)})
	if !ok {
		return 0, 0, 0, false
	}
	return t.Box.Origin.X + r.Origin.X, t.Box.Origin.Y + r.Origin.Y, r.Size.Height, true
}

// selectionContains reports whether page point p lies inside any highlighted
// rectangle of the active selection.
func (m *mounter) selectionContains(p driver.Point) bool {
	sl := m.selecting
	if sl == nil || sl.Model.IsEmpty() {
		return false
	}
	m.listSelection(sl)
	for _, r := range sl.Model.Ranges(sl) {
		t := sl.Texts[r.Text]
		local := driver.Point{X: p.X - t.Box.Origin.X, Y: p.Y - t.Box.Origin.Y}
		for _, box := range t.Layout.RangeRects(r.Start, r.End, nil) {
			if local.X >= box.Origin.X && local.X <= box.Origin.X+box.Size.Width &&
				local.Y >= box.Origin.Y && local.Y <= box.Origin.Y+box.Size.Height {
				return true
			}
		}
	}
	return false
}

// showSelection emits updated highlight boxes ("hl"), handle positions ("sh")
// and EventSelectionChange for the current selection.
func (m *mounter) showSelection() {
	ranges := map[driver.NodeID]driver.SelectionRange{}
	if m.selecting != nil {
		ranges = m.selecting.Ranges()
		m.tellSelection(m.selecting, false)
	}
	for _, v := range m.selected {
		if _, ok := ranges[v.id]; !ok && m.view(v.id) == v {
			m.sendHighlight(v, 0, 0)
		}
	}
	m.selected = m.selected[:0]
	for id, r := range ranges {
		if v := m.view(id); v != nil {
			m.sendHighlight(v, r.Start, r.End)
			m.selected = append(m.selected, v)
		}
	}
	m.sendHandles()
}

// sendHighlight updates text view v's selection highlight rectangles ("hl").
func (m *mounter) sendHighlight(v *view, start, end int) {
	if v.layout == nil || v.text == nil || end <= start {
		if v.selHL != "" {
			v.selHL = ""
			m.o.op("hl", v.id).null()
		}
		return
	}
	rects := v.layout.RangeRects(start, end, nil)
	if len(rects) == 0 {
		if v.selHL != "" {
			v.selHL = ""
			m.o.op("hl", v.id).null()
		}
		return
	}
	c := v.props.SelectionColor
	if c == 0 {
		c = defaultSelectionColor
	}
	b := v.border()
	ox := v.text.Content.Origin.X - b.Left
	oy := v.text.Content.Origin.Y - b.Top
	var obj object
	obj.str("c", webstyle.Color(c))
	list := []byte{'['}
	for i, r := range rects {
		if i > 0 {
			list = append(list, ',')
		}
		list = append(list, '[')
		list = appendNum(list, float64(ox+r.Origin.X))
		list = append(list, ',')
		list = appendNum(list, float64(oy+r.Origin.Y))
		list = append(list, ',')
		list = appendNum(list, float64(r.Size.Width))
		list = append(list, ',')
		list = appendNum(list, float64(r.Size.Height))
		list = append(list, ']')
	}
	obj.raw("r", append(list, ']'))
	raw := obj.bytes()
	if s := string(raw); s != v.selHL {
		v.selHL = s
		m.o.op("hl", v.id).raw(raw)
	}
}

// sendHandles updates the active area's start and end selection handles ("sh").
func (m *mounter) sendHandles() {
	sl := m.selecting
	var area *view
	if sl != nil && !sl.Model.IsEmpty() {
		area = m.view(sl.Area)
	}
	if m.selArea != nil && m.selArea != area {
		if m.view(m.selArea.id) == m.selArea && m.selArea.selSH != "" {
			m.selArea.selSH = ""
			m.o.op("sh", m.selArea.id).null()
		}
		m.selArea = nil
	}
	if area == nil {
		return
	}
	sx, sy, sh, ok0 := m.selectionCaret(sl, driver.HandleStart)
	ex, ey, eh, ok1 := m.selectionCaret(sl, driver.HandleEnd)
	if !ok0 || !ok1 {
		if area.selSH != "" {
			area.selSH = ""
			m.o.op("sh", area.id).null()
		}
		return
	}
	ao := m.origin(area)
	ab := area.border()
	ox, oy := ao.X+ab.Left, ao.Y+ab.Top
	handleColor := m.selectionHandleColor(sl)
	var obj object
	obj.str("c", webstyle.Color(handleColor))
	tuple := func(x, y, h driver.Points) []byte {
		b := []byte{'['}
		b = appendNum(b, float64(x-ox))
		b = append(b, ',')
		b = appendNum(b, float64(y-oy))
		b = append(b, ',')
		b = appendNum(b, float64(h))
		return append(b, ']')
	}
	obj.raw("s", tuple(sx, sy, sh))
	obj.raw("e", tuple(ex, ey, eh))
	raw := obj.bytes()
	m.selArea = area
	if s := string(raw); s != area.selSH {
		area.selSH = s
		m.o.op("sh", area.id).raw(raw)
	}
}

// selectionHandleColor returns the opaque accent color for selection handles.
func (m *mounter) selectionHandleColor(sl *textselect.Selection) driver.Color {
	for _, t := range sl.Texts {
		if v := m.view(t.ID); v != nil && v.props.SelectionColor != 0 {
			return v.props.SelectionColor | 0xff
		}
	}
	return defaultSelectionColor | 0xff
}

// tellSelection reports EventSelectionChange to the selection area when the
// text, extent or per-text ranges changed.
func (m *mounter) tellSelection(sl *textselect.Selection, cleared bool) {
	area := m.view(sl.Area)
	if area == nil {
		return
	}
	if ev, ok := sl.Tell(m.origin(area), cleared); ok {
		m.send(area, ev)
	}
}

// clearSelection dismisses any active selection and clears its highlights and
// handles in the render layer.
func (m *mounter) clearSelection() {
	sl := m.selecting
	if sl == nil {
		return
	}
	m.selDragging = false
	m.selHandleActive = false
	m.tellSelection(sl, true)
	m.selecting = nil
	if root := m.view(m.root); root != nil {
		m.sendEvents(root)
	}
	m.showSelection()
}

// refreshSelection reconciles an active selection after a mount batch changes
// text, frames or visibility.
func (m *mounter) refreshSelection() {
	sl := m.selecting
	if sl == nil {
		return
	}
	area := m.view(sl.Area)
	if area == nil || !m.isShown(area) {
		m.clearSelection()
		return
	}
	m.listSelection(sl)
	sl.Model.TextsChanged(sl)
	if sl.Model.IsEmpty() {
		m.clearSelection()
		return
	}
	m.showSelection()
}

// isShown reports whether v is attached to the active tree and not hidden.
func (m *mounter) isShown(v *view) bool {
	for cur := v; cur != nil; cur = cur.parent {
		if cur.style.Hidden {
			return false
		}
		if cur.id == m.root || cur.kind == driver.KindModal {
			return true
		}
	}
	return false
}

// copySelection copies the currently selected text to the clipboard and returns
// the copied text.
func (m *mounter) copySelection() string {
	sl := m.selecting
	if sl == nil || sl.Model.IsEmpty() {
		return ""
	}
	m.listSelection(sl)
	text := sl.Model.Text(sl)
	if text != "" {
		m.h.setClipboard(text)
	}
	return text
}

// selectAllSelection selects every selectable text in the active selection area.
func (m *mounter) selectAllSelection() {
	sl := m.selecting
	if sl == nil {
		return
	}
	m.listSelection(sl)
	sl.Model.SelectAll(sl)
	m.showSelection()
	m.flush()
}
