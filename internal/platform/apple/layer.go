//go:build darwin

package apple

import (
	"math"
	"runtime"
	"strings"
	"structs"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	sel = Sel

	selSetBackgroundColor = sel("setBackgroundColor:")
	selSetCornerRadius    = sel("setCornerRadius:")
	selSetBorderWidth     = sel("setBorderWidth:")
	selSetBorderColor     = sel("setBorderColor:")
	selSetOpacity         = sel("setOpacity:")
	selSetMasksToBounds   = sel("setMasksToBounds:")
	selSetShadowColor     = sel("setShadowColor:")
	selSetShadowOffset    = sel("setShadowOffset:")
	selSetShadowRadius    = sel("setShadowRadius:")
	selSetShadowOpacity   = sel("setShadowOpacity:")
	selSetShadow          = sel("setShadow:")
	selValueForKey        = sel("valueForKey:")
	selBounds             = sel("bounds")

	caCurrentMediaTime func() float64
	// msgSendFloat64 is objc_msgSend for a message that takes nothing and returns a double, registered once for per-frame reads.
	msgSendFloat64     func(objc.ID, objc.SEL) float64
	selTargetTimestamp            = sel("targetTimestamp")
	selSetPreferredFrameRateRange = sel("setPreferredFrameRateRange:")
)

// StyleLayer draws vs with a view's CALayer. An NSView whose layer it
// is takes the layer's shadow, as AppKit copies a view's shadow onto its
// layer. A style with a transition animates what it lists from the
// values shown; was is the style last drawn, nil for none.
func StyleLayer(layer objc.ID, was, vs *driver.ViewStyle, view objc.ID) {
	StyleLayerBound(layer, was, vs, view, 0)
}

// StyleLayerBound draws vs with layer like StyleLayer, skipping Core Animation transitions for properties named in bound (AnimatedValues.Set) so bound animated values keep moving immediately while unbound properties run vs.Transition.
func StyleLayerBound(layer objc.ID, was, vs *driver.ViewStyle, view objc.ID, bound uint16) {
	styleLayerBound(layer, was, vs, view, bound, nil)
}

