package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/oneo-me/nsgo/core"
)

// inlinePage is an inline page in the gallery's colors, for the
// appearance shown.
func inlinePage() string {
	hex := func(c core.Color) string {
		r, g, b, a := c.Components()
		return fmt.Sprintf("rgba(%d,%d,%d,%.3f)", int(r*255+0.5), int(g*255+0.5), int(b*255+0.5), a)
	}
	shine := 0.6
	if isDark {
		shine = 0.45
	}
	return `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1">
<style>
  html { -webkit-text-size-adjust: 100%; }
  body { margin: 0; padding: 16px; background: ` + hex(colors.inset) + `; color: ` + hex(colors.text) + `; font-family: -apple-system, system-ui, sans-serif; font-size: 15px; line-height: 1.4; }
  h3 { margin: 0 0 4px; font-size: 17px; font-weight: 600; }
  p { margin: 0 0 12px; color: ` + hex(colors.muted) + `; font-size: 15px; }
  button { height: 44px; padding: 0 18px; border: 0; border-radius: 10px; background: linear-gradient(180deg, #d6f78a 0%, #c1ea5c 100%); color: ` + hex(colors.onAccent) + `; font-family: inherit; font-size: 15px; font-weight: 600; box-shadow: inset 0 1px 0 rgba(255,255,255,` + fmt.Sprint(shine) + `); }
  #got { margin-top: 12px; padding: 10px 12px; border-radius: 10px; border: 1px solid ` + hex(colors.border) + `; color: ` + hex(colors.muted) + `; font-size: 15px; }
</style></head>
<body>
<h3>An inline page</h3>
<p>HTML from a string. It sends messages to the app and receives them.</p>
<button onclick="window.nsgo.postMessage('hello from the page')">Post to the app</button>
<div id="got">No message from the app yet.</div>
<script>window.addEventListener('message', (e) => { document.getElementById('got').textContent = 'From the app: ' + e.data; });</script>
</body></html>`
}

func mediaFrame(height core.Points, bg core.Color) core.Style {
	return core.Style{Height: core.Pt(height), CornerRadius: radiusControl, Overflow: core.Hidden, BorderWidth: 1, BorderColor: colors.border, Background: bg}
}

func messaging(cx *core.Scope) core.Node {
	message := core.NewSignal(cx, "")
	sent := core.NewSignal(cx, 0)
	web := &core.Ref{}
	or := func(s string) string {
		if s == "" {
			return "—"
		}
		return s
	}
	return card(cardProps{icon: iconMedia, title: "Web view", subtitle: "Inline HTML, with messages both ways"},
		core.WebView{Ref: web, Source: core.WebHTML(inlinePage(), ""), TestID: "bridge", Style: mediaFrame(210, colors.inset), OnMessage: message.Set},
		caption("From the page", func() string { return or(message.Get()) }),
		caption("To the page", func() string {
			if sent.Get() == 0 {
				return "—"
			}
			return fmt.Sprintf("hi #%d", sent.Get())
		}),
		button(buttonProps{label: "Post to the page"}, func() {
			sent.Set(int(time.Now().UnixMilli() % 1000))
			web.PostMessage(fmt.Sprintf("hi #%d", sent.GetUntracked()))
		}),
	)
}

// navButton is a round icon button, 44 pt.
func navButton(flip bool, enabled func() bool, label string, onPress func()) core.Node {
	s := fin.raised.over(core.Style{Width: core.Pt(44), Height: core.Pt(44), CornerRadius: 22, Background: colors.raised, BorderWidth: 1, BorderColor: colors.border,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter})
	if flip {
		s.Transform.Rotate = 180
	}
	return press(pressProps{onPress: onPress, disabled: func() bool { return !enabled() }, label: label, hitSlop: 4},
		core.View{Style: s, Children: []core.Node{icon(iconChevron, 18, colors.text)}})
}

func site(cx *core.Scope) core.Node {
	state := core.NewSignal(cx, core.WebNavigation{})
	ref := &core.Ref{}
	host := func() string {
		u := state.Get().URL
		u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
		if i := strings.Index(u, "/"); i >= 0 {
			u = u[:i]
		}
		if u == "" {
			return "…"
		}
		return u
	}
	return card(cardProps{icon: iconSearch, title: "A web site", subtitle: "Navigation state, back and forward",
		right: core.Dynamic(func(*core.Scope) core.Node {
			if state.Get().Loading {
				return badge("Loading", toneWarn)
			}
			return badge("Loaded", toneUp)
		})},
		core.View{Style: fin.track.over(core.Style{Height: core.Pt(40), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(0, 14),
			CornerRadius: 20, Background: colors.inset, BorderWidth: 1, BorderColor: colors.border}), Children: []core.Node{
			core.View{StyleFunc: func() core.Style {
				c := colors.up
				if state.Get().Loading {
					c = colors.warn
				}
				return core.Style{Width: core.Pt(7), Height: core.Pt(7), CornerRadius: 3.5, Background: c}
			}},
			core.Text{ContentFunc: host, MaxLines: 1, EllipsizeMode: core.EllipsizeMiddle, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 15, Color: colors.muted}},
		}},
		core.WebView{Ref: ref, Source: core.WebSrc("https://go.dev"), TestID: "site", Style: mediaFrame(340, colors.inset), OnNavigationStateChange: state.Set},
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
			navButton(true, func() bool { return state.Get().CanGoBack }, "Back", ref.GoBack),
			navButton(false, func() bool { return state.Get().CanGoForward }, "Forward", ref.GoForward),
			spacer(),
			button(buttonProps{label: "Reload", variant: secondary}, ref.Reload),
		}},
	)
}

