//go:build linux && !android

package linux

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/ebitengine/purego"

	"github.com/oneo-me/nsgo/core"
)

// WebKitGTK 6.0, the GTK 4 one, loads on the first web view, so apps
// without one neither pay for it nor need it installed.
var (
	webkitOnce sync.Once
	webkitErr  error

	webkitWebViewNew                     func() uintptr
	webkitWebViewGetUserContentManager   func(uintptr) uintptr
	webkitWebViewGetSettings             func(uintptr) uintptr
	webkitWebViewSetBackgroundColor      func(uintptr, *[4]float32)
	webkitWebViewLoadURI                 func(uintptr, *byte)
	webkitWebViewLoadHTML                func(uintptr, *byte, *byte)
	webkitWebViewReload                  func(uintptr)
	webkitWebViewGoBack                  func(uintptr)
	webkitWebViewGoForward               func(uintptr)
	webkitWebViewStopLoading             func(uintptr)
	webkitWebViewEvaluateJavascript      func(uintptr, *byte, int, *byte, *byte, uintptr, uintptr, uintptr)
	webkitWebViewExecuteEditingCommand   func(uintptr, *byte)
	webkitWebViewGetURI                  func(uintptr) uintptr
	webkitWebViewGetTitle                func(uintptr) uintptr
	webkitWebViewCanGoBack               func(uintptr) int32
	webkitWebViewCanGoForward            func(uintptr) int32
	webkitSettingsSetEnableJSMarkup      func(uintptr, int32)
	webkitSettingsSetUserAgent           func(uintptr, *byte)
	webkitSettingsSetBackForwardGestures func(uintptr, int32)
	webkitContentRegisterHandler         func(uintptr, *byte, *byte) int32
	webkitContentUnregisterHandler       func(uintptr, *byte, *byte)
	webkitContentAddScript               func(uintptr, uintptr)
	webkitContentRemoveAllScripts        func(uintptr)
	webkitContentAddStyleSheet           func(uintptr, uintptr)
	webkitContentRemoveAllStyleSheets    func(uintptr)
	webkitUserScriptNew                  func(*byte, int32, int32, uintptr, uintptr) uintptr
	webkitUserScriptUnref                func(uintptr)
	webkitUserStyleSheetNew              func(*byte, int32, int32, uintptr, uintptr) uintptr
	webkitUserStyleSheetUnref            func(uintptr)
	webkitNetworkErrorQuark              func() uint32
	webkitPolicyErrorQuark               func() uint32
	jscValueToString                     func(uintptr) uintptr
)

// openWebKit loads WebKitGTK and JavaScriptCore, reporting what is
// missing.
func openWebKit() error {
	webkitOnce.Do(func() {
		webkit, err := purego.Dlopen("libwebkitgtk-6.0.so.4", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			webkitErr = fmt.Errorf("web views need WebKitGTK 6.0 (libwebkitgtk-6.0.so.4): %w", err)
			return
		}
		jsc, err := purego.Dlopen("libjavascriptcoregtk-6.0.so.1", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			webkitErr = fmt.Errorf("web views need JavaScriptCore 6.0 (libjavascriptcoregtk-6.0.so.1): %w", err)
			return
		}
		defer func() {
			if r := recover(); r != nil {
				webkitErr = fmt.Errorf("web views need WebKitGTK 6.0: %v", r)
			}
		}()
		bindWebKit(webkit, jsc)
	})
	return webkitErr
}