func styleLayerBound(layer objc.ID, was, vs *driver.ViewStyle, view objc.ID, bound uint16, geom *TransformGeometry) {
	tr := vs.Transition
	var keys []string
	var from []objc.ID
	if tr.Duration > 0 {
		shown := layer.Send(sel("presentationLayer"))
		if shown == 0 {
			shown = layer
		}
		keys = transitionKeys(tr.Properties, bound)
		for _, k := range keys {
			value := shown.Send(selValueForKey, NSString(k))
			if value != 0 {
				value = value.Send(sel("retain"))
			}
			from = append(from, value)
		}
		defer func() {
			for _, value := range from {
				if value != 0 {
					value.Send(selRelease)
				}
			}
		}()
	}
	color := func(sel objc.SEL, c driver.Color) {
		if c == 0 {
			Msg(layer, sel, 0)
			return
		}
		ref := CGColor(c)
		Msg(layer, sel, ref)
		Release(ref)
	}
	if was == nil || vs.Background != was.Background {
		color(selSetBackgroundColor, vs.Background)
	}
	if was == nil || vs.CornerRadius != was.CornerRadius {
		layer.Send(selSetCornerRadius, float64(vs.CornerRadius))
	}
	if was == nil || vs.BorderWidth != was.BorderWidth {
		layer.Send(selSetBorderWidth, float64(vs.BorderWidth))
	}
	if was == nil || vs.BorderColor != was.BorderColor {
		color(selSetBorderColor, vs.BorderColor)
	}
	if was == nil || vs.Opacity != was.Opacity {
		layer.Send(selSetOpacity, vs.Opacity)
	}
	if was == nil || vs.Clip != was.Clip {
		var clip uintptr
		if vs.Clip {
			clip = 1
		}
		Msg(layer, selSetMasksToBounds, clip)
	}
	sh := layerShadow(vs)
	switch {
	case view != 0:
		if was == nil || layerShadow(was) != sh {
			setViewShadow(view, sh)
		}
	case was != nil && layerShadow(was) == sh:
	case sh.Color == 0:
		layer.Send(selSetShadowOpacity, float32(0))
	default:
		color(selSetShadowColor, sh.Color)
		layer.Send(selSetShadowOffset, CGSize{Width: float64(sh.Offset.X), Height: float64(sh.Offset.Y)})
		layer.Send(selSetShadowRadius, float64(sh.Blur)/2)
		layer.Send(selSetShadowOpacity, float32(1))
	}
	setBorders(layer, vs, &tr, bound)
	applyDecor(layer, vs, bound)
	if _, isHeld := held[layer]; isHeld || vs.Transform != (driver.Transform{}) {
		SetTransformGeometry(layer, vs.Transform, geom)
	}
	for i, k := range keys {
		to := layer.Send(selValueForKey, NSString(k))
		if from[i] == to || (from[i] != 0 && objc.Send[bool](from[i], sel("isEqual:"), to)) {
			continue
		}
		start := from[i]
		if start == 0 && strings.HasSuffix(k, "Color") {
			clear := CGColor(0)
			start = objc.ID(clear).Send(sel("autorelease"))
		}
		var anim objc.ID
		if k == "transform" {
			anim = moveAnimation(layer, was, vs, geom)
		}
		if anim == 0 {
			anim = objc.ID(Class("CABasicAnimation")).Send(sel("animationWithKeyPath:"), NSString(k))
			anim.Send(sel("setFromValue:"), start)
		}
		anim.Send(sel("setDuration:"), tr.Duration.Seconds())
		anim.Send(sel("setTimingFunction:"), timing(tr.Easing))
		SetAnimationFrameRate(anim, 0)
		if tr.Delay > 0 {
			anim.Send(sel("setBeginTime:"), caCurrentMediaTime()+tr.Delay.Seconds())
			anim.Send(sel("setFillMode:"), NSString("backwards"))
		}
		layer.Send(selAddAnimation, anim, NSString("nsgo."+k))
	}
}

// ClearBoundAnimations removes active style transitions on layer (and its border or cover sublayers) for properties named in bound, leaving transitions on unbound properties running.
func ClearBoundAnimations(layer objc.ID, bound uint16) {
	if bound == 0 {
		return
	}
	remove := func(k string) {
		layer.Send(selRemoveAnimation, NSString("nsgo."+k))
	}
	if bound&(1<<driver.PropOpacity) != 0 {
		remove("opacity")
	}
	if bound&(1<<driver.PropBackground) != 0 {
		remove("backgroundColor")
		if d := decors[layer]; d != nil && d.cover != 0 {
			d.cover.Send(selRemoveAnimation, NSString("nsgo.backgroundColor"))
		}
	}
	if bound&driver.AnimatedTransformMask != 0 {
		remove("transform")
		delete(moves, layer)
	}
	if bound&(1<<driver.PropBorderColor) != 0 {
		remove("borderColor")
		if r := rings[layer]; r != nil {
			for _, side := range r.sides {
				if side != 0 {
					side.Send(selRemoveAnimation, NSString("nsgo.fillColor"))
				}
			}
		}
	}
	if bound&driver.AnimatedShadowColorMask != 0 {
		remove("shadowColor")
		remove("shadowOpacity")
	}
	if bound&(1<<driver.PropShadowRadius) != 0 {
		remove("shadowRadius")
	}
}

// RestyleLayer draws vs with a layer that shows shown, the style StyleLayer last drew there; with none it draws vs at once. When the two differ only in opacity and transform, as an animation's frames do, it sets just those: drawing every property again remade its colors, shadow, filters and gradients each frame, and the window server drew them all again. bound names the properties currently driven by animated values or appearance motions, whose transitions are skipped.
func RestyleLayer(layer objc.ID, shown, vs *driver.ViewStyle, view objc.ID, bound uint16) {
	RestyleLayerGeometry(layer, shown, vs, view, bound, nil)
}

