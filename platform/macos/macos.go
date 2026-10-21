//go:build darwin && !ios

// Package macos is NSGo's AppKit backend: one flipped, layer-backed NSView
// per node, styled through its CALayer, with text the engine laid out
// with CoreText drawn on the main thread. It calls AppKit through purego,
// so it builds without cgo.
package macos

import (
	"runtime"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/engine"
	"github.com/oneo-me/nsgo/platform/internal/apple"
	"github.com/oneo-me/nsgo/platform/internal/queue"
)

func init() {
	// AppKit runs on the process's main thread, which main starts on.
	runtime.LockOSThread()
}

// Options configure the window.
type Options struct {
	Title         string
	Width, Height core.Points
}

// longPressDelay is how long a press is held before it is a long press.
const longPressDelay = 500 * time.Millisecond

// view is the Go side of one native view.
type view struct {
	surface *surface
	id      core.NodeID
	kind    core.ViewKind
	native  objc.ID
	content objc.ID // where children go: native, or a scroll view's document
	events  core.EventMask

	pressing  atomic.Bool
	longFired atomic.Bool
	hover     objc.ID // the tracking area of a view reporting hovers

	layout *apple.Layout // a text view's
	box    core.Bounds

	field  objc.ID // an input's NSTextField
	clip   objc.ID // a scroll view's NSClipView
	masked bool
	inset  core.Bounds
	edits  uint32
}

type surface struct {
	native  objc.ID
	engine  *engine.Engine
	batches queue.Batches
	views   []*view // index is NodeID
}

// views maps native objects (views, fields, clip views) to their Go side.
// Only the main thread uses it.
var views = map[objc.ID]*view{}

var (
	sel = objc.RegisterName

	selAlloc, selInit, selNew       = sel("alloc"), sel("init"), sel("new")
	selRelease                      = sel("release")
	selSetWantsLayer, selLayer      = sel("setWantsLayer:"), sel("layer")
	selSetFrame, selBounds          = sel("setFrame:"), sel("bounds")
	selSetFrameSize                 = sel("setFrameSize:")
	selAddSubview, selSuperview     = sel("addSubview:"), sel("superview")
	selAddSubviewPositioned         = sel("addSubview:positioned:relativeTo:")
	selRemoveFromSuperview          = sel("removeFromSuperview")
	selSetHidden                    = sel("setHidden:")
	selSetNeedsDisplay              = sel("setNeedsDisplay:")
	selSetNeedsLayout               = sel("setNeedsLayout:")
	selConvertPoint                 = sel("convertPoint:fromView:")
	selLocationInWindow             = sel("locationInWindow")
	selStringValue, selSetString    = sel("stringValue"), sel("setStringValue:")
	selSetFont, selSetTextColor     = sel("setFont:"), sel("setTextColor:")
	selSetAlignment                 = sel("setAlignment:")
	selSetPlaceholder               = sel("setPlaceholderString:")
	selSetEditable, selSetEnabled   = sel("setEditable:"), sel("setEnabled:")
	selSetDelegate                  = sel("setDelegate:")
	selSetTarget, selSetAction      = sel("setTarget:"), sel("setAction:")
	selApply, selSubmit             = sel("nsgoApply:"), sel("nsgoSubmit:")
	selPerformOnMain                = sel("performSelectorOnMainThread:withObject:waitUntilDone:")
	selWindow, selMakeFirstResponer = sel("window"), sel("makeFirstResponder:")
	selDocumentView                 = sel("documentView")
	selSetAccessibilityElement      = sel("setAccessibilityElement:")
	selSetAccessibilityRole         = sel("setAccessibilityRole:")
	selSetAccessibilitySubrole      = sel("setAccessibilitySubrole:")
	selSetAccessibilityLabel        = sel("setAccessibilityLabel:")
	selSetAccessibilityEnabled      = sel("setAccessibilityEnabled:")
	selSetAccessibilitySelected     = sel("setAccessibilitySelected:")
	selSetAccessibilityValue        = sel("setAccessibilityValue:")
	selSetAccessibilityIdentifier   = sel("setAccessibilityIdentifier:")
	selIntrinsicContentSize         = sel("intrinsicContentSize")
	selCurrentContext, selCGContext = sel("currentContext"), sel("CGContext")
	selIsEqualToString              = sel("isEqualToString:")

	classView, classSurface, classInput, classField, classSecureField, classClip objc.Class
)

