//go:build darwin

package apple

import (
	"runtime"
	"strings"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/core"
)

// Web is a web view's WKWebView, filling the view that holds it. WebKit
// loads on the first web view, so apps without one do not pay for it.
// The WKWebView is made on the first Configure, and again when
// NoJavaScript changes, since WebKit fixes its preferences then.
type Web struct {
	// Dispatch reports the web view's events.
	Dispatch func(core.Event)

	holder  objc.ID
	web     objc.ID
	handler objc.ID
	config  core.WebConfig
	made    bool
}

var (
	classWebHandler objc.Class
	webHandlers     = map[objc.ID]*Web{}
)

// messageHandler is the name pages post to, as window.webkit.messageHandlers.nsgo.
const messageHandler = "nsgo"

var bridge = core.WebBridge("function (d) { window.webkit.messageHandlers." + messageHandler + ".postMessage(d); }")

// noScroll stops the page scrolling where WebKit has no scroll view to
// turn off (macOS).
const noScroll = "document.documentElement.style.setProperty('overflow', 'hidden', 'important');" +
	"document.body && document.body.style.setProperty('overflow', 'hidden', 'important');"

func loadWebKit() {
	if classWebHandler != 0 {
		return
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/WebKit.framework/WebKit", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		panic(err)
	}
	nav := func(name string, loading bool, kinds ...core.EventKind) objc.MethodDef {
		return objc.MethodDef{Cmd: sel(name), Fn: func(self objc.ID, _ objc.SEL, web, _ objc.ID) {
			if w := webHandlers[self]; w != nil {
				w.navigated(loading, kinds...)
			}
		}}
	}
	failed := func(self objc.ID, _ objc.SEL, _, _, err objc.ID) {
		if w := webHandlers[self]; w != nil {
			w.Dispatch(core.Event{Kind: core.EventError, Text: GoString(err.Send(sel("localizedDescription")))})
		}
	}
	var protocols []*objc.Protocol
	for _, name := range []string{"WKNavigationDelegate", "WKScriptMessageHandler"} {
		if p := objc.GetProtocol(name); p != nil {
			protocols = append(protocols, p)
		}
	}
	classWebHandler = RegisterClass("NsgoWebHandler", objc.GetClass("NSObject"), protocols,
		[]objc.MethodDef{
			nav("webView:didStartProvisionalNavigation:", true, core.EventLoadStart, core.EventNavigationStateChange),
			nav("webView:didCommitNavigation:", true, core.EventNavigationStateChange),
			nav("webView:didFinishNavigation:", false, core.EventLoad, core.EventNavigationStateChange),
			{Cmd: sel("webView:didFailNavigation:withError:"), Fn: failed},
			{Cmd: sel("webView:didFailProvisionalNavigation:withError:"), Fn: failed},
			{Cmd: sel("userContentController:didReceiveScriptMessage:"), Fn: func(self objc.ID, _ objc.SEL, _, message objc.ID) {
				if w := webHandlers[self]; w != nil {
					body := message.Send(sel("body"))
					w.Dispatch(core.Event{Kind: core.EventMessage, Text: GoString(body.Send(sel("description")))})
				}
			}},
		})
}

// NewWeb returns the web view of holder, empty until configured.
func NewWeb(holder objc.ID) *Web {
	return &Web{holder: holder}
}

// View returns the WKWebView, 0 before the first Configure.
func (w *Web) View() objc.ID { return w.web }

// Configure applies c, loading the source again when it or the scripts
// changed.
func (w *Web) Configure(c *core.WebConfig) {
	old, first := w.config, !w.made || c.NoJavaScript != w.config.NoJavaScript
	w.config = *c
	if first {
		w.make()
	}
	scripts := first || c.InjectedJavaScript != old.InjectedJavaScript ||
		c.InjectedJavaScriptBeforeContent != old.InjectedJavaScriptBeforeContent || c.NoScroll != old.NoScroll
	if scripts {
		w.addScripts()
	}
	if c.UserAgent == "" {
		w.web.Send(sel("setCustomUserAgent:"), objc.ID(0))
	} else {
		w.web.Send(sel("setCustomUserAgent:"), NSString(c.UserAgent))
	}
	w.web.Send(sel("setAllowsBackForwardNavigationGestures:"), c.BackForwardGestures)
	if runtime.GOOS == "ios" {
		w.web.Send(sel("scrollView")).Send(sel("setScrollEnabled:"), !c.NoScroll)
	}
	if first || scripts || c.Source != old.Source {
		w.load()
	}
}