// RestyleLayerGeometry is RestyleLayer using geom for layer's bounds size and
// anchor point when known. It reports whether the full style path ran (rather
// than the opacity/transform fast path), so callers need only rehome per-side
// borders when borders could have been created.
func RestyleLayerGeometry(layer objc.ID, shown, vs *driver.ViewStyle, view objc.ID, bound uint16, geom *TransformGeometry) bool {
	if shown == nil && vs.Transition.Duration > 0 {
		// A view's first style shows at once, as CSS transitions none.
		first := *vs
		first.Transition = driver.Transition{}
		styleLayerBound(layer, nil, &first, view, bound, geom)
		return true
	}
	if shown == nil || vs.Transition.Duration > 0 || (!onlyMoves(shown, vs) && !onlyMovesOrRecolors(shown, vs)) {
		styleLayerBound(layer, shown, vs, view, bound, geom)
		return true
	}
	if vs.Background != shown.Background {
		recolorLayer(layer, vs.Background)
	}
	if vs.Opacity != shown.Opacity {
		setLayerOpacity(layer, vs.Opacity)
	}
	if vs.Transform != shown.Transform {
		SetTransformGeometry(layer, vs.Transform, geom)
	}
	if d := decors[layer]; d != nil {
		d.vs.Background, d.vs.Opacity, d.vs.Transform = vs.Background, vs.Opacity, vs.Transform
	}
	return false
}

// onlyMoves reports whether b differs from a in its opacity and transform alone (ignoring Transition, which RestyleLayer checks before calling it). An opacity under filters is drawn with them on iOS (layoutFilters).
func onlyMoves(a, b *driver.ViewStyle) bool {
	x, y := *a, *b
	x.Transition, y.Transition = driver.Transition{}, driver.Transition{}
	x.Transform, y.Transform = driver.Transform{}, driver.Transform{}
	if x.Effects != nil && len(x.Effects.Filters) > 0 {
		return x == y
	}
	x.Opacity, y.Opacity = 0, 0
	return x == y
}

// onlyMovesOrRecolors reports whether b differs from a only in opacity,
// transform and background color without crossing the opaque/translucent or
// transparent/visible boundaries or involving a rimmed border, so RestyleLayer
// can repaint the fill color without rebuilding shadows, corners or filters.
func onlyMovesOrRecolors(a, b *driver.ViewStyle) bool {
	if (a.Background&0xff == 0xff) != (b.Background&0xff == 0xff) ||
		(a.Background&0xff == 0) != (b.Background&0xff == 0) {
		return false
	}
	if _, _, rimA := a.Rim(); rimA {
		return false
	}
	if _, _, rimB := b.Rim(); rimB {
		return false
	}
	x, y := *a, *b
	x.Background, y.Background = 0, 0
	return onlyMoves(&x, &y)
}

func setLayerColor(layer objc.ID, s objc.SEL, c driver.Color) {
	if c == 0 {
		Msg(layer, s, 0)
		return
	}
	ref := CGColor(c)
	Msg(layer, s, ref)
	Release(ref)
}

// recolorLayer updates only layer's background fill (or its decor fill/cover
// sublayer) when onlyMovesOrRecolors took the fast path.
func recolorLayer(layer objc.ID, c driver.Color) {
	if d := decors[layer]; d != nil {
		switch {
		case d.shaped && d.fill != 0:
			setLayerColor(d.fill, selSetFill, c)
			return
		case !d.shaped && d.cover != 0:
			d.paintCover(c)
			d.covers = c
			return
		default:
			d.covers = c
		}
	}
	setLayerColor(layer, selSetBackgroundColor, c)
}

// layerShadow returns the shadow a view's layer casts of what it draws:
// its widest drop shadow, as NSH's Apple backends draw, else its React
// Native shadow, whose radius is half a box shadow's blur.
func layerShadow(vs *driver.ViewStyle) driver.BoxShadow {
	if vs.Effects != nil {
		if _, _, _, shadows := driver.SplitFilters(vs.Effects.Filters); len(shadows) > 0 {
			sh := shadows[0]
			for _, s := range shadows[1:] {
				if s.Blur >= sh.Blur {
					sh = s
				}
			}
			return sh
		}
	}
	return driver.BoxShadow{Offset: vs.ShadowOffset, Blur: 2 * vs.ShadowRadius, Color: vs.ShadowColor}
}

