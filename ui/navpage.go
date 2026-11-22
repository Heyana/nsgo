package ui

import (
	"slices"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// NavPage returns p, a page of a base.Nav, on the page's background under header, a NavHeader whose title and ends default to p's, unless p has a Header of its own. On iOS and Android a page's own Header becomes the system header's title view, at its content's size, so it needs a width of its own to fill the bar; it takes precedence over HeaderTitle and keeps p's left and right views, and only header's Scroll is used. Otherwise header supplies the title and ends. The system's back button pops the page, and the transparent header lies over the page, which scrolls under it with a progressive blur. Elsewhere the page's own Header, or NavHeader, is drawn above the page.
func NavPage(cx *core.Scope, p base.NavPage, header NavHeader) base.NavPage {
	colors := ColorsOf(cx)
	app := p.StyleFunc
	p.StyleFunc = func() core.Style {
		s := core.Style{Background: colors.Background()}
		if app != nil {
			s = s.Merge(app())
		}
		return s
	}
	if systemHeader(cx) {
		if p.Header != nil {
			p.HeaderTitle = p.Header
			header = NavHeader{Scroll: header.Scroll, ScrollFunc: header.ScrollFunc}
		}
		return systemNavPage(cx, p, header)
	}
	if p.Header == nil {
		if header.Title == "" {
			header.Title = p.Title
		}
		if header.TitleFunc == nil {
			header.TitleFunc = p.TitleFunc
		}
		if header.Leading == nil && p.HeaderLeft != nil {
			header.Leading = []core.Node{p.HeaderLeft}
		}
		if header.Trailing == nil && p.HeaderRight != nil {
			header.Trailing = []core.Node{p.HeaderRight}
		}
		p.Header = header
	}
	return p
}

// systemNavPage shows header in the system's header. On iOS and Android it is transparent over p, with a bar under it that blurs the page's scroll view as far as the header reaches, the blur's tail included.
func systemNavPage(cx *core.Scope, p base.NavPage, header NavHeader) base.NavPage {
	colors := ColorsOf(cx)
	p.Header = nil
	transparent := transparentHeader(cx)
	appStyle := p.HeaderStyle
	appHeader := p.HeaderStyleFunc
	p.HeaderStyle = core.Style{}
	p.HeaderStyleFunc = func() core.Style {
		s := core.Style{Color: colors.Text(), TintColor: colors.Text()}
		if !transparent {
			s.Background = colors.Surface()
		}
		s = s.Merge(appStyle)
		if appHeader != nil {
			s = s.Merge(appHeader())
		}
		return s
	}
	if header.Title != "" {
		p.Title = header.Title
	}
	if header.TitleFunc != nil {
		p.TitleFunc = header.TitleFunc
	}
	if len(header.Leading) > 0 {
		p.HeaderLeft = navItems(header.Leading)
	}
	if len(header.Trailing) > 0 {
		p.HeaderRight = navItems(header.Trailing)
		p.IOS.HeaderRightSymbols = header.Symbols
	}
	if !transparent {
		return p
	}
	p.HeaderTransparent = true
	scroll := header.ScrollFunc
	if scroll == nil {
		scroll = func() *core.Ref { return header.Scroll }
	}
	bar := core.Dynamic(func(cx *core.Scope) core.Node {
		return core.View{EdgeBar: core.EdgeBar{Scroll: scroll(), Side: core.SideTop}, PointerEvents: core.PointerEventsNone,
			Style:     core.Style{Position: core.Absolute, Top: core.Pt(0), Right: core.Pt(0), Left: core.Pt(0)},
			StyleFunc: func() core.Style { return core.Style{Height: core.Pt(max(0, core.Insets(cx).Top-navEdgeTail))} }}
	})
	p.Children = append(slices.Clip(p.Children), bar)
	return p
}

// navItems lays out views of the system header in a row.
func navItems(children []core.Node) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter}, Children: children}
}
