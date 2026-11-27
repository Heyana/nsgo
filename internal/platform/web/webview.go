//go:build js && wasm

package web

import (
	"cmp"
	"math/rand/v2"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// webFrame is a web view's <iframe>, as NSH's web backend has it. HTML loads as srcdoc in a sandbox with an origin of its own, so it never reaches the app's page, with a bridge script first: it hands the app a MessagePort with token, which only the srcdoc's document knows, and window.nsgo.postMessage writes to the port, which also runs injected scripts and history steps. The app takes the port only from the frame's own window, so neither a frame within the page nor a page it went to has the bridge, and the page's messages report the source's origin. Restricted views omit allow-popups, allow-popups-to-escape-sandbox and allow-downloads from the sandbox, decode data:text/html sources through the same guarded srcdoc path, add a CSP form-action policy, strip disallowed meta refreshes (and static links when JavaScript is disabled), and enforce WebConfig.Navigable in the bridge before page scripts run. A URL loads as it is (without its origin when it is the app's own); the browser hides what a cross-origin page does after it loads, so it only gets PostMessage, load events and the iframe sandbox's restrictions.
type webFrame struct {
	frame          js.Value
	frameListeners listeners
	config         driver.WebConfig
	source         driver.WebSource // what the frame loaded last: the config's, or Ref.Load's
	loaded         bool             // configured at least once
	token          string
	exposeBridge   bool     // whether the page exposes window.nsgo and accepts eval/history commands
	lastSrcdoc     string   // sanitized restricted srcdoc last installed, for restoring after an unverified navigation
	loadedOnce     bool     // whether the current document finished its initial load
	restoring      bool     // suppressing duplicate load events while restoring after an unverified navigation
	uncheckedCount int      // consecutive unverified-navigation restores since the last clean load
	port           js.Value // the app's end of the bridge of the source's page, while that page shows
	portFn         js.Func
	// bridging is set while the source's page loads, which hands its port; queued are the commands for it until then.
	bridging bool
	queued   []map[string]any
	url      string
	title    string
	warned   bool // that commands do not reach the page
}

// webBridge is the script an HTML source starts with: the port it hands the app queues what the page posts until the app takes it. As the page leaves, for a link or a script setting location, it posts null, which postMessage never does, so the app drops the port, whose other end goes with the page.
func webBridge(token string) string {
	return `(() => {
  const channel = new MessageChannel(), port = channel.port1;
  window.nsgo = { postMessage(data) { port.postMessage(String(data)); } };
  port.onmessage = (m) => {
    const d = m.data;
    if (d && d.eval != null) { try { (0, eval)(String(d.eval)); } catch (err) { console.error(err); } }
    else if (d && d.go) history.go(d.go);
  };
  addEventListener("pagehide", (e) => { if (!e.persisted) port.postMessage(null); });
  parent.postMessage(` + strconv.Quote(token) + `, "*", [channel.port2]);
})();`
}

func (s *surface) createWeb(v *view) {
	w := &webFrame{frame: s.element("iframe")}
	v.web = w
	w.frame.Call("setAttribute", "allowtransparency", "true")
	setStyle(w.frame, map[string]string{
		"width": "100%", "height": "100%", "border": "0", "background": "transparent", "border-radius": "inherit",
	})
	v.inner = w.frame
	v.el.Call("appendChild", w.frame)
	w.frameListeners.on(w.frame, "load", func(js.Value) { s.webLoaded(v) })
	v.listeners.on(js.Global(), "message", func(e js.Value) { s.webBridged(v, e) })
}

// replaceWebFrame swaps in a fresh <iframe> so any in-flight navigation on the
// previous browsing context is discarded before restoring srcdoc or about:blank.
func (s *surface) replaceWebFrame(v *view) {
	w := v.web
	old := w.frame
	w.frameListeners.release()
	fresh := s.element("iframe")
	for _, attr := range []string{"allowtransparency", "sandbox", "allow", "scrolling"} {
		if old.Call("hasAttribute", attr).Bool() {
			fresh.Call("setAttribute", attr, old.Call("getAttribute", attr))
		}
	}
	setStyle(fresh, map[string]string{
		"width": "100%", "height": "100%", "border": "0", "background": "transparent", "border-radius": "inherit",
	})
	w.frame = fresh
	v.inner = fresh
	old.Call("replaceWith", fresh)
	w.frameListeners.on(fresh, "load", func(js.Value) { s.webLoaded(v) })
}

// webBridged takes the port an HTML page's bridge hands the app: from the frame's own window, with the page's token.
func (s *surface) webBridged(v *view, e js.Value) {
	w := v.web
	port := e.Get("ports").Index(0)
	if w.token == "" || !e.Get("source").Equal(w.frame.Get("contentWindow")) || e.Get("data").Type() != js.TypeString ||
		e.Get("data").String() != w.token || !port.Truthy() {
		return
	}
	e.Call("stopImmediatePropagation")
	w.closePort()
	w.port = port
	// The page is the srcdoc, at its base URL when it has one.
	origin, page, expose := w.source.Origin(), cmp.Or(w.source.BaseURL, cmp.Or(w.source.Src, "about:srcdoc")), w.exposeBridge
	w.portFn = js.FuncOf(func(_ js.Value, args []js.Value) any {
		data := args[0].Get("data")
		switch data.Type() {
		case js.TypeString:
			if expose {
				s.send(v, driver.Event{Kind: driver.EventMessage, Text: data.String(), Origin: origin, URL: page, MainFrame: true})
			}
		case js.TypeObject:
			if !data.IsNull() {
				if refused := data.Get("refused"); refused.Type() == js.TypeString {
					url := refused.String()
					s.send(v, driver.Event{Kind: driver.EventError, Text: driver.RefusedNavigation(url),
						Navigation: &driver.WebNavigation{URL: url}})
					return nil
				}
				if openURL := data.Get("openWindow"); openURL.Type() == js.TypeString {
					req := driver.WebWindowRequest{
						URL:           openURL.String(),
						UserInitiated: data.Get("userInitiated").Bool(),
					}
					s.send(v, w.config.OpenWindowEvent(req))
					return nil
				}
				if navURL := data.Get("navigate"); navURL.Type() == js.TypeString {
					dest := navURL.String()
					if !w.config.Navigable(dest) {
						s.send(v, driver.Event{Kind: driver.EventError, Text: driver.RefusedNavigation(dest),
							Navigation: &driver.WebNavigation{URL: dest}})
						return nil
					}
					s.loadFrame(v, driver.WebSource{Src: dest})
					return nil
				}
				if data.Get("uncheckedNav").Bool() {
					s.send(v, driver.Event{Kind: driver.EventError,
						Text:       "nsgo: the web view blocked an unverified navigation (the webview capability's hosts or schemes)",
						Navigation: &driver.WebNavigation{}})
					if w.loadedOnce && w.lastSrcdoc != "" && w.uncheckedCount < 4 {
						w.uncheckedCount++
						w.restoring = true
						oldToken := w.token
						w.token = strconv.FormatUint(rand.Uint64(), 36)
						if oldToken != "" {
							w.lastSrcdoc = strings.ReplaceAll(w.lastSrcdoc, strconv.Quote(oldToken), strconv.Quote(w.token))
						}
						w.awaitBridge()
						s.replaceWebFrame(v)
						w.frame.Set("srcdoc", w.lastSrcdoc)
					} else {
						w.restoring = true
						w.token = ""
						w.exposeBridge = false
						w.lastSrcdoc = ""
						w.awaitBridge()
						s.replaceWebFrame(v)
						w.frame.Set("src", "about:blank")
					}
					return nil
				}
			}
			w.closePort()
			s.webNavigation(v, driver.EventLoadStart, true)
		default:
			// The page left (webBridge): what the app sends it now would be lost.
			w.closePort()
			s.webNavigation(v, driver.EventLoadStart, true)
		}
		return nil
	})
	w.port.Set("onmessage", w.portFn)
	w.bridging = false
	w.flushQueued()
}

func (s *surface) setWeb(v *view, c *driver.WebConfig) {
	w := v.web
	if w.loaded && sameWebFrame(c, &w.config) {
		return
	}
	w.loaded, w.config = true, *c
	s.loadFrame(v, c.Source)
}

// loadFrame loads src in v's frame with the config's settings.
func (s *surface) loadFrame(v *view, src driver.WebSource) {
	w := v.web
	c := &w.config
	if c.Restricted && src.HTML == "" && src.Src != "" && !c.SourceNavigable(src.Src) {
		refusedURL := src.Src
		var report js.Func
		report = js.FuncOf(func(js.Value, []js.Value) any {
			report.Release()
			if s.view(v.id) == v {
				s.send(v, driver.Event{Kind: driver.EventError, Text: driver.RefusedNavigation(refusedURL),
					Navigation: &driver.WebNavigation{URL: refusedURL}})
			}
			return nil
		})
		js.Global().Call("queueMicrotask", report)
		return
	}
	w.source = src
	srcdocHTML, isSrcdoc := src.HTML, src.HTML != ""
	exposeBridge := src.HTML != "" && !c.NoJavaScript
	if c.Restricted && !isSrcdoc && src.Src != "" {
		if decoded, ok := decodeRestrictedDataHTML(src.Src); ok {
			srcdocHTML, isSrcdoc, exposeBridge = decoded, true, false
		}
	}
	w.exposeBridge = exposeBridge
	w.loadedOnce = false
	w.restoring = false
	w.uncheckedCount = 0
	w.lastSrcdoc = ""
	w.token = ""
	if isSrcdoc && !c.NoJavaScript && (exposeBridge || c.Restricted) {
		w.token = strconv.FormatUint(rand.Uint64(), 36)
	}
	w.awaitBridge()
	pageOrigin := js.Global().Get("location").Get("origin").String()
	w.frame.Call("setAttribute", "sandbox", webFrameSandbox(c, src, pageOrigin, isSrcdoc))
	// The browser asks the user for what the frame may capture and
	// refuses the rest, from the frame's next page on.
	var allow []string
	if c.Media&driver.WebMicrophone != 0 {
		allow = append(allow, "microphone")
	}
	if c.Media&driver.WebCamera != 0 {
		allow = append(allow, "camera")
	}
	w.frame.Call("setAttribute", "allow", strings.Join(allow, "; "))
	script := func(js string) string {
		return "<script>" + strings.ReplaceAll(js, "</script", `<\/script`) + "</script>"
	}
	var refusedMeta []string
	if isSrcdoc {
		if c.Restricted {
			var doc string
			doc, w.title, refusedMeta = buildRestrictedSrcdoc(srcdocHTML, src.BaseURL, w.token, exposeBridge, c)
			w.lastSrcdoc = doc
			if src.HTML != "" {
				w.url = cmp.Or(src.BaseURL, "about:srcdoc")
			} else {
				w.url = src.Src
			}
			w.frame.Call("removeAttribute", "src")
			w.frame.Call("removeAttribute", "scrolling")
			w.frame.Set("srcdoc", doc)
		} else {
			var head strings.Builder
			if src.BaseURL != "" {
				head.WriteString(`<base href="` + strings.ReplaceAll(src.BaseURL, `"`, "&quot;") + `">`)
			}
			if w.token != "" {
				head.WriteString(script(webBridge(w.token)))
			}
			if c.NoScroll {
				head.WriteString("<style>html,body{overflow:hidden!important}</style>")
			}
			if c.InjectedJavaScriptBeforeContent != "" {
				head.WriteString(script(c.InjectedJavaScriptBeforeContent))
			}
			tail := ""
			if c.InjectedJavaScript != "" {
				tail = script(c.InjectedJavaScript)
			}
			// After a doctype, which must stay first.
			html, doctype := src.HTML, ""
			if trimmed := strings.TrimLeft(html, " \t\r\n"); len(trimmed) > 9 && strings.EqualFold(trimmed[:9], "<!doctype") {
				if end := strings.IndexByte(trimmed, '>'); end >= 0 {
					doctype, html = trimmed[:end+1], trimmed[end+1:]
				}
			}
			w.url = cmp.Or(src.BaseURL, "about:srcdoc")
			doc := js.Global().Get("DOMParser").New().Call("parseFromString", src.HTML, "text/html")
			w.title = doc.Get("title").String()
			w.frame.Call("removeAttribute", "src")
			w.frame.Call("removeAttribute", "scrolling")
			w.frame.Set("srcdoc", doctype+head.String()+html+tail)
		}
	} else {
		w.url, w.title = src.Src, ""
		w.frame.Call("removeAttribute", "srcdoc")
		if c.NoScroll {
			w.frame.Call("setAttribute", "scrolling", "no")
		} else {
			w.frame.Call("removeAttribute", "scrolling")
		}
		if src.Src != "" {
			w.frame.Set("src", src.Src)
		} else {
			w.frame.Call("removeAttribute", "src")
		}
	}
	if isSrcdoc || src.Src != "" {
		// After the batch, which may carry the view's events after its
		// configuration.
		refused := refusedMeta
		var start js.Func
		start = js.FuncOf(func(js.Value, []js.Value) any {
			start.Release()
			if s.view(v.id) == v {
				s.webNavigation(v, driver.EventLoadStart, true)
				for _, u := range refused {
					s.send(v, driver.Event{Kind: driver.EventError, Text: driver.RefusedNavigation(u),
						Navigation: &driver.WebNavigation{URL: u}})
				}
			}
			return nil
		})
		js.Global().Call("queueMicrotask", start)
	}
}

// webNavigation reports the frame's state, as far as the browser shows
// it, with kind and as a navigation change.
func (s *surface) webNavigation(v *view, kind driver.EventKind, loading bool) {
	nav := driver.WebNavigation{URL: v.web.url, Title: v.web.title, Loading: loading}
	for _, k := range []driver.EventKind{kind, driver.EventNavigationStateChange} {
		n := nav
		s.send(v, driver.Event{Kind: k, Navigation: &n})
	}
}

// webLoaded reports a loaded page.
func (s *surface) webLoaded(v *view) {
	w := v.web
	if !w.frame.Call("hasAttribute", "src").Bool() && !w.frame.Call("hasAttribute", "srcdoc").Bool() {
		return
	}
	w.loadedOnce = true
	w.uncheckedCount = 0
	if w.restoring {
		w.restoring = false
		w.flushQueued()
		return
	}
	s.webNavigation(v, driver.EventLoad, false)
}

func (w *webFrame) close() {
	w.closePort()
	w.frameListeners.release()
}

func (w *webFrame) closePort() {
	if w.port.Truthy() {
		w.port.Call("close")
		w.port.Set("onmessage", js.Null())
		w.portFn.Release()
		w.port = js.Undefined()
	}
}

func (w *webFrame) flushQueued() {
	if w.restoring || !w.port.Truthy() {
		return
	}
	if w.exposeBridge {
		for _, msg := range w.queued {
			w.port.Call("postMessage", msg)
		}
	}
	w.queued = nil
}

// awaitBridge drops the bridge of the page the frame shows, which is going, and waits for the next page's when the source has one.
func (w *webFrame) awaitBridge() {
	w.closePort()
	w.bridging, w.queued = w.token != "", nil
}

func (s *surface) webCommand(v *view, c driver.WebCommand, value string, data any) {
	w := v.web
	// viaPort hands msg to the source's page, once it handed its port, and reports false when no such page shows: the source is a URL, or its page went to another.
	viaPort := func(msg map[string]any) bool {
		if !w.exposeBridge {
			return false
		}
		switch {
		case w.bridging || w.restoring:
			w.queued = append(w.queued, msg)
		case w.port.Truthy():
			w.port.Call("postMessage", msg)
		default:
			return false
		}
		return true
	}
	warn := func(name string) {
		if !w.warned {
			w.warned = true
			js.Global().Get("console").Call("warn", "nsgo: web view: "+name+
				" only reaches the page of an HTML source (a URL's page, or one the source's went to, is the browser's own)")
		}
	}
	switch c {
	case driver.WebReload:
		// The frame is another origin: it loads again rather than is told to.
		if w.frame.Call("hasAttribute", "srcdoc").Bool() {
			w.awaitBridge()
			w.frame.Set("srcdoc", w.frame.Get("srcdoc"))
		} else if src := w.frame.Call("getAttribute", "src"); src.Truthy() {
			w.frame.Set("src", src)
		}
		s.webNavigation(v, driver.EventLoadStart, true)
	case driver.WebPostMessage:
		if win := w.frame.Get("contentWindow"); win.Truthy() {
			win.Call("postMessage", value, "*")
		}
	case driver.WebInjectJavaScript:
		if viaPort(map[string]any{"eval": value}) {
			break
		}
		marker, ok := driver.WebEvaluateMarker(value)
		if !ok {
			warn("InjectJavaScript")
			break
		}
		// No page answers an Evaluate: the view does, after the batch, as a page whose script threw.
		answer := driver.Event{Kind: driver.EventMessage, Text: driver.WebEvaluateFailure(marker,
			"nsgo: the web view shows no page of an HTML source, which alone runs scripts on the web")}
		var fail js.Func
		fail = js.FuncOf(func(js.Value, []js.Value) any {
			fail.Release()
			s.send(v, answer)
			return nil
		})
		js.Global().Call("queueMicrotask", fail)
	case driver.WebGoBack:
		if !viaPort(map[string]any{"go": -1}) {
			warn("GoBack")
		}
	case driver.WebGoForward:
		if !viaPort(map[string]any{"go": 1}) {
			warn("GoForward")
		}
	case driver.WebLoad:
		if src, ok := data.(*driver.WebSource); ok && src != nil && w.loaded {
			s.loadFrame(v, *src)
		}
	}
	// StopLoading does nothing, as in NSH: the frame is another origin.
}