// setViewShadow gives an NSView sh. AppKit owns the shadow of a view's
// layer and resets what is set on the layer to the view's shadow, which
// it copies there unchanged: the offset as the layer's, the blur radius
// as its shadow radius.
func setViewShadow(view objc.ID, sh driver.BoxShadow) {
	if sh.Color == 0 {
		view.Send(selSetShadow, objc.ID(0))
		return
	}
	ns := objc.ID(Class("NSShadow")).Send(selNew)
	ref := CGColor(sh.Color)
	ns.Send(sel("setShadowColor:"), objc.ID(Class("NSColor")).Send(sel("colorWithCGColor:"), ref))
	Release(ref)
	ns.Send(sel("setShadowOffset:"), CGSize{Width: float64(sh.Offset.X), Height: float64(sh.Offset.Y)})
	ns.Send(sel("setShadowBlurRadius:"), float64(sh.Blur)/2)
	view.Send(selSetShadow, ns)
	ns.Send(selRelease)
}

// keyBound reports whether CALayer property k is currently driven by bound (AnimatedValues.Set).
func keyBound(k string, bound uint16) bool {
	switch k {
	case "opacity":
		return bound&(1<<driver.PropOpacity) != 0
	case "backgroundColor":
		return bound&(1<<driver.PropBackground) != 0
	case "transform":
		return bound&driver.AnimatedTransformMask != 0
	case "borderColor":
		return bound&(1<<driver.PropBorderColor) != 0
	case "shadowColor", "shadowOpacity":
		return bound&driver.AnimatedShadowColorMask != 0
	case "shadowRadius":
		return bound&(1<<driver.PropShadowRadius) != 0
	}
	return false
}

// transitionKeys returns the CALayer keys of the properties a transition animates, excluding any driven by bound.
func transitionKeys(p driver.TransitionProperty, bound uint16) []string {
	if p == 0 {
		p = ^driver.TransitionProperty(0)
	}
	var keys []string
	add := func(bit driver.TransitionProperty, ks ...string) {
		if p&bit == 0 {
			return
		}
		for _, k := range ks {
			if !keyBound(k, bound) {
				keys = append(keys, k)
			}
		}
	}
	add(driver.TransitionOpacity, "opacity")
	add(driver.TransitionBackground, "backgroundColor")
	add(driver.TransitionTransform, "transform")
	add(driver.TransitionBorder, "borderWidth", "borderColor", "cornerRadius")
	add(driver.TransitionShadow, "shadowColor", "shadowOpacity", "shadowOffset", "shadowRadius")
	return keys
}

// timing returns the CAMediaTimingFunction of a curve.
func timing(c driver.Curve) objc.ID {
	fn := objc.ID(Class("CAMediaTimingFunction"))
	if c == (driver.Curve{}) {
		return fn.Send(sel("functionWithName:"), NSString("linear"))
	}
	return fn.Send(sel("functionWithControlPoints::::"), c.X1, c.Y1, c.X2, c.Y2)
}

// transform3D is CATransform3D, which multiplies row vectors.
type transform3D struct {
	_                                                                              structs.HostLayout
	M11, M12, M13, M14, M21, M22, M23, M24, M31, M32, M33, M34, M41, M42, M43, M44 float64
}

// TransformGeometry holds a CALayer's bounds size and anchor point so animated
// frames compute transforms from Go state without reading structs through
// purego reflection every frame. Unset fields fall back to reading the layer;
// when a non-nil pointer is passed, Anchor is cached on the first read.
type TransformGeometry struct {
	Size        CGSize
	Anchor      CGPoint
	SizeKnown   bool
	AnchorKnown bool
	imp         setTransformImp
	impLayer    objc.ID
}

// SetSize records the layer's bounds size in points.
func (g *TransformGeometry) SetSize(size driver.Size) {
	g.Size = CGSize{Width: float64(size.Width), Height: float64(size.Height)}
	g.SizeKnown = true
}

