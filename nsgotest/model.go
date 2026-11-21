package nsgotest

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// View is the model of one native view, as the last batch left it.
type View struct {
	ID       driver.NodeID
	Kind     driver.ViewKind
	Parent   *View
	Children []*View
	Frame    driver.Bounds
	Style    driver.ViewStyle
	Props    driver.ViewProps
	Events   driver.EventMask
	Text     string // a text view's text or a field's value
	// TextStyle and Spans are a text view's resolved style and its
	// spans', which SpanStyle reads.
	TextStyle driver.TextStyle
	Spans     []driver.TextSpan
	// TextShadow is drawn behind a text's glyphs; MaxLines and Ellipsize
	// cut its lines.
	TextShadow driver.TextShadow
	MaxLines   int
	Ellipsize  driver.EllipsizeMode
	// Hovered says the mouse is over the view, as Hover left it.
	Hovered bool
	Input   driver.InputConfig
	Content driver.Size  // a scroll view's content
	Offset  driver.Point // and its offset
	// Applied counts the ItemScrollTo and ItemScrollBy items the scroll
	// view applied, which its reports carry; Sticky are its sticky
	// cells.
	Applied uint32
	Sticky  []driver.StickyCell
	// Scroll is a scroll view's configuration, and ZoomScale its zoom
	// (0 until it zooms).
	Scroll    driver.ScrollConfig
	ZoomScale float32
	Image     driver.ImageConfig
	Web       driver.WebConfig
	// WebCommands are what the app asked a web view to do, oldest first;
	// answered counts those AnswerScript went past.
	WebCommands                      []WebCommand
	answered                         int
	Video                            driver.VideoConfig
	PictureInPicture                 driver.PictureInPictureConfig
	PiPState                         core.PictureInPictureState
	PiPStarts, PiPStops, PiPRestores int
	// Seeks are the positions the app asked a video to jump to, oldest
	// first.
	Seeks []time.Duration
	// Component and NativeProps are a native view's component and props,
	// an animated one at the value the frame clock last handed over;
	// NativeCommands what Ref.Command sent it, oldest first.
	Component      string
	NativeProps    map[string]any
	NativeCommands []NativeCommand
	// Animated are the values of the animated properties bound to the
	// view, which the model's animator steps with the clock.
	Animated driver.AnimatedValues
	Focused  bool
	// Selection is a field's selection, as the app last set it or Select
	// moved it, in UTF-16 units.
	Selection driver.TextRange
	// Composing is the text an input method composes in a field (Compose);
	// held is a value the app set meanwhile, applied when it ends.
	Composing string
	held      *driver.MountItem
	// Screen is a screen's system header, sent where the environment
	// has HeaderHeight; Modal a modal's config.
	Screen driver.ScreenConfig
	Modal  driver.ModalConfig
	// Drawing is a canvas's last drawing, and Ops its operations as the
	// platform plays them now, with the values of the animated nodes its
	// arguments are bound to.
	Drawing *driver.Drawing
	Ops     []driver.DrawOp
	// Shader is what the view's shader draws, a shader view's fill or a
	// view's effect, transition or navigation shader, nil for none.
	// ShaderStarts counts the transitions it started
	// (ItemShaderTransition), ShaderInputs the batches that said its
	// pictures changed (ItemShaderInput).
	Shader       *Shader
	ShaderStarts int
	ShaderInputs int
	// Node is the part of a drawing a view of a canvas's names: a canvas
	// shows each of its drawing's nodes as a child, which selectors find
	// by its label (label: and text:), test ID and role, and which presses
	// at its center.
	Node *driver.CanvasNode
	anim *driver.Animator
}

// clamp keeps a scroll view's offset within its content and insets, as
// platforms do.
func (v *View) clamp(p driver.Point) driver.Point {
	in := v.Scroll.Behavior.ContentInset
	p.X = min(max(p.X, -in.Left), max(v.Content.Width-v.Frame.Size.Width, 0)+in.Right)
	p.Y = min(max(p.Y, -in.Top), max(v.Content.Height-v.Frame.Size.Height, 0)+in.Bottom)
	return p
}

// StickyShift is how far the sticky cell id of scroll view v moves from
// its frame at v's offset, as a platform moves it.
func (v *View) StickyShift(id driver.NodeID) driver.Points {
	for _, c := range v.Sticky {
		if c.Cell == id {
			if v.Scroll.Horizontal {
				return v.anim.StickyShift(c, v.Offset.X)
			}
			return v.anim.StickyShift(c, v.Offset.Y)
		}
	}
	return 0
}

// Bindings are the animated values, by node, a view's gestures and
// scrolling set; 0 is none.
type Bindings struct {
	PanX, PanY, ScrollX, ScrollY, Pinch, Rotation, Pull uint32
}

// Bound returns the animated values the gestures or the scrolling of
// the view selector matches set (AnimBindPan, ...), which AnimatedValue
// reads.
func (a *App) Bound(selector string) Bindings {
	a.t.Helper()
	if b := a.bound[a.Find(selector).ID]; b != nil {
		return *b
	}
	return Bindings{}
}

// AnimatedValue returns animated value node's value as the platform's
// animator has it.
func (a *App) AnimatedValue(node uint32) float64 {
	v, _, _ := a.anim.Value(node)
	return v
}

// Animated returns the style the view selector matches shows: its own,
// with the animated values bound to it drawn over it as the platform's
// animator has them.
func (a *App) Animated(selector string) driver.ViewStyle {
	a.t.Helper()
	v := a.Find(selector)
	vals, _ := a.anim.Values(v.ID)
	return vals.Apply(v.Style)
}

// NativeCommand is a command a native view received.
type NativeCommand struct {
	Name string
	Args any
}

// WebCommand is a command a web view received, with its message or script in Value and any structured argument in Data (such as WebLoad's WebSource).
type WebCommand struct {
	Command driver.WebCommand
	Value   string
	Data    any
}

// animate steps the animator to the clock and records the values it
// moved.
func (a *App) animate() {
	if a.anim.Running() {
		a.anim.Step(a.Clock.Now().Sub(a.start))
	}
	a.anim.Changed(func(id driver.NodeID, vals driver.AnimatedValues) {
		if v := a.views[id]; v != nil {
			v.Animated = vals
		}
	})
	a.anim.ChangedCanvases(func(id driver.NodeID) {
		if v := a.views[id]; v != nil && v.Drawing != nil {
			v.Ops = slices.Clone(a.anim.CanvasOps(id, v.Drawing))
		}
	})
	a.anim.ChangedNatives(func(id driver.NodeID, b driver.NativeBinding, num float64, col driver.Color) {
		if v := a.views[id]; v != nil {
			if v.NativeProps == nil {
				v.NativeProps = map[string]any{}
			}
			if b.Color {
				v.NativeProps[b.Name] = col
			} else {
				v.NativeProps[b.Name] = num
			}
		}
	})
}

// drawCanvas keeps canvas v's drawing, binds its slots and shows its
// nodes as children.
func (a *App) drawCanvas(v *View, d *driver.Drawing) {
	if v == nil {
		return
	}
	v.Drawing = d
	a.anim.BindCanvas(v.ID, d.Slots)
	v.Ops = slices.Clone(a.anim.CanvasOps(v.ID, d))
	pass := a.canvasImageUse
	for i := range d.Ops {
		if op := &d.Ops[i]; op.Kind == driver.DrawImage && op.Image != nil {
			a.touchCanvasImage(*op.Image, pass)
		}
	}
	for i := range v.Ops {
		if op := &v.Ops[i]; op.Kind == driver.DrawImage && op.Image != nil && op.Image.Offscreen != nil {
			a.recordOffscreen(op.Image.Offscreen, nil)
		}
	}
	kept := v.Children[:0]
	for _, c := range v.Children {
		if c.Node == nil {
			kept = append(kept, c)
		}
	}
	v.Children = kept
	for i := range d.Nodes {
		n := &d.Nodes[i]
		v.Children = append(v.Children, &View{Kind: driver.KindView, Parent: v, Frame: n.Bounds, Node: n,
			Style: driver.ViewStyle{Opacity: 1},
			Props: driver.ViewProps{Role: n.Role, Label: n.Label, TestID: n.TestID, Selected: n.Selected}})
	}
}

