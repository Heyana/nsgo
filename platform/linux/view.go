//go:build linux && !android

package linux

import (
	"slices"

	"github.com/oneo-me/nsgo/core"
)

// view is the main thread's copy of one mounted view.
type view struct {
	id       core.NodeID
	kind     core.ViewKind
	parent   *view
	children []*view
	frame    core.Bounds
	style    core.ViewStyle
	events   core.EventMask
	props    core.ViewProps
	deleted  bool

	// A text view's layout and where it goes in the view.
	text    core.TextLayout
	content core.Bounds

	// A scroll view's configuration, content size and offset.
	scroll core.ScrollConfig
	size   core.Size
	offset core.Point

	// An input's GtkEntry and what the window keeps for it.
	entry      uintptr
	provider   uintptr // the entry's CSS
	input      core.InputConfig
	edits      uint32 // user edits so far
	setting    bool   // the window is setting the text, not the user
	entryShown bool

	// An image view's configuration, source and placeholder.
	image               core.ImageConfig
	source, placeholder imageSlot

	// A canvas's drawing.
	drawing *core.Drawing

	// A web view's page.
	web *web

	// What animates: the style after the first, and the scroll offset.
	styled     bool
	transition *transition
	scrolling  *scrolling
}

// detach takes v out of its parent's children.
func (v *view) detach() {
	if p := v.parent; p != nil {
		if i := slices.Index(p.children, v); i >= 0 {
			p.children = slices.Delete(p.children, i, i+1)
		}
		v.parent = nil
	}
}

// chrome is the desktop window's title bar and application menu.
type chrome struct {
	titleBar   core.TitleBarStyle
	height     core.Points // the app's title bar height
	fullScreen bool
	max        bool
	menu       []core.MenuEntry
	keys       map[string]menuRef
}

// menuRef is a menu item's ID and role.
type menuRef struct {
	id   uint32
	role core.MenuRole
}

// hidden reports whether the system title bar is hidden now.
func (w *window) hidden() bool {
	return w.chrome.titleBar == core.TitleBarHidden && !w.chrome.fullScreen
}

// titleBarArea is what a hidden title bar still covers. GTK hides its
// decorations and draws no window controls of ours, so only the app's
// own bar height can be reported.
func (w *window) titleBarArea() core.Edges {
	if !w.hidden() {
		return core.Edges{}
	}
	return core.Edges{Top: w.chrome.height}
}

// unsupported reports the roles GTK has no use for.
func unsupported(r core.MenuRole) bool {
	return r == core.MenuAbout || r == core.MenuHide || r == core.MenuHideOthers || r == core.MenuShowAll
}

// shortcutOf returns e's shortcut, its role's when it has none.
func shortcutOf(e *core.MenuEntry) string {
	if e.Shortcut != "" {
		return e.Shortcut
	}
	return e.Role.Shortcut(false)
}

// menuKeys gathers the shortcuts of entries' items, but the editing
// roles': the focused GtkEntry handles those keys itself.
func (w *window) menuKeys(entries []core.MenuEntry) {
	for i := range entries {
		e := &entries[i]
		switch {
		case e.Separator, e.Disabled, unsupported(e.Role):
		case len(e.Items) > 0:
			w.menuKeys(e.Items)
		default:
			if k := shortcutOf(e); k != "" && !e.Role.EditsText() {
				if _, taken := w.chrome.keys[k]; !taken {
					w.chrome.keys[k] = menuRef{e.ID, e.Role}
				}
			}
		}
	}
}

// hit returns the topmost view under p, in the coordinates of v's
// parent's content, that matches.
func hit(v *view, p core.Point, match func(*view) bool) *view {
	if v.style.Hidden || !v.frame.Contains(p) {
		return nil
	}
	local := core.Point{X: p.X - v.frame.Origin.X + v.offset.X, Y: p.Y - v.frame.Origin.Y + v.offset.Y}
	for i := len(v.children) - 1; i >= 0; i-- {
		if h := hit(v.children[i], local, match); h != nil {
			return h
		}
	}
	if match(v) {
		return v
	}
	return nil
}

// scrollsAt appends the scroll views under p, outermost first; p is in
// the coordinates of v's parent's content.
func scrollsAt(v *view, p core.Point, out []*view) []*view {
	if v.style.Hidden || v.style.Opacity <= 0 || !v.frame.Contains(p) {
		return out
	}
	if v.kind == core.KindScroll {
		out = append(out, v)
	}
	p = core.Point{X: p.X - v.frame.Origin.X + v.offset.X, Y: p.Y - v.frame.Origin.Y + v.offset.Y}
	for i := len(v.children) - 1; i >= 0; i-- {
		if c := v.children[i]; !c.style.Hidden && c.frame.Contains(p) {
			return scrollsAt(c, p, out)
		}
	}
	return out
}

// press reports a click at p in the surface: on text with links, with
// the offset of the character under it; elsewhere the engine finds the
// view.
func (w *window) press(at core.Point) {
	if root := w.view(w.root); root != nil {
		v := hit(root, at, func(v *view) bool {
			return v.events.Has(core.EventPress) || v.kind == core.KindInput || v.kind == core.KindWeb
		})
		if v != nil && v.kind == core.KindWeb {
			return // the page's, even where it has not shown yet
		}
		if v != nil && v.kind == core.KindText {
			if l, ok := v.text.(*textLayout); ok {
				b, _ := w.surfaceBounds(v)
				x := at.X - b.Origin.X - v.content.Origin.X
				y := at.Y - b.Origin.Y - v.content.Origin.Y
				w.engine.Dispatch(core.Event{Kind: core.EventPress, ID: v.id, Index: l.indexAt(x, y)})
				return
			}
		}
	}
	w.engine.PressAt(at)
}

// pressOutside reports a press at p to each view that asks for presses
// outside it and does not contain the point.
func (w *window) pressOutside(at core.Point) {
	for _, v := range w.views {
		if v == nil || !v.events.Has(core.EventPressOutside) {
			continue
		}
		if b, shown := w.surfaceBounds(v); shown && !b.Contains(at) {
			w.engine.Dispatch(core.Event{Kind: core.EventPressOutside, ID: v.id, Point: at})
		}
	}
}

// contextMenu reports a secondary click at p to the view under it that
// asks for it, at p in the view's coordinates.
func (w *window) contextMenu(at core.Point) {
	root := w.view(w.root)
	if root == nil {
		return
	}
	if v := hit(root, at, func(v *view) bool { return v.events.Has(core.EventContextMenu) }); v != nil {
		b, _ := w.surfaceBounds(v)
		w.engine.Dispatch(core.Event{Kind: core.EventContextMenu, ID: v.id,
			Point: core.Point{X: at.X - b.Origin.X, Y: at.Y - b.Origin.Y}})
	}
}