func bindWebKit(webkit, jsc uintptr) {
	reg := func(fn any, name string) { purego.RegisterLibFunc(fn, webkit, name) }
	reg(&webkitWebViewNew, "webkit_web_view_new")
	reg(&webkitWebViewGetUserContentManager, "webkit_web_view_get_user_content_manager")
	reg(&webkitWebViewGetSettings, "webkit_web_view_get_settings")
	reg(&webkitWebViewSetBackgroundColor, "webkit_web_view_set_background_color")
	reg(&webkitWebViewLoadURI, "webkit_web_view_load_uri")
	reg(&webkitWebViewLoadHTML, "webkit_web_view_load_html")
	reg(&webkitWebViewReload, "webkit_web_view_reload")
	reg(&webkitWebViewGoBack, "webkit_web_view_go_back")
	reg(&webkitWebViewGoForward, "webkit_web_view_go_forward")
	reg(&webkitWebViewStopLoading, "webkit_web_view_stop_loading")
	reg(&webkitWebViewEvaluateJavascript, "webkit_web_view_evaluate_javascript")
	reg(&webkitWebViewExecuteEditingCommand, "webkit_web_view_execute_editing_command")
	reg(&webkitWebViewGetURI, "webkit_web_view_get_uri")
	reg(&webkitWebViewGetTitle, "webkit_web_view_get_title")
	reg(&webkitWebViewCanGoBack, "webkit_web_view_can_go_back")
	reg(&webkitWebViewCanGoForward, "webkit_web_view_can_go_forward")
	reg(&webkitSettingsSetEnableJSMarkup, "webkit_settings_set_enable_javascript_markup")
	reg(&webkitSettingsSetUserAgent, "webkit_settings_set_user_agent")
	reg(&webkitSettingsSetBackForwardGestures, "webkit_settings_set_enable_back_forward_navigation_gestures")
	reg(&webkitContentRegisterHandler, "webkit_user_content_manager_register_script_message_handler")
	reg(&webkitContentUnregisterHandler, "webkit_user_content_manager_unregister_script_message_handler")
	reg(&webkitContentAddScript, "webkit_user_content_manager_add_script")
	reg(&webkitContentRemoveAllScripts, "webkit_user_content_manager_remove_all_scripts")
	reg(&webkitContentAddStyleSheet, "webkit_user_content_manager_add_style_sheet")
	reg(&webkitContentRemoveAllStyleSheets, "webkit_user_content_manager_remove_all_style_sheets")
	reg(&webkitUserScriptNew, "webkit_user_script_new")
	reg(&webkitUserScriptUnref, "webkit_user_script_unref")
	reg(&webkitUserStyleSheetNew, "webkit_user_style_sheet_new")
	reg(&webkitUserStyleSheetUnref, "webkit_user_style_sheet_unref")
	reg(&webkitNetworkErrorQuark, "webkit_network_error_quark")
	reg(&webkitPolicyErrorQuark, "webkit_policy_error_quark")
	purego.RegisterLibFunc(&jscValueToString, jsc, "jsc_value_to_string")
}

// WebKit's enums.
const (
	webkitTopFrame         = 1 // WEBKIT_USER_CONTENT_INJECT_TOP_FRAME
	webkitAtStart          = 0 // WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START
	webkitAtEnd            = 1 // WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_END
	webkitUserStyle        = 0 // WEBKIT_USER_STYLE_LEVEL_USER
	webkitLoadStarted      = 0
	webkitLoadCommitted    = 2
	webkitLoadFinished     = 3
	webkitNetworkCancelled = 302 // WEBKIT_NETWORK_ERROR_CANCELLED
	webkitPolicyInterrupt  = 102 // WEBKIT_POLICY_ERROR_FRAME_LOAD_INTERRUPTED_BY_POLICY_CHANGE
	gtkOverflowHidden      = 1
)

// webHandler is the name pages post to, as
// window.webkit.messageHandlers.nsgo.
const webHandler = "nsgo"

var webBridge = core.WebBridge("function (d) { window.webkit.messageHandlers." + webHandler + ".postMessage(d); }")

// webNoScroll keeps the top document from scrolling.
const webNoScroll = "html, body { overflow: hidden !important; }"

// web is a web view's WebKitWebView. It sits in a clipping GtkOverlay
// placed over the surface as entries are, so scroll views and clipping
// ancestors cut it off; the overlay gives the page its whole frame.
type web struct {
	holder  uintptr // GtkOverlay, the visible part of the frame
	widget  uintptr // WebKitWebView
	content uintptr // its WebKitUserContentManager
	config  core.WebConfig
	made    bool
	failed  bool // the navigation failed; WebKit still finishes it
	pending bool // a load waits for the page to be mapped
	shown   bool
	placed  core.Bounds // the page's frame in the holder
	visible core.Bounds // the holder's frame in the surface
}

