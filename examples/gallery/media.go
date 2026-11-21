package main

import (
	"encoding/json"
	"fmt"
	"html"
	"slices"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/icons/lucide"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"

	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/ui"
)

func canMutePage(cx *core.Scope) bool {
	env := cx.Env()
	if !env.HasWebView() || env.Simulated() {
		return false
	}
	switch env.OS() {
	case core.OSWindows, core.OSLinux, core.OSIOS:
		return true
	case core.OSMacOS:
		return env.OSVersion().Major >= 12
	}
	return false
}

// inlinePage is an inline page in the gallery's colors, for the
// appearance shown; as in NSH, a page source that reads it loads again
// when they change.
func inlinePage(cx *core.Scope) string {
	colors := ui.ColorsOf(cx).Now()
	isDark := cx.Dark()
	hex := func(c core.Color) string {
		r, g, b, a := c.Components()
		return fmt.Sprintf("rgba(%d,%d,%d,%.3f)", int(r*255+0.5), int(g*255+0.5), int(b*255+0.5), a)
	}
	shine := 0.6
	if isDark {
		shine = 0.45
	}
	var tone string
	if canMutePage(cx) {
		tone = `<button id="tone" aria-pressed="false">` + html.EscapeString(locales.For(cx).MediaWebTone()) + `</button>`
	}
	return `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1">
<style>
  html { -webkit-text-size-adjust: 100%; }
  body { margin: 0; padding: 16px; background: ` + hex(colors.Inset) + `; color: ` + hex(colors.Text) + `; font-family: -apple-system, system-ui, sans-serif; font-size: 15px; line-height: 1.4; }
  h3 { margin: 0 0 4px; font-size: 17px; font-weight: 600; }
  p { margin: 0 0 12px; color: ` + hex(colors.Muted) + `; font-size: 15px; }
  button { height: 44px; padding: 0 18px; border: 0; border-radius: 10px; background: linear-gradient(180deg, #d6f78a 0%, #c1ea5c 100%); color: ` + hex(colors.OnAccent) + `; font-family: inherit; font-size: 15px; font-weight: 600; box-shadow: inset 0 1px 0 rgba(255,255,255,` + fmt.Sprint(shine) + `); }
  a { margin-left: 12px; color: inherit; }
  #tone { margin-left: 12px; }
  #tone[aria-pressed=true] { outline: 2px solid currentColor; outline-offset: -4px; }
  #got { margin-top: 12px; padding: 10px 12px; border-radius: 10px; border: 1px solid ` + hex(colors.Border) + `; color: ` + hex(colors.Muted) + `; font-size: 15px; }
</style></head>
<body>
<h3>An inline page</h3>
<p>HTML from a string. It sends messages to the app and receives them.</p>
<button id="post" onclick="window.nsgo.postMessage(event.isTrusted ? 'hello from the page' : 'an untrusted click')">Post to the app</button>
<a id="save" download="note.txt" href="data:text/plain,Saved%20from%20the%20page">Save a note</a>
<a target="_blank" href="https://nsgo.dev/">` + html.EscapeString(locales.For(cx).MediaNewWindow()) + `</a>
` + tone + `
<div id="got">No message from the app yet.</div>
<script>
window.addEventListener('message', (e) => { document.getElementById('got').textContent = 'From the app: ' + e.data; });
const tone = document.getElementById('tone');
if (tone) {
  let context;
  tone.addEventListener('click', () => {
    if (context) {
      context.close();
      context = undefined;
    } else {
      context = new AudioContext();
      const oscillator = context.createOscillator();
      const gain = context.createGain();
      oscillator.frequency.value = 220;
      gain.gain.value = 0.025;
      oscillator.connect(gain).connect(context.destination);
      oscillator.start();
    }
    tone.setAttribute('aria-pressed', Boolean(context));
  });
}
// Where the button and the link are, for the app's trusted clicks.
const at = (id) => { const r = document.getElementById(id).getBoundingClientRect(); return [r.left + r.width / 2, r.top + r.height / 2]; };
window.nsgo.postMessage('at:' + JSON.stringify({ post: at('post'), save: at('save') }));
</script>
</body></html>`
}

// mediaFrame is a media view's frame, for a StyleFunc.
func mediaFrame(cx *core.Scope, height core.Points, bg core.Color) core.Style {
	return core.Style{Height: core.Pt(height), CornerRadius: ui.RadiusControl, Overflow: core.Hidden, BorderWidth: 1, BorderColor: ui.ColorsOf(cx).Border(), Background: bg}
}