// ClearSize forgets the recorded bounds size so the next transform reads it
// from the layer.
func (g *TransformGeometry) ClearSize() { g.SizeKnown = false }

// identityTransform3D is CATransform3DIdentity.
var identityTransform3D = transform3D{M11: 1, M22: 1, M33: 1, M44: 1}

// pureTranslation reports whether t only translates in the view's plane, so its
// matrix does not depend on the layer's size, anchor point or transform origin.
func pureTranslation(t driver.Transform) bool {
	if t.Perspective != 0 || t.Rotate != 0 || t.RotateX != 0 || t.RotateY != 0 || t.RotateZ != 0 ||
		t.SkewX != 0 || t.SkewY != 0 || t.Then != (driver.Matrix3D{}) {
		return false
	}
	sx, sy := t.Scales()
	return sx == 1 && sy == 1
}

// resolveTransformGeometry returns layer's bounds size and anchor point from geom
// when known, reading any missing field from Core Animation and caching Anchor
// on geom when non-nil.
func resolveTransformGeometry(layer objc.ID, geom *TransformGeometry) (CGSize, CGPoint) {
	var size CGSize
	var anchor CGPoint
	needSize := geom == nil || !geom.SizeKnown
	needAnchor := geom == nil || !geom.AnchorKnown
	if needSize || needAnchor {
		bindTransforms()
	}
	if needSize {
		size = layerBounds(layer, selBounds).Size
		layerBoundsReads++
	} else {
		size = geom.Size
	}
	if needAnchor {
		anchor = layerAnchor(layer, selAnchorPoint)
		layerAnchorReads++
		if geom != nil {
			geom.Anchor = anchor
			geom.AnchorKnown = true
		}
	} else {
		anchor = geom.Anchor
	}
	return size, anchor
}

// transformForGeometry is the transform for t around the transform origin
// offset from the center of a layer with size and anchor, and that origin's
// distance from the anchor point. It works from wherever the anchor point is,
// since AppKit keeps its layers' at their corner.
func transformForGeometry(size CGSize, anchor CGPoint, t driver.Transform, offset driver.Point, z float64) (transform3D, float64) {
	ax, ay := anchor.X*size.Width, anchor.Y*size.Height
	origin := driver.Point{X: driver.Points(size.Width/2) + offset.X, Y: driver.Points(size.Height/2) + offset.Y}
	m := driver.Translate3D(-ax, -ay, z).Mul(t.Matrix(origin)).Mul(driver.Translate3D(ax, ay, -z))
	return toCA(m), math.Hypot(float64(origin.X)-ax, float64(origin.Y)-ay)
}

// layerTransform is layer's transform for t around the transform origin
// offset from its center, and that origin's distance from the anchor
// point.
func layerTransform(layer objc.ID, t driver.Transform, offset driver.Point, z float64) (transform3D, float64) {
	size, anchor := resolveTransformGeometry(layer, nil)
	return transformForGeometry(size, anchor, t, offset, z)
}

// SetTransform sets a layer's transform to t around its transform
// origin: its center, moved by its style's Origin.
func SetTransform(layer objc.ID, t driver.Transform) {
	SetTransformGeometry(layer, t, nil)
}

// SetTransformGeometry is SetTransform using geom for layer's bounds size and
// anchor point when known.
func SetTransformGeometry(layer objc.ID, t driver.Transform, geom *TransformGeometry) {
	var m transform3D
	if pureTranslation(t) {
		m = transform3D{M11: 1, M22: 1, M33: 1, M44: 1, M41: float64(t.TranslateX), M42: float64(t.TranslateY)}
	} else {
		var offset driver.Point
		var z float64
		if d := decors[layer]; d != nil {
			offset, z = d.vs.Origin, float64(d.vs.OriginZ)
		}
		size, anchor := resolveTransformGeometry(layer, geom)
		m, _ = transformForGeometry(size, anchor, t, offset, z)
	}
	if m == identityTransform3D {
		delete(held, layer)
	} else {
		if _, already := held[layer]; !already && HoldTransforms != nil {
			HoldTransforms(layer)
			if geom != nil {
				geom.impLayer = 0
			}
		}
		held[layer] = m
	}
	setLayerTransform(layer, m, geom)
}

