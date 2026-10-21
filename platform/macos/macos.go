//go:build darwin && !ios

// Package macos is NSGo's AppKit backend: one flipped, layer-backed NSView
// per node, CoreText text measured on the engine goroutine and drawn on
// the main thread. It calls AppKit through purego, so it builds without
// cgo.
package macos

import (
	"runtime"

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

// view is the Go side of one native view.
type view struct {
	surface   *surface
	id        core.NodeID
	pressable bool
	pressing  bool
	padding   core.Edges
	text      *apple.Text
	field     objc.ID // an input's NSTextField
}

type surface struct {
	native  objc.ID
	engine  *engine.Engine
	batches queue.Batches
	views   []objc.ID // index is NodeID
}

// views maps native objects to their Go side. Only the main thread uses it.
var views = map[objc.ID]*view{}

var (
	sel = objc.RegisterName

	selAlloc, selInit, selNew       = sel("alloc"), sel("init"), sel("new")
	selRelease                      = sel("release")
	selSetWantsLayer, selLayer      = sel("setWantsLayer:"), sel("layer")
	selSetBackgroundColor           = sel("setBackgroundColor:")
	selSetCornerRadius              = sel("setCornerRadius:")
	selSetFrame, selBounds          = sel("setFrame:"), sel("bounds")
	selAddSubview, selSuperview     = sel("addSubview:"), sel("superview")
	selSubviews, selCount           = sel("subviews"), sel("count")
	selObjectAtIndex                = sel("objectAtIndex:")
	selAddSubviewPositioned         = sel("addSubview:positioned:relativeTo:")
	selSetNeedsDisplay              = sel("setNeedsDisplay:")
	selSetNeedsLayout               = sel("setNeedsLayout:")
	selConvertPoint                 = sel("convertPoint:fromView:")
	selLocationInWindow             = sel("locationInWindow")
	selStringValue, selSetString    = sel("stringValue"), sel("setStringValue:")
	selSetFont, selSetTextColor     = sel("setFont:"), sel("setTextColor:")
	selSetPlaceholder               = sel("setPlaceholderString:")
	selSetDelegate                  = sel("setDelegate:")
	selSetTarget, selSetAction      = sel("setTarget:"), sel("setAction:")
	selApply, selSubmit             = sel("nsgoApply:"), sel("nsgoSubmit:")
	selPerformOnMain                = sel("performSelectorOnMainThread:withObject:waitUntilDone:")
	selSetAccessibilityElement      = sel("setAccessibilityElement:")
	selSetAccessibilityRole         = sel("setAccessibilityRole:")
	selSetAccessibilityLabel        = sel("setAccessibilityLabel:")
	selIntrinsicContentSize         = sel("intrinsicContentSize")
	selCurrentContext, selCGContext = sel("currentContext"), sel("CGContext")
	selIsEqualToString              = sel("isEqualToString:")

	classView, classSurface, classInput, classField objc.Class
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
	classField = register("NsgoField", objc.GetClass("NSTextField"),
		[]*objc.Protocol{objc.GetProtocol("NSTextFieldDelegate")},
		[]objc.MethodDef{
			{Cmd: sel("controlTextDidChange:"), Fn: fieldChanged},
			{Cmd: selSubmit, Fn: fieldSubmit},
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
	s.engine = engine.Start(s, app, core.Size{Width: opts.Width, Height: opts.Height})
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
	item(edit, "Cut", "cut:", "x")
	item(edit, "Copy", "copy:", "c")
	item(edit, "Paste", "paste:", "v")
	item(edit, "Select All", "selectAll:", "a")
	submenu(bar, edit)
	return bar
}

// Measure implements core.TextEngine with CoreText.
func (s *surface) Measure(text string, font core.Font, maxWidth core.Points) core.Size {
	return apple.Measure(text, font, maxWidth)
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
		switch it.Kind {
		case core.ItemCreate:
			for len(s.views) <= int(it.ID) {
				s.views = append(s.views, 0)
			}
			s.views[it.ID] = s.create(it.Tag, it.ID)
		case core.ItemProps:
			s.props(s.views[it.ID], &it.Props)
		case core.ItemInsert:
			parent, child := s.views[it.Parent], s.views[it.ID]
			subviews := parent.Send(selSubviews)
			if it.Index >= int(subviews.Send(selCount)) {
				parent.Send(selAddSubview, child)
			} else {
				const below = -1
				parent.Send(selAddSubviewPositioned, child, below, subviews.Send(selObjectAtIndex, it.Index))
			}
		case core.ItemFrame:
			f := it.Frame
			s.views[it.ID].Send(selSetFrame, apple.Rect(float64(f.Origin.X), float64(f.Origin.Y),
				float64(f.Size.Width), float64(f.Size.Height)))
		}
	}
	if root := s.views[b.Root]; root.Send(selSuperview) != s.native {
		s.native.Send(selAddSubview, root)
	}
}

func (s *surface) create(tag core.Tag, id core.NodeID) objc.ID {
	v := &view{surface: s, id: id}
	class := classView
	if tag == core.TagInput {
		class = classInput
	}
	native := objc.ID(class).Send(selNew)
	native.Send(selSetWantsLayer, true)
	if tag == core.TagInput {
		field := objc.ID(classField).Send(selNew)
		field.Send(sel("setBezeled:"), false)
		field.Send(sel("setBordered:"), false)
		field.Send(sel("setDrawsBackground:"), false)
		field.Send(sel("setFocusRingType:"), 1)
		field.Send(selSetDelegate, field)
		field.Send(selSetTarget, field)
		field.Send(selSetAction, selSubmit)
		cell := field.Send(sel("cell"))
		cell.Send(sel("setScrollable:"), true)
		cell.Send(sel("setUsesSingleLineMode:"), true)
		native.Send(selAddSubview, field)
		v.field = field
		views[field] = v
	}
	views[native] = v
	return native
}

func (s *surface) props(native objc.ID, p *core.Props) {
	v := views[native]
	color := apple.CGColor(p.Background)
	layer := native.Send(selLayer)
	layer.Send(selSetBackgroundColor, color)
	apple.Release(color)
	layer.Send(selSetCornerRadius, float64(p.CornerRadius))
	v.pressable = p.Pressable
	v.padding = p.Padding
	if p.Role == core.RoleButton {
		native.Send(selSetAccessibilityElement, true)
		native.Send(selSetAccessibilityRole, apple.NSString("AXButton"))
	}
	switch {
	case v.field != 0:
		value := apple.NSString(p.Text)
		// Setting the text the user just typed would move the caret.
		if !objc.Send[bool](v.field.Send(selStringValue), selIsEqualToString, value) {
			v.field.Send(selSetString, value)
		}
		font := apple.Font(p.Font)
		v.field.Send(selSetFont, objc.ID(font))
		apple.Release(font)
		v.field.Send(selSetTextColor, nsColor(p.Color))
		v.field.Send(selSetPlaceholder, apple.NSString(p.Placeholder))
		native.Send(selSetNeedsLayout, true)
	case p.Font.Size > 0:
		v.text.Free()
		v.text = apple.NewText(p.Text, p.Font, p.Color)
		native.Send(selSetAccessibilityElement, true)
		native.Send(selSetAccessibilityRole, apple.NSString("AXStaticText"))
		native.Send(selSetAccessibilityLabel, apple.NSString(p.Text))
		native.Send(selSetNeedsDisplay, true)
	}
}

func nsColor(c core.Color) objc.ID {
	r, g, b, a := c.Components()
	return objc.ID(objc.GetClass("NSColor")).Send(sel("colorWithSRGBRed:green:blue:alpha:"), r, g, b, a)
}

func drawRect(self objc.ID, _ objc.SEL, dirty apple.CGRect) {
	v := views[self]
	if v == nil || v.text == nil {
		return
	}
	b := objc.Send[apple.CGRect](self, selBounds)
	ctx := objc.Send[uintptr](objc.ID(objc.GetClass("NSGraphicsContext")).Send(selCurrentContext), selCGContext)
	p := v.padding
	v.text.Draw(ctx, b.Size.Height, apple.Rect(float64(p.Left), float64(p.Top),
		b.Size.Width-float64(p.Left+p.Right), b.Size.Height-float64(p.Top+p.Bottom)))
}

// hitTest lets presses on text and plain containers through to the
// nearest pressable ancestor; text fields keep theirs.
func hitTest(self objc.ID, cmd objc.SEL, p apple.CGPoint) objc.ID {
	hit := objc.SendSuper[objc.ID](self, cmd, p)
	stop := self.Send(selSuperview)
	for n := hit; n != 0 && n != stop; n = n.Send(selSuperview) {
		if v := views[n]; v != nil {
			if v.field == n {
				return hit
			}
			if v.pressable {
				return n
			}
		}
	}
	return hit
}

func mouseDown(self objc.ID, cmd objc.SEL, event objc.ID) {
	if v := views[self]; v != nil && v.pressable {
		v.pressing = true
		return
	}
	self.SendSuper(cmd, event)
}

func mouseUp(self objc.ID, cmd objc.SEL, event objc.ID) {
	v := views[self]
	if v == nil || !v.pressable {
		self.SendSuper(cmd, event)
		return
	}
	p := objc.Send[apple.CGPoint](self, selConvertPoint, objc.Send[apple.CGPoint](event, selLocationInWindow), objc.ID(0))
	b := objc.Send[apple.CGRect](self, selBounds)
	if v.pressing && p.X >= 0 && p.Y >= 0 && p.X < b.Size.Width && p.Y < b.Size.Height {
		v.surface.engine.Press(v.id)
	}
	v.pressing = false
}

func accessibilityPress(self objc.ID, _ objc.SEL) bool {
	v := views[self]
	if v == nil || !v.pressable {
		return false
	}
	v.surface.engine.Press(v.id)
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
	b := objc.Send[apple.CGRect](self, selBounds)
	h := objc.Send[apple.CGSize](v.field, selIntrinsicContentSize).Height
	w := b.Size.Width - float64(v.padding.Left+v.padding.Right)
	v.field.Send(selSetFrame, apple.Rect(float64(v.padding.Left), (b.Size.Height-h)/2, max(w, 0), h))
}

func fieldChanged(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.surface.engine.Change(v.id, apple.GoString(self.Send(selStringValue)))
	}
}

func fieldSubmit(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.surface.engine.Submit(v.id, apple.GoString(self.Send(selStringValue)))
	}
}