func (a *App) recordOffscreen(off *driver.OffscreenCanvas, seen map[uint64]bool) {
	if off == nil {
		return
	}
	if seen != nil && seen[off.ID] {
		return
	}
	if a.offscreenSurfaces == nil {
		a.offscreenSurfaces = map[uint64]*OffscreenSurface{}
	}
	s := a.offscreenSurfaces[off.ID]
	if s == nil {
		s = &OffscreenSurface{ID: off.ID}
		a.offscreenSurfaces[off.ID] = s
	}
	if s.Version == off.Version && !s.Released {
		return
	}
	s.Version = off.Version
	s.RenderCount++
	s.Size = off.Size
	s.Scale = off.EffectiveScale()
	s.Released = false
	s.Ops = nil
	if off.Drawing != nil {
		s.Ops = slices.Clone(off.Drawing.Ops)
		if seen == nil {
			seen = map[uint64]bool{}
		}
		seen[off.ID] = true
		for i := range s.Ops {
			if op := &s.Ops[i]; op.Kind == driver.DrawImage && op.Image != nil && op.Image.Offscreen != nil {
				a.recordOffscreen(op.Image.Offscreen, seen)
			}
		}
	}
}

// Offscreen returns the headless model's record of off, or nil if it has
// never been drawn or released on this app.
func (a *App) Offscreen(off *core.OffscreenImage) *OffscreenSurface {
	if a == nil || off == nil {
		return nil
	}
	return a.offscreenSurfaces[off.ID()]
}

func (a *App) dispatch(ev driver.Event) {
	a.dispatchOn(a.tree, ev)
}

// dispatchOn sends ev to tree, the surface steps act on when nil, and settles: an answer goes to the surface that asked.
func (a *App) dispatchOn(tree *core.Tree, ev driver.Event) {
	if a.target != nil {
		// What the test makes up, a real platform reports itself.
		a.t.Skipf("nsgotest: the test reports what a platform would (%s): it runs headless only", a.stepName)
	}
	if tree == nil {
		tree = a.tree
	}
	if ev.Kind == driver.EventTouch && ev.TouchGeometry == nil && tree == a.tree {
		a.touchGeometry.Capture(&ev, func(p driver.Point) driver.NodeID {
			if v := a.viewAt(p, func(*View) bool { return true }); v != nil {
				return v.ID
			}
			return 0
		}, func(id driver.NodeID) (driver.NodeID, driver.EventMask) {
			v := a.views[id]
			if v == nil {
				return 0, 0
			}
			var up driver.NodeID
			if v.Parent != nil {
				up = v.Parent.ID
			}
			return up, v.Events
		}, func(id driver.NodeID, p driver.Point) (driver.Point, bool) {
			if v := a.views[id]; v != nil {
				return v.localPoint(p), true
			}
			return driver.Point{}, false
		})
	}
	tree.Dispatch(ev)
	a.Settle()
}

// inputBy reports the input the next step gives, a key
// (driver.ModalityKeyboard) or a press (ModalityPointer), as it changes,
// before what it causes, as platforms do.
func (a *App) inputBy(m uint32) {
	if a.target != nil || a.modality == m {
		return
	}
	a.modality = m
	a.tree.Dispatch(driver.Event{Kind: driver.EventInputModality, Count: m})
}

// send dispatches ev to v when v reports its kind, as platforms do.
func (a *App) send(v *View, ev driver.Event) {
	if v.Events.Has(ev.Kind) {
		ev.ID = v.ID
		a.dispatch(ev)
	}
}

// applyAside applies a batch of a surface steps do not act on, the app's own or a window other than the main one: what it asks of the platform, such as alerts, permissions and context menus, which are answered to it, and the app's own application menu, but not its views, keyboard shortcuts, title or window commands, nor another window's menu bar, which the model keeps for the main window.
func (a *App) applyAside(b *driver.MountBatch) {
	aside := *b
	aside.LayoutAnimation = nil
	aside.Items = slices.DeleteFunc(slices.Clone(b.Items), func(it driver.MountItem) bool {
		return surfaceItem(it) && !(it.Kind == driver.ItemMenu && a.responseTree == a.appTree)
	})
	root := a.rootID
	a.apply(&aside)
	a.rootID = root
}

// surfaceItem identifies requests and view changes that belong to the
// window's own model rather than acting on the app's shared services.
func surfaceItem(it driver.MountItem) bool {
	switch it.Kind {
	case driver.ItemAnimation, driver.ItemDismissKeyboard, driver.ItemBlockNative, driver.ItemAccessibilityFocus, driver.ItemTitle, driver.ItemWindow, driver.ItemKeymap, driver.ItemMenu:
		return true
	}
	return it.ID != 0
}

func (a *App) applySurface(b *driver.MountBatch) {
	window := *b
	window.Items = slices.DeleteFunc(slices.Clone(b.Items), func(it driver.MountItem) bool {
		return !surfaceItem(it)
	})
	a.apply(&window)
}

// setMenu records the application menu tree set, nil restoring the platform's.
func (a *App) setMenu(tree *core.Tree, menu []driver.MenuEntry) {
	if a.menus == nil {
		a.menus = map[*core.Tree][]driver.MenuEntry{}
	}
	a.menus[tree] = menu
	a.menuBar = tree
	a.showMenu()
}

// showMenu shows, on macOS, the menu of the surface that sent one last, as its one menu bar and the core (menuHasKey) do: none once that surface restored the platform's or closed. Other desktops show the main window's menu, else the app's own surface's.
func (a *App) showMenu() {
	if a.env != nil && a.env.OS == core.OSMacOS && a.menuBar != nil {
		a.menu, a.menuTree = a.menus[a.menuBar], a.menuBar
	} else {
		a.menu, a.menuTree = a.menus[a.tree], a.tree
		if a.menu == nil && a.appTree != nil {
			a.menu, a.menuTree = a.menus[a.appTree], a.appTree
		}
		if a.menu == nil && a.target != nil {
			a.menu = a.target.ownMenu
		}
	}
	a.Menu = menuItems(a.menu)
}

// closed forgets what a surface that closed left in the model: a context menu it showed, its menu, and for the main window its views and keyboard shortcuts, as nothing is left there for steps to reach.
func (a *App) closed(surface *core.Tree) {
	delete(a.menus, surface)
	if a.dockTree == surface && surface != nil {
		a.dockMenu, a.DockMenu, a.dockTree = nil, nil, nil
	}
	if a.popupTree == surface {
		a.clearPopupMenu()
	}
	if surface == a.tree {
		a.views, a.rootID, a.tree, a.main, a.Keymap, a.keyBindings, a.heldKeys, a.heldMouse = map[driver.NodeID]*View{}, 0, a.appTree, nil, nil, nil, false, false
	}
	// A key or mouse button held stays held, but its up has no window to go to.
	for name, h := range a.held {
		if h.tree == surface {
			a.held[name] = heldKey{}
		}
	}
	for btn, h := range a.heldButtons {
		if h.tree == surface {
			a.heldButtons[btn] = heldMouseButton{}
		}
	}
	a.showMenu()
}

