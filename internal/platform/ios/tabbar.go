//go:build ios

package ios

import (
	"slices"
	"structs"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// directionalEdgeInsets is NSDirectionalEdgeInsets.
type directionalEdgeInsets struct {
	_                              structs.HostLayout
	Top, Leading, Bottom, Trailing float64
}

// tabBar is core.TabBar's component, which the backend registers itself
// as NSH's system_tab_bar.rs: a UITabBar on the bottom edge, or a vertical
// UIKit bar on the leading or trailing edge, their icons SF Symbols,
// reporting the user's choice as "change".
type tabBar struct {
	cx        *driver.NativeContext
	container objc.ID
	bar       objc.ID
	rail      objc.ID
	backdrop  objc.ID
	separator objc.ID
	stack     objc.ID
	buttons   []objc.ID
	items     []objc.ID
	titles    []string
	icons     []string
	label     string
	tint      driver.Color
	edge      tabBarEdge
	metrics   tabBarSizing
	// selected is the selected tab's index, kept for props in any order.
	selected int
}

var (
	tabBars          = map[objc.ID]*tabBar{}
	tabBarContainers = map[objc.ID]*tabBar{}
	tabBarButtons    = map[objc.ID]*tabBar{}
	tabBarDelegate   objc.ID
	classTabBarHost  objc.Class
	tabBarClass      objc.Class
)

func registerTabBar() {
	recheck := func(self objc.ID) {
		if t := tabBars[self]; t != nil {
			t.measure()
		}
	}
	tabBarClass = register("NsgoTabBar", apple.Class("UITabBar"), nil, []objc.MethodDef{
		{Cmd: sel("didMoveToWindow"), Fn: func(self objc.ID, cmd objc.SEL) {
			apple.Super(self, cmd)
			recheck(self)
		}},
		{Cmd: sel("safeAreaInsetsDidChange"), Fn: func(self objc.ID, cmd objc.SEL) {
			apple.Super(self, cmd)
			recheck(self)
		}},
		{Cmd: sel("layoutSubviews"), Fn: func(self objc.ID, cmd objc.SEL) {
			apple.Super(self, cmd)
			recheck(self)
		}},
		{Cmd: sel("traitCollectionDidChange:"), Fn: func(self objc.ID, cmd objc.SEL, previous objc.ID) {
			apple.Super(self, cmd, previous)
			recheck(self)
		}},
	})
	class := register("NsgoTabBarDelegate", apple.Class("NSObject"),
		[]*objc.Protocol{objc.GetProtocol("UITabBarDelegate")}, []objc.MethodDef{
			{Cmd: sel("tabBar:didSelectItem:"), Fn: func(_ objc.ID, _ objc.SEL, bar, item objc.ID) {
				t := tabBars[bar]
				if t == nil {
					return
				}
				for i, it := range t.items {
					if it == item && t.cx != nil {
						t.selected = i
						t.cx.Emit("change", float64(i))
					}
				}
			}},
			{Cmd: sel("nsgoTabBarButtonTapped:"), Fn: func(_ objc.ID, _ objc.SEL, sender objc.ID) {
				t := tabBarButtons[sender]
				if t == nil {
					return
				}
				for i, b := range t.buttons {
					if b == sender {
						t.selected = i
						t.showSelected()
						t.cx.Emit("change", float64(i))
						return
					}
				}
			}},
		})
	tabBarDelegate = objc.ID(class).Send(selNew)
	classTabBarHost = register("NsgoTabBarContainer", apple.Class("UIView"), nil, []objc.MethodDef{
		{Cmd: sel("layoutSubviews"), Fn: func(self objc.ID, cmd objc.SEL) {
			apple.Super(self, cmd)
			if t := tabBarContainers[self]; t != nil {
				t.layoutSubviews()
			}
		}},
		{Cmd: sel("safeAreaInsetsDidChange"), Fn: func(self objc.ID, cmd objc.SEL) {
			apple.Super(self, cmd)
			if t := tabBarContainers[self]; t != nil {
				t.measure()
				t.layoutSubviews()
			}
		}},
	})
}

func newTabBar(cx *driver.NativeContext) driver.NativeComponent {
	if tabBarDelegate == 0 {
		registerTabBar()
	}
	t := &tabBar{
		cx:        cx,
		container: objc.ID(classTabBarHost).Send(selNew),
		bar:       objc.ID(tabBarClass).Send(selNew),
	}
	t.bar.Send(selSetDeleg, tabBarDelegate)
	t.container.Send(selAddSubview, t.bar)
	tabBars[t.bar] = t
	tabBarContainers[t.container] = t
	t.measure()
	return t
}

func (t *tabBar) rtl() bool {
	const rightToLeft = 1
	host := t.container
	if host == 0 {
		host = t.bar
	}
	if host == 0 || !objc.Send[bool](host, sel("respondsToSelector:"), sel("effectiveUserInterfaceLayoutDirection")) {
		return false
	}
	return objc.Send[int](host, sel("effectiveUserInterfaceLayoutDirection")) == rightToLeft
}

func (t *tabBar) safeInsets() edgeInsets {
	host := t.container
	if host == 0 {
		host = t.bar
	}
	if host != 0 && objc.Send[objc.ID](host, sel("window")) != 0 {
		return objc.Send[edgeInsets](host, sel("safeAreaInsets"))
	}
	app := objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication"))
	if window := objc.Send[objc.ID](app, sel("keyWindow")); window != 0 {
		return objc.Send[edgeInsets](window, sel("safeAreaInsets"))
	}
	return edgeInsets{}
}

// measure reports the vertical bar's width on a leading or trailing edge, or asks UIKit for the bottom bar's natural height at its current placement, width, safe area and traits, reporting only changed heights. Before attachment it reports the bar's own fitted height without guessing the key window's bottom safe area; once attached and laid out, UIKit's sizeThatFits: is the authority: a classic bar's adds the bottom safe area it overlaps, while iOS 26's floating bar answers one height at every placement (83 points on an iPhone, detached too), so there it changes only with width and traits.
func (t *tabBar) measure() {
	if t.cx == nil {
		return
	}
	if t.edge != tabBarEdgeBottom || t.bar == 0 {
		// A later bottom edge reports its height afresh.
		t.metrics = tabBarSizing{}
		in := t.safeInsets()
		t.cx.SetSize(tabBarNaturalSize(t.edge, 0, t.rtl(), in.Left, in.Right, 0))
		return
	}
	attached := objc.Send[objc.ID](t.bar, sel("window")) != 0
	bounds := objc.Send[apple.CGRect](t.bar, selBounds)
	width, ok := t.metrics.measureWidth(attached, bounds.Size.Width, bounds.Size.Height)
	if !ok && t.metrics.hasReported {
		return
	}
	if !ok {
		// Just back from a vertical edge, the bar has no bottom frame yet; report its detached height so layout gives it one.
		width = defaultTabBarWidth
	}
	fit := objc.Send[apple.CGSize](t.bar, sel("sizeThatFits:"), apple.CGSize{Width: width})
	if h, changed := t.metrics.update(fit.Height); changed {
		t.cx.SetSize(driver.Size{Height: h})
	}
}

func (t *tabBar) View() any {
	if t.container != 0 {
		return t.container
	}
	return t.bar
}

func (t *tabBar) layoutSubviews() {
	if t.container == 0 {
		return
	}
	bounds := objc.Send[apple.CGRect](t.container, selBounds)
	vertical := t.edge == tabBarEdgeLeading || t.edge == tabBarEdgeTrailing
	if t.bar != 0 {
		t.bar.Send(selSetHidden, vertical)
		if !vertical {
			t.bar.Send(selSetFrame, bounds)
		}
	}
	if !vertical {
		if t.rail != 0 {
			t.rail.Send(selSetHidden, true)
		}
		return
	}
	t.ensureRail()
	t.rail.Send(selSetHidden, false)
	t.rail.Send(selSetFrame, bounds)
	t.backdrop.Send(selSetFrame, bounds)
	sepX := verticalBarSeparatorX(t.edge, t.rtl(), bounds.Size.Width)
	t.separator.Send(selSetFrame, apple.Rect(sepX, 0, 1, bounds.Size.Height))
	in := objc.Send[edgeInsets](t.container, sel("safeAreaInsets"))
	left := in.Left + 4
	right := in.Right + 4
	top := in.Top + 8
	bottom := in.Bottom + 8
	w := max(bounds.Size.Width-left-right, 0)
	h := max(bounds.Size.Height-top-bottom, 0)
	t.stack.Send(selSetFrame, apple.Rect(left, top, w, h))
}

// ensureRail lazily builds the vertical bar hierarchy (Liquid Glass or system
// chrome backdrop, hairline inner separator, and vertical UIStackView of tabs).
func (t *tabBar) ensureRail() {
	if t.rail != 0 || t.container == 0 {
		return
	}
	t.rail = objc.ID(apple.Class("UIView")).Send(selNew)
	t.rail.Send(selSetAccessibilityTraits, traits["TabBar"])
	t.rail.Send(sel("setAccessibilityLabel:"), nsOrNil(t.label))

	var effect objc.ID
	if hasGlass() {
		effect = objc.ID(apple.Class("UIGlassEffect")).Send(sel("effectWithStyle:"), 0)
	} else {
		const systemChromeMaterial = 10
		effect = objc.ID(apple.Class("UIBlurEffect")).Send(sel("effectWithStyle:"), systemChromeMaterial)
	}
	t.backdrop = objc.ID(apple.Class("UIVisualEffectView")).Send(selAlloc).Send(sel("initWithEffect:"), effect)
	t.rail.Send(selAddSubview, t.backdrop)

	t.separator = objc.ID(apple.Class("UIView")).Send(selNew)
	uiColorClass := objc.ID(apple.Class("UIColor"))
	var sepColor objc.ID
	if objc.Send[bool](uiColorClass, sel("respondsToSelector:"), sel("separatorColor")) {
		sepColor = uiColorClass.Send(sel("separatorColor"))
	} else {
		sepColor = uiColorClass.Send(sel("grayColor"))
	}
	t.separator.Send(sel("setBackgroundColor:"), sepColor)
	t.rail.Send(selAddSubview, t.separator)

	t.stack = objc.ID(apple.Class("UIStackView")).Send(selNew)
	const (
		axisVertical            = 1
		alignmentFill           = 0
		distributionFillEqually = 1
	)
	t.stack.Send(sel("setAxis:"), axisVertical)
	t.stack.Send(sel("setAlignment:"), alignmentFill)
	t.stack.Send(sel("setDistribution:"), distributionFillEqually)
	t.stack.Send(sel("setSpacing:"), float64(4))
	t.rail.Send(selAddSubview, t.stack)

	t.container.Send(selAddSubview, t.rail)
	t.rebuildRailButtons()
	t.showSelected()
}

func (t *tabBar) clearRailButtons() {
	for _, b := range t.buttons {
		delete(tabBarButtons, b)
		if t.stack != 0 {
			t.stack.Send(sel("removeArrangedSubview:"), b)
		}
		b.Send(selRemoveFromSuperview)
		b.Send(selRelease)
	}
	t.buttons = t.buttons[:0]
}

func (t *tabBar) rebuildRailButtons() {
	if t.stack == 0 {
		return
	}
	t.clearRailButtons()
	const touchUpInside = 1 << 6
	for range t.icons {
		btn := objc.ID(apple.Class("UIButton")).Send(selNew)
		btn.Send(sel("addTarget:action:forControlEvents:"), tabBarDelegate, sel("nsgoTabBarButtonTapped:"), uint(touchUpInside))
		t.buttons = append(t.buttons, btn)
		tabBarButtons[btn] = t
		t.stack.Send(sel("addArrangedSubview:"), btn)
	}
	t.styleRailButtons()
}

func (t *tabBar) railForeground(selected bool) objc.ID {
	uiColorClass := objc.ID(apple.Class("UIColor"))
	if selected {
		if t.tint != 0 {
			return uiColor(t.tint)
		}
		if objc.Send[bool](uiColorClass, sel("respondsToSelector:"), sel("tintColor")) {
			return uiColorClass.Send(sel("tintColor"))
		}
		return uiColorClass.Send(sel("systemBlueColor"))
	}
	if objc.Send[bool](uiColorClass, sel("respondsToSelector:"), sel("secondaryLabelColor")) {
		return uiColorClass.Send(sel("secondaryLabelColor"))
	}
	return uiColorClass.Send(sel("grayColor"))
}

func (t *tabBar) styleRailButtons() {
	cfgClass := apple.Class("UIButtonConfiguration")
	for i, btn := range t.buttons {
		selected := i == t.selected
		title := ""
		if i < len(t.titles) {
			title = t.titles[i]
		}
		icon := ""
		if i < len(t.icons) {
			icon = t.icons[i]
		}
		var image objc.ID
		if icon != "" {
			image = objc.ID(apple.Class("UIImage")).Send(sel("systemImageNamed:"), apple.NSString(icon))
		}
		fg := t.railForeground(selected)
		if cfgClass != 0 && objc.Send[bool](btn, sel("respondsToSelector:"), sel("setConfiguration:")) {
			cfg := objc.ID(cfgClass).Send(sel("plainButtonConfiguration"))
			cfg.Send(sel("setTitle:"), apple.NSString(title))
			cfg.Send(sel("setImage:"), image)
			const (
				directionalEdgeTop = 1
				titleAlignCenter   = 2
			)
			cfg.Send(sel("setImagePlacement:"), uint(directionalEdgeTop))
			cfg.Send(sel("setImagePadding:"), float64(4))
			cfg.Send(sel("setTitleAlignment:"), titleAlignCenter)
			cfg.Send(sel("setContentInsets:"), directionalEdgeInsets{Top: 6, Leading: 4, Bottom: 6, Trailing: 4})
			cfg.Send(sel("setBaseForegroundColor:"), fg)
			btn.Send(sel("setConfiguration:"), cfg)
		} else {
			btn.Send(sel("setTitle:forState:"), apple.NSString(title), uint(0))
			btn.Send(sel("setImage:forState:"), image, uint(0))
			btn.Send(sel("setTitleColor:forState:"), fg, uint(0))
			btn.Send(sel("setTintColor:"), fg)
		}
		btn.Send(sel("setSelected:"), selected)
		btn.Send(sel("setAccessibilityLabel:"), nsOrNil(title))
		ax := traits["Button"]
		if selected {
			ax |= traits["Selected"]
		}
		btn.Send(selSetAccessibilityTraits, ax)
	}
}

// showSelected selects the selected tab's item; one past them selects none.
func (t *tabBar) showSelected() {
	if t.bar != 0 {
		var item objc.ID
		if t.selected >= 0 && t.selected < len(t.items) {
			item = t.items[t.selected]
		}
		t.bar.Send(sel("setSelectedItem:"), item)
	}
	if len(t.buttons) > 0 {
		t.styleRailButtons()
	}
}

// selectIndex selects item index as tapping it would, for tests.
func (t *tabBar) selectIndex(index int) bool {
	if index < 0 || index >= len(t.items) {
		return false
	}
	if t.edge == tabBarEdgeBottom {
		item := t.items[index]
		t.bar.Send(sel("setSelectedItem:"), item)
		t.bar.Send(sel("delegate")).Send(sel("tabBar:didSelectItem:"), t.bar, item)
		return true
	}
	t.selected = index
	t.showSelected()
	if t.cx != nil {
		t.cx.Emit("change", float64(index))
	}
	return true
}

func (t *tabBar) SetProp(name string, value any) {
	switch name {
	case "items":
		list, _ := value.([]any)
		t.setItems(list)
	case "label":
		label, _ := value.(string)
		t.label = label
		if t.bar != 0 {
			t.bar.Send(sel("setAccessibilityLabel:"), nsOrNil(label))
		}
		if t.rail != 0 {
			t.rail.Send(sel("setAccessibilityLabel:"), nsOrNil(label))
		}
	case "edge":
		edge := parseTabBarEdge(value)
		if edge == t.edge {
			return
		}
		t.edge = edge
		if edge == tabBarEdgeLeading || edge == tabBarEdgeTrailing {
			t.ensureRail()
		}
		t.measure()
		if t.container != 0 {
			t.container.Send(selSetNeedsLayout)
			t.layoutSubviews()
		}
	case "selectedIndex":
		i, _ := value.(float64)
		t.selected = int(i)
		t.showSelected()
	case "tint":
		// The bar draws its selected item in its tintColor; none restores
		// the inherited one.
		c, _ := value.(float64)
		t.tint = driver.Color(c)
		tint := objc.ID(0)
		if t.tint != 0 {
			tint = uiColor(t.tint)
		}
		if t.bar != 0 {
			t.bar.Send(sel("setTintColor:"), tint)
		}
		if len(t.buttons) > 0 {
			t.styleRailButtons()
		}
	}
}

// setItems updates title changes on the current UIKit items, preserving
// selection and VoiceOver's item identity.
func (t *tabBar) setItems(list []any) {
	titles, icons := make([]string, len(list)), make([]string, len(list))
	for i, it := range list {
		tab, _ := it.(map[string]any)
		titles[i], _ = tab["title"].(string)
		icons[i], _ = tab["icon"].(string)
	}
	if slices.Equal(t.icons, icons) {
		t.titles = titles
		for i, item := range t.items {
			if apple.GoString(item.Send(sel("title"))) != titles[i] {
				item.Send(sel("setTitle:"), apple.NSString(titles[i]))
			}
		}
		if len(t.buttons) > 0 {
			t.styleRailButtons()
		}
		return
	}
	t.titles, t.icons = titles, icons
	array := objc.ID(apple.Class("NSMutableArray")).Send(selNew)
	t.items = t.items[:0]
	for i, icon := range icons {
		var image objc.ID
		if icon != "" {
			image = objc.ID(apple.Class("UIImage")).Send(sel("systemImageNamed:"), apple.NSString(icon))
		}
		item := objc.ID(apple.Class("UITabBarItem")).Send(selAlloc).
			Send(sel("initWithTitle:image:tag:"), apple.NSString(titles[i]), image, i)
		t.items = append(t.items, item)
		array.Send(sel("addObject:"), item)
		item.Send(selRelease)
	}
	if t.bar != 0 {
		t.bar.Send(sel("setItems:animated:"), array, false)
	}
	array.Send(selRelease)
	if t.rail != 0 {
		t.rebuildRailButtons()
	}
	t.showSelected()
	t.measure()
}

func (t *tabBar) Command(string, any) {}

func (t *tabBar) Close() {
	t.clearRailButtons()
	delete(tabBars, t.bar)
	delete(tabBarContainers, t.container)
	if t.bar != 0 {
		t.bar.Send(selSetDeleg, objc.ID(0))
		t.bar.Send(selRelease)
	}
	if t.stack != 0 {
		t.stack.Send(selRelease)
	}
	if t.separator != 0 {
		t.separator.Send(selRelease)
	}
	if t.backdrop != 0 {
		t.backdrop.Send(selRelease)
	}
	if t.rail != 0 {
		t.rail.Send(selRelease)
	}
	if t.container != 0 {
		t.container.Send(selRelease)
	}
}
