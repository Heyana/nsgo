//go:build ios

package ios

import (
	"structs"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/apple"
)

// Screens, sheets and the status bar, after NSH's navigation.rs. The
// stack stays the core's: a screen with a system header gets a
// UINavigationBar of its own over its top, attached to the status bar
// above it, holding a back item for the screen below, so UIKit draws the
// back button and its title. The bar's back button and a swipe from the
// leading edge report EventBack with the screen's ID. Sheets are
// presented view controllers holding the modal's view; they report their
// size, and their swipes as close requests.

// navBar is a screen's system header.
type navBar struct {
	bar, item, back objc.ID // UINavigationBar and its items
	config          driver.ScreenConfig
	edge            objc.ID // the swipe back's UIScreenEdgePanGestureRecognizer
	below           *view   // the screen a swipe back uncovers
	swipe           float64 // how far the swipe moved the screen
	itemsSet        bool
	// collapsed: the large title went into the bar as the content
	// scrolled under it.
	collapsed bool
	// fit is the bar's height as UIKit sizes it, reported the one last
	// told to the engine.
	fit, reported driver.Points
}

// sheet is a modal presented as a UIKit sheet.
type sheet struct {
	controller objc.ID
	gone       bool // the user swiped it away
	size       driver.Size
	insets     driver.Edges
}

// affineTransform is CGAffineTransform.
type affineTransform struct {
	_                structs.HostLayout
	A, B, C, D, X, Y float64
}

var identity = affineTransform{A: 1, D: 1}

var (
	classNavDelegate, classSheet objc.Class
	navDelegate                  objc.ID // one delegate for every bar
	bars                         = map[objc.ID]*view{}
	sheets                       = map[objc.ID]*view{}
	selEdgeSwipe                 = sel("nsgoEdgeSwipe:")
)

// UIInterfaceOrientationMask of each core orientation.
var orientationMasks = map[driver.Orientation]uint{
	driver.OrientationPortrait: 1 << 1, driver.OrientationPortraitUpsideDown: 1 << 2,
	driver.OrientationLandscapeLeft: 1 << 4, driver.OrientationLandscapeRight: 1 << 3,
}

func registerNav() {
	classNavDelegate = register("NsgoNavDelegate", objc.GetClass("NSObject"),
		[]*objc.Protocol{objc.GetProtocol("UINavigationBarDelegate")}, []objc.MethodDef{
			// The bar's background reaches up under the status bar where
			// the screen does.
			{Cmd: sel("positionForBar:"), Fn: func(_ objc.ID, _ objc.SEL, bar objc.ID) int {
				const top, topAttached = 2, 3
				if v := bars[bar]; v != nil && v.surface.barTop(v) > 0 {
					return topAttached
				}
				return top
			}},
			{Cmd: sel("navigationBar:shouldPopItem:"), Fn: func(_ objc.ID, _ objc.SEL, bar, _ objc.ID) bool {
				if v := bars[bar]; v != nil {
					v.surface.engine.Dispatch(driver.Event{Kind: driver.EventBack, ID: v.id})
				}
				// The core pops the screen, and the bar goes with it.
				return false
			}},
		})
	navDelegate = objc.ID(classNavDelegate).Send(selNew)
	classSheet = register("NsgoSheetController", objc.GetClass("UIViewController"),
		[]*objc.Protocol{objc.GetProtocol("UIAdaptivePresentationControllerDelegate")}, []objc.MethodDef{
			{Cmd: sel("viewDidLayoutSubviews"), Fn: func(self objc.ID, cmd objc.SEL) {
				apple.Super(self, cmd)
				if v := sheets[self]; v != nil {
					v.surface.sheetLaidOut(v)
				}
			}},
			{Cmd: sel("supportedInterfaceOrientations"), Fn: func(self objc.ID, cmd objc.SEL) uint {
				if m := app.orientations(); m != 0 {
					return m
				}
				return apple.SendSuper[uint](self, cmd)
			}},
			{Cmd: sel("preferredStatusBarStyle"), Fn: func(objc.ID, objc.SEL) int { return app.statusBar }},
			{Cmd: sel("prefersStatusBarHidden"), Fn: func(objc.ID, objc.SEL) bool { return app.hideBar }},
			{Cmd: sel("preferredStatusBarUpdateAnimation"), Fn: func(objc.ID, objc.SEL) int { return app.barAnimation() }},
			{Cmd: sel("presentationControllerDidDismiss:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
				if v := sheets[self]; v != nil && v.sheet != nil {
					v.sheet.gone = true
					s := v.surface
					s.engine.Dispatch(driver.Event{Kind: driver.EventRequestClose, ID: v.id, Text: "swipe", Finished: true})
					s.updateOrientations()
				}
			}},
			{Cmd: sel("presentationControllerDidAttemptToDismiss:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
				if v := sheets[self]; v != nil {
					v.surface.engine.Dispatch(driver.Event{Kind: driver.EventRequestClose, ID: v.id, Text: "swipe"})
				}
			}},
		})
}