func messaging(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	colors := ui.ColorsOf(cx)
	message := core.NewSignal(cx, "")
	sent := core.NewSignal(cx, 0)
	saved := core.NewSignal(cx, "")
	result := core.NewSignal(cx, "")
	pageState := core.NewSignal(cx, core.WebNavigation{})
	muted := core.NewSignal(cx, false)
	var mute core.Node
	if canMutePage(cx) {
		mute = ui.Checkbox{Text: tr.MediaMutePage(), Checked: muted.Get, OnChange: muted.Set}
	}
	// Desktop native hosts serve the same page through an app-owned scheme; targets without that bridge, a web build that runs as a desktop OS included, keep the inline HTML source.
	schemePage := inlinePage(cx)
	_ = cx.ServeScheme("gallery", func(url string) []byte {
		if url == "gallery://inline/index.html" {
			return []byte(schemePage)
		}
		return nil
	})
	source := func() core.WebSource {
		if desktop.IsAvailable(cx) {
			return core.WebSrc("gallery://inline/index.html")
		}
		return core.WebHTML(inlinePage(cx), "")
	}
	// Where the page's button and link are, which it posts as it loads.
	var at struct{ Post, Save [2]core.Points }
	positioned := core.NewSignal(cx, false)
	web := &core.Ref{}
	got := func(data string) {
		if rest, ok := strings.CutPrefix(data, "at:"); ok {
			if json.Unmarshal([]byte(rest), &at) == nil {
				positioned.Set(true)
			}
			return
		}
		message.Set(data)
	}
	download := func(d core.WebDownload) {
		switch d.State {
		case core.WebDownloadFinished:
			saved.Set(fmt.Sprintf("%s, %d B", d.Filename, d.Bytes))
		case core.WebDownloadFailed:
			saved.Set(d.Message)
		}
	}
	or := func(s string) string {
		if s == "" {
			return "—"
		}
		return s
	}
	return Card{Title: tr.MediaWebView(), Description: tr.MediaInlineHtmlWithMessagesBoth(), Children: []core.Node{
		core.WebView{Ref: web, SourceFunc: source, Headers: map[string]string{"X-NSGo-Gallery": "webview"}, Incognito: true, MutedFunc: muted.Get, TestID: "bridge", StyleFunc: func() core.Style { return mediaFrame(cx, 210, colors.Inset()) }, OnMessage: got, OnDownload: download, OnNavigationStateChange: pageState.Set,
			OnOpenWindow: func(r core.WebWindowRequest) {
				if r.UserInitiated {
					message.Set(r.URL)
				}
			}},
		Caption{Label: tr.MediaPageState(), Note: func() string {
			page := pageState.Get()
			if page.Title == "" {
				return or(page.URL)
			}
			return page.Title + " · " + page.URL
		}},
		mute,
		Caption{Label: tr.MediaFromThePage(), Note: func() string { return or(message.Get()) }},
		Caption{Label: tr.MediaToThePage(), Note: func() string {
			if sent.Get() == 0 {
				return "—"
			}
			return fmt.Sprintf("hi #%d", sent.Get())
		}},
		Caption{Label: tr.MediaSaved(), Note: func() string { return or(saved.Get()) }},
		Caption{Label: tr.MediaScriptResult(), Note: func() string { return or(result.Get()) }},
		ui.Wrap{Children: []core.Node{
			ui.Button{Text: tr.MediaPostToThePage(), OnPress: func() {
				sent.Set(int(time.Now().UnixMilli() % 1000))
				web.PostMessage(fmt.Sprintf("hi #%d", sent.GetUntracked()))
			}},
			ui.Button{Text: tr.MediaChangePageState(), Variant: ui.ButtonSecondary, TestID: "change-page-state", OnPress: func() {
				title, _ := json.Marshal(tr.MediaPreviewTitle())
				web.InjectJavaScript(`history.pushState({}, "", "#preview"); document.title = ` + string(title))
			}},
			ui.Button{Text: tr.MediaClickThePage(), Variant: ui.ButtonSecondary, TestID: "click-page",
				Disabled: func() bool { return !positioned.Get() },
				OnPress:  func() { web.SendInput(core.WebClick(at.Post[0], at.Post[1])) }},
			ui.Button{Text: tr.MediaSaveANote(), Variant: ui.ButtonSecondary, TestID: "save-note",
				Disabled: func() bool { return !positioned.Get() },
				OnPress:  func() { web.SendInput(core.WebClick(at.Save[0], at.Save[1])) }},
			// A promise the page settles, answered as JSON.
			ui.Button{Text: tr.MediaRunAScript(), Variant: ui.ButtonSecondary, TestID: "run-script", OnPress: func() {
				web.Evaluate("Promise.resolve(document.body.childElementCount).then(n => ({elements: n > 0, sum: 6 * 7}))",
					func(json string, err error) {
						if err != nil {
							result.Set(err.Error())
							return
						}
						result.Set(json)
					})
			}},
		}},
	}}
}

