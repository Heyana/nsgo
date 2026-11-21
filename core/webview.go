package core

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// WebView shows a web page in the platform's web view, which fills the view and takes its own pointer, wheel and keyboard input. Pages reach the app with window.nsgo.postMessage(data), which OnMessage and OnMessageFrom receive from the pages MessageOrigins allows; Ref.PostMessage dispatches a MessageEvent on the page's window.
type WebView struct {
	// Source loads about:blank when neither Src nor HTML is set, clearing the previous page and reporting the usual load and navigation events.
	Source WebSource
	// The page loads again when SourceFunc's source changes.
	SourceFunc func() WebSource
	// InjectedJavaScript runs after each page loads, and
	// InjectedJavaScriptBeforeContent before each page's own scripts.
	InjectedJavaScript              string
	InjectedJavaScriptBeforeContent string
	// InjectedJavaScriptBeforeContentFunc, when set, gives the script before each page's own scripts in place of InjectedJavaScriptBeforeContent. When its script changes, the view loads its source again with the new script, as when SourceFunc's source changes, replacing the page it shows, one the user or Ref.Load navigated to included.
	InjectedJavaScriptBeforeContentFunc func() string
	// UserAgent replaces the platform's.
	UserAgent string
	// UserAgentSuffix is appended after a space to UserAgent, or to the platform's own user agent when UserAgent is empty, such as Version/26.0 Safari/605.1.15 for sites that want Safari's in WKWebView. Apple platforms add it to their own user agent only as the native view is first made, so changing it afterwards takes effect there only with UserAgent set. HarmonyOS, the web and mini programs ignore it, as they do UserAgent.
	UserAgentSuffix string
	// Headers are sent with the initial URL request where the platform
	// supports custom request headers. HTML sources do not make one.
	Headers map[string]string
	// Incognito keeps website data in memory for this view, apart from the app's other web views, on Apple platforms, Windows and Linux. Android has one cookie store and one set of website data per app, so there it only keeps the view from the HTTP cache, DOM storage and saved form data. It is read when the native view is first made; changing it afterwards has no effect.
	Incognito bool
	// NoScroll keeps the user from scrolling the page; NoJavaScript keeps
	// pages from running scripts.
	NoScroll     bool
	NoJavaScript bool
	// Muted silences the entire web view on Windows and Linux, including frames and Web Audio. macOS 12 and iOS 15 or later suspend media playback until cleared. The setting persists across navigations. MutedFunc updates it in place.
	Muted     bool
	MutedFunc func() bool
	// PinchZoom lets the user magnify the page with a pinch: a trackpad's on macOS, a touchpad's or a touch screen's on Windows, where it also allows Ctrl with the wheel or with + and -, as a touchpad's pinch reaches the page as Ctrl with the wheel, and a touch screen's on Android. Without it the page keeps its size on those platforms. iOS zooms as the page's viewport says, and WebKitGTK on Linux and the browser on the web keep their own handling, having no switch for it.
	PinchZoom bool
	// BackForwardGestures lets swipes go back and forward where the
	// platform has them (iOS, macOS trackpads).
	BackForwardGestures bool
	// Mobile shows the page as a phone sees it on Windows, macOS and Linux, for sites such as YouTube's Shorts that give their swipe-driven layout only to phones: the page gets a phone's user agent unless UserAgent is set, sees a touch screen without hover (navigator.maxTouchPoints, ontouchstart, and matchMedia's pointer: coarse and hover: none), and a drag with the mouse's primary button reaches it as a finger's touches, as does a wheel or trackpad scroll that nothing under it would scroll. Windows emulates Android Chrome through WebView2's DevTools protocol, as Chrome's device mode does, so stylesheets and native scrolling follow too. macOS, presenting iPhone Safari, and Linux, presenting Android Chrome, run a script before each page's own that makes the touch events, which pages see as untrusted, and pans what a drag the page leaves alone is over; stylesheets' media queries keep the desktop's pointer there. Phones, the web and mini programs show pages as they are.
	Mobile bool
	// Media is what the pages may capture through getUserMedia:
	// WebMicrophone, WebCamera or both. The system still asks the user
	// for each device once, and a page asking for one Media leaves out is
	// refused. Packages declare their use: NSMicrophoneUsageDescription
	// and NSCameraUsageDescription in nsgo.yaml's apple.info, without
	// which Apple's platforms refuse the page, and RECORD_AUDIO and
	// CAMERA in android.permissions. It applies to the pages' next
	// requests, and on the web to the next page loaded.
	Media WebMedia
	// Class is Tailwind CSS classes, as View's.
	Class     string
	Style     Style
	StyleFunc func() Style

	// OnLoadStart, OnLoad and OnNavigationStateChange receive the page's
	// state as it starts loading, finishes and navigates.
	OnLoadStart func(WebNavigation)
	OnLoad      func(WebNavigation)
	// OnNavigationStateChange also reports same-document history and title changes on macOS, iOS and Linux.
	OnNavigationStateChange func(WebNavigation)
	// OnOpenWindow receives a page's new-window request on macOS, iOS,
	// Windows and Linux. It may load the URL in this view or open it
	// elsewhere. The platform creates no window, even without a callback.
	OnOpenWindow func(WebWindowRequest)
	// OnError receives a *WebError. A load cut short, by StopLoading, a new source, another navigation or a download it turned into, is no error.
	OnError func(err error)
	// MessageOrigins are the origins whose pages reach OnMessage and OnMessageFrom, written as WebMessage.Origin is ("https://example.com"), or "*" for any. Without them only pages of the source's own origin do: Src's, or for HTML BaseURL's, and "null" for HTML without one. "null" is the origin of every page without one of its own, a data: URL a frame sent the view to as much as the source's HTML, so "null", by default or listed, lets in only the source's page; "*" lets in every page. Only the page posts, never a frame in it, on every platform; a frame reaches the app through the page, with window.parent.postMessage, whose event tells the page the frame's origin.
	MessageOrigins []string
	// OnMessage receives what the page posted, as a string, and OnMessageFrom with the origin of the page that posted it.
	OnMessage     func(data string)
	OnMessageFrom func(WebMessage)
	// OnDownload follows what the page downloads, which only a web view
	// with one saves, to downloads/<id>/ in the app's data directory:
	// files WebKit cannot show and links that ask for a download, on
	// Apple platforms, Linux, Windows and Android.
	OnDownload func(WebDownload)
	OnLayout   func(Bounds)
	Ref        *Ref
	Label      string
	LabelFunc  func() string
	TestID     string
}