// PointerMatrix returns the transform currently drawn by layer, around its top left. Presentation values include style transitions and additive keyframes, which a view's layout coordinates do not know.
func PointerMatrix(layer objc.ID) driver.Matrix3D {
	if layer == 0 {
		return driver.Identity3D
	}
	shown := layer.Send(sel("presentationLayer"))
	if shown == 0 {
		shown = layer
	}
	t := objc.Send[transform3D](shown, sel("transform"))
	m := driver.Matrix3D{t.M11, t.M12, t.M13, t.M14, t.M21, t.M22, t.M23, t.M24,
		t.M31, t.M32, t.M33, t.M34, t.M41, t.M42, t.M43, t.M44}
	b := objc.Send[CGRect](shown, selBounds)
	a := objc.Send[CGPoint](shown, sel("anchorPoint"))
	x, y := a.X*b.Size.Width, a.Y*b.Size.Height
	return driver.Translate3D(x, y, 0).Mul(m).Mul(driver.Translate3D(-x, -y, 0))
}

// The messages that move a layer every frame of an animation, registered
// once: objc.Send registers a function, checking a struct's fields, on
// every call.
var (
	selAnchorPoint, selSetTransform objc.SEL
	layerBounds                     func(objc.ID, objc.SEL) CGRect
	layerAnchor                     func(objc.ID, objc.SEL) CGPoint
	msgSendSetFloat32               func(objc.ID, objc.SEL, float32)
	classGetMethodImplementation    func(objc.Class, objc.SEL) uintptr
	// setTransforms are the setTransform: implementations SetTransform
	// calls, by the layer's class.
	setTransforms = map[objc.Class]setTransformImp{}
	// transformArg and transformCall hold the matrix and syscall arguments
	// setLayerTransform passes on the main thread, avoiding a per-frame slice
	// allocation from purego.SyscallN's uintptrescapes variadic parameter.
	transformArg  transform3D
	transformCall [3]uintptr
	// layerBoundsReads and layerAnchorReads count Core Animation struct reads
	// made when cached geometry is absent, for tests verifying the fast path.
	layerBoundsReads uint64
	layerAnchorReads uint64
)

// setTransformImp is a class's setTransform: implementation: its address,
// and the function that calls it with reflection where it cannot be
// called through purego.SyscallN.
type setTransformImp struct {
	imp uintptr
	fn  func(objc.ID, objc.SEL, transform3D)
}

func bindTransforms() {
	if layerBounds != nil {
		return
	}
	lib := load("/usr/lib/libobjc.A.dylib")
	selAnchorPoint, selSetTransform = sel("anchorPoint"), sel("setTransform:")
	// A CGRect comes back through memory on x86-64 (objc_msgSend_stret)
	// and in registers on arm64.
	rect := "objc_msgSend"
	if runtime.GOARCH == "amd64" {
		rect = "objc_msgSend_stret"
	}
	purego.RegisterLibFunc(&layerBounds, lib, rect)
	purego.RegisterLibFunc(&layerAnchor, lib, "objc_msgSend")
	purego.RegisterLibFunc(&msgSendSetFloat32, lib, "objc_msgSend")
	purego.RegisterLibFunc(&classGetMethodImplementation, lib, "class_getMethodImplementation")
}

// setLayerOpacity sets layer's opacity through a typed registration so animated
// fades do not box float32 into ID.Send's variadic interface slice.
func setLayerOpacity(layer objc.ID, opacity float32) {
	bindTransforms()
	msgSendSetFloat32(layer, selSetOpacity, opacity)
}

