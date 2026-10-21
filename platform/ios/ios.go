//go:build ios

// Package ios is NSGo's UIKit backend: one UIView per node, CoreText text
// measured on the engine goroutine and drawn on the main thread. It calls
// UIKit through purego; Go itself needs cgo to link for iOS.
package ios

import (
	"runtime"
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

// view is the Go side of one native view.
type view struct {
	surface   *surface
	id        core.NodeID
	pressable bool
	pressing  bool
	padding   core.Edges
	text      *apple.Text
	field     objc.ID // an input's UITextField
}

type surface struct {
	app     engine.App
	native  objc.ID
	engine  *engine.Engine
	batches queue.Batches
	views   []objc.ID // index is NodeID
}

// views maps native objects to their Go side. Only the main thread uses it.
var views = map[objc.ID]*view{}

// app is the surface UIApplicationMain's delegate shows; UIKit makes the
// delegate, so it cannot be handed one.
var app *surface

var (
	sel = objc.RegisterName

	selAlloc, selNew              = sel("alloc"), sel("new")
	selLayer                      = sel("layer")
	selSetBackgroundColor         = sel("setBackgroundColor:")
	selSetCornerRadius            = sel("setCornerRadius:")
	selSetFrame, selBounds        = sel("setFrame:"), sel("bounds")
	selAddSubview, selSuperview   = sel("addSubview:"), sel("superview")
	selInsertSubview              = sel("insertSubview:atIndex:")
	selSetNeedsDisplay            = sel("setNeedsDisplay")
	selSetNeedsLayout             = sel("setNeedsLayout")
	selLocationInView             = sel("locationInView:")
	selAnyObject                  = sel("anyObject")
	selText, selSetText           = sel("text"), sel("setText:")
	selSetFont, selSetTextColor   = sel("setFont:"), sel("setTextColor:")
	selSetPlaceholder             = sel("setPlaceholder:")
	selApply                      = sel("nsgoApply:")
	selChanged                    = sel("nsgoChanged:")
	selPerformOnMain              = sel("performSelectorOnMainThread:withObject:waitUntilDone:")
	selSetIsAccessibilityElement  = sel("setIsAccessibilityElement:")
	selSetAccessibilityTraits     = sel("setAccessibilityTraits:")
	selSetAccessibilityLabel      = sel("setAccessibilityLabel:")
	selIsEqualToString            = sel("isEqualToString:")
	selSetContentMode             = sel("setContentMode:")
	selSetClearBackground         = sel("setOpaque:")
	selInitWithFrame, selSetDeleg = sel("initWithFrame:"), sel("setDelegate:")

	classView, classSurface, classInput objc.Class

	uiGraphicsGetCurrentContext func() uintptr
	uiApplicationMain           func(argc int, argv unsafe.Pointer, principal, delegate uintptr) int
	accessibilityButton         uint64
)

func loadUIKit() {
	uikit, err := purego.Dlopen("/System/Library/Frameworks/UIKit.framework/UIKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	purego.RegisterLibFunc(&uiGraphicsGetCurrentContext, uikit, "UIGraphicsGetCurrentContext")
	purego.RegisterLibFunc(&uiApplicationMain, uikit, "UIApplicationMain")
	addr, err := purego.Dlsym(uikit, "UIAccessibilityTraitButton")
	if err != nil {
		panic(err)
	}
	accessibilityButton = **(**uint64)(unsafe.Pointer(&addr))

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
	))
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
	s.engine = engine.Start(s, s.app, core.Size{Width: core.Points(bounds.Size.Width), Height: core.Points(bounds.Size.Height)})
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
			s.views[it.Parent].Send(selInsertSubview, s.views[it.ID], it.Index)
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
	native.Send(selSetClearBackground, false)
	if tag == core.TagText {
		const redraw = 3
		native.Send(selSetContentMode, redraw)
	}
	if tag == core.TagInput {
		field := objc.ID(objc.GetClass("UITextField")).Send(selNew)
		field.Send(selSetDeleg, native)
		const editingChanged = 1 << 17
		field.Send(sel("addTarget:action:forControlEvents:"), native, selChanged, uint(editingChanged))
		native.Send(selAddSubview, field)
		v.field = field
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
		native.Send(selSetIsAccessibilityElement, true)
		native.Send(selSetAccessibilityTraits, accessibilityButton)
	}
	switch {
	case v.field != 0:
		value := apple.NSString(p.Text)
		// Setting the text the user just typed would move the caret.
		if !objc.Send[bool](v.field.Send(selText), selIsEqualToString, value) {
			v.field.Send(selSetText, value)
		}
		font := apple.Font(p.Font)
		v.field.Send(selSetFont, objc.ID(font))
		apple.Release(font)
		r, g, b, a := p.Color.Components()
		v.field.Send(selSetTextColor, objc.ID(objc.GetClass("UIColor")).Send(sel("colorWithRed:green:blue:alpha:"), r, g, b, a))
		v.field.Send(selSetPlaceholder, apple.NSString(p.Placeholder))
		native.Send(selSetNeedsLayout)
	case p.Font.Size > 0:
		v.text.Free()
		v.text = apple.NewText(p.Text, p.Font, p.Color)
		native.Send(selSetIsAccessibilityElement, true)
		native.Send(selSetAccessibilityLabel, apple.NSString(p.Text))
		native.Send(selSetNeedsDisplay)
	}
}