// navButton is a round icon button, 44 pt.
func navButton(cx *core.Scope, flip bool, enabled func() bool, label string, onPress func()) core.Node {
	colors := ui.ColorsOf(cx)
	fin := ui.FinOf(cx)
	s := func() core.Style {
		s := fin.Raised().Over(core.Style{Width: core.Pt(44), Height: core.Pt(44), CornerRadius: 22, Background: colors.Raised(), BorderWidth: 1, BorderColor: colors.Border(),
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter})
		if flip {
			s.Transform.Rotate = 180
		}
		return s
	}
	return ui.Press{OnPress: onPress, Disabled: func() bool { return !enabled() }, Label: label, HitSlop: 4, Children: []core.Node{
		core.View{StyleFunc: s, Children: []core.Node{ui.Icon(lucide.ChevronRight, 18, colors.Text)}},
	}}
}

// docsHome is NSGo's docs on nsgo.dev, in the language the gallery shows.
func docsHome(cx *core.Scope) string {
	if locales.Catalog.Match(i18n.Language(cx)) == "zh-Hans" {
		return "https://nsgo.dev/zh/docs/"
	}
	return "https://nsgo.dev/docs/"
}

func site(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	colors := ui.ColorsOf(cx)
	fin := ui.FinOf(cx)
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
	return Card{Title: tr.MediaAWebSite(), Description: tr.MediaNavigationStateBackAndForward(), Actions: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
		if state.Get().Loading {
			return ui.Badge{Text: tr.MediaLoading(), Variant: ui.BadgeWarn}
		}
		return ui.Badge{Text: tr.MediaLoaded(), Variant: ui.BadgeSuccess}
	})}, Children: []core.Node{
		core.View{StyleFunc: func() core.Style {
			return fin.Track().Over(core.Style{Height: core.Pt(40), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(0, 14),
				CornerRadius: 20, Background: colors.Inset(), BorderWidth: 1, BorderColor: colors.Border()})
		}, Children: []core.Node{
			core.View{StyleFunc: func() core.Style {
				c := colors.Up
				if state.Get().Loading {
					c = colors.Warn
				}
				return core.Style{Width: core.Pt(7), Height: core.Pt(7), CornerRadius: 3.5, Background: c()}
			}},
			core.Text{ContentFunc: host, MaxLines: 1, EllipsizeMode: core.EllipsizeMiddle, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 13}, StyleFunc: ui.Colored(colors.Muted)},
		}},
		core.WebView{Ref: ref, SourceFunc: func() core.WebSource { return core.WebSrc(docsHome(cx)) }, TestID: "site", StyleFunc: func() core.Style { return mediaFrame(cx, 340, colors.Inset()) }, OnNavigationStateChange: state.Set},
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
			navButton(cx, true, func() bool { return state.Get().CanGoBack }, tr.MediaBack(), ref.GoBack),
			navButton(cx, false, func() bool { return state.Get().CanGoForward }, tr.MediaForward(), ref.GoForward),
			ui.Button{Text: tr.MediaHome(), Variant: ui.ButtonSecondary, OnPress: func() { ref.Load(core.WebSrc(docsHome(cx))) }},
			ui.Spacer{},
			ui.Button{Text: tr.MediaReload(), Variant: ui.ButtonSecondary, OnPress: ref.Reload},
		}},
	}}
}