// setLayerTransform sets layer's transform to m. On a layer of an NSGo
// class it calls the superclass's setTransform: past HeldTransformMethods'
// override, which only passes what SetTransform sets on: the message, the
// override's call back into Go and its message to the superclass each
// passed the 128-byte matrix through reflection, most of what animating a
// layer cost.
func setLayerTransform(layer objc.ID, m transform3D, geom *TransformGeometry) {
	var imp setTransformImp
	if geom != nil && geom.impLayer == layer {
		imp = geom.imp
	} else {
		bindTransforms()
		c := ClassOf(layer)
		var ok bool
		imp, ok = setTransforms[c]
		if !ok {
			k := ownClass(layer)
			if ownClasses[k] {
				k = k.SuperClass()
			} else {
				k = c
			}
			imp.imp = classGetMethodImplementation(k, selSetTransform)
			if runtime.GOARCH != "arm64" {
				purego.RegisterFunc(&imp.fn, imp.imp)
			}
			setTransforms[c] = imp
		}
		if geom != nil {
			geom.imp = imp
			geom.impLayer = layer
		}
	}
	if imp.fn != nil {
		imp.fn(layer, selSetTransform, m)
		return
	}
	// arm64 passes a struct of over 16 bytes that is no homogeneous
	// aggregate of four floats or fewer as the address of a copy.
	transformArg = m
	transformCall[0] = uintptr(layer)
	transformCall[1] = uintptr(selSetTransform)
	transformCall[2] = uintptr(unsafe.Pointer(&transformArg))
	purego.SyscallN(imp.imp, transformCall[:]...)
}

// move is a transform transition on a layer, from one style to another
// over times of the media clock.
type move struct {
	from, to        driver.ViewStyle
	begin, duration float64
	curve           driver.Curve
}

// moves are the layers' latest transform transitions.
var moves = map[objc.ID]*move{}

// moveAnimation returns the animation that moves layer's transform from
// what it shows to vs's and remembers it, or 0 if it cannot tell: the two
// differ in matrices of the app's, or nothing was drawn yet. It steps
// through the styles between, as Windows and Linux do. Between two
// matrices Core Animation moves the anchor point in a straight line, so
// a turn around any other point, as all of AppKit's layers make, would
// swing off that point on the way.
func moveAnimation(layer objc.ID, was, vs *driver.ViewStyle, geom *TransformGeometry) objc.ID {
	if was == nil {
		delete(moves, layer)
		return 0
	}
	from, now := *was, caCurrentMediaTime()
	if m := moves[layer]; m != nil && now < m.begin+m.duration {
		from = driver.MixStyles(m.from, m.to, m.curve.At((now-m.begin)/m.duration), driver.TransitionTransform)
	}
	if from.Transform.Then != vs.Transform.Then {
		delete(moves, layer)
		return 0
	}
	tr := vs.Transition
	moves[layer] = &move{from: from, to: *vs, begin: now + tr.Delay.Seconds(), duration: tr.Duration.Seconds(), curve: tr.Easing}
	values := objc.ID(Class("NSMutableArray")).Send(sel("array"))
	size, anchor := resolveTransformGeometry(layer, geom)
	_, radius := transformForGeometry(size, anchor, vs.Transform, vs.Origin, float64(vs.OriginZ))
	n := max(int(math.Ceil(turned(from.Transform, vs.Transform)/turnStep(radius))), 1)
	for i := range n + 1 {
		s := driver.MixStyles(from, *vs, float64(i)/float64(n), driver.TransitionTransform)
		m, _ := transformForGeometry(size, anchor, s.Transform, s.Origin, float64(vs.OriginZ))
		values.Send(selAddObject, objc.ID(Class("NSValue")).Send(sel("valueWithCATransform3D:"), m))
	}
	anim := objc.ID(Class("CAKeyframeAnimation")).Send(selAnimationWithPath, NSString("transform"))
	anim.Send(sel("setValues:"), values)
	return anim
}

// turned returns the degrees t turns or slants through on the way to u.
func turned(t, u driver.Transform) float64 {
	var sum float64
	for _, d := range []float32{u.Rotate - t.Rotate, u.RotateX - t.RotateX, u.RotateY - t.RotateY, u.RotateZ - t.RotateZ,
		u.SkewX - t.SkewX, u.SkewY - t.SkewY} {
		sum += math.Abs(float64(d))
	}
	return sum
}

var (
	// held are the transforms SetTransform gave layers.
	held = map[objc.ID]transform3D{}
)