func drawRect(self objc.ID, _ objc.SEL, dirty apple.CGRect) {
	v := views[self]
	if v == nil || v.text == nil {
		return
	}
	b := objc.Send[apple.CGRect](self, selBounds)
	p := v.padding
	v.text.Draw(uiGraphicsGetCurrentContext(), b.Size.Height, apple.Rect(float64(p.Left), float64(p.Top),
		b.Size.Width-float64(p.Left+p.Right), b.Size.Height-float64(p.Top+p.Bottom)))
}

// Touches a view does not handle go up the responder chain to a pressable
// ancestor.
func touchesBegan(self objc.ID, cmd objc.SEL, touches, event objc.ID) {
	if v := views[self]; v != nil && v.pressable {
		v.pressing = true
		return
	}
	self.SendSuper(cmd, touches, event)
}

func touchesCancelled(self objc.ID, cmd objc.SEL, touches, event objc.ID) {
	if v := views[self]; v != nil && v.pressable {
		v.pressing = false
		return
	}
	self.SendSuper(cmd, touches, event)
}

func touchesEnded(self objc.ID, cmd objc.SEL, touches, event objc.ID) {
	v := views[self]
	if v == nil || !v.pressable {
		self.SendSuper(cmd, touches, event)
		return
	}
	p := objc.Send[apple.CGPoint](touches.Send(selAnyObject), selLocationInView, self)
	b := objc.Send[apple.CGRect](self, selBounds)
	if v.pressing && p.X >= 0 && p.Y >= 0 && p.X < b.Size.Width && p.Y < b.Size.Height {
		v.surface.engine.Press(v.id)
	}
	v.pressing = false
}

func accessibilityActivate(self objc.ID, _ objc.SEL) bool {
	v := views[self]
	if v == nil || !v.pressable {
		return false
	}
	v.surface.engine.Press(v.id)
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
	b := objc.Send[apple.CGRect](self, selBounds)
	p := v.padding
	v.field.Send(selSetFrame, apple.Rect(float64(p.Left), float64(p.Top),
		max(b.Size.Width-float64(p.Left+p.Right), 0), max(b.Size.Height-float64(p.Top+p.Bottom), 0)))
}

func inputChanged(self objc.ID, _ objc.SEL, field objc.ID) {
	if v := views[self]; v != nil {
		v.surface.engine.Change(v.id, apple.GoString(field.Send(selText)))
	}
}

func inputReturn(self objc.ID, _ objc.SEL, field objc.ID) bool {
	if v := views[self]; v != nil {
		v.surface.engine.Submit(v.id, apple.GoString(field.Send(selText)))
	}
	return true
}
