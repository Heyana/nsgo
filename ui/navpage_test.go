package ui

import (
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

// TestNavPage gives a page without a header of its own a themed one with a back button, below the status bar where the system has a header. On iOS and Android the system's transparent header stands in for it over the page, which scrolls under it. A web build that runs as iOS has no system header, so it draws NavHeader.
func TestNavPage(t *testing.T) {
	android := phoneEnv
	android.OS, android.HeaderHeight, android.LargeHeaderHeight, android.SafeArea = core.OSAndroid, 64, 152, core.Edges{Top: 47, Bottom: 34}
	simulated := phoneEnv
	simulated.Simulated = true
	for _, e := range []core.Environment{desktopEnv, android, simulated} {
		popped := false
		var nav *base.Nav[string]
		app, p := start(t, e, func(cx *core.Scope) core.Node {
			nav = base.NewNav(cx, "home", "detail")
			return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, _ func() int) base.NavPage {
				return NavPage(cx, base.NavPage{Title: "Detail " + route, TestID: "page-" + route, Children: []core.Node{
					core.View{TestID: "content-" + route, Style: core.Style{FlexGrow: 1}},
				}}, NavHeader{TestID: "header", OnBack: func() { popped = true }, BackLabel: "Back",
					Trailing: []core.Node{IconButton{Icon: lucide.Plus, Label: "Add", OnPress: func() {}}}})
			})
		})
		if e.OS == core.OSAndroid {
			if app.Exists("page-detail >> header") {
				t.Fatal("Android: the page still draws NavHeader")
			}
			c := app.Find("id:page-detail").Screen
			if c.Title != "Detail detail" || c.HeaderHidden || !c.HeaderTransparent || !c.CanGoBack || c.Right == 0 {
				t.Fatalf("Android: system header = %+v, want a transparent one titled Detail detail with a back button and the trailing views", c)
			}
			if c.Background != 0 || c.TitleColor != p.Text || c.TintColor != p.Text {
				t.Fatalf("Android: header colors = title %x tint %x on %x, want text %x on transparent (0)", c.TitleColor, c.TintColor, c.Background, p.Text)
			}
			// The bar, and the status bar past the padding of start's root.
			if want := core.Points(64 + 47 - 16); c.HeaderHeight != want {
				t.Fatalf("Android: header height = %v, want %v", c.HeaderHeight, want)
			}
			if got := app.Find("content-detail").Frame.Origin.Y; got != 0 {
				t.Fatalf("Android: content top = %v, want 0 (under the transparent header)", got)
			}
			app.Find("Detail detail")
			app.HeaderBack("id:page-detail")
			if got := nav.Routes(); len(got) != 1 {
				t.Fatalf("Android: routes after the header's back = %v, want the detail popped", got)
			}
			continue
		}
		header := app.Find("page-detail >> header")
		if header.Style.Background != p.Surface {
			t.Fatalf("%v: header = %x, want the surface", e.OS, header.Style.Background)
		}
		app.Find("Detail detail")
		app.Press("role:button Back")
		if !popped {
			t.Fatalf("%v: the back button did nothing", e.OS)
		}
	}
}