// cookieStore sets an HttpOnly cookie for NSGo's docs in the store every web view shares, which the site's next load sends and its scripts cannot read, lists the store's cookies for the site and clears the store, as the platform keeps it whether or not a web view shows.
func cookieStore(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	colors := ui.ColorsOf(cx)
	cookies := core.NewSignalFunc(cx, []core.WebCookie{}, slices.Equal)
	failure := core.NewSignal(cx, "")
	answer := func(err error) bool {
		if err != nil {
			failure.Set(err.Error())
			return false
		}
		failure.Set("")
		return true
	}
	list := func() {
		cx.WebCookies(core.WebCookieFilter{URL: docsHome(cx)}, func(c []core.WebCookie, err error) {
			if answer(err) {
				cookies.Set(c)
			}
		})
	}
	set := func() {
		cx.SetWebCookies([]core.WebCookie{{Name: "nsgo-gallery", Value: cx.WallTime().Format("150405"), Domain: "nsgo.dev", Expires: cx.WallTime().Add(time.Hour), Secure: true, HTTPOnly: true}},
			func(err error) {
				if answer(err) {
					list()
				}
			})
	}
	clear := func() {
		cx.ClearWebsiteData(func(err error) {
			if answer(err) {
				list()
			}
		})
	}
	row := func(cx *core.Scope, c func() core.WebCookie, _ func() int) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
			core.Text{ContentFunc: func() string { return c().Name + "=" + c().Value }, MaxLines: 1, EllipsizeMode: core.EllipsizeMiddle, Style: core.Style{FlexShrink: 1, FontSize: 13}, StyleFunc: ui.Colored(colors.Text)},
			core.Text{ContentFunc: func() string { return c().Domain + c().CookiePath() }, MaxLines: 1, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 13}, StyleFunc: ui.Colored(colors.Muted)},
			core.Show{When: func() bool { return c().HTTPOnly }, Then: func(*core.Scope) core.Node { return ui.Badge{Text: "HttpOnly"} }},
		}}
	}
	return Card{Title: tr.MediaCookies(), Description: tr.MediaTheStoreEveryWebView(), Actions: []core.Node{
		ui.Badge{TextFunc: func() string { return tr.MediaCookieCount(len(cookies.Get())) }, Variant: ui.BadgeAccent},
	}, Children: []core.Node{
		core.View{TestID: "cookies", Style: core.Style{Gap: 6}, Children: []core.Node{
			core.Each(cookies.Get, func(c core.WebCookie) string { return c.Name + " " + c.Domain + c.CookiePath() }, row),
		}},
		core.Show{When: func() bool { return failure.Get() != "" }, Then: func(*core.Scope) core.Node { return Note{TextFunc: failure.Get} }},
		core.View{Style: core.Style{Direction: core.Row, Gap: 8, Wrap: core.Wrap}, Children: []core.Node{
			ui.Button{Text: tr.MediaSetACookie(), Variant: ui.ButtonSecondary, OnPress: set},
			ui.Button{Text: tr.MediaList(), Variant: ui.ButtonSecondary, OnPress: list},
			ui.Button{Text: tr.MediaClearWebsiteData(), Variant: ui.ButtonOutline, OnPress: clear},
		}},
	}}
}