// navMethods are the views' methods for a screen's swipe back.
func navMethods() []objc.MethodDef {
	return []objc.MethodDef{{Cmd: selEdgeSwipe, Fn: edgeSwiped}}
}

// headerHeights measures a navigation bar on this device, as NSH does:
// its height, and with a large title the 52-point row large titles add.
func headerHeights() [2]driver.Points {
	bar := objc.ID(objc.GetClass("UINavigationBar")).Send(selNew)
	h := objc.Send[apple.CGSize](bar, sel("sizeThatFits:"), apple.CGSize{Width: 390}).Height
	bar.Send(selRelease)
	if h <= 0 {
		h = 44
	}
	return [2]driver.Points{driver.Points(h), driver.Points(h + 52)}
}

// configureScreen applies screen v's header.
func (s *surface) configureScreen(v *view, c *driver.ScreenConfig) {
	b := v.bar
	if b == nil {
		b = &navBar{}
		b.bar = objc.ID(objc.GetClass("UINavigationBar")).Send(selNew)
		b.bar.Send(selSetDeleg, navDelegate)
		const flexibleWidth = 2
		b.bar.Send(sel("setAutoresizingMask:"), flexibleWidth)
		if objc.Send[bool](b.bar, sel("respondsToSelector:"), sel("setPreferredBehavioralStyle:")) {
			const pad = 2
			b.bar.Send(sel("setPreferredBehavioralStyle:"), pad)
		}
		b.item = objc.ID(objc.GetClass("UINavigationItem")).Send(selNew)
		b.back = objc.ID(objc.GetClass("UINavigationItem")).Send(selNew)
		v.native.Send(selAddSubview, b.bar)
		bars[b.bar] = v
		v.bar = b
	}
	old := b.config
	b.config = *c
	b.bar.Send(selSetHidden, c.HeaderHidden)
	b.item.Send(sel("setTitle:"), apple.NSString(c.Title))
	b.back.Send(sel("setTitle:"), apple.NSString(c.BackTitle))
	b.item.Send(sel("setHidesBackButton:"), c.BackHidden)
	b.item.Send(sel("setLeftItemsSupplementBackButton:"), true)
	if !c.LargeTitle {
		b.collapsed = false
	}
	s.showLargeTitle(b)
	b.item.Send(sel("setLeftBarButtonItem:"), s.barItem(c.Left))
	b.item.Send(sel("setRightBarButtonItem:"), s.barItem(c.Right))
	b.item.Send(sel("setTitleView:"), s.headerWrap(c.TitleView))
	if !b.itemsSet || old.CanGoBack != c.CanGoBack {
		b.itemsSet = true
		items := []objc.ID{b.item}
		if c.CanGoBack {
			items = []objc.ID{b.back, b.item}
		}
		array := objc.ID(objc.GetClass("NSMutableArray")).Send(selNew)
		for _, it := range items {
			array.Send(sel("addObject:"), it)
		}
		b.bar.Send(sel("setItems:animated:"), array, false)
		array.Send(selRelease)
	}
	s.styleBar(b, c)
	s.layoutBar(v)
	v.native.Send(sel("bringSubviewToFront:"), b.bar)
	s.setSwipeBack(v)
}