// configureWeb applies a web view's configuration, making its page on
// the first one and loading the source again when it or the scripts
// changed.
func (w *window) configureWeb(v *view, c *core.WebConfig) {
	if v.web == nil {
		v.web = &web{}
		if err := openWebKit(); err != nil {
			// After the batch, which may carry the view's events after
			// its configuration.
			w.post(func() { w.report(v, core.Event{Kind: core.EventError, ID: v.id, Text: err.Error()}) })
			return
		}
		w.makeWeb(v)
	}
	wv := v.web
	if wv.widget == 0 {
		return
	}
	old, first := wv.config, !wv.made
	wv.config, wv.made = *c, true
	scripts := first || c.InjectedJavaScript != old.InjectedJavaScript ||
		c.InjectedJavaScriptBeforeContent != old.InjectedJavaScriptBeforeContent
	if scripts {
		webkitContentRemoveAllScripts(wv.content)
		addScript(wv.content, webBridge, webkitAtStart)
		addScript(wv.content, c.InjectedJavaScriptBeforeContent, webkitAtStart)
		addScript(wv.content, c.InjectedJavaScript, webkitAtEnd)
	}
	if first || c.NoScroll != old.NoScroll {
		webkitContentRemoveAllStyleSheets(wv.content)
		if c.NoScroll {
			sheet := webkitUserStyleSheetNew(cstr(webNoScroll), webkitTopFrame, webkitUserStyle, 0, 0)
			webkitContentAddStyleSheet(wv.content, sheet)
			webkitUserStyleSheetUnref(sheet)
		}
	}
	settings := webkitWebViewGetSettings(wv.widget)
	// Markup's scripts only: injected ones still run, as WebKit's
	// allowsContentJavaScript has it on Apple's platforms.
	webkitSettingsSetEnableJSMarkup(settings, boolInt(!c.NoJavaScript))
	if c.UserAgent == "" {
		webkitSettingsSetUserAgent(settings, nil)
	} else {
		webkitSettingsSetUserAgent(settings, cstr(c.UserAgent))
	}
	webkitSettingsSetBackForwardGestures(settings, boolInt(c.BackForwardGestures))
	if first || scripts || c.NoScroll != old.NoScroll || c.NoJavaScript != old.NoJavaScript || c.Source != old.Source {
		w.loadWeb(v)
	}
}

func addScript(content uintptr, source string, at int32) {
	if source == "" {
		return
	}
	script := webkitUserScriptNew(cstr(source), webkitTopFrame, at, 0, 0)
	webkitContentAddScript(content, script)
	webkitUserScriptUnref(script)
}

// makeWeb makes v's page and its holder, hidden until placed.
func (w *window) makeWeb(v *view) {
	wv := v.web
	wv.widget = webkitWebViewNew()
	wv.content = webkitWebViewGetUserContentManager(wv.widget)
	webkitContentRegisterHandler(wv.content, cstr(webHandler), nil)
	// WebKit paints white otherwise; a page without a background shows
	// the view's own, as its style expects.
	webkitWebViewSetBackgroundColor(wv.widget, &[4]float32{})
	wv.holder = gtkOverlayNew()
	gtkWidgetSetOverflow(wv.holder, gtkOverflowHidden)
	gtkOverlayAddOverlay(wv.holder, wv.widget)
	gtkWidgetSetVisible(wv.holder, 0)
	gtkFixedPut(w.fixed, wv.holder, 0, 0)
	for _, p := range []uintptr{wv.holder, wv.widget, wv.content} {
		w.webs[p] = v
	}
	gSignalConnect(wv.holder, "get-child-position", cbWebChildPosition, w.handle)
	gSignalConnect(wv.widget, "load-changed", cbWebLoadChanged, w.handle)
	gSignalConnect(wv.widget, "load-failed", cbWebLoadFailed, w.handle)
	gSignalConnect(wv.widget, "map", cbWebMap, w.handle)
	gSignalConnect(wv.content, "script-message-received::"+webHandler, cbWebMessage, w.handle)
}