func loadAppKit() {
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		panic(err)
	}
	nsView := objc.GetClass("NSView")
	common := []objc.MethodDef{
		{Cmd: sel("isFlipped"), Fn: func(self objc.ID, _ objc.SEL) bool { return true }},
		{Cmd: sel("drawRect:"), Fn: drawRect},
		{Cmd: sel("hitTest:"), Fn: hitTest},
		{Cmd: sel("mouseDown:"), Fn: mouseDown},
		{Cmd: sel("mouseUp:"), Fn: mouseUp},
		{Cmd: sel("mouseEntered:"), Fn: mouseEntered},
		{Cmd: sel("mouseExited:"), Fn: mouseExited},
		{Cmd: sel("accessibilityPerformPress"), Fn: accessibilityPress},
	}
	classView = register("NsgoView", nsView, nil, common)
	classSurface = register("NsgoSurface", nsView, nil, append(common,
		objc.MethodDef{Cmd: sel("setFrameSize:"), Fn: surfaceSetFrameSize},
		objc.MethodDef{Cmd: selApply, Fn: surfaceApply},
	))
	classInput = register("NsgoInput", nsView, nil, append(common,
		objc.MethodDef{Cmd: sel("layout"), Fn: inputLayout},
	))
	fieldMethods := []objc.MethodDef{
		{Cmd: sel("controlTextDidChange:"), Fn: fieldChanged},
		{Cmd: sel("controlTextDidEndEditing:"), Fn: fieldEnded},
		{Cmd: sel("becomeFirstResponder"), Fn: fieldFocused},
		{Cmd: selSubmit, Fn: fieldSubmit},
	}
	delegate := []*objc.Protocol{objc.GetProtocol("NSTextFieldDelegate")}
	classField = register("NsgoField", objc.GetClass("NSTextField"), delegate, fieldMethods)
	classSecureField = register("NsgoSecureField", objc.GetClass("NSSecureTextField"), delegate, fieldMethods)
	classClip = register("NsgoClipView", objc.GetClass("NSClipView"), nil, []objc.MethodDef{
		{Cmd: sel("setBoundsOrigin:"), Fn: clipScrolled},
	})
}

func register(name string, super objc.Class, protocols []*objc.Protocol, methods []objc.MethodDef) objc.Class {
	if protocols == nil {
		protocols = []*objc.Protocol{}
	}
	c, err := objc.RegisterClass(name, super, protocols, []objc.FieldDef{}, methods)
	if err != nil {
		panic(err)
	}
	return c
}

// Run opens a window showing app and runs the event loop. Call it from
// main; it does not return.
func Run(app engine.App, opts Options) {
	if opts.Width == 0 {
		opts.Width, opts.Height = 480, 640
	}
	loadAppKit()
	nsApp := objc.ID(objc.GetClass("NSApplication")).Send(sel("sharedApplication"))
	nsApp.Send(sel("setActivationPolicy:"), 0)
	nsApp.Send(sel("setMainMenu:"), mainMenu())
	delegate := objc.ID(register("NsgoAppDelegate", objc.GetClass("NSObject"), nil, []objc.MethodDef{{
		Cmd: sel("applicationShouldTerminateAfterLastWindowClosed:"),
		Fn:  func(self objc.ID, _ objc.SEL, app objc.ID) bool { return true },
	}})).Send(selNew)
	nsApp.Send(sel("setDelegate:"), delegate)

	const titled, closable, miniaturizable, resizable = 1, 2, 4, 8
	window := objc.ID(objc.GetClass("NSWindow")).Send(selAlloc).Send(
		sel("initWithContentRect:styleMask:backing:defer:"),
		apple.Rect(0, 0, float64(opts.Width), float64(opts.Height)),
		uint(titled|closable|miniaturizable|resizable), uint(2), false)
	window.Send(sel("setTitle:"), apple.NSString(opts.Title))
	window.Send(sel("setReleasedWhenClosed:"), false)

	s := &surface{}
	s.native = objc.ID(classSurface).Send(selAlloc).Send(sel("initWithFrame:"),
		apple.Rect(0, 0, float64(opts.Width), float64(opts.Height)))
	s.native.Send(selSetWantsLayer, true)
	views[s.native] = &view{surface: s}
	s.engine = engine.Start(s, app, engine.Options{Size: core.Size{Width: opts.Width, Height: opts.Height}})
	window.Send(sel("setContentView:"), s.native)
	window.Send(sel("center"))
	window.Send(sel("makeKeyAndOrderFront:"), objc.ID(0))
	nsApp.Send(sel("activateIgnoringOtherApps:"), true)
	nsApp.Send(sel("run"))
}

