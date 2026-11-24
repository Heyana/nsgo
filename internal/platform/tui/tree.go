package tui

import (
	"math"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type cell struct {
	text                            string
	fg, bg                          driver.Color
	bold, italic, underline, strike bool
	width                           int // zero is the continuation of a wide character
}

type grid struct {
	width, height    int
	cells            []cell
	cursorX, cursorY int
}

func newGrid(width, height int) *grid {
	g := &grid{width: max(width, 1), height: max(height, 1), cursorX: -1, cursorY: -1}
	g.cells = make([]cell, g.width*g.height)
	for i := range g.cells {
		g.cells[i] = cell{text: " ", width: 1}
	}
	return g
}

func (g *grid) at(x, y int) *cell {
	if x < 0 || y < 0 || x >= g.width || y >= g.height {
		return nil
	}
	return &g.cells[y*g.width+x]
}

func (g *grid) put(x, y int, text string, width int, style driver.TextStyle) {
	if width < 1 || x < 0 || x+width > g.width || y < 0 || y >= g.height {
		return
	}
	// Erase complete old glyphs before replacing any of their cells.
	for cx := x; cx < x+width; cx++ {
		old := g.at(cx, y)
		left := cx
		for old.width == 0 && left > 0 {
			left--
			old = g.at(left, y)
		}
		oldWidth := max(old.width, 1)
		for k := left; k < left+oldWidth; k++ {
			if c := g.at(k, y); c != nil {
				c.text = " "
				c.width = 1
			}
		}
	}
	c := g.at(x, y)
	c.text, c.width, c.fg = text, width, style.Color
	c.bold, c.italic = style.Font.Weight >= driver.FontWeightSemibold, style.Font.Italic
	c.underline = style.Decoration&driver.TextDecorationUnderline != 0
	c.strike = style.Decoration&driver.TextDecorationLineThrough != 0
	for k := 1; k < width; k++ {
		c := g.at(x+k, y)
		c.text = ""
		c.width = 0
	}
}

type view struct {
	id                 driver.NodeID
	kind               driver.ViewKind
	parent             *view
	children           []*view
	frame              driver.Bounds
	style              driver.ViewStyle
	props              driver.ViewProps
	events             driver.EventMask
	text               *driver.TextContent
	input              driver.InputConfig
	value              string
	inputShift         driver.Points
	selection          driver.TextRange
	edits, scrollCount uint32
	offset             driver.Point
	content            driver.Size
	scroll             driver.ScrollConfig
	overflow           driver.Bounds
	transformed        bool
	sticky             []driver.StickyCell
}

type tree struct {
	views                   map[driver.NodeID]*view
	root                    driver.NodeID
	focus, pressed, hovered *view
	keys                    []driver.KeyBinding
	heldKeys                bool
	send                    func(driver.Event)
	quit                    func()
	title                   func(string)
	clipboard               driver.MemoryClipboard
}

func newTree(send func(driver.Event)) *tree {
	return &tree{views: map[driver.NodeID]*view{}, send: send}
}
func (t *tree) emit(e driver.Event) {
	if t.send != nil {
		t.send(e)
	}
}

func detach(v *view) {
	if v.parent != nil {
		v.parent.children = slices.DeleteFunc(v.parent.children, func(c *view) bool { return c == v })
		v.parent = nil
	}
}

func (t *tree) apply(b *driver.MountBatch) {
	t.root = b.Root
	for _, it := range b.Items {
		v := t.views[it.ID]
		switch it.Kind {
		case driver.ItemCreate:
			t.views[it.ID] = &view{id: it.ID, kind: it.View, style: driver.DefaultViewStyle}
		case driver.ItemKeymap:
			t.keys = slices.Clone(it.Keys)
			t.heldKeys = it.HeldKeys
		case driver.ItemTitle:
			if t.title != nil {
				t.title(safeText(it.Value))
			}
		case driver.ItemQuit:
			if t.quit != nil {
				t.quit()
			}
		case driver.ItemAlert:
			if it.Alert != nil {
				t.emit(driver.Event{Kind: driver.EventAlert, Count: it.Alert.ID, Index: -1})
			}
		case driver.ItemPickFiles, driver.ItemPickDirectories, driver.ItemSaveFile, driver.ItemShare:
			t.emit(driver.Event{Kind: driver.EventFiles, Count: it.Count})
		case driver.ItemOpenURL:
			t.emit(driver.Event{Kind: driver.EventOpenURL, Count: it.Count})
		case driver.ItemImageInfo:
			t.emit(driver.Event{Kind: driver.EventImageInfo, Count: it.Count, Text: "images are not supported by the terminal backend"})
		case driver.ItemDismissKeyboard:
			t.setFocus(nil)
		case driver.ItemSetClipboard, driver.ItemReadClipboard, driver.ItemSetClipboardImage:
			if ev, answer, _ := t.clipboard.Take(&it); answer {
				t.emit(ev)
			}
		case driver.ItemNotificationPermission, driver.ItemShowNotification:
			t.emit(driver.NoNotification(&it))
		default:
			if v == nil {
				continue
			}
			switch it.Kind {
			case driver.ItemInsert:
				detach(v)
				v.parent = t.views[it.Parent]
				if v.parent != nil {
					at := len(v.parent.children)
					for i, c := range v.parent.children {
						if c.id == it.Before {
							at = i
							break
						}
					}
					v.parent.children = slices.Insert(v.parent.children, at, v)
				}
			case driver.ItemRemove:
				detach(v)
			case driver.ItemDelete:
				if t.focus == v {
					t.setFocus(nil)
				}
				if t.pressed == v {
					t.pressed = nil
				}
				if t.hovered == v {
					t.hovered = nil
				}
				detach(v)
				if v.text != nil && v.text.Layout != nil {
					v.text.Layout.Release()
				}
				delete(t.views, it.ID)
			case driver.ItemFrame:
				v.frame = it.Frame
			case driver.ItemViewStyle:
				if it.ViewStyle != nil {
					v.style = *it.ViewStyle
				}
			case driver.ItemViewProps:
				if it.ViewProps != nil {
					v.props = *it.ViewProps
				}
			case driver.ItemAccessibilityLabel:
				v.props.Label = it.Value
			case driver.ItemEvents:
				v.events = it.Events
			case driver.ItemText:
				if v.text != nil && v.text.Layout != nil {
					v.text.Layout.Release()
				}
				if it.Text != nil {
					c := *it.Text
					v.text = &c
				} else {
					v.text = nil
				}
			case driver.ItemInput:
				if it.Input != nil {
					v.input = *it.Input
				}
			case driver.ItemInputValue:
				if it.Count >= v.edits {
					v.value = it.Value
					v.selection = driver.TextRange{Start: driver.UTF16Len(v.value), End: driver.UTF16Len(v.value)}
				}
			case driver.ItemSelection:
				v.selection = driver.TextRange{Start: min(max(it.Selection.Start, 0), driver.UTF16Len(v.value)), End: min(max(it.Selection.End, 0), driver.UTF16Len(v.value))}
			case driver.ItemFocus:
				t.setFocus(v)
			case driver.ItemBlur:
				if t.focus == v {
					t.setFocus(nil)
				}
			case driver.ItemScrollConfig:
				if it.Scroll != nil {
					v.scroll = *it.Scroll
				}
			case driver.ItemScrollContent:
				v.content = it.Size
			case driver.ItemScrollTo:
				v.scrollCount++
				t.scrollTo(v, it.Point)
			case driver.ItemScrollBy:
				v.scrollCount++
				v.offset.X += it.Point.X
				v.offset.Y += it.Point.Y
				t.emit(driver.Event{Kind: driver.EventScroll, ID: v.id, Point: v.offset, Count: v.scrollCount, Value: 1})
			case driver.ItemStickyCells:
				v.sticky = slices.Clone(it.Sticky)
			case driver.ItemOverflow:
				v.overflow, v.transformed = it.Frame, it.Transformed
			}
		}
	}
	if t.focus != nil && !enabled(t.focus) {
		t.setFocus(nil)
	}
}

func (t *tree) close() {
	for _, v := range t.views {
		if v.text != nil && v.text.Layout != nil {
			v.text.Layout.Release()
		}
	}
	clear(t.views)
}

func (t *tree) scrollTo(v *view, p driver.Point) {
	p.X = min(max(p.X, -v.scroll.Behavior.ContentInset.Left), max(-v.scroll.Behavior.ContentInset.Left, v.content.Width-v.frame.Size.Width+v.scroll.Behavior.ContentInset.Right))
	p.Y = min(max(p.Y, -v.scroll.Behavior.ContentInset.Top), max(-v.scroll.Behavior.ContentInset.Top, v.content.Height-v.frame.Size.Height+v.scroll.Behavior.ContentInset.Bottom))
	v.offset = p
	t.emit(driver.Event{Kind: driver.EventScroll, ID: v.id, Point: p, Count: v.scrollCount, Value: 1})
}

func visible(v *view) bool { return v != nil && !v.style.Hidden && v.style.Opacity > 0 }
func enabled(v *view) bool {
	for p := v; p != nil; p = p.parent {
		if !visible(p) || p.props.Disabled {
			return false
		}
	}
	return v != nil && !(v.kind == driver.KindInput && v.input.Disabled)
}
func focusable(v *view) bool {
	return enabled(v) && !v.props.NoFocus && (v.kind == driver.KindInput || v.props.Focusable || v.events.Has(driver.EventPress) && !v.props.LabelPress)
}

func (t *tree) setFocus(v *view) {
	if v == t.focus || v != nil && !enabled(v) {
		return
	}
	if t.focus != nil {
		t.emit(driver.Event{Kind: driver.EventBlur, ID: t.focus.id})
	}
	t.focus = v
	if v != nil {
		t.emit(driver.Event{Kind: driver.EventFocus, ID: v.id})
	}
}

func (t *tree) hit(p driver.Point, match func(*view) bool) *view {
	var visit func(*view, driver.Point) *view
	visit = func(v *view, p driver.Point) *view {
		if !visible(v) {
			return nil
		}
		local, ok := driver.IntoView(v.frame, v.style.Matrix(v.frame.Size), p)
		if !ok {
			return nil
		}
		self, children := driver.ReachView(v.frame.Size, &v.props, v.overflow, v.transformed, local)
		if (v.style.Clip || v.kind == driver.KindScroll) && !(driver.Bounds{Size: v.frame.Size}).Contains(local) {
			children = false
		}
		if children {
			for _, child := range slices.Backward(v.children) {
				if got := visit(child, driver.Point{X: local.X + v.offset.X, Y: local.Y + v.offset.Y}); got != nil {
					return got
				}
			}
		}
		if self && match(v) {
			return v
		}
		return nil
	}
	return visit(t.views[t.root], p)
}

// clip is where a view clips its children: box, its padding box (driver.ViewStyle.ClipBox), in the view's coordinates, which matrix maps to the screen's.
type clip struct {
	matrix driver.Matrix3D
	box    driver.Bounds
}

func inside(clips []clip, p driver.Point) bool {
	for _, c := range clips {
		q, ok := c.matrix.Unproject(p)
		if !ok || !c.box.Contains(q) {
			return false
		}
	}
	return true
}

func (t *tree) render(width, height int) *grid {
	g := newGrid(width, height)
	var draw func(*view, driver.Matrix3D, []clip)
	draw = func(v *view, parent driver.Matrix3D, clips []clip) {
		if !visible(v) {
			return
		}
		matrix := parent.Mul(driver.Translate3D(float64(v.frame.Origin.X), float64(v.frame.Origin.Y), 0)).Mul(v.style.Matrix(v.frame.Size))
		for y := range g.height {
			for x := range g.width {
				p := driver.Point{X: (driver.Points(x) + 0.5) * cellWidth, Y: (driver.Points(y) + 0.5) * cellHeight}
				if !inside(clips, p) {
					continue
				}
				q, ok := matrix.Unproject(p)
				if !ok || !(driver.Bounds{Size: v.frame.Size}).Contains(q) {
					continue
				}
				c := g.at(x, y)
				if v.style.Background&255 != 0 {
					g.put(x, y, " ", 1, driver.TextStyle{})
					c = g.at(x, y)
					c.bg = v.style.Background.Over(c.bg)
					c.text = " "
					c.width = 1
				}
				if v.style.BorderWidth > 0 && v.style.BorderColor&255 != 0 {
					left, right, top, bottom := q.X < cellWidth, q.X >= v.frame.Size.Width-cellWidth, q.Y < cellHeight, q.Y >= v.frame.Size.Height-cellHeight
					b := ""
					switch {
					case top && left:
						b = "┌"
					case top && right:
						b = "┐"
					case bottom && left:
						b = "└"
					case bottom && right:
						b = "┘"
					case top || bottom:
						b = "─"
					case left || right:
						b = "│"
					}
					if b != "" {
						g.put(x, y, b, 1, driver.TextStyle{Color: v.style.BorderColor})
					}
				}
			}
		}
		if v.style.Clip || v.kind == driver.KindScroll {
			// Inside the border, as CSS clips overflow to the padding box.
			x, y, w, h, _ := v.style.ClipBox(v.frame.Size)
			box := driver.Bounds{Origin: driver.Point{X: driver.Points(x), Y: driver.Points(y)}, Size: driver.Size{Width: driver.Points(w), Height: driver.Points(h)}}
			clips = append(slices.Clip(clips), clip{matrix, box})
		}
		textMatrix := matrix
		var layout *textLayout
		if v.text != nil {
			layout, _ = v.text.Layout.(*textLayout)
			textMatrix = matrix.Mul(driver.Translate3D(float64(v.text.Content.Origin.X), float64(v.text.Content.Origin.Y), 0))
		}
		if v.kind == driver.KindInput {
			value := v.value
			style := v.input.Style
			if value == "" {
				value = v.input.Placeholder
				if v.input.PlaceholderColor != 0 {
					style.Color = v.input.PlaceholderColor
				}
			}
			layout = v.editLayout(value, style)
			v.inputShift = 0
			textMatrix = matrix.Mul(driver.Translate3D(float64(v.input.Content.Origin.X), float64(v.input.Content.Origin.Y), 0))
			clips = append(slices.Clip(clips), clip{textMatrix, driver.Bounds{Size: v.input.Content.Size}})
			if t.focus == v && !v.input.Multiline {
				if box, ok := layout.CaretRect(driver.Caret{Offset: v.selection.End}); ok {
					shift := max(0, box.Origin.X-v.input.Content.Size.Width+cellWidth)
					v.inputShift = shift
					textMatrix = textMatrix.Mul(driver.Translate3D(-float64(shift), 0, 0))
				}
			}
		}
		if layout != nil {
			for _, glyph := range layout.glyphs {
				if glyph.hidden {
					continue
				}
				x, y := textMatrix.Apply(float64(glyph.x), float64(glyph.y))
				cx, cy := int(math.Round(x/float64(cellWidth))), int(math.Round(y/float64(cellHeight)))
				p := driver.Point{X: (driver.Points(cx) + 0.5) * cellWidth, Y: (driver.Points(cy) + 0.5) * cellHeight}
				end := p
				end.X += driver.Points(glyph.width-1) * cellWidth
				if inside(clips, p) && inside(clips, end) {
					if v.kind == driver.KindInput && glyph.units < max(v.selection.Start, v.selection.End) && glyph.units+driver.UTF16Len(glyph.text) > min(v.selection.Start, v.selection.End) {
						for x := cx; x < cx+glyph.width; x++ {
							if c := g.at(x, cy); c != nil {
								c.bg = v.input.Highlight().Over(c.bg)
							}
						}
					}
					if glyph.text == strings.Repeat(" ", glyph.width) {
						for k := range glyph.width {
							g.put(cx+k, cy, " ", 1, glyph.style)
						}
					} else {
						g.put(cx, cy, glyph.text, glyph.width, glyph.style)
					}
				}
			}
			if t.focus == v && v.kind == driver.KindInput && !v.input.HideCaret {
				if box, ok := layout.CaretRect(driver.Caret{Offset: v.selection.End}); ok {
					x, y := textMatrix.Apply(float64(box.Origin.X), float64(box.Origin.Y))
					g.cursorX, g.cursorY = int(x/float64(cellWidth)), int(y/float64(cellHeight))
				}
			}
		}
		contentMatrix := matrix.Mul(driver.Translate3D(-float64(v.offset.X), -float64(v.offset.Y), 0))
		for _, child := range v.children {
			draw(child, contentMatrix, clips)
		}
	}
	draw(t.views[t.root], driver.Identity3D, nil)
	return g
}