// viewItemNames identifies items that require a view even when ID is zero. Names also keep malformed batches useful as test failures.
var viewItemNames = map[driver.ItemKind]string{
	driver.ItemInsert:             "ItemInsert",
	driver.ItemRemove:             "ItemRemove",
	driver.ItemDelete:             "ItemDelete",
	driver.ItemFrame:              "ItemFrame",
	driver.ItemViewStyle:          "ItemViewStyle",
	driver.ItemText:               "ItemText",
	driver.ItemInput:              "ItemInput",
	driver.ItemInputValue:         "ItemInputValue",
	driver.ItemEvents:             "ItemEvents",
	driver.ItemViewProps:          "ItemViewProps",
	driver.ItemScrollConfig:       "ItemScrollConfig",
	driver.ItemScrollContent:      "ItemScrollContent",
	driver.ItemFocus:              "ItemFocus",
	driver.ItemBlur:               "ItemBlur",
	driver.ItemImage:              "ItemImage",
	driver.ItemModal:              "ItemModal",
	driver.ItemScrollTo:           "ItemScrollTo",
	driver.ItemCanvas:             "ItemCanvas",
	driver.ItemWeb:                "ItemWeb",
	driver.ItemWebCommand:         "ItemWebCommand",
	driver.ItemVideo:              "ItemVideo",
	driver.ItemVideoSeek:          "ItemVideoSeek",
	driver.ItemNative:             "ItemNative",
	driver.ItemNativeCommand:      "ItemNativeCommand",
	driver.ItemZoomTo:             "ItemZoomTo",
	driver.ItemSelection:          "ItemSelection",
	driver.ItemScrollBy:           "ItemScrollBy",
	driver.ItemStickyCells:        "ItemStickyCells",
	driver.ItemAccessibilityLabel: "ItemAccessibilityLabel",
	driver.ItemAccessibilityFocus: "ItemAccessibilityFocus",
	driver.ItemScreen:             "ItemScreen",
	driver.ItemOverflow:           "ItemOverflow",
	driver.ItemShader:             "ItemShader",
	driver.ItemShaderTransition:   "ItemShaderTransition",
	driver.ItemShaderInput:        "ItemShaderInput",
	driver.ItemMatch:              "ItemMatch",
	driver.ItemGPU:                "ItemGPU",
}