// Render implements Node.
func (w WebView) Render(*Scope) Node { return w }

func (w WebView) mountIn(t *Tree, cx *Scope, parent *node) { t.mountWebView(cx, parent, w) }

// WebSrc returns the source loading the page at url.
func WebSrc(url string) WebSource { return WebSource{Src: url} }

// WebHTML returns the source showing html, with relative links resolved
// against baseURL ("" for none).
func WebHTML(html, baseURL string) WebSource { return WebSource{HTML: html, BaseURL: baseURL} }

// WebMessage is what a web view's page posted, and the page's origin: scheme://host, with the port when it is not the scheme's own, in lower case ("https://example.com", "app://bundle"), "file://" for a file and "null" for a page without an origin of its own, such as HTML without a base URL.
type WebMessage struct {
	Data   string
	Origin string
}

// WebWindowRequest is a page's request to open a URL in a new window.
// UserInitiated reports when the platform associates it with user input.
type WebWindowRequest = driver.WebWindowRequest

// WebError is a page that failed to load, or a navigation the
// platform refused; URL is the page's, when known.
type WebError struct {
	URL     string
	Message string
	// Domain and Code are the platform's: NSError's domain and code on Apple platforms, the GError domain's name and its code on Linux, COREWEBVIEW2_WEB_ERROR_STATUS and its value, or HRESULT and the HRESULT of a navigation WebView2 refused, on Windows. Both are empty for a source Access refused and on Android, whose WebView reports its errors without them (the web reports none).
	Domain string
	Code   int
}

func (e *WebError) Error() string { return e.Message }

