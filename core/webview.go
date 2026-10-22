package core

import "errors"

// WebView shows a web page in the platform's web view, which fills the
// view and takes its own pointer, wheel and keyboard input. Pages reach
// the app with window.nsgo.postMessage(data), which OnMessage receives;
// Ref.PostMessage dispatches a MessageEvent on the page's window.
type WebView struct {
	Source WebSource
	// SourceFunc, when set, replaces Source; the page loads again when it
	// changes.
	SourceFunc func() WebSource
	// InjectedJavaScript runs after each page loads, and
	// InjectedJavaScriptBeforeContent before each page's own scripts.
	InjectedJavaScript              string
	InjectedJavaScriptBeforeContent string
	// UserAgent replaces the platform's.
	UserAgent string
	// NoScroll keeps the user from scrolling the page; NoJavaScript keeps
	// pages from running scripts.
	NoScroll     bool
	NoJavaScript bool
	// BackForwardGestures lets swipes go back and forward where the
	// platform has them (iOS, macOS trackpads).
	BackForwardGestures bool
	Style               Style
	StyleFunc           func() Style

	// OnLoadStart, OnLoad and OnNavigationStateChange receive the page's
	// state as it starts loading, finishes and navigates.
	OnLoadStart             func(WebNavigation)
	OnLoad                  func(WebNavigation)
	OnNavigationStateChange func(WebNavigation)
	OnError                 func(err error)
	// OnMessage receives what the page posted, as a string.
	OnMessage func(data string)
	OnLayout  func(Bounds)
	Ref       *Ref
	Label     string
	TestID    string
}

// Render implements Node.
func (w WebView) Render(*Scope) Node { return w }

// WebSource is what a web view shows: the page at Src, or HTML, whose
// relative links resolve against BaseURL, instead when it is set.
type WebSource struct {
	Src     string
	HTML    string
	BaseURL string
}

// WebSrc returns the source loading the page at url.
func WebSrc(url string) WebSource { return WebSource{Src: url} }

// WebHTML returns the source showing html, with relative links resolved
// against baseURL ("" for none).
func WebHTML(html, baseURL string) WebSource { return WebSource{HTML: html, BaseURL: baseURL} }

// WebNavigation is a web view's page and history.
type WebNavigation struct {
	URL          string
	Title        string
	Loading      bool
	CanGoBack    bool
	CanGoForward bool
}

// WebConfig is everything a web view loads and allows (ItemWeb).
// Platforms load the source again when it or the scripts change.
type WebConfig struct {
	Source                          WebSource
	InjectedJavaScript              string
	InjectedJavaScriptBeforeContent string
	UserAgent                       string
	NoScroll                        bool
	NoJavaScript                    bool
	BackForwardGestures             bool
}

// WebCommand is what ItemWebCommand asks a web view to do.
type WebCommand uint8

const (
	WebReload WebCommand = iota + 1
	WebGoBack
	WebGoForward
	WebStopLoading
	// WebPostMessage dispatches a MessageEvent carrying Value on the
	// page's window.
	WebPostMessage
	// WebInjectJavaScript runs Value in the page.
	WebInjectJavaScript
)

// WebBridge is the script a platform runs at the start of every page
// before the app's, given post, the JavaScript expression of a function
// that sends its string argument to the app.
func WebBridge(post string) string {
	return "window.nsgo = window.nsgo || { postMessage: function (data) { (" + post + ")(String(data)); } };"
}

// WebMessageScript is the script WebPostMessage runs in the page.
func WebMessageScript(data string) string {
	return "window.dispatchEvent(new MessageEvent('message', { data: " + JSString(data) + " }));"
}

// JSString quotes s as a JavaScript string literal.
func JSString(s string) string {
	b := make([]byte, 0, len(s)+2)
	b = append(b, '"')
	for _, r := range s {
		switch {
		case r == '"':
			b = append(b, '\\', '"')
		case r == '\\':
			b = append(b, '\\', '\\')
		case r == '\n':
			b = append(b, '\\', 'n')
		case r == '\r':
			b = append(b, '\\', 'r')
		case r < 0x20 || r == 0x2028 || r == 0x2029:
			const hex = "0123456789abcdef"
			b = append(b, '\\', 'u', hex[r>>12&15], hex[r>>8&15], hex[r>>4&15], hex[r&15])
		default:
			b = append(b, string(r)...)
		}
	}
	return string(append(b, '"'))
}

