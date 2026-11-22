//go:build nsgodev

package dev

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	devlocales "github.com/nsgo-dev/nsgo/internal/dev/locales"
	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// Build is whether nsgo dev built the app.
const Build = true

// devServer is nsgo dev's server, set with -ldflags -X for targets that
// pass the app no environment; NSGO_DEV_SERVER overrides it.
var devServer = ""

// devHighlight is "1" in nsgo dev -highlight's builds, set with -ldflags -X.
var devHighlight = ""

// devLayout is what nsgo dev -layout's builds start the layout audit showing, as bounds,grid=8, set with -ldflags -X (parseDevLayout).
var devLayout = ""

// devServerURL is nsgo dev's server, "" when none runs the app, and whether the app runs in a web page the server served.
func devServerURL() (server string, page bool) {
	server = devServer
	if s := os.Getenv("NSGO_DEV_SERVER"); s != "" {
		server = s
	}
	if server == "" {
		return pageOrigin(), true
	}
	return server, false
}

// Start has the server print each commit under -trace, starts DevTools under -devtools and serves -automation's agent. nsgo.Run and nsgo.NewView call it before any graph starts, for an app's function, a root view or an embedded view, as all read what it sets; it starts them once, however many views or engines the process runs.
func Start() { start() }

var start = sync.OnceFunc(func() {
	server, page := devServerURL()
	if server == "" {
		return
	}
	if devTrace == "1" {
		traceTo(server)
	}
	if devProfile == "1" {
		go receiveRuntimeProfile(server)
	}
	if devDevtools == "1" {
		startDevTools(server)
	}
	if !page {
		startAutomation(server)
	}
})

// Wrap runs app under nsgo dev: build failures the server reports and the app's own render failures show in an overlay over it, in the language the app shows, and with -trace the server prints each commit. On the web the page is served from the server and shows build failures itself, in the words the app gives it in that language (showPageMessages).
func Wrap(app engine.App) engine.App {
	server, page := devServerURL()
	if server == "" {
		return app
	}
	Start()
	if devHighlight == "1" || devDevtools == "1" || devLayout != "" {
		app = withDevOverlay(app, devHighlight == "1", parseDevLayout(devLayout))
	}
	if page {
		return func(cx *core.Scope) core.Node {
			cx.Effect(func() { showPageMessages(devlocales.For(cx)) })
			return core.Func(app)
		}
	}
	return func(cx *core.Scope) core.Node {
		failure := core.NewSignal(cx, devFailure{})
		go listenDev(server, func(f devFailure) { cx.Post(func() { failure.Set(f) }) })
		reload := reloadFunc(server)
		// Command or Control+R builds and starts the app again, as the overlay's Reload button does, from every window and panel of the app, over the overlay and the app's own modals too. It leaves the key to the app: bound before the app renders, and in every other window before that window's own bindings, it yields to the app's own binding of the key while that one may fire, and to an enabled item of the application menu with the key as its shortcut.
		cx.OnKeyWith("mod+r", core.KeyOptions{OverModals: true, YieldsToMenu: true, AllSurfaces: true}, reload)
		return core.Group{
			core.Catch{
				Try: app,
				Fallback: func(cx *core.Scope, err error, reset func()) core.Node {
					return devOverlay(cx, devFailure{kind: devAppFailed, Text: err.Error()}, reset, reload)
				},
			},
			core.Show{When: func() bool { return failure.Get().Text != "" }, Then: func(cx *core.Scope) core.Node {
				return devOverlay(cx, failure.Get(), func() { failure.Set(devFailure{}) }, reload)
			}},
		}
	}
}

// WrapApp runs an app function under nsgo dev. Its reload binding belongs to the app's own scope, which outlives the main view when another window keeps the app running after that view closes.
func WrapApp(app func(*core.Scope)) func(*core.Scope) {
	server, page := devServerURL()
	if server == "" || page {
		return app
	}
	Start()
	return func(cx *core.Scope) {
		cx.OnKeyWith("mod+r", core.KeyOptions{OverModals: true, YieldsToMenu: true, AllSurfaces: true}, reloadFunc(server))
		app(cx)
	}
}