func (a *App) apply(b *driver.MountBatch) {
	a.rootID = b.Root
	if b.LayoutAnimation != nil {
		a.LayoutAnimation = b.LayoutAnimation
	}
	var resized []*View
	var gpuConfigured []*View
	for _, it := range b.Items {
		v := a.views[it.ID]
		if name := viewItemNames[it.Kind]; v == nil && (name != "" || it.ID != 0 && it.Kind != driver.ItemCreate) {
			if name == "" {
				name = fmt.Sprintf("item kind %d", it.Kind)
			}
			a.t.Fatalf("nsgotest: %s names unknown view %d", name, it.ID)
		}
		switch it.Kind {
		case driver.ItemCreate:
			a.views[it.ID] = &View{ID: it.ID, Kind: it.View, Style: driver.DefaultViewStyle, anim: a.anim}
		case driver.ItemDelete:
			if v != nil {
				v.detach()
			}
			a.anim.Forget(it.ID)
			a.deleteGPUView(it.ID)
			delete(a.views, it.ID)
			delete(a.edits, it.ID)
		case driver.ItemRemove:
			v.detach()
		case driver.ItemInsert:
			v.detach()
			p := a.views[it.Parent]
			if p == nil {
				a.t.Fatalf("nsgotest: ItemInsert for view %d names unknown parent %d", it.ID, it.Parent)
			}
			at := len(p.Children)
			for i, c := range p.Children {
				if c.ID == it.Before {
					at = i
				}
			}
			p.Children = append(p.Children[:at], append([]*View{v}, p.Children[at:]...)...)
			v.Parent = p
		case driver.ItemFrame:
			v.Frame = it.Frame
			resized = append(resized, v)
		case driver.ItemViewStyle:
			v.Style = *it.ViewStyle
		case driver.ItemViewProps:
			v.Props = *it.ViewProps
		case driver.ItemAccessibilityLabel:
			v.Props.Label = it.Value
		case driver.ItemAccessibilityFocus:
			a.AccessibilityFocus = v
		case driver.ItemEvents:
			v.Events = it.Events
		case driver.ItemText:
			v.Text, v.TextStyle, v.Spans, v.TextShadow = it.Text.Text, it.Text.Style, it.Text.Spans, it.Text.Shadow
			v.MaxLines, v.Ellipsize = it.Text.MaxLines, it.Text.Ellipsize
		case driver.ItemInput:
			v.Input = *it.Input
		case driver.ItemInputValue:
			if v.Composing != "" {
				held := it
				v.held = &held
			} else if a.edits[it.ID] <= it.Count {
				v.Text = it.Value
			}
		case driver.ItemSelection:
			n := driver.UTF16Len(v.Text)
			r := it.Selection
			v.Selection = driver.TextRange{Start: min(max(r.Start, 0), n), End: min(max(r.End, 0), n)}
		case driver.ItemScrollContent:
			v.Content = it.Size
			resized = append(resized, v)
		case driver.ItemScrollConfig:
			if !v.Scroll.Zooms() && it.Scroll.Zooms() || it.Scroll.ZoomScale != v.Scroll.ZoomScale {
				v.ZoomScale = it.Scroll.ZoomScale
			}
			v.Scroll = *it.Scroll
			a.pullValue(v)
		case driver.ItemZoomTo:
			v.ZoomScale = it.Zoom.Scale
			id, at, z := it.ID, v.Offset, v.ZoomScale
			a.post(driver.Event{Kind: driver.EventScroll, ID: id, Point: at, Value: z})
		case driver.ItemImage:
			v.Image = *it.Image
			if it.Image != nil && it.Image.Source.Offscreen != nil {
				a.recordOffscreen(it.Image.Source.Offscreen, nil)
			}
		case driver.ItemWeb:
			v.Web = *it.Web
		case driver.ItemWebCommand:
			v.WebCommands = append(v.WebCommands, WebCommand{Command: it.WebCommand, Value: it.Value, Data: it.Data})
		case driver.ItemVideo:
			enabled := v.Video.PictureInPicture
			v.Video = *it.Video
			if it.Video.PictureInPicture && !enabled {
				a.configurePiP(v)
			}
		case driver.ItemPictureInPicture:
			first := v.PictureInPicture.Size == (driver.Size{})
			v.PictureInPicture = *it.PictureInPicture
			if first {
				a.configurePiP(v)
			}
		case driver.ItemPictureInPictureCommand:
			if v != nil {
				a.commandPiP(v, it.Count)
			}
		case driver.ItemVideoSeek:
			v.Seeks = append(v.Seeks, it.Time)
		case driver.ItemNative:
			v.Component = it.Native.Component
			if v.NativeProps == nil {
				v.NativeProps = map[string]any{}
			}
			for k, p := range it.Native.Props {
				if p == nil {
					delete(v.NativeProps, k)
				} else {
					v.NativeProps[k] = p
				}
			}
			if it.Native.Bind {
				a.anim.BindNative(it.ID, it.Native.Bindings)
			}
		case driver.ItemNativeCommand:
			v.NativeCommands = append(v.NativeCommands, NativeCommand{it.Value, it.Data})
		case driver.ItemHotKeys:
			// As the desktop platforms do, a combination the app registered already is taken.
			held := map[string]bool{}
			for _, key := range it.Keys {
				state := core.HotKeyRegistered
				if configured, ok := a.hotKeyStates[key.Key]; ok {
					state = configured
				} else if held[key.Key] {
					state = core.HotKeyTaken
				}
				held[key.Key] = true
				a.post(driver.Event{Kind: driver.EventHotKeyState, Count: key.ID, Index: int(state)})
			}
		case driver.ItemCanvas:
			a.drawCanvas(v, it.Drawing)
		case driver.ItemShader:
			v.Shader = nil
			if sh := it.Shader; sh != nil {
				v.Shader = &Shader{Role: sh.Role, WGSL: sh.Program != nil && sh.Program.WGSL, Lanes: sh.Lanes, Nodes: sh.Nodes,
					Pictures: sh.Pictures, Overflow: sh.Overflow, Running: sh.Running, Backwards: sh.Backwards}
				pass := a.canvasImageUse
				for _, src := range sh.Images {
					a.touchCanvasImage(src, pass)
				}
			}
		case driver.ItemShaderTransition:
			v.ShaderStarts++
		case driver.ItemShaderInput:
			v.ShaderInputs++
		case driver.ItemGPU:
			if it.GPU != nil {
				a.configureGPUView(v, *it.GPU)
				gpuConfigured = append(gpuConfigured, v)
			}
		case driver.ItemAnimation:
			a.anim.Apply(it.Animation)
			for _, op := range it.Animation {
				if a.bound == nil {
					a.bound = map[driver.NodeID]*Bindings{}
				}
				b := a.bound[op.View]
				if b == nil {
					b = &Bindings{}
					a.bound[op.View] = b
				}
				switch op.Kind {
				case driver.AnimBindPan:
					b.PanX, b.PanY = op.X, op.Y
				case driver.AnimBindScroll:
					b.ScrollX, b.ScrollY = op.X, op.Y
				case driver.AnimBindPinch:
					b.Pinch = op.Node
				case driver.AnimBindRotation:
					b.Rotation = op.Node
				case driver.AnimBindPull:
					b.Pull = op.Node
				}
			}
		case driver.ItemFocus, driver.ItemBlur:
			if a.target != nil && a.target.reportsNativeFocus() {
				continue
			}
			if it.Kind == driver.ItemBlur {
				a.setFocus(v, false)
				continue
			}
			if !a.canFocus(v) {
				continue
			}
			for _, o := range a.views {
				if o != v {
					a.setFocus(o, false)
				}
			}
			a.setFocus(v, true)
		case driver.ItemScrollTo, driver.ItemScrollBy:
			v.Applied++
			if it.Kind == driver.ItemScrollBy {
				v.Offset.X += it.Point.X
				v.Offset.Y += it.Point.Y
			} else {
				v.Offset = v.clamp(it.Point)
			}
			a.anim.Scroll(v.ID, v.Offset)
			// A platform reports where a scroll lands, and that an animated
			// one came to rest.
			id, at, n := it.ID, v.Offset, v.Applied
			a.post(driver.Event{Kind: driver.EventScroll, ID: id, Point: at, Count: n})
			if it.Animated && v.Events.Has(driver.EventScrollEnd) {
				a.post(driver.Event{Kind: driver.EventScrollEnd, ID: id, Point: at, Count: n})
			}
		case driver.ItemStickyCells:
			v.Sticky = it.Sticky
		case driver.ItemSetClipboard:
			a.Clipboard, a.ClipboardImage, a.ClipboardFiles = it.Value, nil, nil
		case driver.ItemSetClipboardImage:
			data, _ := it.Data.([]byte)
			a.Clipboard, a.ClipboardImage, a.ClipboardFiles = "", data, nil
		case driver.ItemReadClipboard:
			a.readClipboard(&it)
		case driver.ItemOpenURL:
			a.OpenedURLs = append(a.OpenedURLs, it.Value)
			opened := a.OpenURL == nil || a.OpenURL(it.Value)
			a.post(driver.Event{Kind: driver.EventOpenURL, Count: it.Count, Finished: opened})
		case driver.ItemCheckForUpdates:
			a.checkForUpdates(&it)
		case driver.ItemNotificationPermission:
			if it.On && a.NotificationPermission == core.NotificationsNotAsked {
				a.NotificationPermission = cmp.Or(a.NotificationAnswer, core.NotificationsGranted)
			}
			a.post(driver.Event{Kind: driver.EventNotification, Count: it.Count, Index: int(a.NotificationPermission)})
		case driver.ItemShowNotification:
			shown := a.NotificationPermission == core.NotificationsGranted
			if shown {
				d := it.Notification
				n := core.Notification{ID: d.ID, Title: d.Title, Body: d.Body, Silent: d.Silent, At: d.At, Repeat: core.NotificationRepeat(d.Repeat)}
				a.PendingNotifications = slices.DeleteFunc(a.PendingNotifications, func(o core.Notification) bool { return o.ID == n.ID })
				if n.At.IsZero() {
					a.showNotification(n)
				} else {
					a.PendingNotifications = append(a.PendingNotifications, n)
				}
			}
			a.post(driver.Event{Kind: driver.EventNotification, Count: it.Count, Finished: shown})
		case driver.ItemCloseNotification:
			if it.Value == "" {
				a.Notifications, a.PendingNotifications = nil, nil
			} else {
				a.Notifications = slices.DeleteFunc(a.Notifications, func(o core.Notification) bool { return o.ID == it.Value })
				a.PendingNotifications = slices.DeleteFunc(a.PendingNotifications, func(o core.Notification) bool { return o.ID == it.Value })
			}
		case driver.ItemOrientation:
			a.Orientation = core.Orientation(it.Count)
		case driver.ItemBadge:
			a.Badge = int(it.Count)
		case driver.ItemKeepAwake:
			if it.Awake != nil {
				hold := AwakeHold{ID: it.Count, Reason: it.Awake.Reason, Display: it.Awake.Display}
				a.AwakeHolds = append(a.AwakeHolds, hold)
				a.Awake = append(a.Awake, hold)
			}
		case driver.ItemReleaseAwake:
			a.AwakeReleases = append(a.AwakeReleases, it.Count)
			a.Awake = slices.DeleteFunc(a.Awake, func(h AwakeHold) bool { return h.ID == it.Count })
		case driver.ItemReleaseOffscreen:
			if a.offscreenSurfaces == nil {
				a.offscreenSurfaces = map[uint64]*OffscreenSurface{}
			}
			s := a.offscreenSurfaces[it.OffscreenID]
			if s == nil {
				s = &OffscreenSurface{ID: it.OffscreenID}
				a.offscreenSurfaces[it.OffscreenID] = s
			}
			s.Released = true
			s.Ops = nil
		case driver.ItemTaskProgress:
			if it.Progress == nil {
				a.TaskProgress = core.TaskProgress{}
			} else {
				a.TaskProgress = core.TaskProgress{State: core.TaskProgressState(it.Progress.State), Value: it.Progress.Value}
			}
		case driver.ItemBiometricAvailability:
			a.post(driver.Event{Kind: driver.EventBiometric, Count: it.Count, Index: int(a.BiometricAvailability)})
		case driver.ItemBiometricAuthenticate:
			a.AuthenticationReasons = append(a.AuthenticationReasons, it.Value)
			result := a.BiometricAnswer
			if result == 0 {
				result = core.BiometricUnavailable
			}
			a.post(driver.Event{Kind: driver.EventBiometric, Count: it.Count, Index: int(result)})
		case driver.ItemSignInWithApple:
			a.signInWithApple(&it)
		case driver.ItemAppleCredentialState:
			state := cmp.Or(a.AppleCredentialStates[it.Value], core.AppleCredentialNotFound)
			a.post(driver.Event{Kind: driver.EventSignInWithApple, Count: it.Count, Index: int(state)})
		case driver.ItemPurchase:
			a.purchase(it.Count, *it.Purchase)
		case driver.ItemPermission:
			a.permission(&it)
		case driver.ItemPickImages:
			a.Picks = append(a.Picks, *it.Pick)
			var files []driver.PickedFile
			if a.target == nil && a.PickImages != nil {
				files = a.PickImages(*it.Pick)
			}
			if a.target == nil {
				a.post(driver.Event{Kind: driver.EventPick, Count: it.Count, Data: files})
			}
		case driver.ItemTakePhoto:
			if a.target != nil {
				// The target observer routes this request through testhost.Options.TakePhoto.
				continue
			}
			a.Photos = append(a.Photos, *it.Pick)
			ev := driver.Event{Kind: driver.EventPick, Count: it.Count}
			if a.TakePhoto != nil {
				photo, err := a.TakePhoto(*it.Pick)
				switch {
				case err != nil:
					ev.Text = err.Error()
				case photo != nil:
					ev.Data = []driver.PickedFile{*photo}
				}
			}
			a.post(ev)
		case driver.ItemScanCode:
			a.Scans = append(a.Scans, *it.Scan)
			if a.target != nil {
				// The target's engine asks targetOptions' ScanCode.
				break
			}
			ev := driver.Event{Kind: driver.EventPick, Count: it.Count}
			if a.ScanCode != nil {
				code, err := a.ScanCode(*it.Scan)
				switch {
				case err != nil:
					ev.Text = err.Error()
				case code != nil:
					ev.Data = code
				}
			}
			a.post(ev)
		case driver.ItemPickFiles:
			a.FilePicks = append(a.FilePicks, *it.Files)
			var files []driver.File
			if a.PickFiles != nil {
				files = a.PickFiles(*it.Files)
			}
			a.post(driver.Event{Kind: driver.EventFiles, Count: it.Count, Data: files})
		case driver.ItemPickDirectories:
			a.DirectoryPicks = append(a.DirectoryPicks, *it.Directories)
			var dirs []driver.Directory
			if a.PickDirectories != nil {
				dirs = a.PickDirectories(*it.Directories)
			}
			a.post(driver.Event{Kind: driver.EventFiles, Count: it.Count, Data: dirs})
		case driver.ItemPickScreenColor:
			ev := driver.Event{Kind: driver.EventFiles, Count: it.Count}
			if a.PickScreenColor != nil {
				if c, ok := a.PickScreenColor(); ok {
					ev.Data, ev.Finished = c, true
				}
			}
			a.post(ev)
		case driver.ItemSaveFile:
			a.Saves = append(a.Saves, *it.Save)
			ev := driver.Event{Kind: driver.EventFiles, Count: it.Count}
			if a.SaveFile != nil {
				if path := a.SaveFile(*it.Save); path != "" {
					if err := it.Save.SaveTo(path); err != nil {
						ev.Text = err.Error()
					} else {
						ev.Finished, ev.Data = true, path
					}
				}
			}
			a.post(ev)
		case driver.ItemOpenFile:
			a.OpenedFiles = append(a.OpenedFiles, *it.Open)
			opened := a.OpenFile == nil || a.OpenFile(*it.Open)
			a.post(driver.Event{Kind: driver.EventFiles, Count: it.Count, Finished: opened})
		case driver.ItemRevealFile:
			a.RevealedFiles = append(a.RevealedFiles, it.Open.Path)
			ok := a.RevealFile == nil || a.RevealFile(it.Open.Path)
			a.post(driver.Event{Kind: driver.EventFiles, Count: it.Count, Finished: ok})
		case driver.ItemTrashFile:
			a.TrashedFiles = append(a.TrashedFiles, it.Open.Path)
			ev := driver.Event{Kind: driver.EventFiles, Count: it.Count}
			if a.TrashFile == nil {
				ev.Finished = true
			} else if path, err := a.TrashFile(it.Open.Path); errors.Is(err, core.ErrNoTrash) {
				ev.Text = driver.NoTrash
			} else if err != nil {
				ev.Text = err.Error()
			} else {
				ev.Finished, ev.Data = true, path
			}
			a.post(ev)
		case driver.ItemInstallPackage:
			a.Installs = append(a.Installs, *it.Open)
			a.post(driver.Event{Kind: driver.EventFiles, Count: it.Count, Finished: true})
		case driver.ItemCanInstallPackages:
			a.post(driver.Event{Kind: driver.EventFiles, Count: it.Count, Finished: a.InstallAllowed})
		case driver.ItemInstallSettings:
			a.InstallSettings++
			a.post(driver.Event{Kind: driver.EventFiles, Count: it.Count, Finished: true})
		case driver.ItemLookUp:
			a.LookUps = append(a.LookUps, LookUp{Text: it.Value, At: it.Frame})
		case driver.ItemRequestReview:
			a.ReviewRequests++
		case driver.ItemShare:
			a.Shares = append(a.Shares, *it.Share)
			a.post(driver.Event{Kind: driver.EventFiles, Count: it.Count, Finished: true})
		case driver.ItemAppearance:
			a.Appearance = it.Appearance
		case driver.ItemQuit:
			a.Quits++
			if it.On {
				a.Restarts++
			}
		case driver.ItemQuitResponse:
			// OnQuitRequest has accepted the request, and the platform quits.
			if it.On {
				a.Quits++
			}
		case driver.ItemPopupMenu:
			// One context menu shows at a time, whichever surface asks.
			if a.popupID != 0 {
				a.postTo(a.popupTree, driver.Event{Kind: driver.EventMenuClosed, Count: a.popupID})
			}
			a.popupMenu, a.PopupMenu = it.Menu, menuItems(it.Menu)
			a.PopupAt, a.popupID, a.popupTree = it.Point, it.Count, a.responseTree
		case driver.ItemImageInfo:
			ev := driver.Event{Kind: driver.EventImageInfo, Count: it.Count}
			if size, ok := a.ImageSizes[it.Image.Source.URI]; ok {
				ev.Size = size
			} else {
				ev.Text = "nsgotest: no image at " + it.Image.Source.URI
			}
			a.post(ev)
		case driver.ItemKeepImage:
			a.keepCanvasImage(it.Value)
		case driver.ItemReleaseImage:
			a.releaseCanvasImage(it.Value)
		case driver.ItemKeymap:
			a.keyBindings = slices.Clone(it.Keys)
			a.heldKeys = it.HeldKeys
			a.heldMouse = it.HeldMouseButtons
			a.Keymap = a.Keymap[:0]
			for _, b := range it.Keys {
				if !slices.Contains(a.Keymap, b.Key) {
					a.Keymap = append(a.Keymap, b.Key)
				}
			}
		case driver.ItemTitle:
			a.Title = it.Value
		case driver.ItemStatusBar:
			a.StatusBar = StatusBarConfig{Style: it.StatusBar.Style, Hidden: it.StatusBar.Hidden, Animated: it.StatusBar.Animated}
		case driver.ItemScreen:
			v.Screen = *it.Screen
		case driver.ItemModal:
			v.Modal = *it.Modal
		case driver.ItemDirection:
			a.Language, a.LanguageForced = it.Language, it.Forced
		case driver.ItemMenu:
			a.setMenu(a.responseTree, it.Menu)
		case driver.ItemDockMenu:
			// The app has one, whichever surface set it last.
			a.dockMenu, a.DockMenu, a.dockTree = it.Menu, menuItems(it.Menu), a.responseTree
		case driver.ItemWindow:
			a.Window = append(a.Window, *it.Window)
		case driver.ItemOpenSurface:
			a.Opened = append(a.Opened, surfaceRecord(it.Surface))
			if it.Surface.Kind == driver.SurfaceOverlay {
				a.openOverlay(it.Surface)
			}
			if it.Surface.Transparent && (it.Surface.Kind == driver.SurfaceWindow || it.Surface.Kind == driver.SurfacePanel) {
				if a.clear == nil {
					a.clear = map[uint32]bool{}
				}
				a.clear[it.Surface.ID] = true
			}
		case driver.ItemOverlayPermission:
			if it.On && a.OverlayPermission != core.OverlayGranted && a.OverlayPermission != core.OverlayUnavailable {
				a.OverlaySettings++
				a.OverlayPermission = a.OverlayAnswer
			}
			a.post(driver.Event{Kind: driver.EventOverlayPermission, Count: it.Count, Index: int(a.OverlayPermission)})
		case driver.ItemWebCookies:
			a.webCookies(it.Count, it.Cookies)
		case driver.ItemAnnounce:
			a.Announced = append(a.Announced, it.Value)
		case driver.ItemInputSource:
			if it.Value != "" {
				a.inputSourceWas, a.InputSource = a.InputSource, it.Value
			} else {
				a.InputSource = a.inputSourceWas
			}
		case driver.ItemSensor:
			a.sensor(it.Count, *it.Sensor)
		case driver.ItemSpeech:
			a.speech(it.Count, *it.Speech)
		case driver.ItemAuthSession:
			a.authSession(it.Count, *it.Auth)
		case driver.ItemAudio:
			a.audio(it.Count, *it.Audio)
		case driver.ItemSpeak:
			a.speak(it.Count, *it.Speak)
		case driver.ItemAlert:
			a.alerts = append(a.alerts, *it.Alert)
			if a.target == nil {
				a.alertTrees = append(a.alertTrees, a.responseTree)
			}
		case driver.ItemBlockNative:
			// The touch responder keeps native drags from starting.
			a.blocked = it.ID != 0
		case driver.ItemBackUnhandled:
			a.BackUnhandled++
		case driver.ItemDismissKeyboard:
			for _, o := range a.views {
				a.setFocus(o, false)
			}
		}
	}
	a.syncGPUViews(gpuConfigured, resized)
	// When content shrinks or the viewport grows, pull an offset past the new end back after the batch's scroll commands keep the visible content in place.
	for _, v := range resized {
		if v.Kind != driver.KindScroll || v.Scroll.Zooms() || a.views[v.ID] != v {
			continue
		}
		end := v.clamp(v.Offset)
		if at := (driver.Point{X: min(v.Offset.X, end.X), Y: min(v.Offset.Y, end.Y)}); at != v.Offset {
			v.Offset = at
			a.anim.Scroll(v.ID, at)
			a.post(driver.Event{Kind: driver.EventScroll, ID: v.ID, Point: at, Count: v.Applied})
		}
	}
}