func mainMenu() objc.ID {
	menu := func(title string) objc.ID {
		return objc.ID(objc.GetClass("NSMenu")).Send(selAlloc).Send(sel("initWithTitle:"), apple.NSString(title))
	}
	item := func(m objc.ID, title, action, key string) {
		m.Send(sel("addItemWithTitle:action:keyEquivalent:"), apple.NSString(title), sel(action), apple.NSString(key))
	}
	submenu := func(bar, m objc.ID) {
		it := objc.ID(objc.GetClass("NSMenuItem")).Send(selNew)
		it.Send(sel("setSubmenu:"), m)
		bar.Send(sel("addItem:"), it)
	}
	bar := menu("")
	app := menu("")
	item(app, "Quit", "terminate:", "q")
	submenu(bar, app)
	edit := menu("Edit")
	item(edit, "Undo", "undo:", "z")
	item(edit, "Redo", "redo:", "Z")
	item(edit, "Cut", "cut:", "x")
	item(edit, "Copy", "copy:", "c")
	item(edit, "Paste", "paste:", "v")
	item(edit, "Select All", "selectAll:", "a")
	submenu(bar, edit)
	return bar
}

// Prepare implements core.TextEngine with CoreText.
func (s *surface) Prepare(t *core.AttributedText) core.PreparedText {
	return apple.TextEngine{}.Prepare(t)
}

// Mount implements engine.Platform.
func (s *surface) Mount(b *core.MountBatch) {
	if s.batches.Push(b) {
		s.native.Send(selPerformOnMain, selApply, objc.ID(0), false)
	}
}

func (s *surface) apply(b *core.MountBatch) {
	for i := range b.Items {
		it := &b.Items[i]
		if it.Kind == core.ItemCreate {
			for len(s.views) <= int(it.ID) {
				s.views = append(s.views, nil)
			}
			s.views[it.ID] = s.create(it.View, it.ID)
			continue
		}
		v := s.views[it.ID]
		switch it.Kind {
		case core.ItemDelete:
			s.delete(v)
		case core.ItemRemove:
			v.native.Send(selRemoveFromSuperview)
		case core.ItemInsert:
			parent := s.views[it.Parent].content
			if it.Before == 0 {
				parent.Send(selAddSubview, v.native)
			} else {
				const below = -1
				parent.Send(selAddSubviewPositioned, v.native, below, s.views[it.Before].native)
			}
		case core.ItemFrame:
			f := it.Frame
			v.native.Send(selSetFrame, apple.Rect(float64(f.Origin.X), float64(f.Origin.Y),
				float64(f.Size.Width), float64(f.Size.Height)))
		case core.ItemViewStyle:
			apple.StyleLayer(v.native.Send(selLayer), it.ViewStyle, true)
			v.native.Send(selSetHidden, it.ViewStyle.Hidden)
		case core.ItemViewProps:
			setAccessibility(v.native, it.ViewProps)
		case core.ItemEvents:
			v.setEvents(it.Events)
		case core.ItemText:
			v.layout.Release()
			v.layout, _ = it.Text.Layout.(*apple.Layout)
			v.box = it.Text.Content
			v.native.Send(selSetAccessibilityLabel, apple.NSString(it.Text.Text))
			v.native.Send(selSetNeedsDisplay, true)
		case core.ItemInput:
			v.configure(it.Input)
		case core.ItemInputValue:
			if v.edits <= it.Count {
				value := apple.NSString(it.Value)
				// Setting the text the field shows would move the caret.
				if !objc.Send[bool](v.field.Send(selStringValue), selIsEqualToString, value) {
					v.field.Send(selSetString, value)
				}
			}
		case core.ItemScrollConfig:
			v.native.Send(sel("setHasVerticalScroller:"), !it.Scroll.Horizontal && !it.Scroll.HideIndicator)
			v.native.Send(sel("setHasHorizontalScroller:"), it.Scroll.Horizontal && !it.Scroll.HideIndicator)
		case core.ItemScrollContent:
			v.content.Send(selSetFrameSize, apple.CGSize{Width: float64(it.Size.Width), Height: float64(it.Size.Height)})
		case core.ItemFocus:
			if w := v.native.Send(selWindow); w != 0 && v.field != 0 {
				w.Send(selMakeFirstResponer, v.field)
			}
		case core.ItemBlur:
			if w := v.native.Send(selWindow); w != 0 {
				w.Send(selMakeFirstResponer, objc.ID(0))
			}
		}
	}
	if root := s.views[b.Root]; root.native.Send(selSuperview) != s.native {
		s.native.Send(selAddSubview, root.native)
	}
}