// styleBar sets the bar's appearance from the header's colors, UIKit's
// own where they are unset, as NSH's header_appearance.
func (s *surface) styleBar(b *navBar, c *driver.ScreenConfig) {
	tint := objc.ID(0)
	if c.TintColor != 0 {
		tint = uiColor(c.TintColor)
	}
	b.bar.Send(sel("setTintColor:"), tint)
	a := objc.ID(objc.GetClass("UINavigationBarAppearance")).Send(selNew)
	switch {
	case c.HeaderTransparent:
		a.Send(sel("configureWithTransparentBackground"))
	case c.Background != 0:
		a.Send(sel("configureWithOpaqueBackground"))
		a.Send(sel("setBackgroundColor:"), uiColor(c.Background))
	default:
		a.Send(sel("configureWithDefaultBackground"))
	}
	if c.TitleColor != 0 {
		key := apple.Global(uikitPath, "NSForegroundColorAttributeName")
		attrs := objc.ID(objc.GetClass("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"), uiColor(c.TitleColor), key)
		a.Send(sel("setTitleTextAttributes:"), attrs)
		a.Send(sel("setLargeTitleTextAttributes:"), attrs)
	}
	if c.TintColor != 0 {
		// The back button's title takes the tint only through the
		// appearance's button styles.
		key := apple.Global(uikitPath, "NSForegroundColorAttributeName")
		attrs := objc.ID(objc.GetClass("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"), tint, key)
		for _, part := range []string{"buttonAppearance", "backButtonAppearance"} {
			a.Send(sel(part)).Send(sel("normal")).Send(sel("setTitleTextAttributes:"), attrs)
		}
	}
	b.bar.Send(sel("setStandardAppearance:"), a)
	b.bar.Send(sel("setScrollEdgeAppearance:"), a)
	b.bar.Send(sel("setCompactAppearance:"), a)
	a.Send(selRelease)
}

// showLargeTitle sets whether bar b shows its large title.
func (s *surface) showLargeTitle(b *navBar) {
	const always, never = 1, 2
	large := b.config.LargeTitle && !b.collapsed
	mode := never
	if large {
		mode = always
	}
	b.item.Send(sel("setLargeTitleDisplayMode:"), mode)
	b.bar.Send(sel("setPrefersLargeTitles:"), large)
}

// layoutBar places screen v's bar at the bottom of its header area: the
// part above it is the status bar, which the bar's background reaches
// under. It tells the engine the bar's height as UIKit sizes it, with
// its items and title view.
func (s *surface) layoutBar(v *view) {
	b := v.bar
	w := objc.Send[apple.CGRect](v.native, selBounds).Size.Width
	if !b.config.LargeTitle {
		if fit := objc.Send[apple.CGSize](b.bar, sel("sizeThatFits:"), apple.CGSize{Width: w}); fit.Height > 0 {
			b.fit = driver.Points(fit.Height)
		}
	}
	top := s.barTop(v)
	b.bar.Send(selSetFrame, apple.Rect(0, float64(top), w, float64(s.barHeight(v))))
	if bar := s.expandedBar(v); !b.config.HeaderHidden && bar != b.reported {
		b.reported = bar
		s.engine.Dispatch(driver.Event{Kind: driver.EventScreenHeader, ID: v.id, Value: float32(bar)})
	}
}

// expandedBar is screen v's bar's height with its large title shown.
func (s *surface) expandedBar(v *view) driver.Points {
	switch {
	case v.bar.config.LargeTitle:
		return s.headers[1]
	case v.bar.fit > 0:
		return v.bar.fit
	}
	return s.headers[0]
}

// barHeight is screen v's bar's height now, barTop how much of the
// status bar is above it.
func (s *surface) barHeight(v *view) driver.Points {
	if v.bar.collapsed {
		return s.headers[0]
	}
	return s.expandedBar(v)
}

func (s *surface) barTop(v *view) driver.Points {
	return max(v.bar.config.HeaderHeight-s.expandedBar(v), 0)
}