// canFocus follows the platform's imperative focus contract. A command to a plain view, a hidden stop or a view below a modal changes nothing.
func (a *App) canFocus(v *View) bool {
	if v == nil || v.disabled(nil) {
		return false
	}
	if v.Kind != driver.KindInput && (!a.tabs() || !v.Props.FocusStop(v.Events)) {
		return false
	}
	for p := v; p != nil; p = p.Parent {
		if p.Style.Hidden {
			return false
		}
		if p.ID == a.rootID {
			shown := a.shown()
			for i := len(shown) - 1; i >= 0; i-- {
				if shown[i].Kind == driver.KindModal {
					return v.within(shown[i])
				}
			}
			return true
		}
	}
	return false
}

// post has ev reach the app when it next settles, as a platform's answer
// does.
// speech answers an ItemSpeech as a recognizer would: it listens once
// started, and a stop finishes with what it heard.
func (a *App) speech(id uint32, r driver.SpeechRequest) {
	report := func(e driver.SpeechEvent) { a.post(driver.Event{Kind: driver.EventSpeech, Count: id, Data: &e}) }
	switch r.Command {
	case driver.SpeechStart:
		a.Speech, a.speechID, a.speechText = &SpeechSession{Lang: r.Lang, Interim: r.Interim}, id, ""
		report(driver.SpeechEvent{Phase: driver.SpeechStarted})
	case driver.SpeechStop:
		if a.speechID == id {
			a.Speech = nil
			report(driver.SpeechEvent{Phase: driver.SpeechResult, Text: a.speechText, Final: true})
			report(driver.SpeechEvent{Phase: driver.SpeechEnded, Text: a.speechText})
		}
	case driver.SpeechAbort:
		if a.speechID == id {
			a.Speech = nil
		}
	}
}