type webNode struct {
	config                      driver.WebConfig
	sent                        driver.WebConfig
	loadStart, load, navigation func(WebNavigation)
	loadError                   func(error)
	openWindow                  func(WebWindowRequest)
	message                     func(string)
	messageFrom                 func(WebMessage)
	download                    func(WebDownload)
	// origins are MessageOrigins, as WebOrigin writes them; dropped is whether a message they refused was logged.
	origins []string
	dropped bool
	// refused is the source Access refused last.
	refused string
	// evals are the Evaluates waiting for their pages' answers, by the
	// marker their answers start with.
	evals map[string]func(string, error)
}

func cloneWebHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	return maps.Clone(headers)
}

func (t *Tree) mountWebView(cx *Scope, parent *node, w WebView) {
	n := t.newNode(driver.KindWeb, parent, cx)
	t.setClasses(n, w.Class)
	t.bindStyle(cx, n, w.Style, w.StyleFunc)
	t.setRef(n, w.Ref)
	n.h = handlers{layout: w.OnLayout}
	n.props = driver.ViewProps{Label: w.Label, TestID: w.TestID}
	t.bindAccessibilityText(cx, n, &n.props.Label, w.LabelFunc)
	n.web = &webNode{
		config: driver.WebConfig{Source: w.Source, InjectedJavaScript: w.InjectedJavaScript,
			InjectedJavaScriptBeforeContent: w.InjectedJavaScriptBeforeContent, UserAgent: w.UserAgent, UserAgentSuffix: w.UserAgentSuffix,
			Headers: cloneWebHeaders(w.Headers), Incognito: w.Incognito,
			SchemeHandlers: webSchemesOf(cx),
			NoScroll:       w.NoScroll, NoJavaScript: w.NoJavaScript, PinchZoom: w.PinchZoom, BackForwardGestures: w.BackForwardGestures,
			Mobile: w.Mobile,
			Media:  w.Media, Muted: w.Muted},
		loadStart: w.OnLoadStart, load: w.OnLoad, navigation: w.OnNavigationStateChange,
		loadError: w.OnError, openWindow: w.OnOpenWindow, message: w.OnMessage, messageFrom: w.OnMessageFrom, download: w.OnDownload,
		origins: messageOrigins(w.MessageOrigins),
	}
	if w.OnDownload != nil && cx.rt.dataDir != "" {
		n.web.config.DownloadDir = filepath.Join(cx.rt.dataDir, "downloads")
	}
	if w.SourceFunc != nil {
		cx.renderEffect(func() {
			if src := w.SourceFunc(); src != n.web.config.Source {
				n.web.config.Source = src
				t.touch(n)
			}
		})
	}
	if f := w.InjectedJavaScriptBeforeContentFunc; f != nil {
		cx.renderEffect(func() {
			if script := f(); script != n.web.config.InjectedJavaScriptBeforeContent {
				n.web.config.InjectedJavaScriptBeforeContent = script
				t.touch(n)
			}
		})
	}
	if f := w.MutedFunc; f != nil {
		cx.renderEffect(func() {
			if muted := f(); muted != n.web.config.Muted {
				n.web.config.Muted = muted
				t.touch(n)
			}
		})
	}
}

// messageOrigins returns origins as WebOrigin writes them, leaving out, with a log, what is no origin, so that "example.com" does not allow every page without an origin, as WebOrigin's "null" would. A list left with none keeps the default, the source's origin.
func messageOrigins(origins []string) []string {
	var out []string
	for _, o := range origins {
		switch w := driver.WebOrigin(o); {
		case o == "*" || o == "null":
			out = append(out, o)
		case w == "null":
			log.Printf("nsgo: WebView.MessageOrigins: %q is no origin, such as https://example.com", o)
		default:
			out = append(out, w)
		}
	}
	return out
}

// allows reports whether a message ev reports reaches the app: one the page's top frame posted, from an origin MessageOrigins allows, or without them the origin of the source the platform loaded.
func (wn *webNode) allows(ev driver.Event) bool {
	if !ev.MainFrame || ev.Origin == "" {
		return false
	}
	if slices.Contains(wn.origins, "*") {
		return true
	}
	// Every page without an origin of its own has "null", the source's HTML without a base URL as much as a data: URL a frame sent the view to, so the page's URL tells them apart.
	if ev.Origin == "null" && !wn.sent.Source.Shows(ev.URL) {
		return false
	}
	if wn.origins == nil {
		return ev.Origin == wn.sent.Source.Origin()
	}
	return slices.Contains(wn.origins, ev.Origin)
}