// followLargeTitle collapses the large title of the screen around scroll
// view sv into its bar once the content scrolled half the title's row
// under the header, and brings it back nearer the top, as UIKit's
// navigation controller does.
func (s *surface) followLargeTitle(sv *view, offset driver.Points) {
	if sv.sc == nil || sv.sc.config.HeaderInset <= 0 {
		return
	}
	var screen *view
	for n := sv.native.Send(selSuperview); n != 0; n = n.Send(selSuperview) {
		if v := views[n]; v != nil && v.bar != nil {
			screen = v
			break
		}
	}
	if screen == nil || !screen.bar.config.LargeTitle {
		return
	}
	scrolled := offset + sv.sc.config.Behavior.ContentInset.Top
	collapsed := scrolled > (s.headers[1]-s.headers[0])/2
	b := screen.bar
	if collapsed == b.collapsed {
		return
	}
	b.collapsed = collapsed
	animations := objc.NewBlock(func(objc.Block) {
		s.showLargeTitle(b)
		s.layoutBar(screen)
		b.bar.Send(sel("layoutIfNeeded"))
	})
	objc.ID(objc.GetClass("UIView")).Send(sel("animateWithDuration:animations:"), 0.25, animations)
	animations.Release()
}

// barItem returns a bar button item showing header view id, or nil.
func (s *surface) barItem(id driver.NodeID) objc.ID {
	wrap := s.headerWrap(id)
	if wrap == 0 {
		return 0
	}
	item := objc.ID(objc.GetClass("UIBarButtonItem")).Send(selAlloc).Send(sel("initWithCustomView:"), wrap)
	item.Send(sel("autorelease"))
	return item
}

// headerWrap returns the view the bar holds for header view id: a plain
// view at its size, which UIKit may move while the core keeps the header
// view at its origin.
func (s *surface) headerWrap(id driver.NodeID) objc.ID {
	if int(id) >= len(s.views) || s.views[id] == nil {
		return 0
	}
	v := s.views[id]
	if v.headerWrap == 0 {
		v.headerWrap = objc.ID(objc.GetClass("UIView")).Send(selNew)
		v.headerWrap.Send(selAddSubview, v.native)
	}
	s.sizeHeaderWrap(v)
	return v.headerWrap
}

func (s *surface) sizeHeaderWrap(v *view) {
	w := v.headerWrap
	f := objc.Send[apple.CGRect](w, sel("frame"))
	size := v.frame.Size
	w.Send(selSetFrame, apple.Rect(f.Origin.X, f.Origin.Y, float64(size.Width), float64(size.Height)))
	w.Send(sel("invalidateIntrinsicContentSize"))
	if up := w.Send(selSuperview); up != 0 {
		up.Send(selSetNeedsLayout)
	}
}

// setSwipeBack adds or removes screen v's swipe back.
func (s *surface) setSwipeBack(v *view) {
	c := &v.bar.config
	want := c.CanGoBack && !c.NoPopOnSwipe && !c.NoPopOnBack
	b := v.bar
	switch {
	case want && b.edge == 0:
		b.edge = objc.ID(objc.GetClass("UIScreenEdgePanGestureRecognizer")).Send(selAlloc).
			Send(sel("initWithTarget:action:"), v.native, selEdgeSwipe)
		v.native.Send(sel("addGestureRecognizer:"), b.edge)
		b.edge.Send(selRelease)
	case !want && b.edge != 0:
		v.native.Send(sel("removeGestureRecognizer:"), b.edge)
		b.edge = 0
	}
	if b.edge != 0 {
		const left, right = 2, 8
		edge := left
		if s.env.RTL {
			edge = right
		}
		b.edge.Send(sel("setEdges:"), edge)
	}
}

// screenBelow returns the screen under v in its stack.
func (s *surface) screenBelow(v *view) *view {
	up := v.native.Send(selSuperview)
	if up == 0 {
		return nil
	}
	var below *view
	for _, sub := range apple.Subviews(up) {
		if sub == v.native {
			return below
		}
		if o := views[sub]; o != nil && o.bar != nil {
			below = o
		}
	}
	return nil
}

