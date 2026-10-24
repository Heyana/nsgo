package main

import (
	"fmt"

	"github.com/oneo-me/nsgo/core"
)

// bridgePage talks to the app both ways: its button posts, and messages
// from the app replace its text.
const bridgePage = `<!doctype html>
<html><head><meta name="viewport" content="width=device-width">
<title>Bridge</title>
<style>
  body { font: 15px -apple-system, system-ui, sans-serif; margin: 12px; color: #333; }
  button { font: inherit; padding: 6px 12px; }
  @media (prefers-color-scheme: dark) { body { color: #ddd; } }
</style></head>
<body>
  <p id="from-app">No message from the app yet.</p>
  <button onclick="window.nsgo.postMessage('clicked ' + (++window.clicks))">Post to the app</button>
  <script>
    window.clicks = 0;
    addEventListener('message', (e) => { document.getElementById('from-app').textContent = 'The app said: ' + e.data; });
  </script>
</body></html>`

func webPage(cx *core.Scope, t *theme) core.Node {
	var bridge, site core.Ref
	received := core.NewSignal(cx, "nothing yet")
	nav := core.NewSignal(cx, core.WebNavigation{})
	sent := 0
	frame := core.Style{Height: core.Pt(220), CornerRadius: 6, Overflow: core.Hidden, Background: t.sunken,
		BorderWidth: 1, BorderColor: t.line}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("HTML and messages", "Inline HTML; the page posts with window.nsgo.postMessage and the app answers with Ref.PostMessage.",
			core.WebView{Source: core.WebHTML(bridgePage, ""), Ref: &bridge, TestID: "bridge", Style: frame,
				OnMessage: func(data string) { received.Set(data) }},
			hstack(
				t.primary("Post to the page", func() { sent++; bridge.PostMessage(fmt.Sprintf("hello #%d", sent)) }),
				t.plain("Run a script", func() { bridge.InjectJavaScript("document.body.style.background = '#fde68a'") }),
			),
			t.live(func() string { return "The page said: " + received.Get() }),
		),
		t.demo("A site", "A URL with its history: back, forward and reload follow the page's navigation. On the web, a site's history stays the browser's.",
			core.WebView{Source: core.WebSrc("https://en.m.wikipedia.org/wiki/Go_(programming_language)"), Ref: &site, TestID: "site",
				Style:                   core.Style{Height: core.Pt(320), CornerRadius: 6, Overflow: core.Hidden, BorderWidth: 1, BorderColor: t.line},
				OnNavigationStateChange: nav.Set,
				OnError:                 func(err error) { received.Set("error: " + err.Error()) }},
			hstack(
				t.plain("Back", site.GoBack),
				t.plain("Forward", site.GoForward),
				t.plain("Reload", site.Reload),
			),
			t.live(func() string {
				n := nav.Get()
				state := "loaded"
				if n.Loading {
					state = "loading"
				}
				return fmt.Sprintf("%s — %s (%s, back %t, forward %t)", n.Title, n.URL, state, n.CanGoBack, n.CanGoForward)
			}),
		),
	}}
}