func reloadFunc(server string) func() {
	return func() {
		go func() {
			if resp, err := devtools.Fetch("POST", server+"/_nsgo/reload", "", nil); err == nil {
				resp.Body.Close()
			}
		}()
	}
}

// devFailureKind is what failed, which picks the overlay's title.
type devFailureKind uint8

const (
	devAppFailed devFailureKind = iota + 1
	devBuildFailed
)

// devFailure is what the overlay shows: what failed and its text, "" for none.
type devFailure struct {
	kind devFailureKind
	Text string
}

// listenDev follows the server's events, reconnecting while it runs: a
// failed build's output, and its clearing.
func listenDev(server string, report func(devFailure)) {
	for {
		resp, err := devtools.Fetch("GET", server+"/_nsgo/events", "", nil)
		if err == nil {
			sc := bufio.NewScanner(resp.Body)
			sc.Buffer(make([]byte, 1<<16), 16<<20)
			for sc.Scan() {
				data, ok := strings.CutPrefix(sc.Text(), "data: ")
				if !ok {
					continue
				}
				switch kind, rest, _ := strings.Cut(data, " "); kind {
				case "error":
					if text, err := strconv.Unquote(rest); err == nil {
						report(devFailure{kind: devBuildFailed, Text: text})
					}
				case "ok":
					report(devFailure{})
				}
			}
			resp.Body.Close()
		}
		time.Sleep(time.Second)
	}
}

// devOverlay covers the app with what failed: a centered panel of its title, the text in a box that scrolls, and buttons to copy the text and to build again, their words in the language the app shows.
func devOverlay(cx *core.Scope, f devFailure, dismiss, reload func()) core.Node {
	// The words are bindings, so they follow the language the app shows as it changes.
	title := func() string {
		if f.kind == devBuildFailed {
			return devlocales.BuildFailed(cx)
		}
		return devlocales.AppFailed(cx)
	}
	label := core.Style{Color: core.RGB(0xf2f2f2), FontSize: 13}
	button := core.Style{Padding: core.Symmetric(6, 14), CornerRadius: 6, Background: core.RGBA(0xffffff, 0.12)}
	return core.Modal{Animation: core.ModalNone, OnRequestClose: func(string) { dismiss() }, TestID: "nsgo-dev-overlay",
		Children: []core.Node{core.View{
			Style: core.Style{FlexGrow: 1, Padding: core.Symmetric(48, 24), JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter,
				Background: core.RGBA(0x111111, 0.6)},
			Children: []core.Node{core.View{
				Style: core.Style{Width: core.Pct(100), MaxWidth: core.Pt(720), MaxHeight: core.Pct(100), FlexShrink: 1, Padding: core.All(20), Gap: 16,
					CornerRadius: 12, Background: core.RGB(0x1c1c1c), BorderWidth: 1, BorderColor: core.RGBA(0xffffff, 0.1)},
				Children: []core.Node{
					core.Text{ContentFunc: title, Style: core.Style{Color: core.RGB(0xff5c5c), FontSize: 18, FontWeight: core.FontWeightBold}},
					core.View{Style: core.Style{MaxHeight: core.Pt(360), FlexShrink: 1, CornerRadius: 8, Background: core.RGBA(0x000000, 0.35)},
						Children: []core.Node{core.Scroll{Style: core.Style{FlexShrink: 1}, Children: []core.Node{
							core.View{Style: core.Style{Padding: core.All(12)}, Children: []core.Node{
								core.Text{Content: f.Text, Style: core.Style{Color: label.Color, FontSize: 12, FontFamily: core.FontMonospace}},
							}},
						}}}},
					core.View{Style: core.Style{Direction: core.Row, Gap: 8, JustifyContent: core.JustifyEnd}, Children: []core.Node{
						core.View{Style: button, Role: core.RoleButton, OnPress: func(core.PressEvent) { cx.SetClipboard(f.Text) },
							Children: []core.Node{core.Text{ContentFunc: func() string { return devlocales.Copy(cx) }, Style: label}}},
						core.View{Style: button, Role: core.RoleButton, OnPress: func(core.PressEvent) { reload() },
							Children: []core.Node{core.Text{ContentFunc: func() string { return devlocales.Reload(cx) }, Style: label}}},
					}},
				},
			}},
		}}}
}