func TestNavPageKeepsCustomHeader(t *testing.T) {
	for _, os := range []core.OS{core.OSMacOS, core.OSWindows, core.OSLinux, core.OSWeb, core.OSIOS, core.OSAndroid} {
		t.Run(string(os), func(t *testing.T) {
			e := desktopEnv
			e.OS = os
			if os == core.OSIOS || os == core.OSAndroid {
				e.Idiom, e.HeaderHeight = core.IdiomPhone, 44
			}
			var pressed int
			app, _ := start(t, e, func(cx *core.Scope) core.Node {
				nav := base.NewNav(cx, "home", "detail")
				return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, _ func() int) base.NavPage {
					return NavPage(cx, base.NavPage{
						Title: "Page " + route, TestID: "page-" + route,
						Header: core.View{TestID: "custom-" + route, Style: core.Style{Width: core.Pt(160), Height: core.Pt(30)},
							OnPress: func(core.PressEvent) { pressed++ }},
						HeaderTitle: core.View{TestID: "other-title"},
						HeaderLeft:  core.View{TestID: "left-" + route},
						HeaderRight: core.View{TestID: "right-" + route},
						Children:    []core.Node{core.View{TestID: "content-" + route, Style: core.Style{FlexGrow: 1}}},
					}, NavHeader{TestID: "default-header", Title: "Ignored", Leading: []core.Node{core.View{TestID: "ignored-left"}},
						Trailing: []core.Node{core.View{TestID: "ignored-right"}}})
				})
			})
			if got := len(app.QueryMounted("id:custom-detail")); got != 1 {
				t.Fatalf("custom header mounted %d times, want once", got)
			}
			for _, id := range []string{"default-header", "other-title", "ignored-left", "ignored-right"} {
				if len(app.QueryMounted("id:"+id)) != 0 {
					t.Fatalf("custom header did not take precedence over %s", id)
				}
			}
			custom := app.Find("id:custom-detail")
			if custom.Frame.Size != (core.Size{Width: 160, Height: 30}) {
				t.Fatalf("custom header size = %v", custom.Frame.Size)
			}
			if os == core.OSIOS || os == core.OSAndroid {
				c := app.Find("id:page-detail").Screen
				if c.TitleView == 0 || custom.Parent.ID != c.TitleView || c.Left == 0 || c.Right == 0 || !c.CanGoBack || c.Title != "Page detail" {
					t.Fatalf("custom header was not retained as the native title view: %+v", c)
				}
			}
			app.Press("id:custom-detail")
			if pressed != 1 {
				t.Fatal("the custom header did not receive its press")
			}
		})
	}
}

// TestNavInset leaves room for the header only where the content runs under
// it, and the page's content starts under it there (both iOS and Android).
func TestNavInset(t *testing.T) {
	phone := phoneEnv
	phone.HeaderHeight, phone.SafeArea = 44, core.Edges{Top: 47, Bottom: 34}
	android := phone
	android.OS, android.HeaderHeight = core.OSAndroid, 64
	for _, c := range []struct {
		env   core.Environment
		inset core.Points // past the status bar, when above 0
		top   core.Points
	}{{phone, 1, 0}, {android, 1, 0}, {desktopEnv, 0, 44}} {
		var inset core.Points
		app, _ := start(t, c.env, func(cx *core.Scope) core.Node {
			nav := base.NewNav(cx, "home")
			return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, _ string, _ func() int) base.NavPage {
				return NavPage(cx, base.NavPage{Title: "Home", Children: []core.Node{core.Func(func(cx *core.Scope) core.Node {
					return core.View{TestID: "content", Style: core.Style{FlexGrow: 1}, StyleFunc: func() core.Style {
						inset = NavInset(cx)
						return core.Style{}
					}}
				})}}, NavHeader{})
			})
		})
		if (inset > 47) != (c.inset > 0) {
			t.Fatalf("%v: inset = %v, want %v", c.env.OS, inset, c.inset)
		}
		if got := app.Find("content").Frame.Origin.Y; got != c.top {
			t.Fatalf("%v: content top = %v, want %v", c.env.OS, got, c.top)
		}
	}
}