// HoldTransforms, when a backend sets it, gives layer the class of
// HeldTransformMethods as SetTransform first holds a transform for it.
// Only layers that hold one take that class: AppKit sets each layer's
// transform as it places its view, and the override's call into Go cost
// every frame change a few microseconds.
var HoldTransforms func(layer objc.ID)

// HeldTransformMethods are the methods of a CALayer subclass that keeps
// the transform SetTransform gave it: AppKit sets its views' layers back
// to its own transform whenever it syncs their geometry.
func HeldTransformMethods() []objc.MethodDef {
	return []objc.MethodDef{
		{Cmd: sel("setTransform:"), Fn: func(self objc.ID, cmd objc.SEL, m transform3D) {
			if h, ok := held[self]; ok {
				m = h
			}
			Super(self, cmd, m)
		}},
		{Cmd: sel("setAffineTransform:"), Fn: func(self objc.ID, cmd objc.SEL, m affine) {
			if h, ok := held[self]; ok {
				Super(self, sel("setTransform:"), h)
				return
			}
			Super(self, cmd, m)
		}},
	}
}

// ForgetTransform forgets the transform SetTransform gave a layer that is
// going away.
func ForgetTransform(layer objc.ID) {
	delete(held, layer)
	delete(moves, layer)
}

// SetSublayerTranslation moves layer's sublayers by (dx, dy) points, as a
// scroll view shows an offset its bounds do not have yet.
func SetSublayerTranslation(layer objc.ID, dx, dy float64) {
	layer.Send(sel("setSublayerTransform:"), transform3D{M11: 1, M22: 1, M33: 1, M44: 1, M41: dx, M42: dy})
}

// RotateLayer turns layer by radians around its anchor point.
func RotateLayer(layer objc.ID, radians float64) {
	c, s := math.Cos(radians), math.Sin(radians)
	layer.Send(sel("setTransform:"), transform3D{M11: c, M12: s, M21: -s, M22: c, M33: 1, M44: 1})
}

// MediaTime returns Core Animation's clock: the time since the system
// started, on which display links and NSEvent timestamps count too.
func MediaTime() time.Duration { return time.Duration(caCurrentMediaTime() * float64(time.Second)) }

// CAFrameRateRange is the display link's range, in Hz.
type CAFrameRateRange struct {
	Minimum, Maximum, Preferred float32
}

// Clamped limits the request to rates the screen can show.
func (r CAFrameRateRange) Clamped(screen float32) CAFrameRateRange {
	screen = max(screen, 1)
	return CAFrameRateRange{Minimum: min(max(1, r.Minimum), screen), Maximum: min(max(1, r.Maximum), screen),
		Preferred: min(max(1, r.Preferred), screen)}
}

// MsgFloat64 sends a zero-argument Objective-C message returning float64
// through a function registered once instead of objc.Send's per-call
// registration.
func MsgFloat64(id objc.ID, sel objc.SEL) float64 {
	return msgSendFloat64(id, sel)
}

// SetAnimationFrameRate hints Core Animation to drive anim at the display's
// high refresh rate (e.g. 80-120 Hz on ProMotion displays) instead of falling
// back to 60 Hz or lower during deceleration.
func SetAnimationFrameRate(anim objc.ID, maxFPS int) {
	if anim == 0 || !objc.Send[bool](anim, sel("respondsToSelector:"), selSetPreferredFrameRateRange) {
		return
	}
	if maxFPS <= 0 {
		maxFPS = 120
	}
	top := float32(maxFPS)
	low := min(float32(80), top)
	anim.Send(selSetPreferredFrameRateRange, CAFrameRateRange{
		Minimum:   low,
		Maximum:   top,
		Preferred: top,
	})
}

// LinkFrameTime returns when the frame of a display link (a CADisplayLink, or the one an NSView makes) will show, its targetTimestamp, on time.Now's base: a frame's time for engine.Engine.FrameAt, from within the link's callback.
func LinkFrameTime(link objc.ID) time.Time {
	target := time.Duration(msgSendFloat64(link, selTargetTimestamp) * float64(time.Second))
	return driver.DisplayTime(target, MediaTime())
}
