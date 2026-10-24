package ui

import (
	"time"

	"github.com/oneo-me/nsgo/core"
)

// Toast is a short notice shown over the app for a while: a title, an
// optional description and an icon in its Tone's color.
type Toast struct {
	Title, Description string
	Tone               Tone
	Icon               *core.SVG
	// Duration is how long it shows; four seconds when 0.
	Duration time.Duration
}

var toastKey = core.NewKey[func(Toast)]("toast", nil)

// UseToast returns what shows a toast in the nearest ToastView, which
// also has the screen reader say its title. Without a ToastView it does
// nothing.
func UseToast(cx *core.Scope) func(Toast) {
	if show := toastKey.Use(cx); show != nil {
		return show
	}
	return func(Toast) {}
}

// ToastView lays out its children as a view of Style and shows the toasts
// they post with UseToast at its bottom end, newest last. A press on a
// toast dismisses it. Toasts sit under modals, as they belong to the app
// below them.
type ToastView struct {
	Style    core.Style
	Children []core.Node
	TestID   string
}

// toastExit is how long a toast takes to fade out.
const toastExit = 180 * time.Millisecond

// Render implements core.Node.
func (v ToastView) Render(cx *core.Scope) core.Node {
	type shown struct {
		id int
		Toast
	}
	toasts := core.NewSignalFunc[[]shown](cx, nil, nil)
	next := 0
	remove := func(id int) {
		toasts.Update(func(all []shown) []shown {
			out := make([]shown, 0, len(all))
			for _, s := range all {
				if s.id != id {
					out = append(out, s)
				}
			}
			return out
		})
	}
	toastKey.Provide(cx, func(t Toast) {
		next++
		toasts.Update(func(all []shown) []shown { return append(all, shown{next, t}) })
		cx.Announce(t.Title)
	})
	theme := UseTheme(cx)
	env := cx.Env()
	stack := core.Each(toasts.Get, func(s shown) int { return s.id }, func(cx *core.Scope, item func() shown, _ func() int) core.Node {
		s := core.Untracked(cx, item)
		in := core.NewSignal(cx, false)
		leaving := false
		leave := func() {
			if leaving {
				return
			}
			leaving = true
			in.Set(false)
			cx.AfterFunc(toastExit, func() { remove(s.id) })
		}
		d := s.Duration
		if d <= 0 {
			d = 4 * time.Second
		}
		// The first commit places it hidden, so its transition shows it.
		cx.AfterFunc(time.Millisecond, func() { in.Set(!leaving) })
		cx.AfterFunc(d, leave)
		var children []core.Node
		if s.Icon != nil {
			children = append(children, core.Icon{SVG: s.Icon, StyleFunc: func() core.Style {
				t := theme()
				color, ok := toneColor(t, s.Tone)
				if !ok {
					color = t.MutedForeground
				}
				return core.Style{Width: core.Pt(18), Height: core.Pt(18), FlexShrink: 0, Color: color}
			}})
		}
		text := []core.Node{core.Text{Content: s.Title, Style: core.Style{FontWeight: core.FontWeightSemibold}}}
		if s.Description != "" {
			text = append(text, core.Text{Content: s.Description, StyleFunc: func() core.Style {
				t := theme()
				return core.Style{Color: t.MutedForeground, FontSize: core.Points(float32(t.FontSize)*0.85 + 0.5)}
			}})
		}
		children = append(children, core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: text})
		return core.View{OnPress: func(core.PressEvent) { leave() }, Role: core.RoleAlert, Label: s.Title, TestID: "toast",
			StyleFunc: func() core.Style {
				t := theme()
				style := popupStyle(t)
				style.Direction, style.AlignItems, style.Gap = core.Row, core.AlignStart, 10
				style.Padding = core.Symmetric(12, 14)
				style.Transition = core.Transition{Duration: toastExit, Easing: core.EaseOut}
				if !in.Get() {
					style.Opacity = core.Alpha(0)
					if !env.ReduceMotion() {
						style.Transform = core.Transform{TranslateY: 8}
					}
				}
				return style
			},
			Children: children}
	})
	return core.View{Style: v.Style, TestID: v.TestID, Children: append(append([]core.Node{}, v.Children...),
		core.View{Style: core.Style{Position: core.Absolute, Right: core.Pt(16), Bottom: core.Pt(16),
			Width: core.Pt(340), MaxWidth: core.Pct(90), Gap: 8}, Children: []core.Node{stack}})}
}