// clockText is a duration as minutes and seconds.
func clockText(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// flowerClip is a five-second CC0 clip from MDN's examples.
const flowerClip = "https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4"

func player(cx *core.Scope) core.Node {
	controls := core.NewSignal(cx, "own")
	paused, buffering, failed := core.NewSignal(cx, true), core.NewSignal(cx, false), core.NewSignal(cx, false)
	pos, dur := core.NewSignal(cx, time.Duration(0)), core.NewSignal(cx, time.Duration(0))
	fit := core.NewSignal(cx, core.Contain)
	ref := &core.Ref{}
	return card(cardProps{icon: iconPlay, title: "Video", subtitle: "The platform's player",
		right: core.Dynamic(func(*core.Scope) core.Node {
			switch {
			case failed.Get():
				return badge("Error", toneDanger)
			case buffering.Get():
				return badge("Buffering", toneWarn)
			case paused.Get():
				return badge("Paused", toneNeutral)
			}
			return badge("Playing", toneAccent)
		})},
		core.Dynamic(func(*core.Scope) core.Node {
			// The platform's controls are fixed as the player mounts.
			sys := controls.Get() == "system"
			return core.Video{Ref: ref, Source: flowerClip, PausedFunc: paused.Get, Muted: true, Repeat: true, Controls: sys, ResizeModeFunc: fit.Get, TestID: "player",
				Style:      mediaFrame(200, core.RGB(0x000000)),
				OnLoad:     func(i core.VideoInfo) { dur.Set(i.Duration) },
				OnError:    func(error) { failed.Set(true) },
				OnBuffer:   buffering.Set,
				OnProgress: func(p, d time.Duration) { pos.Set(p); dur.Set(d) }}
		}),
		core.Show{When: func() bool { return controls.Get() == "own" }, Then: func(*core.Scope) core.Node {
			return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
				core.Dynamic(func(*core.Scope) core.Node {
					if paused.Get() {
						return roundButton(iconPlay, "Play", primary, 0, func() { paused.Set(false) })
					}
					return roundButton(iconPause, "Pause", primary, 0, func() { paused.Set(true) })
				}),
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 8}, Children: []core.Node{
					meter(func() float64 {
						if d := dur.Get(); d > 0 {
							return float64(pos.Get()) / float64(d)
						}
						return 0
					}, toneAccent, -1),
					core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
						core.Text{ContentFunc: func() string { return clockText(pos.Get()) }, Style: core.Style{FlexGrow: 1, FontSize: 13, Color: colors.muted}},
						textf(func() string { return clockText(dur.Get()) }, 13, regular, colors.faint),
					}},
				}},
				button(buttonProps{label: "Restart", variant: secondary}, func() { ref.Seek(0) }),
			}}
		}},
		segmented([]option[string]{{value: "own", label: "Own controls"}, {value: "system", label: "System controls"}}, controls.Get, controls.Set, false),
		segmented([]option[core.ResizeMode]{{value: core.Contain, label: "Contain"}, {value: core.Cover, label: "Cover"}, {value: core.Stretch, label: "Stretch"}}, fit.Get, fit.Set, true),
	)
}

// nativeView is a platform view of the app's own, registered with
// nsgo.Component and shown by a Native element: the platform's slider.
func nativeView(cx *core.Scope) core.Node {
	value := core.NewSignal(cx, 0.4)
	events := core.NewSignal(cx, "none yet")
	slider := &core.Ref{}
	_, ok := components[sliderComponent]
	noteText := "This platform registers no slider, so the box stays empty."
	if ok {
		noteText = "Dragging the platform's own slider reports change events; the buttons set its value and send it a command."
	}
	return card(cardProps{icon: iconControls, title: "Native views", subtitle: "The app's own platform views in the tree", right: badge("NSGo", toneNeutral)},
		core.Native{Component: sliderComponent, Ref: slider, TestID: "native-slider", Label: "Native slider",
			PropsFunc: func() map[string]any { return map[string]any{"value": value.Get()} },
			OnEvent: func(name string, data any) {
				if v, ok := data.(float64); ok && name == "change" {
					value.Set(v)
				}
				events.Set(fmt.Sprintf("%s %v", name, data))
			},
			Style: core.Style{AlignSelf: core.AlignStretch, MinWidth: core.Pt(200), MinHeight: core.Pt(28)}},
		note(noteText),
		caption("Value", func() string { return fmt.Sprintf("%.2f", value.Get()) }),
		core.Text{TestID: "native-value", ContentFunc: func() string { return "Last event: " + events.Get() }, Style: core.Style{FontSize: 12, FontFamily: mono, Color: colors.muted}},
		core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
			cell(button(buttonProps{label: "Set 0.8", variant: secondary}, func() { value.Set(0.8) })),
			cell(button(buttonProps{label: "Reset command", variant: secondary}, func() { slider.Command("reset", nil) })),
		}},
	)
}

// mediaDemo is the platform's hosted views: a web view with messages both
// ways, a site with its history, a video player and the app's own views.
func mediaDemo(cx *core.Scope) core.Node {
	return demoPage(cx, pageProps{}, messaging, site, player, nativeView)
}