func (w *Web) make() {
	loadWebKit()
	if w.web != 0 {
		w.close()
	}
	config := objc.ID(objc.GetClass("WKWebViewConfiguration")).Send(sel("new"))
	config.Send(sel("defaultWebpagePreferences")).Send(sel("setAllowsContentJavaScript:"), !w.config.NoJavaScript)
	w.handler = objc.ID(classWebHandler).Send(sel("new"))
	webHandlers[w.handler] = w
	config.Send(sel("userContentController")).Send(sel("addScriptMessageHandler:name:"), w.handler, NSString(messageHandler))
	bounds := objc.Send[CGRect](w.holder, sel("bounds"))
	w.web = objc.ID(objc.GetClass("WKWebView")).Send(sel("alloc")).Send(sel("initWithFrame:configuration:"), bounds, config)
	config.Send(sel("release"))
	w.web.Send(sel("setNavigationDelegate:"), w.handler)
	const flexibleSize = 2 | 16 // width and height, in AppKit and UIKit alike
	w.web.Send(sel("setAutoresizingMask:"), uint(flexibleSize))
	// WebKit paints white otherwise; a page without a background shows
	// the view's own, as its style expects.
	if runtime.GOOS == "ios" {
		clear := objc.ID(objc.GetClass("UIColor")).Send(sel("clearColor"))
		w.web.Send(sel("setOpaque:"), false)
		w.web.Send(sel("setBackgroundColor:"), clear)
		w.web.Send(sel("scrollView")).Send(sel("setBackgroundColor:"), clear)
	} else {
		no := objc.ID(objc.GetClass("NSNumber")).Send(sel("numberWithBool:"), false)
		w.web.Send(sel("setValue:forKey:"), no, NSString("drawsBackground"))
	}
	w.holder.Send(sel("addSubview:"), w.web)
	w.made = true
}

func (w *Web) addScripts() {
	content := w.web.Send(sel("configuration")).Send(sel("userContentController"))
	content.Send(sel("removeAllUserScripts"))
	add := func(source string, at int) {
		if source == "" {
			return
		}
		script := objc.ID(objc.GetClass("WKUserScript")).Send(sel("alloc")).Send(
			sel("initWithSource:injectionTime:forMainFrameOnly:"), NSString(source), at, true)
		content.Send(sel("addUserScript:"), script)
		script.Send(sel("release"))
	}
	const atStart, atEnd = 0, 1 // WKUserScriptInjectionTime
	add(bridge, atStart)
	add(w.config.InjectedJavaScriptBeforeContent, atStart)
	if w.config.NoScroll && runtime.GOOS != "ios" {
		add(noScroll, atEnd)
	}
	add(w.config.InjectedJavaScript, atEnd)
}

func (w *Web) load() {
	src := w.config.Source
	switch {
	case src.HTML != "":
		base := objc.ID(0)
		if src.BaseURL != "" {
			base = nsURL(src.BaseURL)
		}
		w.web.Send(sel("loadHTMLString:baseURL:"), NSString(src.HTML), base)
	case src.Src != "":
		url := nsURL(src.Src)
		if url == 0 {
			w.Dispatch(core.Event{Kind: core.EventError, Text: "invalid URL " + src.Src})
			return
		}
		if objc.Send[bool](url, sel("isFileURL")) {
			w.web.Send(sel("loadFileURL:allowingReadAccessToURL:"), url, url.Send(sel("URLByDeletingLastPathComponent")))
			return
		}
		request := objc.ID(objc.GetClass("NSURLRequest")).Send(sel("requestWithURL:"), url)
		w.web.Send(sel("loadRequest:"), request)
	}
}

// nsURL parses s, taking a path without a scheme as a file's.
func nsURL(s string) objc.ID {
	if strings.HasPrefix(s, "/") {
		return objc.ID(objc.GetClass("NSURL")).Send(sel("fileURLWithPath:"), NSString(s))
	}
	return objc.ID(objc.GetClass("NSURL")).Send(sel("URLWithString:"), NSString(s))
}

// navigated reports the page's state as kinds.
func (w *Web) navigated(loading bool, kinds ...core.EventKind) {
	nav := core.WebNavigation{
		URL:          GoString(w.web.Send(sel("URL")).Send(sel("absoluteString"))),
		Title:        GoString(w.web.Send(sel("title"))),
		Loading:      loading,
		CanGoBack:    objc.Send[bool](w.web, sel("canGoBack")),
		CanGoForward: objc.Send[bool](w.web, sel("canGoForward")),
	}
	for _, k := range kinds {
		n := nav
		w.Dispatch(core.Event{Kind: k, Navigation: &n})
	}
}

// Command does c, with value for messages and scripts.
func (w *Web) Command(c core.WebCommand, value string) {
	if w.web == 0 {
		return
	}
	switch c {
	case core.WebReload:
		w.web.Send(sel("reload"))
	case core.WebGoBack:
		w.web.Send(sel("goBack"))
	case core.WebGoForward:
		w.web.Send(sel("goForward"))
	case core.WebStopLoading:
		w.web.Send(sel("stopLoading"))
	case core.WebPostMessage:
		w.evaluate(core.WebMessageScript(value))
	case core.WebInjectJavaScript:
		w.evaluate(value)
	}
}

func (w *Web) evaluate(script string) {
	w.web.Send(sel("evaluateJavaScript:completionHandler:"), NSString(script), objc.ID(0))
}

// Close stops the page and lets the WKWebView go.
func (w *Web) Close() {
	if w.web != 0 {
		w.close()
	}
}

func (w *Web) close() {
	w.web.Send(sel("stopLoading"))
	w.web.Send(sel("setNavigationDelegate:"), objc.ID(0))
	// The content controller holds the handler, which would keep it alive.
	w.web.Send(sel("configuration")).Send(sel("userContentController")).
		Send(sel("removeScriptMessageHandlerForName:"), NSString(messageHandler))
	w.web.Send(sel("removeFromSuperview"))
	w.web.Send(sel("release"))
	delete(webHandlers, w.handler)
	w.handler.Send(sel("release"))
	w.web, w.handler = 0, 0
}