// authSession answers an ItemAuthSession as a platform with the system's session and a loopback server would: a start reports the redirect, an open asks AuthAnswer.
func (a *App) authSession(id uint32, r driver.AuthRequest) {
	report := func(e driver.AuthEvent) { a.post(driver.Event{Kind: driver.EventAuthSession, Count: id, Data: &e}) }
	var s *AuthSession
	for _, o := range a.AuthSessions {
		if o.id == id {
			s = o
		}
	}
	switch r.Command {
	case driver.AuthStart:
		s = &AuthSession{id: id, Ephemeral: r.Ephemeral, scheme: r.Scheme != "", Redirect: AuthLoopback + r.Path}
		if s.scheme {
			s.Redirect = driver.AuthRedirectURI(r)
		}
		a.AuthSessions = append(a.AuthSessions, s)
		report(driver.AuthEvent{Phase: driver.AuthRedirect, Redirect: s.Redirect})
	case driver.AuthOpen:
		if s == nil {
			return
		}
		s.URL = r.URL
		if a.AuthAnswer == nil {
			return
		}
		callback, err := a.AuthAnswer(*s)
		var code core.AuthError
		switch {
		case errors.As(err, &code) && err == error(code):
			report(driver.AuthEvent{Phase: driver.AuthEnded, Error: string(code)})
		case errors.As(err, &code):
			report(driver.AuthEvent{Phase: driver.AuthEnded, Error: string(code), Message: err.Error()})
		case err != nil:
			report(driver.AuthEvent{Phase: driver.AuthEnded, Error: driver.AuthFailed, Message: err.Error()})
		case callback != "":
			report(driver.AuthEvent{Phase: driver.AuthEnded, Callback: callback})
		}
	case driver.AuthCancel:
		if s != nil {
			s.Cancelled = true
		}
	}
}

// signInWithApple records an ItemSignInWithApple and answers it as AppleSignIn says, cancelled without it.
func (a *App) signInWithApple(it *driver.MountItem) {
	r := AppleSignInRequest{FullName: it.AppleSignIn.FullName, Email: it.AppleSignIn.Email, Nonce: it.AppleSignIn.Nonce}
	a.AppleSignIns = append(a.AppleSignIns, r)
	ev := driver.Event{Kind: driver.EventSignInWithApple, Count: it.Count, Text: driver.AppleSignInCancelled}
	if a.AppleSignIn != nil {
		c, err := a.AppleSignIn(r)
		var code core.AppleSignInError
		switch {
		case err == nil:
			cred := driver.AppleCredential(c)
			ev.Text, ev.Data = "", &cred
		case errors.As(err, &code):
			ev.Text = string(code)
		default:
			ev.Text = driver.AppleSignInFailed
		}
	}
	a.post(ev)
}

// webCookies answers an ItemWebCookies as a platform with a web view cookie store does: it sets cookies in WebCookies, lists those of them the filter picks and that have not expired at the clock's wall time, or clears the store.
func (a *App) webCookies(id uint32, r *driver.WebCookiesRequest) {
	switch r.Op {
	case driver.WebCookiesList:
		a.post(driver.Event{Kind: driver.EventWebCookies, Count: id, Data: driver.FilterWebCookies(a.WebCookies, r.Filter, a.Clock.WallTime())})
	case driver.WebCookiesClear:
		a.WebCookies = nil
		a.WebsiteDataClears++
		a.post(driver.Event{Kind: driver.EventWebCookies, Count: id})
	default:
		a.setWebCookies(id, r.Cookies)
	}
}

