//go:build ios

// Package ios is NSGo's UIKit backend: one UIView per node, styled through
// its CALayer, with text the engine laid out with CoreText drawn on the
// main thread. It calls UIKit through purego; Go itself needs cgo to link
// for iOS.
package ios

import (
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/engine"
	"github.com/oneo-me/nsgo/platform/internal/apple"
	"github.com/oneo-me/nsgo/platform/internal/queue"
)

func init() {
	// UIKit runs on the process's main thread, which main starts on.
	runtime.LockOSThread()
}

// longPressDelay is how long a touch is held before it is a long press.
const longPressDelay = 500 * time.Millisecond

// view is the Go side of one native view.
type view struct {
	surface *surface
	id      core.NodeID
	kind    core.ViewKind
	native  objc.ID
	events  core.EventMask

	pressing  atomic.Bool
	longFired atomic.Bool

	layout *apple.Layout // a text view's
	box    core.Bounds

	field    objc.ID // an input's UITextField
	inset    core.Bounds
	readonly bool
	edits    uint32
}

type surface struct {
	app     engine.App
	native  objc.ID
	engine  *engine.Engine
	batches queue.Batches
	views   []*view // index is NodeID
}

// views maps native objects to their Go side. Only the main thread uses it.
var views = map[objc.ID]*view{}

// app is the surface UIApplicationMain's delegate shows; UIKit makes the
// delegate, so it cannot be handed one.
var app *surface

var (
	sel = objc.RegisterName

	selAlloc, selNew              = sel("alloc"), sel("new")
	selRelease                    = sel("release")
	selLayer                      = sel("layer")
	selSetFrame, selBounds        = sel("setFrame:"), sel("bounds")
	selAddSubview, selSuperview   = sel("addSubview:"), sel("superview")
	selInsertBelow                = sel("insertSubview:belowSubview:")
	selRemoveFromSuperview        = sel("removeFromSuperview")
	selSetHidden                  = sel("setHidden:")
	selSetNeedsDisplay            = sel("setNeedsDisplay")
	selSetNeedsLayout             = sel("setNeedsLayout")
	selLocationInView             = sel("locationInView:")
	selAnyObject                  = sel("anyObject")
	selText, selSetText           = sel("text"), sel("setText:")
	selSetFont, selSetTextColor   = sel("setFont:"), sel("setTextColor:")
	selSetPlaceholder             = sel("setPlaceholder:")
	selSetTextAlignment           = sel("setTextAlignment:")
	selSetSecure                  = sel("setSecureTextEntry:")
	selSetKeyboardType            = sel("setKeyboardType:")
	selSetEnabled                 = sel("setEnabled:")
	selBecomeFirst, selResign     = sel("becomeFirstResponder"), sel("resignFirstResponder")
	selApply                      = sel("nsgoApply:")
	selChanged                    = sel("nsgoChanged:")
	selPerformOnMain              = sel("performSelectorOnMainThread:withObject:waitUntilDone:")
	selSetIsAccessibilityElement  = sel("setIsAccessibilityElement:")
	selSetAccessibilityTraits     = sel("setAccessibilityTraits:")
	selSetAccessibilityLabel      = sel("setAccessibilityLabel:")
	selSetAccessibilityValue      = sel("setAccessibilityValue:")
	selSetAccessibilityIdentifier = sel("setAccessibilityIdentifier:")
	selIsEqualToString            = sel("isEqualToString:")
	selSetContentMode             = sel("setContentMode:")
	selSetOpaque                  = sel("setOpaque:")
	selInitWithFrame, selSetDeleg = sel("initWithFrame:"), sel("setDelegate:")
	selContentOffset              = sel("contentOffset")
	selSetContentSize             = sel("setContentSize:")

	classView, classSurface, classInput, classScroll objc.Class

	uiGraphicsGetCurrentContext func() uintptr
	uiApplicationMain           func(argc int, argv unsafe.Pointer, principal, delegate uintptr) int
	traits                      = map[string]uint64{}
)