// loadWeb loads v's source once its page is mapped. With WebKit's
// DMA-BUF renderer a page loaded before it is mapped can stay blank for
// good (NSH's Linux backend found so), so WebKit's MiniBrowser loads
// after mapping too.
func (w *window) loadWeb(v *view) {
	wv := v.web
	if gtkWidgetGetMapped(wv.widget) == 0 {
		wv.pending = true
		return
	}
	wv.pending = false
	src := wv.config.Source
	switch {
	case src.HTML != "":
		var base *byte
		if src.BaseURL != "" {
			base = cstr(webURI(src.BaseURL))
		}
		webkitWebViewLoadHTML(wv.widget, cstr(src.HTML), base)
	case src.Src != "":
		webkitWebViewLoadURI(wv.widget, cstr(webURI(src.Src)))
	}
}

// webURI takes a path without a scheme as a file's.
func webURI(s string) string {
	if strings.HasPrefix(s, "/") {
		return (&url.URL{Scheme: "file", Path: s}).String()
	}
	return s
}

// webCommand has v's page do c, with value for messages and scripts.
func (w *window) webCommand(v *view, c core.WebCommand, value string) {
	if v.web == nil || v.web.widget == 0 {
		return
	}
	page := v.web.widget
	switch c {
	case core.WebReload:
		webkitWebViewReload(page)
	case core.WebGoBack:
		webkitWebViewGoBack(page)
	case core.WebGoForward:
		webkitWebViewGoForward(page)
	case core.WebStopLoading:
		webkitWebViewStopLoading(page)
	case core.WebPostMessage:
		evaluate(page, core.WebMessageScript(value))
	case core.WebInjectJavaScript:
		evaluate(page, value)
	}
}

func evaluate(page uintptr, script string) {
	webkitWebViewEvaluateJavascript(page, cstr(script), -1, nil, nil, 0, 0, 0)
}

// closeWeb stops v's page and lets it go with its holder.
func (w *window) closeWeb(v *view) {
	wv := v.web
	v.web = nil
	if wv == nil || wv.widget == 0 {
		return
	}
	for _, p := range []uintptr{wv.holder, wv.widget, wv.content} {
		delete(w.webs, p)
	}
	webkitWebViewStopLoading(wv.widget)
	webkitContentUnregisterHandler(wv.content, cstr(webHandler), nil)
	// Unparenting drops GTK's only reference, which gtk_fixed_put took.
	gtkFixedRemove(w.fixed, wv.holder)
}

// placeWebs moves each page's holder to the part of its frame its
// clipping ancestors leave, and hides the ones that are hidden, detached
// or clipped away.
func (w *window) placeWebs() {
	for p, v := range w.webs {
		wv := v.web
		if p != wv.widget {
			continue
		}
		at := w.frameAt(v)
		b := core.Bounds{Origin: at, Size: v.frame.Size}
		visible := intersectBounds(b, w.surfaceClip(v))
		if !w.shownInTree(v) || visible.Size.Width <= 0 || visible.Size.Height <= 0 {
			if wv.shown {
				gtkWidgetSetVisible(wv.holder, 0)
				wv.shown = false
			}
			continue
		}
		placed := core.Bounds{
			Origin: core.Point{X: b.Origin.X - visible.Origin.X, Y: b.Origin.Y - visible.Origin.Y},
			Size:   b.Size,
		}
		if visible != wv.visible {
			gtkFixedMove(w.fixed, wv.holder, float64(visible.Origin.X), float64(visible.Origin.Y))
			gtkWidgetSetSizeRequest(wv.holder, int32(visible.Size.Width), int32(visible.Size.Height))
			wv.visible = visible
		}
		if placed != wv.placed {
			wv.placed = placed
			gtkWidgetQueueAllocate(wv.holder)
		}
		if !wv.shown {
			gtkWidgetSetVisible(wv.holder, 1)
			wv.shown = true
		}
	}
}

// shownInTree reports whether v is in the root's tree with it and every
// ancestor shown.
func (w *window) shownInTree(v *view) bool {
	top := v
	for p := v; p != nil; p = p.parent {
		if p.deleted || p.style.Hidden || p.style.Opacity <= 0 {
			return false
		}
		top = p
	}
	return top == w.view(w.root)
}

// focusedWeb returns the web view whose page has keyboard focus, or nil.
func (w *window) focusedWeb() *view {
	if len(w.webs) == 0 {
		return nil
	}
	for f := gtkWindowGetFocus(w.win); f != 0; f = gtkWidgetGetParent(f) {
		if v := w.webs[f]; v != nil && v.web.widget == f {
			return v
		}
	}
	return nil
}