// setWebCookies answers an ItemWebCookies as a platform with a web view cookie store does, keeping the cookies in WebCookies unless SetWebCookies refuses them.
func (a *App) setWebCookies(id uint32, cookies []core.WebCookie) {
	ev := driver.Event{Kind: driver.EventWebCookies, Count: id}
	if a.SetWebCookies != nil {
		if err := a.SetWebCookies(slices.Clone(cookies)); err != nil {
			ev.Text = err.Error()
			a.post(ev)
			return
		}
	}
	for _, c := range cookies {
		// The store keeps a cookie without a path at "/", as the platforms list it.
		c.Path = c.CookiePath()
		a.WebCookies = slices.DeleteFunc(a.WebCookies, func(o core.WebCookie) bool {
			return o.Name == c.Name && strings.EqualFold(o.Domain, c.Domain) && o.CookiePath() == c.CookiePath()
		})
		a.WebCookies = append(a.WebCookies, c)
	}
	a.post(ev)
}

// permission answers an ItemPermission from Permissions, as a platform whose system keeps every permission does.
func (a *App) permission(it *driver.MountItem) {
	r := it.Permission
	p := core.Permission(r.Name)
	switch r.Command {
	case driver.PermissionCheck:
	case driver.PermissionAsk:
		if a.Permissions[p] == core.PermissionNotAsked {
			a.Permissions[p] = cmp.Or(a.PermissionAnswers[p], core.PermissionGranted)
		}
	case driver.PermissionOpenSettings:
		a.PermissionSettings = append(a.PermissionSettings, p)
		a.post(driver.Event{Kind: driver.EventPermission, Count: it.Count, Finished: true})
		return
	case driver.PermissionGuide:
		// One guide shows at a time, and none for what the app has.
		a.endGuide()
		a.guideTree = a.responseTree
		if !a.Permissions[p].Allowed() {
			a.Guide, a.guideID = p, it.Count
			return
		}
	case driver.PermissionStopGuide:
		if it.Count == a.guideID {
			a.endGuide()
		}
		return
	}
	a.post(driver.Event{Kind: driver.EventPermission, Count: it.Count, Index: int(a.Permissions[p])})
}

// endGuide ends the guide showing, answering its request with its permission's status.
func (a *App) endGuide() {
	if a.Guide == "" {
		return
	}
	a.postTo(a.guideTree, driver.Event{Kind: driver.EventPermission, Count: a.guideID, Index: int(a.Permissions[a.Guide])})
	a.Guide, a.guideID = "", 0
	a.guideTree = nil
}

// GrantPermission has the user turn p on in the system's settings, which ends a guide of p, and settles.
func (a *App) GrantPermission(p core.Permission) {
	a.t.Helper()
	a.Permissions[p] = core.PermissionGranted
	if a.Guide == p {
		a.endGuide()
	}
	a.Settle()
}

// CloseGuide has the user close the guide's panel or the system's settings, which ends the guide, and settles.
func (a *App) CloseGuide() {
	a.t.Helper()
	if a.Guide == "" {
		a.t.Fatal("nsgotest: no permission guide shows")
	}
	a.endGuide()
	a.Settle()
}

// showNotification shows n, replacing the one of its ID.
func (a *App) showNotification(n core.Notification) {
	a.Notifications = append(slices.DeleteFunc(a.Notifications, func(o core.Notification) bool { return o.ID == n.ID }), n)
}

// showDueNotifications shows the pending notifications whose time the clock's wall time reached, in the order they come due, as the system does with the app running or not, while the permission is granted; a repeated one stays pending for its next time, its At moved on a day or a week.
func (a *App) showDueNotifications() {
	now := a.Clock.WallTime()
	for {
		i := -1
		for j, n := range a.PendingNotifications {
			if !n.At.After(now) && (i < 0 || n.At.Before(a.PendingNotifications[i].At)) {
				i = j
			}
		}
		if i < 0 {
			return
		}
		n := a.PendingNotifications[i]
		if a.NotificationPermission == core.NotificationsGranted {
			a.showNotification(n)
		}
		switch n.Repeat {
		case core.NotificationDaily:
			a.PendingNotifications[i].At = n.At.AddDate(0, 0, 1)
		case core.NotificationWeekly:
			a.PendingNotifications[i].At = n.At.AddDate(0, 0, 7)
		default:
			a.PendingNotifications = slices.Delete(a.PendingNotifications, i, i+1)
		}
	}
}

// PressNotification has the user press the notification called id,
// which goes as the system takes it away, and settles.
func (a *App) PressNotification(id string) {
	a.t.Helper()
	i := slices.IndexFunc(a.Notifications, func(n core.Notification) bool { return n.ID == id })
	if i < 0 {
		a.t.Fatalf("nsgotest: no notification %q shows", id)
	}
	a.Notifications = slices.Delete(a.Notifications, i, i+1)
	a.post(driver.Event{Kind: driver.EventNotificationPress, Text: id})
	a.Settle()
}

// HearSpeech has the recognizer of the session running hear text, the
// whole transcript so far, final once it will not change it, and settles.
func (a *App) HearSpeech(text string, final bool) {
	a.t.Helper()
	if a.Speech == nil {
		a.t.Fatal("nsgotest: no speech session runs")
	}
	a.speechText = text
	if final || a.Speech.Interim {
		a.post(driver.Event{Kind: driver.EventSpeech, Count: a.speechID,
			Data: &driver.SpeechEvent{Phase: driver.SpeechResult, Text: text, Final: final}})
	}
	a.Settle()
}

// EndSpeech ends the session running with what it heard, and err, one of
// driver's Speech errors or "" for none, and settles.
func (a *App) EndSpeech(err string) {
	a.t.Helper()
	if a.Speech == nil {
		a.t.Fatal("nsgotest: no speech session runs")
	}
	a.Speech = nil
	a.post(driver.Event{Kind: driver.EventSpeech, Count: a.speechID,
		Data: &driver.SpeechEvent{Phase: driver.SpeechEnded, Text: a.speechText, Error: err}})
	a.Settle()
}

func (a *App) post(ev driver.Event) {
	a.postTo(a.responseTree, ev)
}

// postTo posts ev for tree, the surface steps act on when nil: with a desktop environment the app's own surface and each window ask the platform for their own.
func (a *App) postTo(tree *core.Tree, ev driver.Event) {
	if a.target != nil {
		// The platform answers itself.
		return
	}
	if tree == nil {
		tree = a.tree
	}
	if a.owner != nil {
		a.owner.postTo(tree, ev)
		return
	}
	a.mu.Lock()
	a.posts = append(a.posts, func() { tree.Dispatch(ev) })
	a.mu.Unlock()
}

// readClipboard answers a clipboard read from the model's clipboard.
func (a *App) readClipboard(it *driver.MountItem) {
	ev := driver.Event{Kind: driver.EventClipboard, Count: it.Count}
	switch it.Clipboard {
	case driver.ClipboardReadText:
		ev.Text = a.Clipboard
	case driver.ClipboardReadHasText:
		ev.Finished = a.Clipboard != ""
	case driver.ClipboardReadImage:
		if len(a.ClipboardImage) > 0 {
			ev.Data = driver.ClipboardPNG(a.ClipboardImage)
		}
	case driver.ClipboardReadHasImage:
		ev.Finished = len(a.ClipboardImage) > 0
	case driver.ClipboardReadFiles:
		ev.URIs = slices.Clone(a.ClipboardFiles)
	}
	a.post(ev)
}

// setFocus moves focus to or from v and, as a platform does, reports the
// change to the app.
func (a *App) setFocus(v *View, on bool) {
	if v.Focused == on {
		return
	}
	v.Focused = on
	kind := driver.EventBlur
	if on {
		kind = driver.EventFocus
	}
	a.post(driver.Event{Kind: kind, ID: v.ID})
}