// edgeSwiped follows a swipe back: the screen follows the finger over the
// one below, then either goes, reporting EventBack, or comes back.
func edgeSwiped(self objc.ID, _ objc.SEL, r objc.ID) {
	v := views[self]
	if v == nil || v.bar == nil {
		return
	}
	s, b := v.surface, v.bar
	up := self.Send(selSuperview)
	w := objc.Send[apple.CGRect](self, selBounds).Size.Width
	sign := 1.0
	if s.env.RTL {
		sign = -1
	}
	switch objc.Send[int](r, selState) {
	case stateBegan:
		b.below = s.screenBelow(v)
		if b.below != nil {
			b.below.native.Send(selSetHidden, false)
		}
		fallthrough
	case stateChanged:
		x := objc.Send[apple.CGPoint](r, sel("translationInView:"), up).X * sign
		b.swipe = min(max(x, 0), w)
		s.placeSwipe(v, b.swipe/w)
	case stateEnded:
		velocity := objc.Send[apple.CGPoint](r, sel("velocityInView:"), up).X * sign
		s.endSwipe(v, b.swipe > w/3 || velocity > 500)
	case stateCancelled, stateFailed:
		s.endSwipe(v, false)
	}
}

// placeSwipe moves screen v a fraction p of the way off, and the screen
// below from a third of the way in.
func (s *surface) placeSwipe(v *view, p float64) {
	w := objc.Send[apple.CGRect](v.native, selBounds).Size.Width
	if s.env.RTL {
		w = -w
	}
	v.native.Send(sel("setTransform:"), affineTransform{A: 1, D: 1, X: p * w})
	if below := v.bar.below; below != nil {
		below.native.Send(sel("setTransform:"), affineTransform{A: 1, D: 1, X: -(1 - p) * w / 3})
	}
}