type webNode struct {
	config                      WebConfig
	sent                        WebConfig
	loadStart, load, navigation func(WebNavigation)
	loadError                   func(error)
	message                     func(string)
}

func (t *Tree) mountWebView(cx *Scope, parent *node, w WebView) {
	n := t.newNode(KindWeb, parent, cx)
	t.bindStyle(cx, n, w.Style, w.StyleFunc)
	t.setRef(n, w.Ref)
	n.h = handlers{layout: w.OnLayout}
	n.props = ViewProps{Label: w.Label, TestID: w.TestID}
	n.web = &webNode{
		config: WebConfig{Source: w.Source, InjectedJavaScript: w.InjectedJavaScript,
			InjectedJavaScriptBeforeContent: w.InjectedJavaScriptBeforeContent, UserAgent: w.UserAgent,
			NoScroll: w.NoScroll, NoJavaScript: w.NoJavaScript, BackForwardGestures: w.BackForwardGestures},
		loadStart: w.OnLoadStart, load: w.OnLoad, navigation: w.OnNavigationStateChange,
		loadError: w.OnError, message: w.OnMessage,
	}
	if w.SourceFunc != nil {
		cx.renderEffect(func() {
			if src := w.SourceFunc(); src != n.web.config.Source {
				n.web.config.Source = src
				t.touch(n)
			}
		})
	}
}

// dispatchWeb handles a web view's events.
func (t *Tree) dispatchWeb(n *node, ev Event) {
	wn := n.web
	var nav WebNavigation
	if ev.Navigation != nil {
		nav = *ev.Navigation
	}
	call := func(fn func(WebNavigation)) {
		if fn != nil {
			t.within(n.owner, func() { fn(nav) })
		}
	}
	switch ev.Kind {
	case EventLoadStart:
		call(wn.loadStart)
	case EventLoad:
		call(wn.load)
	case EventNavigationStateChange:
		call(wn.navigation)
	case EventError:
		if wn.loadError != nil {
			t.within(n.owner, func() { wn.loadError(errors.New(ev.Text)) })
		}
	case EventMessage:
		if wn.message != nil {
			t.within(n.owner, func() { wn.message(ev.Text) })
		}
	}
}

// webEvents is what a web view reports.
func (wn *webNode) events() EventMask {
	var m EventMask
	bit := func(k EventKind, on bool) {
		if on {
			m |= 1 << k
		}
	}
	bit(EventLoadStart, wn.loadStart != nil)
	bit(EventLoad, wn.load != nil)
	bit(EventNavigationStateChange, wn.navigation != nil)
	bit(EventError, wn.loadError != nil)
	bit(EventMessage, wn.message != nil)
	return m
}

// webCommand sends c to the web view r names.
func (r *Ref) webCommand(c WebCommand, value string) {
	if r.n == nil || r.n.deleted || r.n.web == nil {
		return
	}
	r.t.commands = append(r.t.commands, MountItem{Kind: ItemWebCommand, ID: r.n.id, WebCommand: c, Value: value})
	r.t.changed = true
}

// Reload loads the web view's page again.
func (r *Ref) Reload() { r.webCommand(WebReload, "") }

// GoBack goes back in the web view's history.
func (r *Ref) GoBack() { r.webCommand(WebGoBack, "") }

// GoForward goes forward in the web view's history.
func (r *Ref) GoForward() { r.webCommand(WebGoForward, "") }

// StopLoading stops the web view's page loading.
func (r *Ref) StopLoading() { r.webCommand(WebStopLoading, "") }

// PostMessage dispatches a MessageEvent carrying data on the web view's
// page.
func (r *Ref) PostMessage(data string) { r.webCommand(WebPostMessage, data) }

// InjectJavaScript runs script in the web view's page.
func (r *Ref) InjectJavaScript(script string) { r.webCommand(WebInjectJavaScript, script) }
