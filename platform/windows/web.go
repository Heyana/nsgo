//go:build windows

package windows

// Web views are WebView2 controllers, each in a child window of its own
// that is placed, clipped and hidden over its view as an input's EDIT is.
// WebView2 is asynchronous: the window's environment and then the
// controller are made on its message loop, and the configuration and
// commands that come before the controller wait for it. Without the
// runtime the view shows its background and reports one EventError.
//
// The bridge, NoScroll's style and InjectedJavaScriptBeforeContent are
// one script run as each document is created; InjectedJavaScript runs
// when a navigation completes. HTML with a base URL gets a <base>
// element, so relative links resolve against it while the page's origin
// stays NavigateToString's. The page's default background is the view's
// own color, opaque, since GDI draws nothing under a child window.

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
)

var webBridge = core.WebBridge("function (d) { window.chrome.webview.postMessage(d); }")

// webNoScroll stops the page's viewport scrolling; scripts still may.
const webNoScroll = "(function () { var hide = function () {" +
	" document.documentElement.style.setProperty('overflow', 'hidden', 'important');" +
	" document.body && document.body.style.setProperty('overflow', 'hidden', 'important'); };" +
	" if (document.readyState === 'loading') { document.addEventListener('DOMContentLoaded', hide, { once: true }); }" +
	" else { hide(); } })();"

// webView is a web view's host window and WebView2 objects.
type webView struct {
	w    *window
	v    *view
	host uintptr

	controller, web, settings, settings2 *comObject
	ready, closed, failed                bool

	config    core.WebConfig
	applied   core.WebConfig // what the web view last loaded with
	commands  []webCommand   // waiting for the controller
	userAgent string         // WebView2's own

	script    string // the document script's ID
	scriptGen int    // the latest document script asked for
	loadAfter bool   // load once that script is added
	executed  *comHandler
	loading   bool

	// Where the host window is, as placeEdits does for EDITs.
	shown  bool
	placed uint32
	rect   rect
	clip   rect
	radius int32
}

type webCommand struct {
	command core.WebCommand
	value   string
}

const webHostClass = "NsgoWebHost"

var (
	webHosts            = map[uintptr]*view{} // by host window, the main thread's
	webHostRegistered   bool
	webHostProcCallback = syscall.NewCallback(webHostProc)
)

// newWeb makes v's host window and starts making its controller.
func (w *window) newWeb(v *view) *webView {
	instance, _, _ := getModuleHandleW.Call(0)
	if !webHostRegistered {
		wc := wndClassEx{WndProc: webHostProcCallback, Instance: instance, ClassName: wide(webHostClass)}
		wc.Size = uint32(unsafe.Sizeof(wc))
		registerClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		webHostRegistered = true
	}
	wv := &webView{w: w, v: v}
	wv.host, _, _ = createWindowExW.Call(0, uintptr(unsafe.Pointer(wide(webHostClass))), 0,
		wsChild|wsClipChildren, 0, 0, 0, 0, w.hwnd, 0, instance, 0)
	if wv.host != 0 {
		webHosts[wv.host] = v
		w.webs[wv.host] = v
	}
	w.withWebEnvironment(wv.environmentReady)
	return wv
}

func (wv *webView) environmentReady(env *comObject, err error) {
	if wv.closed {
		return
	}
	if err != nil {
		wv.fail(err.Error())
		return
	}
	var h *comHandler
	h = newHandler(&iidControllerCompleted, func(hr uintptr, result unsafe.Pointer) {
		defer h.release()
		if wv.closed {
			if c := (*comObject)(result); c != nil && !failed(hr) {
				c.call(controllerClose)
			}
			return
		}
		if failed(hr) || result == nil {
			wv.fail(hresultError("WebView2 could not make the web view", hr).Error())
			return
		}
		c := (*comObject)(result)
		c.call(comAddRef)
		wv.controllerReady(c)
	})
	if hr := env.call(envCreateController, wv.host, h.ptr()); failed(hr) {
		h.release()
		wv.fail(hresultError("WebView2 could not make the web view", hr).Error())
	}
}

// fail reports why the view stays empty, once.
func (wv *webView) fail(why string) {
	if wv.failed {
		return
	}
	wv.failed = true
	wv.report(core.Event{Kind: core.EventError, Text: why})
}

func (wv *webView) report(e core.Event) {
	if wv.v.events.Has(e.Kind) {
		e.ID = wv.v.id
		wv.w.engine.Dispatch(e)
	}
}