// restrictWeb holds c to the Access of n's owner: a source it refuses
// shows nothing, the platform refuses navigations to hosts it does not
// allow and the devices it does not grant, and nothing is downloaded.
func (t *Tree) restrictWeb(n *node, c *driver.WebConfig) {
	a := accessOf(n.owner)
	if a == nil {
		return
	}
	wn := n.web
	report := func(err error) {
		if wn.loadError != nil {
			wn.loadError(&WebError{URL: c.Source.Src, Message: err.Error()})
		}
	}
	t.refuseSource(n, AccessWebView, &c.Source.Src, &wn.refused, report)
	if c.Source.BaseURL != "" && sourceRefusal(n.owner, AccessWebView, c.Source.BaseURL) != nil {
		c.Source.BaseURL = ""
	}
	c.Restricted, c.Hosts, c.Schemes, c.DownloadDir = true, strings.Join(a.WebHosts(), " "), strings.Join(webSchemes(a), " "), ""
	for _, m := range webMediaNames {
		if c.Media&m.media != 0 && !a.Allows(AccessRequest{Permission: AccessWebView, Resource: m.name, Method: AccessMedia}) {
			c.Media &^= m.media
			// The refusal is logged once per device, under a key of its own beside the services'.
			key := service(AccessMedia + m.name)
			if !t.rt.refusals[key] {
				if t.rt.refusals == nil {
					t.rt.refusals = map[service]bool{}
				}
				t.rt.refusals[key] = true
				log.Printf("nsgo: a web view's %s needs the %q capability's media", m.name, AccessWebView)
			}
		}
	}
}

// WebMedia is the devices a web view's pages may capture (WebView.Media).
type WebMedia = driver.WebMedia

const (
	WebMicrophone = driver.WebMicrophone
	WebCamera     = driver.WebCamera
)

// webMediaNames are the devices by their names in AccessRequest.Resource.
var webMediaNames = [...]struct {
	media WebMedia
	name  string
}{{WebMicrophone, "microphone"}, {WebCamera, "camera"}}

// How far a WebDownload got.
const (
	WebDownloadStarted  = driver.WebDownloadStarted
	WebDownloadFinished = driver.WebDownloadFinished
	WebDownloadFailed   = driver.WebDownloadFailed
)

// dispatchWeb handles a web view's events.
func (t *Tree) dispatchWeb(n *node, ev driver.Event) {
	wn := n.web
	var nav WebNavigation
	if ev.Navigation != nil {
		nav = *ev.Navigation
		// Chromium's web views, WebView2 and Android's WebView, name an untitled about:blank page by its URL, where WebKit and browsers give it no title; an empty source reports no title on every backend.
		if nav.URL == "about:blank" && nav.Title == "about:blank" {
			nav.Title = ""
		}
	}
	call := func(fn func(WebNavigation)) {
		if fn != nil {
			t.within(n.owner, func() { fn(nav) })
		}
	}
	switch ev.Kind {
	case driver.EventLoadStart:
		// The page that would answer is going.
		t.endEvals(n, errWebPageLeft)
		call(wn.loadStart)
	case driver.EventLoad:
		call(wn.load)
	case driver.EventNavigationStateChange:
		call(wn.navigation)
	case driver.EventWebOpenWindow:
		if r, ok := ev.Data.(WebWindowRequest); ok && wn.openWindow != nil {
			t.within(n.owner, func() { wn.openWindow(r) })
		}
	case driver.EventError:
		if wn.loadError != nil {
			failure := driver.WebFailure{}
			switch f := ev.Data.(type) {
			case *driver.WebFailure:
				if f != nil {
					failure = *f
				}
			case driver.WebFailure:
				failure = f
			}
			t.within(n.owner, func() {
				wn.loadError(&WebError{URL: nav.URL, Message: ev.Text, Domain: failure.Domain, Code: failure.Code})
			})
		}
	case driver.EventDownload:
		if d, ok := ev.Data.(*WebDownload); ok && wn.download != nil {
			t.within(n.owner, func() { wn.download(*d) })
		}
	case driver.EventMessage:
		if t.answerEval(n, ev.Text) {
			return
		}
		if !wn.allows(ev) {
			if !wn.dropped {
				wn.dropped = true
				from := "a frame of the page"
				if ev.MainFrame {
					from = "a page of " + ev.Origin
				}
				log.Printf("nsgo: a web view dropped a message from %s, which its MessageOrigins do not allow", from)
			}
			return
		}
		if wn.message != nil {
			t.within(n.owner, func() { wn.message(ev.Text) })
		}
		if wn.messageFrom != nil {
			t.within(n.owner, func() { wn.messageFrom(WebMessage{Data: ev.Text, Origin: ev.Origin}) })
		}
	}
}