func (s *surface) create(kind core.ViewKind, id core.NodeID) *view {
	v := &view{surface: s, id: id, kind: kind}
	switch kind {
	case core.KindScroll:
		v.native = objc.ID(objc.GetClass("NSScrollView")).Send(selNew)
		v.native.Send(sel("setDrawsBackground:"), false)
		v.native.Send(sel("setAutohidesScrollers:"), true)
		clip := objc.ID(classClip).Send(selNew)
		clip.Send(sel("setDrawsBackground:"), false)
		v.native.Send(sel("setContentView:"), clip)
		clip.Send(selRelease)
		v.content = objc.ID(classView).Send(selNew)
		v.content.Send(selSetWantsLayer, true)
		v.native.Send(sel("setDocumentView:"), v.content)
		v.content.Send(selRelease)
		v.clip = clip
		views[clip] = v
	case core.KindInput:
		v.native = objc.ID(classInput).Send(selNew)
		v.setField(false)
	default:
		v.native = objc.ID(classView).Send(selNew)
	}
	v.native.Send(selSetWantsLayer, true)
	if v.content == 0 {
		v.content = v.native
	}
	views[v.native] = v
	return v
}

func (s *surface) delete(v *view) {
	v.native.Send(selRemoveFromSuperview)
	v.layout.Release()
	if v.hover != 0 {
		v.hover.Send(selRelease)
	}
	delete(views, v.native)
	delete(views, v.field)
	delete(views, v.clip)
	v.native.Send(selRelease)
	s.views[v.id] = nil
}

// setField replaces an input's text field, keeping its text.
func (v *view) setField(masked bool) {
	text := apple.NSString("")
	if v.field != 0 {
		text = v.field.Send(selStringValue)
		v.field.Send(selRemoveFromSuperview)
		delete(views, v.field)
	}
	class := classField
	if masked {
		class = classSecureField
	}
	field := objc.ID(class).Send(selNew)
	field.Send(sel("setBezeled:"), false)
	field.Send(sel("setBordered:"), false)
	field.Send(sel("setDrawsBackground:"), false)
	field.Send(sel("setFocusRingType:"), 1)
	field.Send(selSetDelegate, field)
	field.Send(selSetTarget, field)
	field.Send(selSetAction, selSubmit)
	field.Send(selSetString, text)
	cell := field.Send(sel("cell"))
	cell.Send(sel("setScrollable:"), true)
	cell.Send(sel("setUsesSingleLineMode:"), true)
	v.native.Send(selAddSubview, field)
	field.Send(selRelease)
	v.field, v.masked = field, masked
	views[field] = v
}

// AppKit's text alignments.
const nsAlignLeft, nsAlignRight, nsAlignCenter = 0, 1, 2

func (v *view) configure(c *core.InputConfig) {
	if c.Masked != v.masked {
		v.setField(c.Masked)
	}
	font := apple.Font(c.Style.Font)
	v.field.Send(selSetFont, objc.ID(font))
	apple.Release(font)
	v.field.Send(selSetTextColor, nsColor(c.Style.Color))
	align := nsAlignLeft
	switch c.Style.Align {
	case core.TextAlignCenter:
		align = nsAlignCenter
	case core.TextAlignEnd:
		align = nsAlignRight
	}
	v.field.Send(selSetAlignment, align)
	v.field.Send(selSetPlaceholder, apple.NSString(c.Placeholder))
	v.field.Send(selSetEditable, !c.Readonly)
	v.field.Send(selSetEnabled, !c.Disabled)
	v.inset = c.Content
	v.native.Send(selSetNeedsLayout, true)
}