func (wv *webView) controllerReady(c *comObject) {
	wv.controller = c
	c.call(controllerGetCoreWebView2, uintptr(unsafe.Pointer(&wv.web)))
	if wv.web == nil {
		wv.fail("WebView2 gave no web view")
		return
	}
	wv.web.call(webGetSettings, uintptr(unsafe.Pointer(&wv.settings)))
	if wv.settings != nil {
		if wv.settings2 = wv.settings.query(&iidSettings2); wv.settings2 != nil {
			wv.userAgent = wv.settings2.getString(settingsGetUserAgent)
		}
	}
	wv.restyle()
	wv.resize()
	wv.controller.call(controllerPutIsVisible, boolArg(wv.shown))
	wv.listen()
	wv.executed = newHandler(&iidExecuteScriptCompleted, nil)
	wv.ready = true
	wv.apply(true)
	commands := wv.commands
	wv.commands = nil
	for _, c := range commands {
		wv.command(c.command, c.value)
	}
}

func (wv *webView) listen() {
	web := wv.web
	web.subscribe(webAddNavigationStarting, newHandler(&iidNavigationStarting, func(uintptr, unsafe.Pointer) {
		if !wv.closed {
			wv.loading = true
			wv.navigated(core.EventLoadStart, core.EventNavigationStateChange)
		}
	}))
	web.subscribe(webAddNavigationCompleted, newHandler(&iidNavigationCompleted, func(_ uintptr, args unsafe.Pointer) {
		if wv.closed {
			return
		}
		wv.loading = false
		a := (*comObject)(args)
		var status int32
		a.call(navigationGetWebErrorStatus, uintptr(unsafe.Pointer(&status)))
		if a.getBool(navigationGetIsSuccess) {
			if js := wv.config.InjectedJavaScript; js != "" {
				wv.execute(js)
			}
			wv.navigated(core.EventLoad, core.EventNavigationStateChange)
			return
		}
		if why := webError(status); why != "" {
			wv.report(core.Event{Kind: core.EventError, Text: why})
		}
		wv.navigated(core.EventNavigationStateChange)
	}))
	changed := func(uintptr, unsafe.Pointer) {
		if !wv.closed {
			wv.navigated(core.EventNavigationStateChange)
		}
	}
	web.subscribe(webAddDocumentTitleChanged, newHandler(&iidDocumentTitleChanged, changed))
	web.subscribe(webAddHistoryChanged, newHandler(&iidHistoryChanged, changed))
	web.subscribe(webAddWebMessageReceived, newHandler(&iidWebMessageReceived, func(_ uintptr, args unsafe.Pointer) {
		if wv.closed || !wv.v.events.Has(core.EventMessage) {
			return
		}
		// The bridge posts strings; anything else has no string to read.
		var p *uint16
		if failed((*comObject)(args).call(messageTryGetString, uintptr(unsafe.Pointer(&p)))) {
			return
		}
		wv.report(core.Event{Kind: core.EventMessage, Text: takeString(p)})
	}))
}

// navigated reports the page's state as kinds.
func (wv *webView) navigated(kinds ...core.EventKind) {
	var mask core.EventMask
	for _, k := range kinds {
		mask |= 1 << k
	}
	if wv.v.events&mask == 0 {
		return
	}
	nav := core.WebNavigation{
		URL:          wv.web.getString(webGetSource),
		Title:        wv.web.getString(webGetDocumentTitle),
		Loading:      wv.loading,
		CanGoBack:    wv.web.getBool(webGetCanGoBack),
		CanGoForward: wv.web.getBool(webGetCanGoForward),
	}
	for _, k := range kinds {
		n := nav
		wv.report(core.Event{Kind: k, Navigation: &n})
	}
}

// webError is what a failed navigation's COREWEBVIEW2_WEB_ERROR_STATUS
// reports, "" for one that was replaced or cancelled.
func webError(status int32) string {
	switch status {
	case 1:
		return "The certificate's common name does not match the host."
	case 2:
		return "The certificate has expired."
	case 3:
		return "The client certificate contains errors."
	case 4:
		return "The certificate has been revoked."
	case 5:
		return "The certificate is invalid."
	case 6:
		return "The server is unreachable."
	case 7:
		return "The connection timed out."
	case 8:
		return "The server returned an invalid response."
	case 9:
		return "The connection was aborted."
	case 10:
		return "The connection was reset."
	case 11:
		return "The internet connection was lost."
	case 12:
		return "Cannot connect to the server."
	case 13:
		return "The host name could not be resolved."
	case 14:
		return ""
	case 15:
		return "A redirect failed."
	case 17:
		return "Authentication is required."
	case 18:
		return "Proxy authentication is required."
	}
	return "The page could not be loaded."
}

// configure takes c, applying it once the controller exists.
func (wv *webView) configure(c *core.WebConfig) {
	wv.config = *c
	if wv.ready {
		wv.apply(false)
	}
}