func loadUIKit() {
	uikit, err := purego.Dlopen("/System/Library/Frameworks/UIKit.framework/UIKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	purego.RegisterLibFunc(&uiGraphicsGetCurrentContext, uikit, "UIGraphicsGetCurrentContext")
	purego.RegisterLibFunc(&uiApplicationMain, uikit, "UIApplicationMain")
	for _, name := range []string{"Button", "Link", "Header", "Image", "StaticText", "Selected", "NotEnabled"} {
		addr, err := purego.Dlsym(uikit, "UIAccessibilityTrait"+name)
		if err != nil {
			panic(err)
		}
		traits[name] = **(**uint64)(unsafe.Pointer(&addr))
	}

	uiView := objc.GetClass("UIView")
	common := []objc.MethodDef{
		{Cmd: sel("drawRect:"), Fn: drawRect},
		{Cmd: sel("touchesBegan:withEvent:"), Fn: touchesBegan},
		{Cmd: sel("touchesEnded:withEvent:"), Fn: touchesEnded},
		{Cmd: sel("touchesCancelled:withEvent:"), Fn: touchesCancelled},
		{Cmd: sel("accessibilityActivate"), Fn: accessibilityActivate},
	}
	classView = register("NsgoView", uiView, nil, common)
	classSurface = register("NsgoSurface", uiView, nil, append(common,
		objc.MethodDef{Cmd: sel("layoutSubviews"), Fn: surfaceLayout},
		objc.MethodDef{Cmd: selApply, Fn: surfaceApply},
	))
	classInput = register("NsgoInput", uiView, []*objc.Protocol{objc.GetProtocol("UITextFieldDelegate")}, append(common,
		objc.MethodDef{Cmd: sel("layoutSubviews"), Fn: inputLayout},
		objc.MethodDef{Cmd: selChanged, Fn: inputChanged},
		objc.MethodDef{Cmd: sel("textFieldShouldReturn:"), Fn: inputReturn},
		objc.MethodDef{Cmd: sel("textFieldShouldBeginEditing:"), Fn: inputShouldBegin},
		objc.MethodDef{Cmd: sel("textFieldDidBeginEditing:"), Fn: inputBegan},
		objc.MethodDef{Cmd: sel("textFieldDidEndEditing:"), Fn: inputEnded},
	))
	classScroll = register("NsgoScrollView", objc.GetClass("UIScrollView"),
		[]*objc.Protocol{objc.GetProtocol("UIScrollViewDelegate")},
		[]objc.MethodDef{{Cmd: sel("scrollViewDidScroll:"), Fn: scrolled}})
	register("NsgoAppDelegate", objc.GetClass("UIResponder"),
		[]*objc.Protocol{objc.GetProtocol("UIApplicationDelegate")},
		[]objc.MethodDef{{Cmd: sel("application:didFinishLaunchingWithOptions:"),
			Fn: func(self objc.ID, _ objc.SEL, application, options objc.ID) bool { return true }}})
	// The SDK requires the scene life cycle; Info.plist names this class.
	register("NsgoSceneDelegate", objc.GetClass("UIResponder"),
		[]*objc.Protocol{objc.GetProtocol("UIWindowSceneDelegate")},
		[]objc.MethodDef{{Cmd: sel("scene:willConnectToSession:options:"), Fn: sceneWillConnect}})
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

// Run shows app full screen and runs UIKit's event loop. Call it from
// main; it does not return.
func Run(root engine.App) {
	app = &surface{app: root}
	loadUIKit()
	argv := []*byte{&[]byte("nsgo\x00")[0], nil}
	uiApplicationMain(1, unsafe.Pointer(&argv[0]), 0, apple.CFString("NsgoAppDelegate"))
}

// window keeps the key window alive.
var window objc.ID

func sceneWillConnect(self objc.ID, _ objc.SEL, scene, session, options objc.ID) {
	window = objc.ID(objc.GetClass("UIWindow")).Send(selAlloc).Send(sel("initWithWindowScene:"), scene)
	bounds := objc.Send[apple.CGRect](window, selBounds)
	controller := objc.ID(objc.GetClass("UIViewController")).Send(selNew)
	s := app
	s.native = objc.ID(classSurface).Send(selAlloc).Send(selInitWithFrame, bounds)
	views[s.native] = &view{surface: s}
	controller.Send(sel("setView:"), s.native)
	window.Send(sel("setRootViewController:"), controller)
	window.Send(sel("makeKeyAndVisible"))
	s.engine = engine.Start(s, s.app, engine.Options{
		Size: core.Size{Width: core.Points(bounds.Size.Width), Height: core.Points(bounds.Size.Height)},
	})
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
			parent := s.views[it.Parent].native
			if it.Before == 0 {
				parent.Send(selAddSubview, v.native)
			} else {
				parent.Send(selInsertBelow, v.native, s.views[it.Before].native)
			}
		case core.ItemFrame:
			f := it.Frame
			v.native.Send(selSetFrame, apple.Rect(float64(f.Origin.X), float64(f.Origin.Y),
				float64(f.Size.Width), float64(f.Size.Height)))
		case core.ItemViewStyle:
			apple.StyleLayer(v.native.Send(selLayer), it.ViewStyle, false)
			v.native.Send(selSetHidden, it.ViewStyle.Hidden)
		case core.ItemViewProps:
			setAccessibility(v.native, it.ViewProps)
		case core.ItemEvents:
			v.events = it.Events
		case core.ItemText:
			v.layout.Release()
			v.layout, _ = it.Text.Layout.(*apple.Layout)
			v.box = it.Text.Content
			v.native.Send(selSetAccessibilityLabel, apple.NSString(it.Text.Text))
			v.native.Send(selSetNeedsDisplay)
		case core.ItemInput:
			v.configure(it.Input)
		case core.ItemInputValue:
			if v.edits <= it.Count {
				value := apple.NSString(it.Value)
				// Setting the text the field shows would move the caret.
				if !objc.Send[bool](v.field.Send(selText), selIsEqualToString, value) {
					v.field.Send(selSetText, value)
				}
			}
		case core.ItemScrollConfig:
			c := it.Scroll
			v.native.Send(sel("setShowsVerticalScrollIndicator:"), !c.Horizontal && !c.HideIndicator)
			v.native.Send(sel("setShowsHorizontalScrollIndicator:"), c.Horizontal && !c.HideIndicator)
			v.native.Send(sel("setAlwaysBounceVertical:"), !c.Horizontal)
			v.native.Send(sel("setAlwaysBounceHorizontal:"), c.Horizontal)
		case core.ItemScrollContent:
			v.native.Send(selSetContentSize, apple.CGSize{Width: float64(it.Size.Width), Height: float64(it.Size.Height)})
		case core.ItemFocus:
			if v.field != 0 {
				v.field.Send(selBecomeFirst)
			}
		case core.ItemBlur:
			if v.field != 0 {
				v.field.Send(selResign)
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
		v.native = objc.ID(classScroll).Send(selNew)
		v.native.Send(selSetDeleg, v.native)
	case core.KindInput:
		v.native = objc.ID(classInput).Send(selNew)
		field := objc.ID(objc.GetClass("UITextField")).Send(selNew)
		field.Send(selSetDeleg, v.native)
		const editingChanged = 1 << 17
		field.Send(sel("addTarget:action:forControlEvents:"), v.native, selChanged, uint(editingChanged))
		v.native.Send(selAddSubview, field)
		field.Send(selRelease)
		v.field = field
	default:
		v.native = objc.ID(classView).Send(selNew)
		if kind == core.KindText {
			const redraw = 3
			v.native.Send(selSetContentMode, redraw)
		}
	}
	v.native.Send(selSetOpaque, false)
	views[v.native] = v
	return v
}

func (s *surface) delete(v *view) {
	v.native.Send(selRemoveFromSuperview)
	v.layout.Release()
	delete(views, v.native)
	v.native.Send(selRelease)
	s.views[v.id] = nil
}

// UIKit's text alignments.
const uiAlignLeft, uiAlignCenter, uiAlignRight = 0, 1, 2

// UIKeyboardType of each core keyboard type.
var keyboards = map[core.KeyboardType]int{
	core.KeyboardDefault: 0, core.KeyboardURL: 3, core.KeyboardNumber: 4,
	core.KeyboardPhone: 5, core.KeyboardEmail: 7, core.KeyboardDecimal: 8,
}

func (v *view) configure(c *core.InputConfig) {
	f := v.field
	font := apple.Font(c.Style.Font)
	f.Send(selSetFont, objc.ID(font))
	apple.Release(font)
	r, g, b, a := c.Style.Color.Components()
	f.Send(selSetTextColor, objc.ID(objc.GetClass("UIColor")).Send(sel("colorWithRed:green:blue:alpha:"), r, g, b, a))
	align := uiAlignLeft
	switch c.Style.Align {
	case core.TextAlignCenter:
		align = uiAlignCenter
	case core.TextAlignEnd:
		align = uiAlignRight
	}
	f.Send(selSetTextAlignment, align)
	f.Send(selSetPlaceholder, apple.NSString(c.Placeholder))
	f.Send(selSetSecure, c.Masked)
	f.Send(selSetKeyboardType, keyboards[c.KeyboardType])
	f.Send(selSetEnabled, !c.Disabled)
	v.readonly = c.Readonly
	v.inset = c.Content
	v.native.Send(selSetNeedsLayout)
}

func setAccessibility(native objc.ID, p *core.ViewProps) {
	if p.TestID != "" {
		native.Send(selSetAccessibilityIdentifier, apple.NSString(p.TestID))
	}
	var t uint64
	switch p.Role {
	case core.RoleNone:
		native.Send(selSetIsAccessibilityElement, false)
		return
	case core.RoleButton, core.RoleCheckbox, core.RoleSwitch, core.RoleRadio, core.RoleTab:
		t = traits["Button"]
	case core.RoleLink:
		t = traits["Link"]
	case core.RoleHeader:
		t = traits["Header"]
	case core.RoleImage:
		t = traits["Image"]
	case core.RoleText:
		t = traits["StaticText"]
	}
	if p.Selected {
		t |= traits["Selected"]
	}
	if p.Disabled {
		t |= traits["NotEnabled"]
	}
	native.Send(selSetIsAccessibilityElement, true)
	native.Send(selSetAccessibilityTraits, t)
	if p.Label != "" {
		native.Send(selSetAccessibilityLabel, apple.NSString(p.Label))
	}
	if p.Role == core.RoleCheckbox || p.Role == core.RoleSwitch || p.Role == core.RoleRadio {
		value := "0"
		if p.Checked || p.Selected {
			value = "1"
		}
		native.Send(selSetAccessibilityValue, apple.NSString(value))
	}
}

func (v *view) dispatch(kind core.EventKind) {
	if v.events.Has(kind) {
		v.surface.engine.Dispatch(core.Event{Kind: kind, ID: v.id})
	}
}

func (v *view) pressable() bool {
	return v.events.Has(core.EventPress) || v.events.Has(core.EventLongPress) ||
		v.events.Has(core.EventPressDown) || v.events.Has(core.EventPressUp)
}

func drawRect(self objc.ID, _ objc.SEL, dirty apple.CGRect) {
	v := views[self]
	if v == nil || v.layout == nil {
		return
	}
	b := objc.Send[apple.CGRect](self, selBounds)
	box := v.box
	v.layout.Draw(uiGraphicsGetCurrentContext(), b.Size.Height, apple.Rect(float64(box.Origin.X), float64(box.Origin.Y),
		float64(box.Size.Width), float64(box.Size.Height)))
}

// Touches a view does not handle go up the responder chain to a pressable
// ancestor.
func touchesBegan(self objc.ID, cmd objc.SEL, touches, event objc.ID) {
	v := views[self]
	if v == nil || !v.pressable() {
		self.SendSuper(cmd, touches, event)
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

func touchesCancelled(self objc.ID, cmd objc.SEL, touches, event objc.ID) {
	v := views[self]
	if v == nil || !v.pressable() {
		self.SendSuper(cmd, touches, event)
		return
	}
	if v.pressing.Swap(false) {
		v.dispatch(core.EventPressUp)
	}
}

func touchesEnded(self objc.ID, cmd objc.SEL, touches, event objc.ID) {
	v := views[self]
	if v == nil || !v.pressable() {
		self.SendSuper(cmd, touches, event)
		return
	}
	p := objc.Send[apple.CGPoint](touches.Send(selAnyObject), selLocationInView, self)
	b := objc.Send[apple.CGRect](self, selBounds)
	pressed := v.pressing.Swap(false)
	v.dispatch(core.EventPressUp)
	if pressed && !v.longFired.Load() && p.X >= 0 && p.Y >= 0 && p.X < b.Size.Width && p.Y < b.Size.Height {
		v.dispatch(core.EventPress)
	}
}

func accessibilityActivate(self objc.ID, _ objc.SEL) bool {
	v := views[self]
	if v == nil || !v.events.Has(core.EventPress) {
		return false
	}
	v.dispatch(core.EventPress)
	return true
}

func surfaceLayout(self objc.ID, cmd objc.SEL) {
	self.SendSuper(cmd)
	if v := views[self]; v != nil && v.surface.engine != nil {
		b := objc.Send[apple.CGRect](self, selBounds)
		v.surface.engine.Resize(core.Size{Width: core.Points(b.Size.Width), Height: core.Points(b.Size.Height)})
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
	in := v.inset
	v.field.Send(selSetFrame, apple.Rect(float64(in.Origin.X), float64(in.Origin.Y),
		float64(in.Size.Width), float64(in.Size.Height)))
}

func inputChanged(self objc.ID, _ objc.SEL, field objc.ID) {
	if v := views[self]; v != nil {
		v.edits++
		v.surface.engine.Dispatch(core.Event{Kind: core.EventChangeText, ID: v.id,
			Text: apple.GoString(field.Send(selText)), Count: v.edits})
	}
}

func inputReturn(self objc.ID, _ objc.SEL, field objc.ID) bool {
	if v := views[self]; v != nil {
		v.surface.engine.Dispatch(core.Event{Kind: core.EventSubmitEditing, ID: v.id,
			Text: apple.GoString(field.Send(selText))})
	}
	return true
}

func inputShouldBegin(self objc.ID, _ objc.SEL, _ objc.ID) bool {
	v := views[self]
	return v == nil || !v.readonly
}

func inputBegan(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.dispatch(core.EventFocus)
	}
}

func inputEnded(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil {
		v.dispatch(core.EventBlur)
	}
}

func scrolled(self objc.ID, _ objc.SEL, _ objc.ID) {
	if v := views[self]; v != nil && v.events.Has(core.EventScroll) {
		o := objc.Send[apple.CGPoint](self, selContentOffset)
		v.surface.engine.Dispatch(core.Event{Kind: core.EventScroll, ID: v.id,
			Point: core.Point{X: core.Points(o.X), Y: core.Points(o.Y)}})
	}
}