func (v *view) setEvents(m core.EventMask) {
	v.events = m
	wantHover := m.Has(core.EventHoverIn) || m.Has(core.EventHoverOut)
	switch {
	case wantHover && v.hover == 0:
		const enteredAndExited, activeInActiveApp, inVisibleRect = 0x01, 0x40, 0x200
		v.hover = objc.ID(objc.GetClass("NSTrackingArea")).Send(selAlloc).Send(
			sel("initWithRect:options:owner:userInfo:"), apple.Rect(0, 0, 0, 0),
			uint(enteredAndExited|activeInActiveApp|inVisibleRect), v.native, objc.ID(0))
		v.native.Send(sel("addTrackingArea:"), v.hover)
	case !wantHover && v.hover != 0:
		v.native.Send(sel("removeTrackingArea:"), v.hover)
		v.hover.Send(selRelease)
		v.hover = 0
	}
}

func (v *view) dispatch(kind core.EventKind) {
	if v.events.Has(kind) {
		v.surface.engine.Dispatch(core.Event{Kind: kind, ID: v.id})
	}
}

// AppKit accessibility roles of core roles: role and subrole.
var roles = map[core.Role][2]string{
	core.RoleButton:   {"AXButton", ""},
	core.RoleLink:     {"AXLink", ""},
	core.RoleHeader:   {"AXStaticText", ""},
	core.RoleImage:    {"AXImage", ""},
	core.RoleCheckbox: {"AXCheckBox", ""},
	core.RoleSwitch:   {"AXCheckBox", "AXSwitch"},
	core.RoleRadio:    {"AXRadioButton", ""},
	core.RoleTab:      {"AXRadioButton", "AXTabButton"},
	core.RoleText:     {"AXStaticText", ""},
}

func setAccessibility(native objc.ID, p *core.ViewProps) {
	if p.TestID != "" {
		native.Send(selSetAccessibilityIdentifier, apple.NSString(p.TestID))
	}
	r, ok := roles[p.Role]
	native.Send(selSetAccessibilityElement, ok)
	if !ok {
		return
	}
	native.Send(selSetAccessibilityRole, apple.NSString(r[0]))
	if r[1] != "" {
		native.Send(selSetAccessibilitySubrole, apple.NSString(r[1]))
	}
	if p.Label != "" {
		native.Send(selSetAccessibilityLabel, apple.NSString(p.Label))
	}
	native.Send(selSetAccessibilityEnabled, !p.Disabled)
	native.Send(selSetAccessibilitySelected, p.Selected)
	if p.Role == core.RoleCheckbox || p.Role == core.RoleSwitch || p.Role == core.RoleRadio {
		n := 0
		if p.Checked || p.Selected {
			n = 1
		}
		native.Send(selSetAccessibilityValue, objc.ID(objc.GetClass("NSNumber")).Send(sel("numberWithInt:"), n))
	}
}

func nsColor(c core.Color) objc.ID {
	r, g, b, a := c.Components()
	return objc.ID(objc.GetClass("NSColor")).Send(sel("colorWithSRGBRed:green:blue:alpha:"), r, g, b, a)
}

func drawRect(self objc.ID, _ objc.SEL, dirty apple.CGRect) {
	v := views[self]
	if v == nil || v.layout == nil {
		return
	}
	b := objc.Send[apple.CGRect](self, selBounds)
	ctx := objc.Send[uintptr](objc.ID(objc.GetClass("NSGraphicsContext")).Send(selCurrentContext), selCGContext)
	box := v.box
	v.layout.Draw(ctx, b.Size.Height, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y),
		float64(box.Size.Width), float64(box.Size.Height)))
}

// hitTest lets presses on text and plain containers through to the
// nearest pressable ancestor; text fields keep theirs.
func hitTest(self objc.ID, cmd objc.SEL, p apple.CGPoint) objc.ID {
	hit := objc.SendSuper[objc.ID](self, cmd, p)
	stop := self.Send(selSuperview)
	for n := hit; n != 0 && n != stop; n = n.Send(selSuperview) {
		if v := views[n]; v != nil {
			if v.field == n || v.kind == core.KindScroll {
				return hit
			}
			if v.pressable() && v.native == n {
				return n
			}
		}
	}
	return hit
}