// TestNavPageScrollUnderTransparentHeader verifies that both iOS and Android
// lay a NavPage's scroll view under the transparent header, inset its initial
// content below the header, and attach a top progressive-blur edge effect
// covering the header height.
func TestNavPageScrollUnderTransparentHeader(t *testing.T) {
	iosEnv := phoneEnv
	iosEnv.OS, iosEnv.HeaderHeight, iosEnv.SafeArea = core.OSIOS, 44, core.Edges{Top: 47, Bottom: 34}
	androidEnv := phoneEnv
	androidEnv.OS, androidEnv.HeaderHeight, androidEnv.LargeHeaderHeight, androidEnv.SafeArea = core.OSAndroid, 64, 152, core.Edges{Top: 47, Bottom: 34}

	for _, e := range []core.Environment{iosEnv, androidEnv} {
		t.Run(string(e.OS), func(t *testing.T) {
			app, p := start(t, e, func(cx *core.Scope) core.Node {
				nav := base.NewNav(cx, "home")
				return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, _ string, _ func() int) base.NavPage {
					scroll := &core.Ref{}
					return NavPage(cx, base.NavPage{
						Title:  "Feed",
						TestID: "page",
						Children: []core.Node{
							core.Scroll{
								Ref:    scroll,
								TestID: "feed",
								Style:  core.Style{FlexGrow: 1},
								Children: []core.Node{
									core.View{Style: core.Style{Height: core.Pt(1200)}},
								},
							},
						},
					}, NavHeader{Scroll: scroll})
				})
			})
			screen := app.Find("id:page").Screen
			if !screen.HeaderTransparent || screen.Background != 0 {
				t.Fatalf("%v: screen header = %+v, want transparent with unset background", e.OS, screen)
			}
			wantHeader := e.HeaderHeight + e.SafeArea.Top - 16
			if screen.HeaderHeight != wantHeader {
				t.Fatalf("%v: header height = %v, want %v", e.OS, screen.HeaderHeight, wantHeader)
			}
			feed := app.Find("id:feed")
			if feed.Frame.Origin.Y != 0 {
				t.Fatalf("%v: scroll top = %v, want 0 (scrolling under header)", e.OS, feed.Frame.Origin.Y)
			}
			if feed.Scroll.HeaderInset != wantHeader || feed.Scroll.Behavior.ContentInset.Top != wantHeader {
				t.Fatalf("%v: scroll insets = (header=%v, top=%v), want %v", e.OS, feed.Scroll.HeaderInset, feed.Scroll.Behavior.ContentInset.Top, wantHeader)
			}
			topEdge := feed.Scroll.Edges[0]
			if topEdge.Drawn() != core.EdgeEffectSoft {
				t.Fatalf("%v: top edge drawn = %v, want soft", e.OS, topEdge.Drawn())
			}
			if wantExtent := wantHeader - navEdgeTail; topEdge.Extent != wantExtent {
				t.Fatalf("%v: top edge extent = %v, want %v", e.OS, topEdge.Extent, wantExtent)
			}
			if topEdge.Band() != wantHeader {
				t.Fatalf("%v: top edge band = %v, want %v", e.OS, topEdge.Band(), wantHeader)
			}
			if topEdge.Tint != p.Background {
				t.Fatalf("%v: top edge tint = %x, want page background %x", e.OS, topEdge.Tint, p.Background)
			}
		})
	}
}

// TestNavPagePreservesCustomHeaderBackground verifies that explicit custom
// HeaderStyle / HeaderStyleFunc backgrounds survive ui.NavPage on Android and iOS.
func TestNavPagePreservesCustomHeaderBackground(t *testing.T) {
	customBg := core.RGB(0x112233)
	customTitle := core.RGB(0xeeeeee)
	for _, os := range []core.OS{core.OSAndroid, core.OSIOS} {
		t.Run(string(os), func(t *testing.T) {
			e := phoneEnv
			e.OS = os
			e.HeaderHeight = 64
			app, p := start(t, e, func(cx *core.Scope) core.Node {
				nav := base.NewNav(cx, "static", "dynamic")
				return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, _ func() int) base.NavPage {
					page := base.NavPage{Title: route, TestID: "page-" + route}
					if route == "static" {
						page.HeaderStyle = core.Style{Background: customBg, Color: customTitle}
					} else {
						page.HeaderStyleFunc = func() core.Style {
							return core.Style{Background: customBg, Color: customTitle}
						}
					}
					return NavPage(cx, page, NavHeader{})
				})
			})
			c := app.Find("id:page-dynamic").Screen
			if c.Background != customBg || c.TitleColor != customTitle || c.TintColor != p.Text {
				t.Fatalf("%v dynamic: got bg=%x title=%x tint=%x, want bg=%x title=%x tint=%x",
					os, c.Background, c.TitleColor, c.TintColor, customBg, customTitle, p.Text)
			}
			app.HeaderBack("id:page-dynamic")
			c = app.Find("id:page-static").Screen
			if c.Background != customBg || c.TitleColor != customTitle || c.TintColor != p.Text {
				t.Fatalf("%v static: got bg=%x title=%x tint=%x, want bg=%x title=%x tint=%x",
					os, c.Background, c.TitleColor, c.TintColor, customBg, customTitle, p.Text)
			}
		})
	}
}
