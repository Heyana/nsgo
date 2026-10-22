//go:build js && wasm

package web

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/oneo-me/nsgo/core"
)

// webFrame is a web view's <iframe>, as NSH's web backend has it. HTML
// loads as srcdoc in a sandbox with an origin of its own, so it never
// reaches the app's page, with a bridge script first: the first message
// carrying token hands the page a MessagePort, which
// window.nsgo.postMessage writes to and which runs injected scripts and
// history steps. A URL loads as it is (without its origin when it is the
// app's own); the browser hides what such a page does, so it only gets
// PostMessage and load events with the URL it was given.
type webFrame struct {
	frame  js.Value
	config core.WebConfig
	loaded bool // configured at least once
	token  string
	port   js.Value // the page's end of the bridge, once it loaded
	portFn js.Func
	url    string
	title  string
}

// webBridge is the script an HTML source starts with.
func webBridge(token string) string {
	return `(() => {
  let port = null;
  const queue = [];
  window.nsgo = { postMessage(data) { data = String(data); port ? port.postMessage(data) : queue.push(data); } };
  addEventListener("message", (e) => {
    if (port || e.source !== parent || e.data !== ` + strconv.Quote(token) + ` || !e.ports[0]) return;
    e.stopImmediatePropagation();
    port = e.ports[0];
    port.onmessage = (m) => {
      const d = m.data;
      if (d && d.eval != null) { try { (0, eval)(String(d.eval)); } catch (err) { console.error(err); } }
      else if (d && d.go) history.go(d.go);
    };
    for (const data of queue.splice(0)) port.postMessage(data);
  });
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
	v.listeners.on(w.frame, "load", func(js.Value) { s.webLoaded(v) })
}

func (s *surface) setWeb(v *view, c *core.WebConfig) {
	w := v.web
	if w.loaded && *c == w.config {
		return
	}
	w.loaded, w.config = true, *c
	w.closePort()
	w.token = ""
	src := c.Source
	if src.HTML != "" && !c.NoJavaScript {
		w.token = strconv.FormatUint(rand.Uint64(), 36)
	}
	sandbox := []string{"allow-forms", "allow-popups", "allow-popups-to-escape-sandbox", "allow-modals", "allow-downloads"}
	if !c.NoJavaScript {
		sandbox = append(sandbox, "allow-scripts")
	}
	if src.HTML == "" && src.Src != "" {
		// Pages of other sites keep their origin, so their storage works;
		// one of the app's own must not reach the app.
		url := js.Global().Get("URL")
		same := false
		if u, ok := newURL(url, src.Src); ok {
			same = u.Get("origin").String() == js.Global().Get("location").Get("origin").String()
		}
		if !same {
			sandbox = append(sandbox, "allow-same-origin")
		}
	}
	w.frame.Call("setAttribute", "sandbox", strings.Join(sandbox, " "))
	script := func(js string) string {
		return "<script>" + strings.ReplaceAll(js, "</script", `<\/script`) + "</script>"
	}
	if src.HTML != "" {
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
		w.url = src.BaseURL
		if w.url == "" {
			w.url = "about:srcdoc"
		}
		doc := js.Global().Get("DOMParser").New().Call("parseFromString", src.HTML, "text/html")
		w.title = doc.Get("title").String()
		w.frame.Call("removeAttribute", "src")
		w.frame.Call("removeAttribute", "scrolling")
		w.frame.Set("srcdoc", doctype+head.String()+html+tail)
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
	if src.HTML != "" || src.Src != "" {
		s.webNavigation(v, core.EventLoadStart, true)
	}
}

func newURL(ctor js.Value, s string) (u js.Value, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return ctor.New(s, js.Global().Get("location").Get("href")), true
}

// webNavigation reports the frame's state, as far as the browser shows
// it, with kind and as a navigation change.
func (s *surface) webNavigation(v *view, kind core.EventKind, loading bool) {
	nav := core.WebNavigation{URL: v.web.url, Title: v.web.title, Loading: loading}
	for _, k := range []core.EventKind{kind, core.EventNavigationStateChange} {
		n := nav
		s.send(v, core.Event{Kind: k, Navigation: &n})
	}
}

// webLoaded hands a loaded HTML page its end of the bridge.
func (s *surface) webLoaded(v *view) {
	w := v.web
	if !w.frame.Call("hasAttribute", "src").Bool() && !w.frame.Call("hasAttribute", "srcdoc").Bool() {
		return
	}
	w.closePort()
	if w.token != "" {
		channel := js.Global().Get("MessageChannel").New()
		w.port = channel.Get("port1")
		w.portFn = js.FuncOf(func(_ js.Value, args []js.Value) any {
			s.send(v, core.Event{Kind: core.EventMessage, Text: args[0].Get("data").String()})
			return nil
		})
		w.port.Set("onmessage", w.portFn)
		if win := w.frame.Get("contentWindow"); win.Truthy() {
			win.Call("postMessage", w.token, "*", []any{channel.Get("port2")})
		}
	}
	s.webNavigation(v, core.EventLoad, false)
}

func (w *webFrame) closePort() {
	if w.port.Truthy() {
		w.port.Call("close")
		w.port.Set("onmessage", js.Null())
		w.portFn.Release()
		w.port = js.Undefined()
	}
}

func (s *surface) webCommand(v *view, c core.WebCommand, value string) {
	w := v.web
	viaPort := func(msg map[string]any) {
		if w.port.Truthy() {
			w.port.Call("postMessage", msg)
		} else {
			js.Global().Get("console").Call("warn", "nsgo: a web view reaches only the pages of its HTML sources")
		}
	}
	switch c {
	case core.WebReload:
		// The frame is another origin: it loads again rather than is told to.
		if w.frame.Call("hasAttribute", "srcdoc").Bool() {
			w.frame.Set("srcdoc", w.frame.Get("srcdoc"))
		} else if src := w.frame.Call("getAttribute", "src"); src.Truthy() {
			w.frame.Set("src", src)
		}
		s.webNavigation(v, core.EventLoadStart, true)
	case core.WebPostMessage:
		if win := w.frame.Get("contentWindow"); win.Truthy() {
			win.Call("postMessage", value, "*")
		}
	case core.WebInjectJavaScript:
		viaPort(map[string]any{"eval": value})
	case core.WebStopLoading:
		viaPort(map[string]any{"eval": "window.stop()"})
	case core.WebGoBack:
		viaPort(map[string]any{"go": -1})
	case core.WebGoForward:
		viaPort(map[string]any{"go": 1})
	}
}