// apply sets what changed since the last load, loading the source again
// when it or the scripts changed.
func (wv *webView) apply(first bool) {
	c, old := wv.config, wv.applied
	wv.applied = c
	if wv.settings != nil && (first || c.NoJavaScript != old.NoJavaScript) {
		wv.settings.call(settingsPutIsScriptEnabled, boolArg(!c.NoJavaScript))
	}
	if wv.settings2 != nil && (first || c.UserAgent != old.UserAgent) {
		ua := c.UserAgent
		if ua == "" {
			ua = wv.userAgent
		}
		wv.settings2.call(settingsPutUserAgent, uintptr(unsafe.Pointer(wide(ua))))
	}
	scripts := first || c.InjectedJavaScriptBeforeContent != old.InjectedJavaScriptBeforeContent || c.NoScroll != old.NoScroll
	reload := scripts || c.Source != old.Source || c.InjectedJavaScript != old.InjectedJavaScript ||
		c.NoJavaScript != old.NoJavaScript
	if scripts {
		wv.loadAfter = true
		wv.addScript()
		return
	}
	if reload {
		wv.load()
	}
}

// documentScript runs as each document is created, in the main frame.
func (wv *webView) documentScript() string {
	var b strings.Builder
	b.WriteString("if (window === window.top) {\n")
	b.WriteString(webBridge)
	if wv.config.NoScroll {
		b.WriteString("\n" + webNoScroll)
	}
	if js := wv.config.InjectedJavaScriptBeforeContent; js != "" {
		b.WriteString("\n" + js)
	}
	b.WriteString("\n}")
	return b.String()
}

// addScript replaces the document script, then loads if asked to: the
// script is only sure to run on navigations after it is added.
func (wv *webView) addScript() {
	if wv.script != "" {
		wv.web.call(webRemoveScriptOnDocumentCreated, uintptr(unsafe.Pointer(wide(wv.script))))
		wv.script = ""
	}
	wv.scriptGen++
	gen := wv.scriptGen
	var h *comHandler
	h = newHandler(&iidAddScriptCompleted, func(hr uintptr, id unsafe.Pointer) {
		defer h.release()
		if wv.closed {
			return
		}
		added := ""
		if !failed(hr) {
			added = wideString((*uint16)(id))
		}
		if gen != wv.scriptGen {
			// A later script replaces this one.
			if added != "" {
				wv.web.call(webRemoveScriptOnDocumentCreated, uintptr(unsafe.Pointer(wide(added))))
			}
			return
		}
		wv.script = added
		if wv.loadAfter {
			wv.loadAfter = false
			wv.load()
		}
	})
	if failed(wv.web.call(webAddScriptOnDocumentCreated, uintptr(unsafe.Pointer(wide(wv.documentScript()))), h.ptr())) {
		h.release()
		if wv.loadAfter {
			wv.loadAfter = false
			wv.load()
		}
	}
}

func (wv *webView) load() {
	src := wv.config.Source
	switch {
	case src.HTML != "":
		html := src.HTML
		if src.BaseURL != "" {
			html = withBase(html, src.BaseURL)
		}
		wv.web.call(webNavigateToString, uintptr(unsafe.Pointer(wide(html))))
	case src.Src != "":
		if failed(wv.web.call(webNavigate, uintptr(unsafe.Pointer(wide(webURL(src.Src)))))) {
			wv.report(core.Event{Kind: core.EventError, Text: "invalid URL " + src.Src})
		}
	}
}

// webURL is s as WebView2 navigates to it: an absolute path as a file
// URL.
func webURL(s string) string {
	if filepath.IsAbs(s) {
		return "file:///" + filepath.ToSlash(s)
	}
	return s
}