// clockText is a duration as minutes and seconds.
func clockText(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// flowerClip is a five-second CC0 clip from MDN's examples.
const flowerClip = "https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4"

func player(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	colors := ui.ColorsOf(cx)
	controls := core.NewSignal(cx, "own")
	paused, buffering, failed := core.NewSignal(cx, true), core.NewSignal(cx, false), core.NewSignal(cx, false)
	pos, dur := core.NewSignal(cx, time.Duration(0)), core.NewSignal(cx, time.Duration(0))
	fit := core.NewSignal(cx, core.Contain)
	ref := &core.Ref{}
	return Card{Title: tr.MediaVideo(), Description: tr.MediaThePlatformSPlayer(), Actions: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
		switch {
		case failed.Get():
			return ui.Badge{Text: tr.MediaError(), Variant: ui.BadgeDanger}
		case buffering.Get():
			return ui.Badge{Text: tr.MediaBuffering(), Variant: ui.BadgeWarn}
		case paused.Get():
			return ui.Badge{Text: tr.MediaPaused()}
		}
		return ui.Badge{Text: tr.MediaPlaying(), Variant: ui.BadgeAccent}
	})}, Children: []core.Node{
		core.Dynamic(func(*core.Scope) core.Node {
			// The platform's controls are fixed as the player mounts.
			sys := controls.Get() == "system"
			return core.Video{Ref: ref, Source: flowerClip, PausedFunc: paused.Get, Muted: true, Repeat: true, Controls: sys, ResizeModeFunc: fit.Get, TestID: "player",
				StyleFunc:  func() core.Style { return mediaFrame(cx, 200, core.RGB(0x000000)) },
				OnLoad:     func(i core.VideoInfo) { dur.Set(i.Duration) },
				OnError:    func(error) { failed.Set(true) },
				OnBuffer:   buffering.Set,
				OnProgress: func(p, d time.Duration) { pos.Set(p); dur.Set(d) }}
		}),
		core.Show{When: func() bool { return controls.Get() == "own" }, Then: func(*core.Scope) core.Node {
			return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
				core.Dynamic(func(*core.Scope) core.Node {
					if paused.Get() {
						return ui.IconButton{Variant: ui.IconButtonPrimary, Icon: lucide.Play, Label: tr.MediaPlay(), OnPress: func() { paused.Set(false) }}
					}
					return ui.IconButton{Variant: ui.IconButtonPrimary, Icon: lucide.Pause, Label: tr.MediaPause(), OnPress: func() { paused.Set(true) }}
				}),
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 8}, Children: []core.Node{
					ui.Meter{Value: func() float64 {
						if d := dur.Get(); d > 0 {
							return float64(pos.Get()) / float64(d)
						}
						return 0
					}, Variant: ui.MeterAccent},
					core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
						core.Text{ContentFunc: func() string { return clockText(pos.Get()) }, Style: core.Style{FlexGrow: 1, FontSize: 13}, StyleFunc: ui.Colored(colors.Muted)},
						ui.Textf(func() string { return clockText(dur.Get()) }, 13, ui.Regular, colors.Faint),
					}},
				}},
				ui.Button{Text: tr.MediaRestart(), Variant: ui.ButtonSecondary, OnPress: func() { ref.Seek(0) }},
			}}
		}},
		ui.Tabs[string]{Equal: true, Options: []ui.Option[string]{{Value: "own", Label: tr.MediaOwnControls()}, {Value: "system", Label: tr.MediaSystemControls()}}, Value: controls.Get, OnChange: controls.Set},
		ui.Tabs[core.ResizeMode]{Equal: true, Options: []ui.Option[core.ResizeMode]{{Value: core.Contain, Label: "Contain"}, {Value: core.Cover, Label: "Cover"}, {Value: core.Stretch, Label: "Stretch"}}, Value: fit.Get, OnChange: fit.Set},
	}}
}

// nativeView is a platform view of the app's own, registered with
// nsgo.Component and shown by a Native element: the platform's slider.
func nativeView(cx *core.Scope) core.Node {
	tr := locales.For(cx)
	value := core.NewSignal(cx, 0.4)
	slider := &core.Ref{}
	_, ok := components[sliderComponent]
	noteText := tr.MediaThisPlatformRegistersNoSlider()
	if ok {
		noteText = tr.MediaDraggingThePlatformSOwn()
	}
	return Card{Title: tr.MediaNativeViews(), Description: tr.MediaTheAppSOwnPlatform(), Actions: []core.Node{ui.Badge{Text: "NSGo"}}, Children: []core.Node{
		core.Native{Component: sliderComponent, Ref: slider, TestID: "native-slider", Label: tr.MediaNativeSlider(),
			PropsFunc: func() map[string]any { return map[string]any{"value": value.Get()} },
			OnEvent: func(name string, data any) {
				if v, ok := data.(float64); ok && name == "change" {
					value.Set(v)
				}
			},
			Style: core.Style{AlignSelf: core.AlignStretch, MinWidth: core.Pt(200), MinHeight: core.Pt(28)}},
		Note{Text: noteText},
		Caption{Label: tr.MediaValue(), Note: func() string { return fmt.Sprintf("%.2f", value.Get()) }},
		core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
			ui.Cell{Child: ui.Button{Text: tr.MediaSet08(), Variant: ui.ButtonSecondary, OnPress: func() { value.Set(0.8) }}},
			ui.Cell{Child: ui.Button{Text: tr.MediaResetCommand(), Variant: ui.ButtonSecondary, OnPress: func() { slider.Command("reset", nil) }}},
		}},
	}}
}

// webViewDemo is the platform's hosted views: a web view with messages both ways, NSGo's docs with their history, the store every web view shares and the app's own views.
func webViewDemo(cx *core.Scope) core.Node {
	return demoPage(cx, pageProps{}, messaging, site, cookieStore, nativeView)
}

// videoDemo is the platform's video player.
func videoDemo(cx *core.Scope) core.Node {
	return demoPage(cx, pageProps{}, player)
}
