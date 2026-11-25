// Package terminal is base/terminal's View in ui's look. It is a package of
// its own, as base/terminal is, so only apps that embed a terminal link
// libghostty-vt. Its types are base/terminal's: this package adds the look
// alone.
package terminal

import (
	"github.com/nsgo-dev/nsgo/base/terminal"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// View is terminal.View in ui's look: the default context menu on ui's
// menu surface with hover, active and disabled row states and Copy's icon,
// and the touch key bar on a raised bar with bordered keys and accented
// latched modifiers. The app's MenuStyle, MenuItemStyle, MenuItemIcon,
// KeyBarStyle, KeyStyle and KeyOnStyle (and their StyleFuncs) merge over ui's.
type View terminal.View

// Render implements core.Node.
func (v View) Render(cx *core.Scope) core.Node {
	out := terminal.View(v)
	out.MenuStyle = core.Style{}
	out.MenuStyleFunc = mergePart(func() core.Style { return menuStyle(cx) }, v.MenuStyle, v.MenuStyleFunc)
	out.MenuItemStyle = core.Style{}
	out.MenuItemStyleFunc = mergePart(func() core.Style { return menuItemStyle(cx) }, v.MenuItemStyle, v.MenuItemStyleFunc)
	if out.MenuItemIcon == nil {
		out.MenuItemIcon = func(cx *core.Scope, a terminal.Action) core.Node {
			if a == terminal.CopyToClipboard {
				return ui.Icon(lucide.Copy, 14, ui.ColorsOf(cx).Muted)
			}
			return nil
		}
	}
	out.KeyBarStyle = core.Style{}
	out.KeyBarStyleFunc = mergePart(func() core.Style { return keyBarStyle(cx) }, v.KeyBarStyle, v.KeyBarStyleFunc)
	out.KeyStyle = core.Style{}
	out.KeyStyleFunc = mergePart(func() core.Style { return keyStyle(cx) }, v.KeyStyle, v.KeyStyleFunc)
	out.KeyOnStyle = core.Style{}
	out.KeyOnStyleFunc = mergePart(func() core.Style { return keyOnStyle(cx) }, v.KeyOnStyle, v.KeyOnStyleFunc)
	return out
}

func mergePart(uiFn func() core.Style, appStyle core.Style, appFn func() core.Style) func() core.Style {
	return func() core.Style {
		s := uiFn().Merge(appStyle)
		if appFn != nil {
			s = s.Merge(appFn())
		}
		return s
	}
}

func menuStyle(cx *core.Scope) core.Style {
	return ui.FinOf(cx).Menu().Over(core.Style{
		Padding:      core.All(4),
		MinWidth:     core.Pt(180),
		CornerRadius: ui.RadiusControl + 2,
		BorderWidth:  1,
		Color:        ui.ColorsOf(cx).Text(),
	})
}

func menuItemStyle(cx *core.Scope) core.Style {
	colors := ui.ColorsOf(cx)
	return ui.TypeBody.Merge(core.Style{
		Direction:    core.Row,
		AlignItems:   core.AlignCenter,
		Gap:          6,
		MinHeight:    core.Pt(26),
		Padding:      core.Symmetric(0, 8),
		CornerRadius: ui.RadiusControl - 2,
		Color:        colors.Text(),
		Hover:        &core.Style{Background: colors.Hover()},
		Active:       &core.Style{Background: colors.Pressed()},
		Disabled:     ui.Dimmed(),
	})
}

func keyBarStyle(cx *core.Scope) core.Style {
	colors := ui.ColorsOf(cx)
	return core.Style{
		Borders:      core.Edges{Top: 1},
		BorderColors: core.EdgeColors{Top: colors.Line()},
		Background:   colors.Raised(),
		Padding:      core.Symmetric(6, 6),
		FlexShrink:   0,
	}
}

func keyStyle(cx *core.Scope) core.Style {
	colors := ui.ColorsOf(cx)
	return core.Style{
		MinWidth:       core.Pt(40),
		Height:         core.Pt(34),
		Padding:        core.Symmetric(0, 10),
		Margin:         core.Edges{Right: 6},
		CornerRadius:   6,
		BorderCurve:    core.BorderCurveContinuous,
		AlignItems:     core.AlignCenter,
		JustifyContent: core.JustifyCenter,
		BorderWidth:    1,
		FontSize:       15,
		Background:     colors.Surface(),
		BorderColor:    colors.Border(),
		Color:          colors.Text(),
		Hover:          &core.Style{Background: colors.Hover()},
		Active:         &core.Style{Background: colors.Pressed()},
	}
}

func keyOnStyle(cx *core.Scope) core.Style {
	colors := ui.ColorsOf(cx)
	p := cx.Palette()
	return core.Style{
		Background:  colors.AccentFill(),
		BorderColor: colors.AccentFill(),
		Color:       colors.OnAccent(),
		Hover:       &core.Style{Background: p.AccentHover, BorderColor: p.AccentHover},
		Active:      &core.Style{Background: p.AccentPressed, BorderColor: p.AccentPressed},
	}
}