// webEvents is what a web view reports.
func (wn *webNode) events() driver.EventMask {
	var m driver.EventMask
	bit := func(k driver.EventKind, on bool) {
		if on {
			m |= 1 << k
		}
	}
	bit(driver.EventLoadStart, wn.loadStart != nil || len(wn.evals) > 0)
	bit(driver.EventLoad, wn.load != nil)
	bit(driver.EventNavigationStateChange, wn.navigation != nil)
	bit(driver.EventError, wn.loadError != nil)
	bit(driver.EventMessage, wn.message != nil || wn.messageFrom != nil || len(wn.evals) > 0)
	bit(driver.EventDownload, wn.download != nil)
	bit(driver.EventWebOpenWindow, wn.openWindow != nil)
	return m
}

// webCommand sends c to the web view r names.
func (r *Ref) webCommand(c driver.WebCommand, value string) {
	if r.n == nil || r.n.deleted || r.n.web == nil {
		return
	}
	r.t.commands = append(r.t.commands, driver.MountItem{Kind: driver.ItemWebCommand, ID: r.n.id, WebCommand: c, Value: value})
	r.t.changed = true
}

// Load loads src in the web view, as a new Source would, even when it is the source the view already shows: the page loads with the view's Headers, and a source Access refuses reaches OnError and loads nothing. It navigates the view, as a link does, and leaves the source alone: Source or SourceFunc's next change loads again, a change to the scripts or headers loads the source again, and messages come by default from the source's origin, not src's.
func (r *Ref) Load(src WebSource) {
	if r.n == nil || r.n.deleted || r.n.web == nil {
		return
	}
	wn := r.n.web
	if r.t.rt.restricted {
		url := src.Src
		report := func(err error) {
			if wn.loadError != nil {
				wn.loadError(&WebError{URL: url, Message: err.Error()})
			}
		}
		var refused string
		r.t.refuseSource(r.n, AccessWebView, &src.Src, &refused, report)
		if refused != "" {
			return
		}
		if src.BaseURL != "" && sourceRefusal(r.n.owner, AccessWebView, src.BaseURL) != nil {
			src.BaseURL = ""
		}
	}
	src = driver.ResolveWebSource(src)
	r.t.commands = append(r.t.commands, driver.MountItem{Kind: driver.ItemWebCommand, ID: r.n.id,
		WebCommand: driver.WebLoad, Data: &src})
	r.t.changed = true
}

// Reload loads the web view's page again. On native targets, reloading the page of the app's own source is not held to AccessWebSchemes.
func (r *Ref) Reload() { r.webCommand(driver.WebReload, "") }

// GoBack goes back in the web view's history. On native targets, going back to the page of the app's own source is not held to AccessWebSchemes.
func (r *Ref) GoBack() { r.webCommand(driver.WebGoBack, "") }

// GoForward goes forward in the web view's history. On native targets, going forward to the page of the app's own source is not held to AccessWebSchemes.
func (r *Ref) GoForward() { r.webCommand(driver.WebGoForward, "") }

// StopLoading stops the web view's page loading.
func (r *Ref) StopLoading() { r.webCommand(driver.WebStopLoading, "") }

// PostMessage dispatches a MessageEvent carrying data on the web view's
// page.
func (r *Ref) PostMessage(data string) { r.webCommand(driver.WebPostMessage, data) }

// InjectJavaScript runs script in the web view's page.
func (r *Ref) InjectJavaScript(script string) { r.webCommand(driver.WebInjectJavaScript, script) }