func (v *view) pressable() bool {
	return v.events.Has(core.EventPress) || v.events.Has(core.EventLongPress) ||
		v.events.Has(core.EventPressDown) || v.events.Has(core.EventPressUp)
}

func mouseDown(self objc.ID, cmd objc.SEL, event objc.ID) {
	v := views[self]
	if v == nil || !v.pressable() {
		self.SendSuper(cmd, event)
		return
	}
	v.pressing.Store(true)
	v.longFired.Store(false)
	v.dispatch(core.EventPressDown)
	if v.events.Has(core.EventLongPress) {
		e := v.surface.engine
		time.AfterFunc(longPressDelay, func() {
			if v.pressing.Load() && v.longFired.CompareAndSwap(false, true) {
				e.Dispatch(core.Event{Kind: core.EventLongPress, ID: v.id})
			}
		})
	}
}

func mouseUp(self objc.ID, cmd objc.SEL, event objc.ID) {
	v := views[self]
	if v == nil || !v.pressable() {
		self.SendSuper(cmd, event)
		return
	}
	p := objc.Send[apple.CGPoint](self, selConvertPoint, objc.Send[apple.CGPoint](event, selLocationInWindow), objc.ID(0))
	b := objc.Send[apple.CGRect](self, selBounds)
	pressed := v.pressing.Swap(false)
	v.dispatch(core.EventPressUp)
	if pressed && !v.longFired.Load() && p.X >= 0 && p.Y >= 0 && p.X < b.Size.Width && p.Y < b.Size.Height {
		v.dispatch(core.EventPress)
	}
}

func mouseEntered(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.dispatch(core.EventHoverIn)
	}
}

func mouseExited(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.dispatch(core.EventHoverOut)
	}
}

func accessibilityPress(self objc.ID, _ objc.SEL) bool {
	v := views[self]
	if v == nil || !v.events.Has(core.EventPress) {
		return false
	}
	v.dispatch(core.EventPress)
	return true
}

func surfaceSetFrameSize(self objc.ID, cmd objc.SEL, size apple.CGSize) {
	self.SendSuper(cmd, size)
	if v := views[self]; v != nil && v.surface.engine != nil {
		v.surface.engine.Resize(core.Size{Width: core.Points(size.Width), Height: core.Points(size.Height)})
	}
}

func surfaceApply(self objc.ID, _ objc.SEL, _ objc.ID) {
	s := views[self].surface
	for _, b := range s.batches.Drain() {
		s.apply(b)
	}
}

func inputLayout(self objc.ID, cmd objc.SEL) {
	self.SendSuper(cmd)
	v := views[self]
	h := objc.Send[apple.CGSize](v.field, selIntrinsicContentSize).Height
	in := v.inset
	y := float64(in.Origin.Y) + (float64(in.Size.Height)-h)/2
	v.field.Send(selSetFrame, apple.Rect(float64(in.Origin.X), y, float64(in.Size.Width), h))
}

func fieldChanged(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.edits++
		v.surface.engine.Dispatch(core.Event{Kind: core.EventChangeText, ID: v.id,
			Text: apple.GoString(self.Send(selStringValue)), Count: v.edits})
	}
}

func fieldSubmit(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.surface.engine.Dispatch(core.Event{Kind: core.EventSubmitEditing, ID: v.id,
			Text: apple.GoString(self.Send(selStringValue))})
	}
}

func fieldFocused(self objc.ID, cmd objc.SEL) bool {
	ok := objc.SendSuper[bool](self, cmd)
	if v := views[self]; v != nil && ok {
		v.dispatch(core.EventFocus)
	}
	return ok
}

func fieldEnded(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.dispatch(core.EventBlur)
	}
}

func clipScrolled(self objc.ID, cmd objc.SEL, origin apple.CGPoint) {
	self.SendSuper(cmd, origin)
	if v := views[self]; v != nil && v.events.Has(core.EventScroll) {
		v.surface.engine.Dispatch(core.Event{Kind: core.EventScroll, ID: v.id,
			Point: core.Point{X: core.Points(origin.X), Y: core.Points(origin.Y)}})
	}
}
