package ui

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/mobile"
)

// navEdgeTail is how far a soft edge's blur runs past its bars.
const navEdgeTail core.Points = 24

// systemHeader reports whether a page shows the platform's header, iOS's navigation bar or Android's toolbar, rather than NavHeader, which a watch and a web build that runs as them show: where mobile's system components show.
func systemHeader(cx *core.Scope) bool { return mobile.IsAvailable(cx) }

// transparentHeader reports whether the system's header lies over the page's content without a background, which scrolls under it with a progressive blur: iOS's and Android's.
func transparentHeader(cx *core.Scope) bool {
	return systemHeader(cx)
}

// NavInset is the room to leave at the top of a page's content that does not scroll for a NavPage's system header, and 0 where the header takes room of its own. Scroll views under the header are inset already. It is tracked as the safe area changes.
func NavInset(cx *core.Scope) core.Points {
	if !transparentHeader(cx) {
		return 0
	}
	return core.Insets(cx).Top
}

// NavHeader is the bar at the top of a page: a back button named BackLabel while OnBack is set, the title, and the app's views at either end. On iOS and Android NavPage gives the title and the views to the system's header, whose own back button pops the page, so OnBack and BackLabel go unused there.
type NavHeader struct {
	Title string
	// TitleFunc replaces Title as the graph changes.
	TitleFunc func() string
	OnBack    func()
	BackLabel string
	Leading   []core.Node
	Trailing  []core.Node
	// Scroll is the page's scroll view, ScrollFunc when it changes: under iOS's and Android's system headers it blurs progressively toward the top.
	Scroll     *core.Ref
	ScrollFunc func() *core.Ref
	// Symbols has iOS's system header show its own buttons for what takes presses in Trailing, one each in order, as the back button is drawn.
	Symbols func() []core.HeaderSymbol
	TestID  string
}

// Render implements core.Node.
func (h NavHeader) Render(cx *core.Scope) core.Node {
	colors := ColorsOf(cx)
	var children []core.Node
	if h.OnBack != nil {
		children = append(children, IconButton{Icon: lucide.ChevronLeft, Label: h.BackLabel, Size: 32, OnPress: h.OnBack})
	}
	children = append(children, h.Leading...)
	children = append(children, core.Text{Content: h.Title, ContentFunc: h.TitleFunc, MaxLines: 1, Role: core.RoleHeader,
		Style: TypeTitle.Merge(core.Style{FlexGrow: 1, FlexShrink: 1}), StyleFunc: Colored(colors.Text)})
	children = append(children, h.Trailing...)
	return core.View{TestID: h.TestID, Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
		MinHeight: core.Pt(44), Padding: core.Symmetric(0, 12), PaddingEnv: core.EnvEdges{Left: core.EnvSafeArea, Right: core.EnvSafeArea}, Borders: core.Edges{Bottom: 1}},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Surface(), BorderColors: core.EdgeColors{Bottom: colors.Line()}}
		}, Children: children}
}