// SendInput gives the web view's page input as the user's own, whose
// events are trusted and carry user activation, which no script the page
// runs can make: a click, typed text or a key. It takes the keyboard focus
// first. It does nothing where Env.CanSendInput is false.
func (r *Ref) SendInput(in WebInput) {
	if r.n == nil || r.n.deleted || r.n.web == nil {
		return
	}
	if r.t.rt.restricted && refusal(accessOf(r.n.owner), AccessRequest{Permission: AccessWebView, Method: AccessInput}) != nil {
		log.Printf("nsgo: a web view's trusted input needs the %q capability's input", AccessWebView)
		return
	}
	if !Untracked(r.n.owner, r.n.owner.Env().CanSendInput) {
		return
	}
	r.t.commands = append(r.t.commands, driver.MountItem{Kind: driver.ItemWebCommand, ID: r.n.id, WebCommand: driver.WebSendInput, Data: &in})
	r.t.changed = true
}

// WebClick is a primary click at x, y in CSS pixels from the page's
// viewport, as getBoundingClientRect reports them.
func WebClick(x, y Points) WebInput {
	return WebInput{Kind: driver.WebInputClick, Point: Point{X: x, Y: y}}
}

// WebText types text into what has the page's focus.
func WebText(text string) WebInput { return WebInput{Kind: driver.WebInputText, Text: text} }

// WebKey presses key, as KeyboardEvent.key names it: Enter, Tab, Escape
// or Backspace.
func WebKey(key string) WebInput { return WebInput{Kind: driver.WebInputKey, Key: key} }

// errWebPageLeft ends an Evaluate whose page left before it answered.
var errWebPageLeft = errors.New("nsgo: the web view's page left before the script answered")

// evalMarker starts a page's answer to an Evaluate, before its random
// key, so no message of the page's own is taken for one. It is plain text:
// a platform's bridge may read the message as a C string.
const evalMarker = "nsgo-evaluate:"

// Evaluate runs script in the web view's page, as an expression, and
// calls done on the engine's goroutine, while the web view lives, with
// its result as JSON, or the
// error it threw: a promise it returns is awaited, undefined is null, and
// what JSON cannot hold (a function, a cycle) is an error. The answer
// comes through the page's channel to the app, as OnMessage's, on every
// backend with web views. A page that leaves first ends it with an error,
// as does a view with NoJavaScript, and a view that goes never answers.
func (r *Ref) Evaluate(script string, done func(json string, err error)) {
	if r.n == nil || r.n.deleted || r.n.web == nil {
		done("", errors.New("nsgo: Evaluate on no web view"))
		return
	}
	n, wn := r.n, r.n.web
	if wn.config.NoJavaScript {
		done("", errors.New("nsgo: the web view runs no JavaScript"))
		return
	}
	var key [8]byte
	_, _ = rand.Read(key[:])
	marker := evalMarker + hex.EncodeToString(key[:]) + ":"
	if wn.evals == nil {
		wn.evals = map[string]func(string, error){}
		r.t.touch(n)
	}
	t := r.t
	wn.evals[marker] = func(json string, err error) {
		if !n.owner.disposed {
			t.within(n.owner, func() { done(json, err) })
		}
	}
	r.webCommand(driver.WebInjectJavaScript, driver.WebEvaluateScript(script, marker))
}

// answerEval hands a page's answer to the Evaluate it is for, and
// reports whether text was one.
func (t *Tree) answerEval(n *node, text string) bool {
	wn := n.web
	if len(wn.evals) == 0 || !strings.HasPrefix(text, evalMarker) {
		return false
	}
	i := strings.IndexByte(text[len(evalMarker):], ':')
	if i < 0 {
		return false
	}
	marker := text[:len(evalMarker)+i+1]
	done, ok := wn.evals[marker]
	if !ok {
		return false
	}
	delete(wn.evals, marker)
	if len(wn.evals) == 0 {
		wn.evals = nil
		t.touch(n)
	}
	done(driver.WebEvaluateAnswer(text[len(marker):]))
	return true
}

// endEvals ends the Evaluates n waits for with err.
func (t *Tree) endEvals(n *node, err error) {
	evals := n.web.evals
	if len(evals) == 0 {
		return
	}
	n.web.evals = nil
	t.touch(n)
	for _, done := range evals {
		done("", err)
	}
}