// endSwipe finishes a swipe back: popping reports EventBack once the
// screen is off, and cancelling brings it back.
func (s *surface) endSwipe(v *view, pop bool) {
	b := v.bar
	w := objc.Send[apple.CGRect](v.native, selBounds).Size.Width
	done := 0.0
	if pop {
		done = 1
	}
	const seconds = 0.25
	remaining := seconds * max(abs(done-b.swipe/max(w, 1)), 0.2)
	animations := objc.NewBlock(func(objc.Block) { s.placeSwipe(v, done) })
	objc.ID(objc.GetClass("UIView")).Send(sel("animateWithDuration:animations:"), remaining, animations)
	animations.Release()
	below := b.below
	b.below, b.swipe = nil, 0
	time.AfterFunc(time.Duration(remaining*float64(time.Second)), func() {
		apple.OnMain(func() {
			if s.views[v.id] != v {
				return
			}
			v.native.Send(sel("setTransform:"), identity)
			if below != nil && s.views[below.id] == below {
				below.native.Send(sel("setTransform:"), identity)
				if !pop {
					below.native.Send(selSetHidden, true)
				}
			}
			if pop {
				// Until the core hides it, as it pops it.
				v.native.Send(selSetHidden, true)
				s.engine.Dispatch(driver.Event{Kind: driver.EventBack, ID: v.id})
			}
		})
	})
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// isSheet reports whether modal v is presented as a UIKit sheet.
func (v *view) isSheet() bool { return v.modal.Presentation != driver.ModalFullScreen }

// presentSheet presents modal v as a sheet over the top view controller.
func (s *surface) presentSheet(v *view) {
	c := v.modal
	ctrl := objc.ID(classSheet).Send(selNew)
	container := ctrl.Send(sel("view"))
	container.Send(sel("setBackgroundColor:"), objc.ID(objc.GetClass("UIColor")).Send(sel("systemBackgroundColor")))
	container.Send(selAddSubview, v.native)
	const pageSheet, formSheet = 1, 2
	style := pageSheet
	if c.Presentation == driver.ModalFormSheet {
		style = formSheet
	}
	ctrl.Send(sel("setModalPresentationStyle:"), style)
	if c.Animation == driver.ModalFade {
		const crossDissolve = 2
		ctrl.Send(sel("setModalTransitionStyle:"), crossDissolve)
	}
	ctrl.Send(sel("setModalInPresentation:"), c.NoCloseOnSwipe)
	ctrl.Send(sel("setModalPresentationCapturesStatusBarAppearance:"), true)
	ctrl.Send(sel("presentationController")).Send(selSetDeleg, ctrl)
	v.sheet = &sheet{controller: ctrl}
	sheets[ctrl] = v
	topController().Send(sel("presentViewController:animated:completion:"), ctrl, c.Animation != driver.ModalNone, objc.ID(0))
}

// dismissSheet takes modal v's sheet away, unless the user did.
func (s *surface) dismissSheet(v *view) {
	sh := v.sheet
	if sh == nil {
		return
	}
	v.sheet = nil
	delete(sheets, sh.controller)
	if !sh.gone {
		// Through the presenter, which dismisses what this sheet presents
		// too.
		from := sh.controller.Send(sel("presentingViewController"))
		if from == 0 {
			from = sh.controller
		}
		from.Send(sel("dismissViewControllerAnimated:completion:"), v.modal.Animation != driver.ModalNone, objc.ID(0))
	}
	sh.controller.Send(selRelease)
}

// sheetLaidOut reports the size UIKit gave sheet v and what of it the
// system covers.
func (s *surface) sheetLaidOut(v *view) {
	sh := v.sheet
	container := sh.controller.Send(sel("view"))
	size := objc.Send[apple.CGRect](container, selBounds).Size
	in := objc.Send[edgeInsets](container, sel("safeAreaInsets"))
	got := driver.Size{Width: driver.Points(size.Width), Height: driver.Points(size.Height)}
	insets := driver.Edges{Top: driver.Points(in.Top), Right: driver.Points(in.Right), Bottom: driver.Points(in.Bottom), Left: driver.Points(in.Left)}
	if got == sh.size && insets == sh.insets {
		return
	}
	sh.size, sh.insets = got, insets
	s.engine.Dispatch(driver.Event{Kind: driver.EventModalSize, ID: v.id, Size: got, Insets: insets})
}

// topController returns the view controller on top of the window's.
func topController() objc.ID {
	top := window.Send(sel("rootViewController"))
	for p := top.Send(sel("presentedViewController")); p != 0; p = top.Send(sel("presentedViewController")) {
		top = p
	}
	return top
}

// orientations is the UIInterfaceOrientationMask of the last presented
// modal that limits them, or 0.
func (s *surface) orientations() uint {
	for i := len(s.oriented) - 1; i >= 0; i-- {
		v := s.oriented[i]
		if s.views[v.id] != v || (v.sheet == nil && !v.presented) || (v.sheet != nil && v.sheet.gone) {
			continue
		}
		var m uint
		for o, bit := range orientationMasks {
			if v.modal.Orientations&o != 0 {
				m |= bit
			}
		}
		return m
	}
	return 0
}

// updateOrientations has UIKit ask the orientations again.
func (s *surface) updateOrientations() {
	live := s.oriented[:0]
	for _, v := range s.oriented {
		if s.views[v.id] == v {
			live = append(live, v)
		}
	}
	s.oriented = live
	for _, c := range []objc.ID{controller, topController()} {
		if objc.Send[bool](c, sel("respondsToSelector:"), sel("setNeedsUpdateOfSupportedInterfaceOrientations")) {
			c.Send(sel("setNeedsUpdateOfSupportedInterfaceOrientations"))
		}
	}
}

// setStatusBar applies the status bar the core merged.
func (s *surface) setStatusBar(c *driver.StatusBarConfig) {
	const lightContent, darkContent = 1, 3
	s.statusBar = 0
	switch c.Style {
	case driver.StatusBarDark:
		s.statusBar = darkContent
	case driver.StatusBarLight:
		s.statusBar = lightContent
	}
	s.hideBar, s.barAnimated = c.Hidden, c.Animated
	update := func() {
		controller.Send(sel("setNeedsStatusBarAppearanceUpdate"))
		for c := range sheets {
			c.Send(sel("setNeedsStatusBarAppearanceUpdate"))
		}
	}
	if !c.Animated {
		update()
		return
	}
	block := objc.NewBlock(func(objc.Block) { update() })
	objc.ID(objc.GetClass("UIView")).Send(sel("animateWithDuration:animations:"), 0.25, block)
	block.Release()
}

// barAnimation is the UIStatusBarAnimation of the last change.
func (s *surface) barAnimation() int {
	if s.barAnimated {
		const fade = 1
		return fade
	}
	return 0
}