// withBase puts a <base> naming baseURL at the start of html, after its
// doctype, which must stay first.
func withBase(html, baseURL string) string {
	base := `<base href="` + strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", "<", "&lt;").Replace(baseURL) + `">`
	trimmed := strings.TrimLeft(html, " \t\r\n\ufeff")
	if strings.HasPrefix(strings.ToLower(trimmed), "<!doctype") {
		if end := strings.IndexByte(trimmed, '>'); end >= 0 {
			return trimmed[:end+1] + base + trimmed[end+1:]
		}
	}
	return base + html
}

// command does c, or keeps it until the controller exists.
func (wv *webView) command(c core.WebCommand, value string) {
	if wv.failed || wv.closed {
		return
	}
	if !wv.ready {
		wv.commands = append(wv.commands, webCommand{c, value})
		return
	}
	switch c {
	case core.WebReload:
		wv.web.call(webReload)
	case core.WebGoBack:
		wv.web.call(webGoBack)
	case core.WebGoForward:
		wv.web.call(webGoForward)
	case core.WebStopLoading:
		wv.web.call(webStop)
	case core.WebPostMessage:
		wv.execute(core.WebMessageScript(value))
	case core.WebInjectJavaScript:
		wv.execute(value)
	}
}

func (wv *webView) execute(script string) {
	wv.web.call(webExecuteScript, uintptr(unsafe.Pointer(wide(script))), wv.executed.ptr())
}

// restyle gives the page the view's background and repaints the host.
func (wv *webView) restyle() {
	if wv.host != 0 {
		invalidateRect.Call(wv.host, 0, 1)
	}
	if wv.controller == nil {
		return
	}
	if c2 := wv.controller.query(&iidController2); c2 != nil {
		bg := background(wv.v)
		// COREWEBVIEW2_COLOR is A, R, G, B bytes, passed by value; only
		// opaque or fully transparent colors are allowed.
		color := uintptr(0xff) | uintptr(bg>>24&0xff)<<8 | uintptr(bg>>16&0xff)<<16 | uintptr(bg>>8&0xff)<<24
		c2.call(controllerPutDefaultBackgroundColor, color)
		c2.release()
	}
}

// resize fits the controller to its host window.
func (wv *webView) resize() {
	if wv.controller != nil {
		putBounds(wv.controller, rect{0, 0, wv.rect.Right - wv.rect.Left, wv.rect.Bottom - wv.rect.Top})
	}
}

// place shows the host window over r, the view's frame in the client
// area, clipped to clip.
func (wv *webView) place(r, clip rect, pass uint32) {
	visible := r.intersect(clip)
	if visible.empty() || wv.host == 0 {
		return
	}
	wv.placed = pass
	if r != wv.rect {
		moveWindow.Call(wv.host, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 1)
		sized := r.Right-r.Left != wv.rect.Right-wv.rect.Left || r.Bottom-r.Top != wv.rect.Bottom-wv.rect.Top
		wv.rect = r
		if sized {
			wv.resize()
		}
		if wv.controller != nil {
			wv.controller.call(controllerNotifyParentPositionChanged)
		}
	}
	visible = rect{visible.Left - r.Left, visible.Top - r.Top, visible.Right - r.Left, visible.Bottom - r.Top}
	full := rect{0, 0, r.Right - r.Left, r.Bottom - r.Top}
	radius := int32(0)
	if visible == full {
		radius = wv.w.px(wv.v.style.CornerRadius)
	}
	if visible != wv.clip || radius != wv.radius {
		var rgn uintptr
		switch {
		case radius > 0:
			rgn, _, _ = createRoundRectRgn.Call(0, 0, uintptr(full.Right+1), uintptr(full.Bottom+1),
				uintptr(2*radius), uintptr(2*radius))
		case visible != full:
			rgn, _, _ = createRectRgn.Call(uintptr(visible.Left), uintptr(visible.Top), uintptr(visible.Right), uintptr(visible.Bottom))
		}
		// The window owns the region from now on.
		setWindowRgn.Call(wv.host, rgn, 1)
		wv.clip, wv.radius = visible, radius
	}
	if !wv.shown {
		showWindow.Call(wv.host, swShowNA)
		wv.shown = true
		if wv.controller != nil {
			wv.controller.call(controllerPutIsVisible, 1)
		}
	}
}

func (wv *webView) hide() {
	if !wv.shown {
		return
	}
	showWindow.Call(wv.host, swHide)
	wv.shown = false
	if wv.controller != nil {
		wv.controller.call(controllerPutIsVisible, 0)
	}
}

// moved tells WebView2 the window moved, which it places pop-ups by.
func (wv *webView) moved() {
	if wv.controller != nil {
		wv.controller.call(controllerNotifyParentPositionChanged)
	}
}

// close lets the controller and its host window go.
func (wv *webView) close() {
	if wv.closed {
		return
	}
	wv.closed = true
	if wv.controller != nil {
		wv.controller.call(controllerClose)
	}
	wv.settings2.release()
	wv.settings.release()
	wv.web.release()
	wv.controller.release()
	wv.settings2, wv.settings, wv.web, wv.controller = nil, nil, nil, nil
	if wv.executed != nil {
		wv.executed.release()
		wv.executed = nil
	}
	if wv.host != 0 {
		delete(webHosts, wv.host)
		delete(wv.w.webs, wv.host)
		destroyWindow.Call(wv.host)
		wv.host = 0
	}
}

// webHostProc paints a host window with its view's background, which
// shows until the page draws and when there is no runtime.
func webHostProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		dc, _, _ := beginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if v := webHosts[hwnd]; v != nil && dc != 0 {
			brush, _, _ := createSolidBrush.Call(colorRef(background(v)))
			fillRect.Call(dc, uintptr(unsafe.Pointer(&ps.Paint)), brush)
			deleteObject.Call(brush)
		}
		endPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	r, _, _ := defWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}
