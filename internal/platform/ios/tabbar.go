//go:build ios

package ios

import (
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/apple"
)

// tabBar is core.TabBar's component, which the backend registers itself
// as NSH's system_tab_bar.rs: a UITabBar whose items are the app's tabs,
// their icons SF Symbols, reporting the user's choice as "change".
type tabBar struct {
	cx    *driver.NativeContext
	bar   objc.ID
	items []objc.ID
}

var (
	tabBars        = map[objc.ID]*tabBar{}
	tabBarDelegate objc.ID
)

func registerTabBar() {
	class := register("NsgoTabBarDelegate", objc.GetClass("NSObject"),
		[]*objc.Protocol{objc.GetProtocol("UITabBarDelegate")}, []objc.MethodDef{
			{Cmd: sel("tabBar:didSelectItem:"), Fn: func(_ objc.ID, _ objc.SEL, bar, item objc.ID) {
				t := tabBars[bar]
				if t == nil {
					return
				}
				for i, it := range t.items {
					if it == item {
						t.cx.Emit("change", float64(i))
					}
				}
			}},
		})
	tabBarDelegate = objc.ID(class).Send(selNew)
}

func newTabBar(cx *driver.NativeContext) driver.NativeComponent {
	if tabBarDelegate == 0 {
		registerTabBar()
	}
	t := &tabBar{cx: cx, bar: objc.ID(objc.GetClass("UITabBar")).Send(selNew)}
	t.bar.Send(selSetDeleg, tabBarDelegate)
	tabBars[t.bar] = t
	// The system's height, the home indicator below it included.
	fit := objc.Send[apple.CGSize](t.bar, sel("sizeThatFits:"), apple.CGSize{Width: 390})
	if fit.Height <= 0 {
		fit.Height = 49
	}
	cx.SetSize(driver.Size{Height: driver.Points(fit.Height)})
	return t
}

func (t *tabBar) View() any { return t.bar }

func (t *tabBar) SetProp(name string, value any) {
	switch name {
	case "items":
		list, _ := value.([]any)
		array := objc.ID(objc.GetClass("NSMutableArray")).Send(selNew)
		t.items = t.items[:0]
		for i, it := range list {
			tab, _ := it.(map[string]any)
			title, _ := tab["title"].(string)
			icon, _ := tab["icon"].(string)
			var image objc.ID
			if icon != "" {
				image = objc.ID(objc.GetClass("UIImage")).Send(sel("systemImageNamed:"), apple.NSString(icon))
			}
			item := objc.ID(objc.GetClass("UITabBarItem")).Send(selAlloc).
				Send(sel("initWithTitle:image:tag:"), apple.NSString(title), image, i)
			t.items = append(t.items, item)
			array.Send(sel("addObject:"), item)
			item.Send(selRelease)
		}
		t.bar.Send(sel("setItems:animated:"), array, false)
		array.Send(selRelease)
	case "selectedIndex":
		i, _ := value.(float64)
		var item objc.ID
		if n := int(i); n >= 0 && n < len(t.items) {
			item = t.items[n]
		}
		t.bar.Send(sel("setSelectedItem:"), item)
	}
}

func (t *tabBar) Command(string, any) {}

func (t *tabBar) Close() {
	delete(tabBars, t.bar)
	t.bar.Send(selSetDeleg, objc.ID(0))
	t.bar.Send(selRelease)
}