func (v *View) detach() {
	if p := v.Parent; p != nil {
		for i, c := range p.Children {
			if c == v {
				p.Children = append(p.Children[:i], p.Children[i+1:]...)
				break
			}
		}
		v.Parent = nil
	}
}

// Dump returns the visible views as an indented outline, for failure
// messages and golden files.
func (a *App) Dump() string {
	var b strings.Builder
	var walk func(*View, int)
	walk = func(v *View, depth int) {
		if v.Style.Hidden {
			return
		}
		f := v.Frame
		fmt.Fprintf(&b, "%s%s %g,%g %gx%g", strings.Repeat("  ", depth), v.Kind,
			f.Origin.X, f.Origin.Y, f.Size.Width, f.Size.Height)
		if r := roleName(v.Props.Role); r != "" && r != "text" && r != "textbox" {
			fmt.Fprintf(&b, " role=%s", r)
		}
		if v.Props.TestID != "" {
			fmt.Fprintf(&b, " id=%s", v.Props.TestID)
		}
		if v.Kind == driver.KindText || v.Kind == driver.KindInput {
			fmt.Fprintf(&b, " %q", v.Text)
		}
		b.WriteByte('\n')
		for _, c := range v.Children {
			walk(c, depth+1)
		}
	}
	if r := a.views[a.rootID]; r != nil {
		walk(r, 0)
	}
	return b.String()
}

// center returns the middle of v in its own and in surface points.
func (v *View) center() (local, page driver.Point) {
	local = driver.Point{X: v.Frame.Size.Width / 2, Y: v.Frame.Size.Height / 2}
	return local, v.pagePoint(local)
}

// pointerMatrix is the transform the model draws, including animated bindings and the parent's perspective, in the view's own coordinates.
func (v *View) pointerMatrix() driver.Matrix3D {
	s := v.Animated.Apply(v.Style)
	m := s.Matrix(v.Frame.Size)
	if p := v.Parent; p != nil {
		ps := p.Animated.Apply(p.Style)
		at := driver.Point{X: v.Frame.Origin.X - p.Offset.X, Y: v.Frame.Origin.Y - p.Offset.Y}
		m = driver.ChildPerspective(ps.ChildPerspective, p.Frame.Size, at).Mul(m)
	}
	return m
}

func (v *View) pagePoint(p driver.Point) driver.Point {
	x, y := v.pageMatrix().Apply(float64(p.X), float64(p.Y))
	return driver.Point{X: driver.Points(x), Y: driver.Points(y)}
}

func (v *View) localPoint(p driver.Point) driver.Point {
	if parent := v.Parent; parent != nil {
		p = parent.localPoint(p)
		p.X += parent.Offset.X
		p.Y += parent.Offset.Y
	}
	local, ok := driver.IntoView(v.Frame, v.pointerMatrix(), p)
	if !ok {
		return driver.Point{X: driver.Points(math.Inf(1)), Y: driver.Points(math.Inf(1))}
	}
	return local
}

// Bounds is v's layout frame in surface points, as its ancestors and their scroll offsets place it, before transforms: what Ref.Bounds reports.
func (v *View) Bounds() core.Bounds { return v.surfaceFrame() }

// surfaceFrame is v's frame in surface points, as its ancestors and their
// scroll offsets place it; a modal's views are placed over the window.
func (v *View) surfaceFrame() driver.Bounds {
	b := v.Frame
	for p := v.Parent; p != nil; p = p.Parent {
		b.Origin.X += p.Frame.Origin.X - p.Offset.X
		b.Origin.Y += p.Frame.Origin.Y - p.Offset.Y
	}
	return b
}

// within reports whether v is o or inside it.
func (v *View) within(o *View) bool {
	for p := v; p != nil; p = p.Parent {
		if p == o {
			return true
		}
	}
	return false
}

func (v *View) pageMatrix() driver.Matrix3D {
	x, y := v.Frame.Origin.X, v.Frame.Origin.Y
	if parent := v.Parent; parent != nil {
		x -= parent.Offset.X
		y -= parent.Offset.Y
	}
	m := driver.Translate3D(float64(x), float64(y), 0).Mul(v.pointerMatrix())
	if parent := v.Parent; parent != nil {
		m = parent.pageMatrix().Mul(m)
	}
	return m
}

func (a *App) configurePiP(v *View) {
	v.PiPState = core.PictureInPictureState{Supported: a.pipSupported, Possible: a.pipPossible}
	a.postPiP(v)
}
func (a *App) postPiP(v *View) {
	a.post(driver.Event{Kind: driver.EventNative, ID: v.ID, Text: driver.PictureInPictureEventName, Data: &driver.PictureInPictureEvent{State: v.PiPState}})
}
func (a *App) commandPiP(v *View, command uint32) {
	switch command {
	case driver.PictureInPictureStart:
		if v.PiPState.Active {
			return
		}
		if !v.PiPState.Supported {
			a.postPiP(v)
			return
		}
		if !v.PiPState.Possible {
			v.PiPState.Error = "Picture in Picture is not possible yet"
			a.postPiP(v)
			return
		}
		v.PiPStarts++
		v.PiPState.Active = true
		v.PiPState.Error = ""
		a.postPiP(v)
	case driver.PictureInPictureStop:
		if !v.PiPState.Active {
			return
		}
		v.PiPStops++
		v.PiPState.Active = false
		a.postPiP(v)
	case driver.PictureInPictureRestored:
		v.PiPRestores++
	}
}

const maxCanvasImages = 64

// KeptImage reports whether src is currently pinned via core.Scope.KeepImage.
func (a *App) KeptImage(src core.ImageSource) bool {
	return a.KeptImages[src.Key()] > 0
}

// CanvasImageCached reports whether src is currently held in the model's
// canvas image cache (kept or within the 64-source unkept LRU bound).
func (a *App) CanvasImageCached(src core.ImageSource) bool {
	_, ok := a.canvasImages[src.Key()]
	return ok
}

func (a *App) keepCanvasImage(key string) {
	if key == "" {
		return
	}
	if a.KeptImages == nil {
		a.KeptImages = map[string]int{}
	}
	a.KeptImages[key]++
}

func (a *App) releaseCanvasImage(key string) {
	if key == "" {
		return
	}
	n := a.KeptImages[key]
	if n <= 0 {
		return
	}
	if n == 1 {
		delete(a.KeptImages, key)
	} else {
		a.KeptImages[key] = n - 1
	}
	a.evictCanvasImages(0, a.canvasImageUse)
}

func (a *App) touchCanvasImage(src driver.ImageSource, pass uint64) {
	if src.Pixels != nil || src.Empty() {
		return
	}
	key := src.Key()
	if key == "" {
		return
	}
	a.canvasImageUse++
	if a.canvasImages == nil {
		a.canvasImages = map[string]uint64{}
	}
	if _, ok := a.canvasImages[key]; ok {
		a.canvasImages[key] = a.canvasImageUse
		return
	}
	reserve := 0
	if a.KeptImages[key] == 0 {
		reserve = 1
	}
	a.evictCanvasImages(reserve, pass)
	a.canvasImages[key] = a.canvasImageUse
}

func (a *App) evictCanvasImages(reserve int, pass uint64) {
	unkept := 0
	for k := range a.canvasImages {
		if a.KeptImages[k] == 0 {
			unkept++
		}
	}
	for unkept+reserve > maxCanvasImages {
		var oldest string
		var at uint64 = math.MaxUint64
		for k, used := range a.canvasImages {
			if a.KeptImages[k] == 0 && used < at {
				oldest, at = k, used
			}
		}
		if at > pass {
			break
		}
		delete(a.canvasImages, oldest)
		unkept--
	}
}
