//go:build ios && !tvos && nsgotest

package ios

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// TestDirectionMirrorsUIKitControls verifies in a real UIWindow on the iOS
// Simulator that ItemDirection mirrors UIKit views (UITextField, UITextView,
// UISwitch, UITabBar, UINavigationBar, UIAlertController), input text alignment
// and writing direction, UITabBar button layout order, navigation edge-swipe
// direction, and editor swaps while keeping an embedded surface isolated from
// UIView's appearance proxy (#1615).
func TestDirectionMirrorsUIKitControls(t *testing.T) {
	onMainSync(func() {
		if classView == 0 {
			loadUIKit()
		}
		appearance := objc.ID(apple.Class("UIView")).Send(sel("appearance"))
		origAppearance := objc.Send[int](appearance, sel("semanticContentAttribute"))
		defer appearance.Send(selSetSemantic, origAppearance)

		win := objc.ID(apple.Class("UIWindow")).Send(selAlloc).Send(selInitWithFrame, apple.Rect(0, 0, 390, 844))
		defer win.Send(selRelease)
		rootVC := objc.ID(apple.Class("UIViewController")).Send(selNew)
		defer rootVC.Send(selRelease)
		win.Send(sel("setRootViewController:"), rootVC)
		win.Send(sel("makeKeyAndVisible"))

		// The switch, the tab bar and the navigation bar report to the engine as they are made and laid out, so the surfaces need a running one: an Engine's zero value has no runner to dispatch to.
		eng := engine.Start(idlePlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{})
		defer eng.Close()
		s := &surface{
			native:   objc.ID(classView).Send(selAlloc).Send(selInitWithFrame, apple.Rect(0, 0, 390, 844)),
			views:    make([]*view, 10),
			engine:   eng,
			animator: driver.NewAnimator(func(driver.Event) {}),
			slides:   map[*view]*slide{},
		}
		defer s.native.Send(selRelease)
		rootVC.Send(sel("view")).Send(selAddSubview, s.native)

		// Build a tree: root (1) -> single-line input (2), multiline input (3),
		// UISwitch (4), UITabBar (5), stack (6) -> screen with UINavigationBar (7).
		root := s.create(driver.KindView, 1)
		s.views[1] = root
		root.native.Send(selSetFrame, apple.Rect(0, 0, 390, 844))
		s.native.Send(selAddSubview, root.native)
		s.root = 1

		single := s.create(driver.KindInput, 2)
		s.views[2] = single
		single.native.Send(selSetFrame, apple.Rect(16, 120, 358, 44))
		root.native.Send(selAddSubview, single.native)

		multi := s.create(driver.KindInput, 3)
		s.views[3] = multi
		multi.native.Send(selSetFrame, apple.Rect(16, 180, 358, 80))
		root.native.Send(selAddSubview, multi.native)

		swView := s.create(driver.KindView, 4)
		s.views[4] = swView
		swView.native.Send(selSetFrame, apple.Rect(16, 280, 51, 31))
		root.native.Send(selAddSubview, swView.native)
		s.configureNative(swView, &driver.NativeConfig{
			Component: driver.SwitchComponent,
			Props:     map[string]any{"checked": true},
		})

		tabView := s.create(driver.KindView, 5)
		s.views[5] = tabView
		tabView.native.Send(selSetFrame, apple.Rect(0, 760, 390, 49))
		root.native.Send(selAddSubview, tabView.native)
		s.configureNative(tabView, &driver.NativeConfig{
			Component: driver.TabBarComponent,
			Props: map[string]any{
				"items": []any{
					map[string]any{"title": "Home", "icon": "house"},
					map[string]any{"title": "Search", "icon": "magnifyingglass"},
				},
				"selectedIndex": float64(0),
			},
		})

		stack := s.create(driver.KindView, 6)
		s.views[6] = stack
		stack.native.Send(selSetFrame, apple.Rect(0, 0, 390, 100))
		root.native.Send(selAddSubview, stack.native)

		screen := s.create(driver.KindView, 7)
		s.views[7] = screen
		screen.native.Send(selSetFrame, apple.Rect(0, 0, 390, 100))
		stack.native.Send(selAddSubview, screen.native)
		s.configureScreen(screen, &driver.ScreenConfig{
			Title:        "Details",
			BackTitle:    "Back",
			CanGoBack:    true,
			HeaderHeight: 44,
		})

		defer func() {
			for _, v := range []*view{screen, stack, tabView, swView, multi, single, root} {
				if v != nil {
					s.delete(v, false)
				}
			}
		}()

		configureInputs := func(rtl bool) {
			single.configure(&driver.InputConfig{
				Placeholder: "Name",
				Style:       driver.TextStyle{Align: driver.TextAlignStart, RTL: rtl, Font: driver.Font{Size: 16}, Color: driver.RGB(0x111111)},
				Content:     driver.Bounds{Size: driver.Size{Width: 358, Height: 44}},
			})
			multi.configure(&driver.InputConfig{
				Multiline:   true,
				Placeholder: "Notes",
				Style:       driver.TextStyle{Align: driver.TextAlignStart, RTL: rtl, Font: driver.Font{Size: 16}, Color: driver.RGB(0x111111)},
				Content:     driver.Bounds{Size: driver.Size{Width: 358, Height: 80}},
			})
		}

		checkTree := func(v objc.ID, wantSem, wantEff int) error {
			var walk func(objc.ID) error
			walk = func(node objc.ID) error {
				if got := objc.Send[int](node, sel("semanticContentAttribute")); got != wantSem {
					return fmt.Errorf("view %s semanticContentAttribute = %d, want %d",
						apple.GoString(node.Send(sel("description"))), got, wantSem)
				}
				if got := objc.Send[int](node, sel("effectiveUserInterfaceLayoutDirection")); got != wantEff {
					return fmt.Errorf("view %s effectiveUserInterfaceLayoutDirection = %d, want %d",
						apple.GoString(node.Send(sel("description"))), got, wantEff)
				}
				subs := node.Send(sel("subviews"))
				n := objc.Send[uint](subs, sel("count"))
				for i := uint(0); i < n; i++ {
					if err := walk(subs.Send(sel("objectAtIndex:"), i)); err != nil {
						return err
					}
				}
				return nil
			}
			return walk(v)
		}

		// tabButtonXs returns the laid-out origin.x of each UITabBarButton subview in tabBar.
		tabButtonXs := func(bar objc.ID) []float64 {
			bar.Send(sel("setNeedsLayout"))
			bar.Send(sel("layoutIfNeeded"))
			var xs []float64
			subs := bar.Send(sel("subviews"))
			n := objc.Send[uint](subs, sel("count"))
			for i := uint(0); i < n; i++ {
				sub := subs.Send(sel("objectAtIndex:"), i)
				cls := apple.GoString(sub.Send(sel("class")).Send(sel("description")))
				if strings.Contains(cls, "TabBarButton") {
					xs = append(xs, objc.Send[apple.CGRect](sub, sel("frame")).Origin.X)
				}
			}
			return xs
		}

		verifyPhase := func(rtl bool) {
			t.Helper()
			wantSem, wantEff := semanticLTR, 0
			wantAlign, wantWriting, wantEdge := uiAlignLeft, writingLTR, uint(2) // UIRectEdgeLeft
			if rtl {
				wantSem, wantEff = semanticRTL, 1
				wantAlign, wantWriting, wantEdge = uiAlignRight, writingRTL, uint(8) // UIRectEdgeRight
			}
			if got := objc.Send[int](appearance, sel("semanticContentAttribute")); got != wantSem {
				t.Errorf("UIView.appearance semanticContentAttribute = %d, want %d (rtl=%v)", got, wantSem, rtl)
			}
			if err := checkTree(s.native, wantSem, wantEff); err != nil {
				t.Errorf("rtl=%v: %v", rtl, err)
			}
			if got := objc.Send[int](single.input.field, sel("textAlignment")); got != wantAlign {
				t.Errorf("UITextField textAlignment = %d, want %d (rtl=%v)", got, wantAlign, rtl)
			}
			if got := objc.Send[int](multi.input.field, sel("textAlignment")); got != wantAlign {
				t.Errorf("UITextView textAlignment = %d, want %d (rtl=%v)", got, wantAlign, rtl)
			}
			if got := objc.Send[int](multi.input.label, sel("textAlignment")); got != wantAlign {
				t.Errorf("UITextView placeholder textAlignment = %d, want %d (rtl=%v)", got, wantAlign, rtl)
			}
			if a := textAttrs(); a.paragraph != 0 {
				para := multi.input.field.Send(sel("typingAttributes")).Send(sel("objectForKey:"), a.paragraph)
				if para == 0 {
					t.Errorf("UITextView typingAttributes missing paragraph style (rtl=%v)", rtl)
				} else if got := objc.Send[int](para, sel("baseWritingDirection")); got != wantWriting {
					t.Errorf("UITextView baseWritingDirection = %d, want %d (rtl=%v)", got, wantWriting, rtl)
				}
			}
			if screen.bar == nil || screen.bar.edge == 0 {
				t.Fatalf("screen missing swipe-back gesture")
			}
			if got := objc.Send[uint](screen.bar.edge, sel("edges")); got != wantEdge {
				t.Errorf("swipe-back edges = %d, want %d (rtl=%v)", got, wantEdge, rtl)
			}
			sl := &slide{stack: stack}
			if sw := s.slideWidth(sl); rtl && sw >= 0 || !rtl && sw <= 0 {
				t.Errorf("slideWidth = %v, want sign matching rtl=%v", sw, rtl)
			}
			if xs := tabButtonXs(tabView.componentView); len(xs) >= 2 {
				if rtl && !(xs[0] > xs[1]) {
					t.Errorf("RTL UITabBar button Xs = %v, want item 0 right of item 1", xs)
				}
				if !rtl && !(xs[0] < xs[1]) {
					t.Errorf("LTR UITabBar button Xs = %v, want item 0 left of item 1", xs)
				}
			}
			alert := objc.ID(apple.Class("UIAlertController")).Send(
				sel("alertControllerWithTitle:message:preferredStyle:"),
				apple.NSString("Title"), apple.NSString("Body"), 1)
			s.directTree(alert.Send(sel("view")))
			if got := objc.Send[int](alert.Send(sel("view")), sel("semanticContentAttribute")); got != wantSem {
				t.Errorf("UIAlertController view semanticContentAttribute = %d, want %d (rtl=%v)", got, wantSem, rtl)
			}
		}

		// Phase 1: LTR.
		s.rtl, s.directed = false, true
		s.applyDirection()
		configureInputs(false)
		verifyPhase(false)

		// Phase 2: Switch dynamically to RTL.
		s.rtl, s.directed = true, true
		s.applyDirection()
		configureInputs(true)
		verifyPhase(true)

		// Phase 3: Swap single-line input to multiline while RTL and check newly created UITextView.
		single.configure(&driver.InputConfig{
			Multiline:   true,
			Placeholder: "Swapped",
			Style:       driver.TextStyle{Align: driver.TextAlignStart, RTL: true, Font: driver.Font{Size: 16}, Color: driver.RGB(0x111111)},
			Content:     driver.Bounds{Size: driver.Size{Width: 358, Height: 60}},
		})
		if err := checkTree(single.native, semanticRTL, 1); err != nil {
			t.Errorf("swapped multiline in RTL: %v", err)
		}
		if got := objc.Send[int](single.input.field, sel("textAlignment")); got != uiAlignRight {
			t.Errorf("swapped multiline alignment = %d, want %d", got, uiAlignRight)
		}

		// Phase 4: Switch dynamically back to LTR.
		s.rtl, s.directed = false, true
		s.applyDirection()
		configureInputs(false)
		verifyPhase(false)

		// Phase 5: Embedded surface mirrors its own tree without changing UIView.appearance.
		embedded := &surface{
			embedded: true,
			native:   objc.ID(classView).Send(selAlloc).Send(selInitWithFrame, apple.Rect(0, 0, 200, 100)),
			views:    make([]*view, 4),
			engine:   eng,
			animator: driver.NewAnimator(func(driver.Event) {}),
			slides:   map[*view]*slide{},
		}
		defer embedded.native.Send(selRelease)
		rootVC.Send(sel("view")).Send(selAddSubview, embedded.native)
		ev := embedded.create(driver.KindInput, 1)
		embedded.views[1] = ev
		embedded.native.Send(selAddSubview, ev.native)
		defer embedded.delete(ev, false)

		embedded.rtl, embedded.directed = true, true
		embedded.applyDirection()
		if err := checkTree(embedded.native, semanticRTL, 1); err != nil {
			t.Errorf("embedded RTL surface: %v", err)
		}
		if got := objc.Send[int](appearance, sel("semanticContentAttribute")); got != semanticLTR {
			t.Errorf("embedded RTL changed UIView.appearance to %d, want %d", got, semanticLTR)
		}
	})
}

// idlePlatform is a platform that shows nothing, for an engine that only takes a test surface's events.
type idlePlatform struct{ apple.TextEngine }

func (idlePlatform) Mount(*driver.MountBatch)  {}
func (idlePlatform) RequestFrame()             {}
func (idlePlatform) SubmitGPU(driver.GPUFrame) {}