// webEdit runs an editing menu role in the focused page, if any.
func (w *window) webEdit(role core.MenuRole) bool {
	v := w.focusedWeb()
	command := map[core.MenuRole]string{
		core.MenuUndo: "Undo", core.MenuRedo: "Redo", core.MenuCut: "Cut",
		core.MenuCopy: "Copy", core.MenuPaste: "Paste", core.MenuSelectAll: "SelectAll",
	}[role]
	if v == nil || command == "" {
		return false
	}
	webkitWebViewExecuteEditingCommand(v.web.widget, cstr(command))
	return true
}

// webOf returns the live web view p belongs to.
func webOf(data, p uintptr) (*window, *view) {
	w := windowOf(data)
	if w == nil || w.closing {
		return nil, nil
	}
	v := w.webs[p]
	if v == nil || v.deleted || v.web == nil {
		return nil, nil
	}
	return w, v
}

// navigated reports the page's state as kinds.
func (w *window) navigated(v *view, loading bool, kinds ...core.EventKind) {
	page := v.web.widget
	nav := core.WebNavigation{
		URL:          goString(webkitWebViewGetURI(page)),
		Title:        goString(webkitWebViewGetTitle(page)),
		Loading:      loading,
		CanGoBack:    webkitWebViewCanGoBack(page) != 0,
		CanGoForward: webkitWebViewCanGoForward(page) != 0,
	}
	for _, k := range kinds {
		n := nav
		w.report(v, core.Event{Kind: k, ID: v.id, Navigation: &n})
	}
}

func webLoadChangedCallback(page uintptr, event uint32, data uintptr) {
	w, v := webOf(data, page)
	if v == nil {
		return
	}
	switch event {
	case webkitLoadStarted:
		v.web.failed = false
		w.navigated(v, true, core.EventLoadStart, core.EventNavigationStateChange)
	case webkitLoadCommitted:
		w.navigated(v, true, core.EventNavigationStateChange)
	case webkitLoadFinished:
		// A failed navigation finishes too; it was reported as an error.
		if !v.web.failed {
			w.navigated(v, false, core.EventLoad, core.EventNavigationStateChange)
		}
	}
}

// gError is GLib's GError.
type gError struct {
	domain  uint32
	code    int32
	message uintptr
}

func webLoadFailedCallback(page uintptr, event uint32, uri, gerr uintptr, data uintptr) int32 {
	w, v := webOf(data, page)
	if v == nil {
		return 0
	}
	v.web.failed = true
	if gerr != 0 {
		e := (*gError)(cPointer(gerr))
		// Stopped or replaced by another navigation: not errors.
		cancelled := e.domain == webkitNetworkErrorQuark() && e.code == webkitNetworkCancelled ||
			e.domain == webkitPolicyErrorQuark() && e.code == webkitPolicyInterrupt
		if !cancelled {
			w.report(v, core.Event{Kind: core.EventError, ID: v.id, Text: goString(e.message)})
		}
	}
	return 1 // the app shows failures itself, without WebKit's error page
}

func webMessageCallback(content, value uintptr, data uintptr) {
	w, v := webOf(data, content)
	if v == nil {
		return
	}
	p := jscValueToString(value)
	text := goString(p)
	if p != 0 {
		gFree(p)
	}
	w.report(v, core.Event{Kind: core.EventMessage, ID: v.id, Text: text})
}

func webMapCallback(page uintptr, data uintptr) {
	w, v := webOf(data, page)
	if v != nil && v.web.pending {
		w.loadWeb(v)
	}
}

// webChildPositionCallback gives the page its whole frame in the holder,
// which clips it to the visible part.
func webChildPositionCallback(holder, child, allocation uintptr, data uintptr) int32 {
	_, v := webOf(data, holder)
	if v == nil || allocation == 0 {
		return 0
	}
	r := (*[4]int32)(cPointer(allocation))
	p := v.web.placed
	*r = [4]int32{int32(p.Origin.X), int32(p.Origin.Y), int32(p.Size.Width), int32(p.Size.Height)}
	return 1
}
