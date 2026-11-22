//go:build ios && !tvos && nsgotest

package ios

import (
	"math"
	"slices"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestTabBarRetitlesUIKitItemsInPlace(t *testing.T) {
	onMainSync(func() {
		bar := &tabBar{bar: objc.ID(apple.Class("UITabBar")).Send(selNew), selected: 1}
		defer bar.bar.Send(selRelease)
		items := func(title string) []any {
			return []any{
				map[string]any{"title": "Home", "icon": "house"},
				map[string]any{"title": title, "icon": "magnifyingglass"},
			}
		}
		bar.SetProp("items", items("Search"))
		before := slices.Clone(bar.items)
		bar.SetProp("label", "Rubriques")
		bar.SetProp("items", items("Rechercher"))
		if len(before) != 2 || !slices.Equal(before, bar.items) {
			t.Errorf("UIKit item identities changed with the title")
			return
		}
		if selected := bar.bar.Send(sel("selectedItem")); selected != before[1] {
			t.Errorf("UIKit selection changed after retitling")
		}
		if name := apple.GoString(bar.bar.Send(sel("accessibilityLabel"))); name != "Rubriques" {
			t.Errorf("UIKit tab bar accessibility label = %q", name)
		}
		if title := apple.GoString(before[1].Send(sel("title"))); title != "Rechercher" {
			t.Errorf("UIKit tab title = %q", title)
		}
	})
}

func TestTabBarVerticalEdgeSwitchesContainerAndSelection(t *testing.T) {
	onMainSync(func() {
		var sizes []driver.Size
		var changed []int
		comp := newTabBar(driver.NewNativeContext(
			func(name string, data any) {
				if name == "change" {
					if i, ok := data.(float64); ok {
						changed = append(changed, int(i))
					}
				}
			},
			func(s driver.Size) { sizes = append(sizes, s) },
		))
		bar := comp.(*tabBar)
		defer bar.Close()
		bar.SetProp("items", []any{
			map[string]any{"title": "Home", "icon": "house"},
			map[string]any{"title": "Search", "icon": "magnifyingglass"},
		})
		bar.SetProp("edge", "trailing")
		if bar.rail == 0 || len(bar.buttons) != 2 {
			t.Fatalf("vertical rail not built: rail=%v buttons=%d", bar.rail, len(bar.buttons))
		}
		if last := sizes[len(sizes)-1]; last.Width < verticalTabBarWidth || last.Height != 0 {
			t.Fatalf("vertical measured size = %+v, want width >= %v and height 0", last, verticalTabBarWidth)
		}
		if !bar.selectIndex(1) || len(changed) != 1 || changed[0] != 1 {
			t.Fatalf("selectIndex(1) emitted %v, want [1]", changed)
		}
		bar.SetProp("edge", "bottom")
		if last := sizes[len(sizes)-1]; last.Height <= 0 || last.Width != 0 {
			t.Fatalf("bottom measured size = %+v, want height > 0 and width 0", last)
		}
	})
}

func TestTabBarSizeFollowsAttachmentAndPlacement(t *testing.T) {
	onMainSync(func() {
		var sizes []driver.Size
		cx := driver.NewNativeContext(func(string, any) {}, func(s driver.Size) {
			sizes = append(sizes, s)
		})
		bar := newTabBar(cx).(*tabBar)
		defer bar.Close()

		detachedFit := objc.Send[apple.CGSize](bar.bar, sel("sizeThatFits:"), apple.CGSize{Width: defaultTabBarWidth}).Height
		if detachedFit <= 0 {
			detachedFit = defaultTabBarHeight
		}
		if len(sizes) != 1 || math.Abs(float64(sizes[0].Height)-detachedFit) > 0.01 {
			t.Fatalf("initial detached sizes = %v, want single base height %v without keyWindow safe area", sizes, detachedFit)
		}
		if sizes[0].Width != 0 {
			t.Fatalf("initial width = %v, want 0", sizes[0].Width)
		}

		// Calling layoutSubviews while detached does not emit duplicate sizes.
		bar.bar.Send(sel("layoutSubviews"))
		if len(sizes) != 1 {
			t.Fatalf("detached layoutSubviews reported extra sizes: %v", sizes)
		}

		app := objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication"))
		window := objc.Send[objc.ID](app, sel("keyWindow"))
		if window == 0 {
			return
		}
		winBounds := objc.Send[apple.CGRect](window, selBounds)
		winInsets := objc.Send[edgeInsets](window, sel("safeAreaInsets"))
		if winBounds.Size.Width <= 0 || winBounds.Size.Height <= 0 {
			return
		}

		container := objc.ID(apple.Class("UIView")).Send(selNew)
		defer func() {
			bar.bar.Send(selRemoveFromSuperview)
			container.Send(selRemoveFromSuperview)
			container.Send(selRelease)
		}()

		w := winBounds.Size.Width
		container.Send(selSetFrame, apple.Rect(0, 60, w, detachedFit))
		bar.bar.Send(selSetFrame, apple.Rect(0, 0, w, detachedFit))
		container.Send(selAddSubview, bar.bar)
		window.Send(selAddSubview, container)
		window.Send(sel("layoutIfNeeded"))

		// Repeated layout at the same top placement must not emit redundant sizes.
		countAfterTop := len(sizes)
		bar.bar.Send(sel("layoutSubviews"))
		if len(sizes) != countAfterTop {
			t.Fatalf("unchanged top layoutSubviews emitted extra sizes: %v", sizes)
		}
		fitNow := func() float64 {
			return objc.Send[apple.CGSize](bar.bar, sel("sizeThatFits:"), apple.CGSize{Width: w}).Height
		}
		topFit := fitNow()
		if got := sizes[len(sizes)-1].Height; math.Abs(float64(got)-topFit) > 0.51 {
			t.Fatalf("top placement height = %v, want UIKit's %v (sizes=%v)", got, topFit, sizes)
		}

		if winInsets.Bottom <= 0 {
			return
		}

		// Move ancestor container to the bottom of the window so the bar overlaps the bottom safe area.
		container.Send(selSetFrame, apple.Rect(0, winBounds.Size.Height-detachedFit, w, detachedFit))
		window.Send(sel("layoutIfNeeded"))
		// A classic bar's sizeThatFits: adds the bottom safe area it overlaps; iOS 26's floating bar keeps one height at every placement (83 points on an iPhone, detached too), its glass platter sitting above the home indicator either way. The reported height follows whichever UIKit answers.
		bottomFit := fitNow()
		if bottomFit > topFit+0.51 && math.Abs(bottomFit-(topFit+winInsets.Bottom)) > 0.51 {
			t.Fatalf("UIKit's bottom fit = %v, want the top fit %v plus the %v bottom safe area", bottomFit, topFit, winInsets.Bottom)
		}
		bottomHeight := sizes[len(sizes)-1].Height
		if math.Abs(float64(bottomHeight)-bottomFit) > 0.51 {
			t.Fatalf("bottom placement height = %v, want UIKit's %v (sizes=%v)", bottomHeight, bottomFit, sizes)
		}

		// Resize bar to its reported bottom height: steady state must not re-emit.
		countAfterBottom := len(sizes)
		container.Send(selSetFrame, apple.Rect(0, winBounds.Size.Height-float64(bottomHeight), w, float64(bottomHeight)))
		bar.bar.Send(selSetFrame, apple.Rect(0, 0, w, float64(bottomHeight)))
		window.Send(sel("layoutIfNeeded"))
		if len(sizes) != countAfterBottom {
			t.Fatalf("steady bottom layout emitted extra sizes: %v", sizes)
		}

		// Move ancestor back to Top: 60: height must shrink back to base height.
		container.Send(selSetFrame, apple.Rect(0, 60, w, float64(bottomHeight)))
		window.Send(sel("layoutIfNeeded"))
		restored := sizes[len(sizes)-1].Height
		if math.Abs(float64(restored)-topFit) > 0.51 {
			t.Fatalf("restored top placement height = %v, want %v (sizes=%v)", restored, topFit, sizes)
		}
	})
}

// NativeTabBarFit returns UIKit's sizeThatFits: height for the one live bottom-edge tab bar at its laid-out width, or 0 when there is none, for the placement test in package ios_test.
func NativeTabBarFit() float64 {
	var h float64
	onMainSync(func() {
		for bar, t := range tabBars {
			if t.edge != tabBarEdgeBottom {
				continue
			}
			w := objc.Send[apple.CGRect](bar, selBounds).Size.Width
			if w <= 0 {
				continue
			}
			h = objc.Send[apple.CGSize](bar, sel("sizeThatFits:"), apple.CGSize{Width: w}).Height
		}
	})
	return h
}

func TestTabBarSVGTemplateImages(t *testing.T) {
	const (
		rectDoc   = `<svg viewBox="0 0 24 24"><rect x="2" y="2" width="20" height="20" fill="currentColor"/></svg>`
		circleDoc = `<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="9" fill="currentColor"/></svg>`
	)
	draw := func(doc string) *driver.Drawing {
		svg, err := driver.ParseSVG([]byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		d := &driver.Drawing{}
		svg.Draw(d, driver.Size{Width: tabBarIconPoints, Height: tabBarIconPoints}, driver.RGB(0xffffff))
		return d
	}

	onMainSync(func() {
		bar := &tabBar{bar: objc.ID(apple.Class("UITabBar")).Send(selNew), selected: 0}
		defer bar.bar.Send(selRelease)

		items := func(title, doc string) []any {
			return []any{
				map[string]any{"title": title, "drawing": draw(doc)},
				map[string]any{"title": "Fallback", "icon": "android:ic_menu_search", "drawing": draw(doc)},
				map[string]any{"title": "Symbol", "icon": "house", "drawing": draw(doc)},
			}
		}
		bar.SetProp("items", items("Home", rectDoc))
		if len(bar.items) != 3 {
			t.Fatalf("items = %d, want 3", len(bar.items))
		}
		wantScale := bar.displayScale()
		for i := range 2 {
			img := bar.items[i].Send(sel("image"))
			if img == 0 {
				t.Fatalf("tab %d has nil image", i)
			}
			if mode := objc.Send[int](img, sel("renderingMode")); mode != uiImageRenderingModeAlwaysTemplate {
				t.Errorf("tab %d renderingMode = %d, want AlwaysTemplate (%d)", i, mode, uiImageRenderingModeAlwaysTemplate)
			}
			if scale := objc.Send[float64](img, sel("scale")); scale != wantScale {
				t.Errorf("tab %d scale = %v, want %v", i, scale, wantScale)
			}
			if sz := objc.Send[apple.CGSize](img, sel("size")); sz.Width != tabBarIconPoints || sz.Height != tabBarIconPoints {
				t.Errorf("tab %d size = %+v, want %dx%d", i, sz, tabBarIconPoints, tabBarIconPoints)
			}
			if apple.MsgBool(img, sel("isSymbolImage")) {
				t.Errorf("tab %d unexpectedly resolved as an SF Symbol", i)
			}
		}
		symImg := bar.items[2].Send(sel("image"))
		if symImg == 0 || !apple.MsgBool(symImg, sel("isSymbolImage")) {
			t.Errorf("tab 2 did not preserve SF Symbol precedence")
		}

		// Retitling with a freshly drawn identical SVG preserves UIKit item identities.
		before := slices.Clone(bar.items)
		firstImg := before[0].Send(sel("image"))
		bar.SetProp("items", items("Accueil", rectDoc))
		if !slices.Equal(before, bar.items) {
			t.Errorf("UIKit item identities changed when only the title changed")
		}
		if title := apple.GoString(bar.items[0].Send(sel("title"))); title != "Accueil" {
			t.Errorf("retitled tab 0 = %q, want Accueil", title)
		}

		// Changing the SVG replaces the UIKit items with the new template image.
		bar.SetProp("items", items("Accueil", circleDoc))
		if slices.Equal(before, bar.items) {
			t.Errorf("UIKit items were not rebuilt when the SVG changed")
		}
		if nextImg := bar.items[0].Send(sel("image")); nextImg == 0 || nextImg == firstImg {
			t.Errorf("SVG update did not produce a new template image")
		}
	})
}
