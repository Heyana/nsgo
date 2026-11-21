//go:build linux && !android

package linux

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// WebKitGTK 6.0, the GTK 4 one, loads on the first web view, so apps
// without one neither pay for it nor need it installed.
var (
	webkitOnce sync.Once
	webkitErr  error

	webkitWebViewGetType                     func() uintptr
	webkitWebViewGetContext                  func(uintptr) uintptr
	webkitWebContextRegisterURIScheme        func(uintptr, *byte, uintptr, uintptr, uintptr)
	webkitURISchemeRequestGetURI             func(uintptr) uintptr
	webkitURISchemeRequestGetWebView         func(uintptr) uintptr
	webkitURISchemeRequestGetHTTPMethod      func(uintptr) uintptr
	webkitURISchemeRequestGetHTTPHeaders     func(uintptr) uintptr
	webkitURISchemeResponseNew               func(uintptr, int64) uintptr
	webkitURISchemeResponseSetStatus         func(uintptr, int32, *byte)
	webkitURISchemeResponseSetContentType    func(uintptr, *byte)
	webkitURISchemeResponseSetHTTPHeaders    func(uintptr, uintptr)
	soupMessageHeadersNew                    func(int32) uintptr
	webkitURISchemeRequestFinishError        func(uintptr, uintptr)
	webkitURISchemeRequestFinishWithResponse func(uintptr, uintptr)
	webkitWebContextGetSecurityManager       func(uintptr) uintptr
	webkitSecurityManagerRegisterCORSScheme  func(uintptr, *byte)
	soupMessageHeadersIterInit               func(uintptr, uintptr)
	soupMessageHeadersIterNext               func(uintptr, uintptr, uintptr) int32
	gErrorNewLiteral                         func(uint32, int32, *byte) uintptr
	gQuarkToString                           func(uint32) uintptr
	gObjectRef                               func(uintptr) uintptr
	gMemoryInputStreamNewFromBytes           func(uintptr) uintptr
	webkitWebViewGetBackgroundColor          func(uintptr, *[4]float32)
	webkitNetworkSessionNew                  func(*byte, *byte) uintptr
	webkitNetworkSessionNewEphemeral         func() uintptr
	webkitNetworkSessionGetCookieManager     func(uintptr) uintptr
	webkitDownloadGetWebView                 func(uintptr) uintptr
	webkitDownloadGetRequest                 func(uintptr) uintptr
	webkitDownloadSetDestination             func(uintptr, *byte)
	webkitDownloadGetDestination             func(uintptr) uintptr
	webkitDownloadGetReceivedDataLength      func(uintptr) uint64
	webkitDownloadCancel                     func(uintptr)
	webkitCookieManagerSetPersistent         func(uintptr, *byte, int32)
	webkitWebViewGetUserContentManager       func(uintptr) uintptr
	webkitWebViewGetSettings                 func(uintptr) uintptr
	webkitWebViewSetBackgroundColor          func(uintptr, *[4]float32)
	webkitWebViewLoadURI                     func(uintptr, *byte)
	webkitWebViewLoadRequest                 func(uintptr, uintptr)
	webkitWebViewLoadHTML                    func(uintptr, *byte, *byte)
	webkitWebViewReload                      func(uintptr)
	webkitWebViewGoBack                      func(uintptr)
	webkitWebViewGoToHistory                 func(uintptr, uintptr)
	webkitWebViewGoForward                   func(uintptr)
	webkitWebViewStopLoading                 func(uintptr)
	webkitWebViewEvaluateJavascript          func(uintptr, *byte, int, *byte, *byte, uintptr, uintptr, uintptr)
	webkitWebViewExecuteEditingCommand       func(uintptr, *byte)
	webkitWebViewGetURI                      func(uintptr) uintptr
	webkitWebViewGetMainResource             func(uintptr) uintptr
	webkitWebViewGetTitle                    func(uintptr) uintptr
	webkitWebViewIsLoading                   func(uintptr) int32
	webkitWebViewCanGoBack                   func(uintptr) int32
	webkitWebViewCanGoForward                func(uintptr) int32
	webkitWebViewGetBackForwardList          func(uintptr) uintptr
	webkitBackForwardListGetCurrentItem      func(uintptr) uintptr
	webkitBackForwardListGetNthItem          func(uintptr, int32) uintptr
	webkitBackForwardListGetBackItem         func(uintptr) uintptr
	webkitBackForwardListGetForwardItem      func(uintptr) uintptr
	webkitBackForwardListItemGetURI          func(uintptr) uintptr
	webkitSettingsSetEnableJSMarkup          func(uintptr, int32)
	webkitSettingsSetUserAgent               func(uintptr, *byte)
	webkitSettingsGetUserAgent               func(uintptr) uintptr
	webkitSettingsSetBackForwardGestures     func(uintptr, int32)
	webkitSettingsSetEnableMediaStream       func(uintptr, int32)
	webkitUserMediaRequestGetType            func() uintptr
	webkitUserMediaIsForAudio                func(uintptr) int32
	webkitUserMediaIsForVideo                func(uintptr) int32
	webkitUserMediaIsForDisplay              func(uintptr) int32
	webkitPermissionRequestAllow             func(uintptr)
	webkitPermissionRequestDeny              func(uintptr)
	webkitContentRegisterHandler             func(uintptr, *byte, *byte) int32
	webkitContentUnregisterHandler           func(uintptr, *byte, *byte)
	webkitContentAddScript                   func(uintptr, uintptr)
	webkitContentRemoveAllScripts            func(uintptr)
	webkitContentAddStyleSheet               func(uintptr, uintptr)
	webkitContentRemoveAllStyleSheets        func(uintptr)
	webkitUserScriptNew                      func(*byte, int32, int32, uintptr, uintptr) uintptr
	webkitUserScriptUnref                    func(uintptr)
	webkitUserStyleSheetNew                  func(*byte, int32, int32, uintptr, uintptr) uintptr
	webkitUserStyleSheetUnref                func(uintptr)
	webkitNetworkErrorQuark                  func() uint32
	webkitPolicyErrorQuark                   func() uint32
	webkitNavigationDecisionGetAction        func(uintptr) uintptr
	webkitNavigationActionGetRequest         func(uintptr) uintptr
	webkitNavigationActionGetType            func(uintptr) uint32
	webkitURIRequestNew                      func(*byte) uintptr
	webkitURIRequestGetHTTPHeaders           func(uintptr) uintptr
	webkitURIRequestGetURI                   func(uintptr) uintptr
	webkitPolicyDecisionIgnore               func(uintptr)
	webkitResponseDecisionGetRequest         func(uintptr) uintptr
	webkitResponseDecisionIsMainFrame        func(uintptr) bool
	webkitMessageHeadersAppend               func(uintptr, *byte, *byte)
	jscValueToString                         func(uintptr) uintptr
)

// openWebKit loads WebKitGTK and JavaScriptCore and checks that the system
// can start WebKit's sandbox.
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
		soup, err := purego.Dlopen("libsoup-3.0.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			webkitErr = fmt.Errorf("web views need libsoup 3 (libsoup-3.0.so.0): %w", err)
			return
		}
		defer func() {
			if r := recover(); r != nil {
				webkitErr = fmt.Errorf("web views need WebKitGTK 6.0: %v", r)
			}
		}()
		if webkitErr = checkWebKitSandbox(webKitUserNSRestriction, "/usr/bin/bwrap"); webkitErr != nil {
			return
		}
		bindWebKit(webkit, jsc, soup)
	})
	return webkitErr
}

func bindWebKit(webkit, jsc, soup uintptr) {
	reg := func(fn any, name string) { purego.RegisterLibFunc(fn, webkit, name) }
	reg(&webkitWebViewGetType, "webkit_web_view_get_type")
	reg(&webkitWebViewGetContext, "webkit_web_view_get_context")
	reg(&webkitWebContextRegisterURIScheme, "webkit_web_context_register_uri_scheme")
	reg(&webkitURISchemeRequestGetHTTPMethod, "webkit_uri_scheme_request_get_http_method")
	reg(&webkitURISchemeRequestGetHTTPHeaders, "webkit_uri_scheme_request_get_http_headers")
	reg(&webkitURISchemeResponseNew, "webkit_uri_scheme_response_new")
	reg(&webkitURISchemeResponseSetStatus, "webkit_uri_scheme_response_set_status")
	reg(&webkitURISchemeResponseSetContentType, "webkit_uri_scheme_response_set_content_type")
	reg(&webkitURISchemeResponseSetHTTPHeaders, "webkit_uri_scheme_response_set_http_headers")
	reg(&webkitURISchemeRequestFinishWithResponse, "webkit_uri_scheme_request_finish_with_response")
	reg(&webkitWebContextGetSecurityManager, "webkit_web_context_get_security_manager")
	reg(&webkitSecurityManagerRegisterCORSScheme, "webkit_security_manager_register_uri_scheme_as_cors_enabled")
	purego.RegisterLibFunc(&soupMessageHeadersIterInit, soup, "soup_message_headers_iter_init")
	purego.RegisterLibFunc(&soupMessageHeadersIterNext, soup, "soup_message_headers_iter_next")
	purego.RegisterLibFunc(&soupMessageHeadersNew, soup, "soup_message_headers_new")
	reg(&webkitURISchemeRequestGetURI, "webkit_uri_scheme_request_get_uri")
	reg(&webkitURISchemeRequestGetWebView, "webkit_uri_scheme_request_get_web_view")
	reg(&webkitURISchemeRequestFinishError, "webkit_uri_scheme_request_finish_error")
	purego.RegisterLibFunc(&gErrorNewLiteral, glibLib, "g_error_new_literal")
	purego.RegisterLibFunc(&gQuarkToString, glibLib, "g_quark_to_string")
	purego.RegisterLibFunc(&gObjectRef, gobjectLib, "g_object_ref")
	purego.RegisterLibFunc(&gMemoryInputStreamNewFromBytes, gioLib, "g_memory_input_stream_new_from_bytes")
	reg(&webkitWebViewGetBackgroundColor, "webkit_web_view_get_background_color")
	reg(&webkitNetworkSessionNew, "webkit_network_session_new")
	reg(&webkitNetworkSessionNewEphemeral, "webkit_network_session_new_ephemeral")
	reg(&webkitNetworkSessionGetCookieManager, "webkit_network_session_get_cookie_manager")
	reg(&webkitDownloadGetWebView, "webkit_download_get_web_view")
	reg(&webkitDownloadGetRequest, "webkit_download_get_request")
	reg(&webkitDownloadSetDestination, "webkit_download_set_destination")
	reg(&webkitDownloadGetDestination, "webkit_download_get_destination")
	reg(&webkitDownloadGetReceivedDataLength, "webkit_download_get_received_data_length")
	reg(&webkitDownloadCancel, "webkit_download_cancel")
	reg(&webkitCookieManagerSetPersistent, "webkit_cookie_manager_set_persistent_storage")
	reg(&webkitWebViewGetUserContentManager, "webkit_web_view_get_user_content_manager")
	reg(&webkitWebViewGetSettings, "webkit_web_view_get_settings")
	reg(&webkitWebViewSetBackgroundColor, "webkit_web_view_set_background_color")
	reg(&webkitWebViewLoadURI, "webkit_web_view_load_uri")
	reg(&webkitWebViewLoadRequest, "webkit_web_view_load_request")
	reg(&webkitWebViewLoadHTML, "webkit_web_view_load_html")
	reg(&webkitWebViewReload, "webkit_web_view_reload")
	reg(&webkitWebViewGoBack, "webkit_web_view_go_back")
	reg(&webkitWebViewGoToHistory, "webkit_web_view_go_to_back_forward_list_item")
	reg(&webkitWebViewGoForward, "webkit_web_view_go_forward")
	reg(&webkitWebViewStopLoading, "webkit_web_view_stop_loading")
	reg(&webkitWebViewEvaluateJavascript, "webkit_web_view_evaluate_javascript")
	reg(&webkitWebViewExecuteEditingCommand, "webkit_web_view_execute_editing_command")
	reg(&webkitWebViewGetURI, "webkit_web_view_get_uri")
	reg(&webkitWebViewGetMainResource, "webkit_web_view_get_main_resource")
	reg(&webkitWebViewGetTitle, "webkit_web_view_get_title")
	reg(&webkitWebViewIsLoading, "webkit_web_view_is_loading")
	reg(&webkitWebViewCanGoBack, "webkit_web_view_can_go_back")
	reg(&webkitWebViewCanGoForward, "webkit_web_view_can_go_forward")
	reg(&webkitWebViewGetBackForwardList, "webkit_web_view_get_back_forward_list")
	reg(&webkitBackForwardListGetCurrentItem, "webkit_back_forward_list_get_current_item")
	reg(&webkitBackForwardListGetNthItem, "webkit_back_forward_list_get_nth_item")
	reg(&webkitBackForwardListGetBackItem, "webkit_back_forward_list_get_back_item")
	reg(&webkitBackForwardListGetForwardItem, "webkit_back_forward_list_get_forward_item")
	reg(&webkitBackForwardListItemGetURI, "webkit_back_forward_list_item_get_uri")
	reg(&webkitSettingsSetEnableJSMarkup, "webkit_settings_set_enable_javascript_markup")
	reg(&webkitSettingsSetUserAgent, "webkit_settings_set_user_agent")
	reg(&webkitSettingsGetUserAgent, "webkit_settings_get_user_agent")
	reg(&webkitSettingsSetBackForwardGestures, "webkit_settings_set_enable_back_forward_navigation_gestures")
	reg(&webkitSettingsSetEnableMediaStream, "webkit_settings_set_enable_media_stream")
	reg(&webkitUserMediaRequestGetType, "webkit_user_media_permission_request_get_type")
	reg(&webkitUserMediaIsForAudio, "webkit_user_media_permission_is_for_audio_device")
	reg(&webkitUserMediaIsForVideo, "webkit_user_media_permission_is_for_video_device")
	reg(&webkitUserMediaIsForDisplay, "webkit_user_media_permission_is_for_display_device")
	reg(&webkitPermissionRequestAllow, "webkit_permission_request_allow")
	reg(&webkitPermissionRequestDeny, "webkit_permission_request_deny")
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
	reg(&webkitNavigationDecisionGetAction, "webkit_navigation_policy_decision_get_navigation_action")
	reg(&webkitNavigationActionGetRequest, "webkit_navigation_action_get_request")
	reg(&webkitNavigationActionGetType, "webkit_navigation_action_get_navigation_type")
	reg(&webkitURIRequestNew, "webkit_uri_request_new")
	reg(&webkitURIRequestGetHTTPHeaders, "webkit_uri_request_get_http_headers")
	reg(&webkitURIRequestGetURI, "webkit_uri_request_get_uri")
	reg(&webkitPolicyDecisionIgnore, "webkit_policy_decision_ignore")
	reg(&webkitResponseDecisionGetRequest, "webkit_response_policy_decision_get_request")
	reg(&webkitResponseDecisionIsMainFrame, "webkit_response_policy_decision_is_main_frame_main_resource")
	purego.RegisterLibFunc(&webkitMessageHeadersAppend, soup, "soup_message_headers_append")
	purego.RegisterLibFunc(&jscValueToString, jsc, "jsc_value_to_string")
	bindWebCookies(webkit, soup)
}

// WebKit's enums.
const (
	webkitAllFrames        = 0 // WEBKIT_USER_CONTENT_INJECT_ALL_FRAMES
	webkitTopFrame         = 1 // WEBKIT_USER_CONTENT_INJECT_TOP_FRAME
	webkitAtStart          = 0 // WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_START
	webkitAtEnd            = 1 // WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_END
	webkitUserStyle        = 0 // WEBKIT_USER_STYLE_LEVEL_USER
	webkitLoadStarted      = 0
	webkitLoadCommitted    = 2
	webkitLoadFinished     = 3
	webkitNetworkCancelled = 302 // WEBKIT_NETWORK_ERROR_CANCELLED
	webkitNetworkUnknown   = 301 // WEBKIT_NETWORK_ERROR_UNKNOWN_PROTOCOL
	webkitNetworkFailed    = 399 // WEBKIT_NETWORK_ERROR_FAILED
	webkitPolicyInterrupt  = 102 // WEBKIT_POLICY_ERROR_FRAME_LOAD_INTERRUPTED_BY_POLICY_CHANGE
	webkitNewWindowAction  = 1   // WEBKIT_POLICY_DECISION_TYPE_NEW_WINDOW_ACTION
	webkitResponse         = 2   // WEBKIT_POLICY_DECISION_TYPE_RESPONSE
	webkitBackForward      = 2   // WEBKIT_NAVIGATION_TYPE_BACK_FORWARD
	webkitCookieSQLite     = 1   // WEBKIT_COOKIE_PERSISTENT_STORAGE_SQLITE
	gtkOverflowHidden      = 1
)

// webHandler is the name pages post to, as
// window.webkit.messageHandlers.nsgo.
const webHandler = "nsgo"

// webBridge is the bridge of a page whose messages start with token. WebKitGTK tells no message's frame, and WebKit gives every frame of a page its handlers, so only the top frame's bridge, which keeps the token in a closure and the handler's postMessage as the page found it, gets the app's messages; a frame posting to the handler itself is dropped (webMessageCallback).
func webBridge(token string) string {
	return "(function () { var h = window.webkit.messageHandlers." + webHandler + ", send = h.postMessage.bind(h), t = " + driver.JSString(token) + "; " +
		driver.WebBridge("function (d) { send(t + d); }") + " })();"
}

// webNoScroll keeps the top document from scrolling.
const webNoScroll = "html, body { overflow: hidden !important; }"

// web is a web view's WebKitWebView, hosted in the view's widget as NSH's
// Linux backend hosts it. Without WebKitGTK it is a placeholder label and
// widget is 0.
type web struct {
	hosted
	widget        uintptr // WebKitWebView
	session       uintptr // the ephemeral network session an Incognito page owns, released with it; 0 for the app's
	content       uintptr // its WebKitUserContentManager
	token         string  // what its top frame's messages start with (webBridge)
	page          string  // the URI of the page last committed, which its messages report
	config        driver.WebConfig
	made          bool
	failed        bool                         // the navigation failed; WebKit still finishes it
	pending       bool                         // a load waits for the page to be mapped
	ownNavigation bool                         // loadWeb started the app's source, until it commits or fails
	ownStep       string                       // the URI of the entry a history command granted ownNavigation for, until a back-forward action to it; "" for loadWeb's load or a reload
	aboutPending  bool                         // a refused about: action, possibly a frame's, may start the next top-level load
	startPending  bool                         // hold that load's start until its top-level URI commits
	uriPending    bool                         // a page-made load awaits its main resource request to name its destination
	ownPage       bool                         // the page last committed is the app's source (loadWeb's)
	ownHistory    map[uintptr]driver.WebSource // retained native history entries created by the app
	htmlStart     *driver.WebSource            // the initial HTML, which load_html omits from native history
	htmlAnchor    uintptr                      // retained first native entry after that HTML
	htmlShown     bool                         // the initial HTML is shown in place of its anchor
	refused       bool                         // the top level committed an about: page the config refuses, which restore leaves; until the next commit it reports no load and posts nothing
	restoring     bool                         // restore started a navigation back to the page before a refused one, until a page commits
	restored      time.Time                    // when the page last committed did, if restore brought it back; zero for another
	source        driver.WebSource             // what the pending or last load shows: the config's, or Ref.Load's
	background    [4]float32                   // WebKit's own, for views without an opaque one
	pointer       driver.Point                 // where the pointer is in the page

	lastNavigation   driver.WebNavigation
	navigationQueued bool
}

// webPage is the window and view a WebKitWebView belongs to.
type webPage struct {
	window *window
	view   *view
}

// webPages are the live pages by WebKitWebView, whatever their window and network session, so that what WebKit hands over with only a page (scheme requests, downloads) finds its view.
var webPages = map[uintptr]webPage{}

// webPageOf returns the live view showing page, or nils.
func webPageOf(page uintptr) (*window, *view) {
	p, ok := webPages[page]
	if !ok || p.window.closing || p.view.deleted || p.view.web == nil || p.view.web.widget != page {
		return nil, nil
	}
	return p.window, p.view
}

// webSchemes are the schemes registered on each WebKitWebContext. A context keeps a scheme for its life, so each is registered once, and its requests find their view by page (webSchemeCallback).
var webSchemes = map[uintptr]map[string]bool{}

// webDownload is a download a view saves, which holds a reference to its WebKitDownload until it ends or the view closes.
type webDownload struct {
	window *window
	view   *view
	item   driver.WebDownload
}

var (
	webDownloads      = map[uintptr]*webDownload{}
	webDownloadSerial uint64
)

// report reports dl as it is now. The engine reads the event after the backend has gone on, so it gets a copy rather than the item later signals change.
func (dl *webDownload) report() {
	item := dl.item
	dl.window.report(dl.view, driver.Event{Kind: driver.EventDownload, ID: dl.view.id, Data: &item})
}

// What a Mobile web view presents without a user agent of its own: Android Chrome's, naming driver.WebMobileChromeVersion, and the platform such a phone reports. Sites take iPhone Safari's for WebKit on iOS, which WebKitGTK is not: given it, x.com showed "Something went wrong" where it loads with Android Chrome's (2026-11).
var mobileUserAgent = fmt.Sprintf(driver.WebMobileUserAgentAndroid, driver.WebMobileChromeVersion)

const mobilePlatform = "Linux armv8l"

// webWarned is the warning, once, that web views show a placeholder.
var webWarned sync.Once

// configureWeb applies a web view's configuration, making its page on
// the first one and loading the source again when it or the scripts
// changed; the rest applies to the page as it is.
func (w *window) configureWeb(v *view, c *driver.WebConfig) {
	if v.web == nil {
		v.web = &web{config: *c}
		if err := openWebKit(); err != nil {
			webWarned.Do(func() { log.Printf("nsgo: WebView is not available: %v; it shows a placeholder", err) })
			w.host(v, &v.web.hosted, webPlaceholder(), false)
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
		c.InjectedJavaScriptBeforeContent != old.InjectedJavaScriptBeforeContent || c.Mobile != old.Mobile
	if scripts {
		webkitContentRemoveAllScripts(wv.content)
		if c.Mobile {
			addScriptIn(wv.content, driver.WebMobileScript(mobilePlatform, true), webkitAllFrames, webkitAtStart)
		}
		addScript(wv.content, webBridge(wv.token), webkitAtStart)
		addScript(wv.content, c.InjectedJavaScriptBeforeContent, webkitAtStart)
		addScript(wv.content, c.InjectedJavaScript, webkitAtEnd)
	}
	if first || c.NoScroll != old.NoScroll {
		// Wheels over a page that does not scroll go to the scroll views
		// around it (webScrollCallback).
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
	ua := c.UserAgent
	if ua == "" && c.Mobile {
		ua = mobileUserAgent
	}
	if ua == "" {
		// NULL sets WebKitGTK's default, which the suffix then follows.
		webkitSettingsSetUserAgent(settings, nil)
		if c.UserAgentSuffix != "" {
			ua = goString(webkitSettingsGetUserAgent(settings))
		}
	}
	if ua = driver.WebUserAgent(ua, c.UserAgentSuffix); ua != "" {
		webkitSettingsSetUserAgent(settings, cstr(ua))
	}
	webkitSettingsSetBackForwardGestures(settings, boolInt(c.BackForwardGestures))
	// WebKitGTK has no setting for pinches, so PinchZoom keeps its own handling.
	// Pages see no media devices unless they may capture one.
	webkitSettingsSetEnableMediaStream(settings, boolInt(c.Media != 0))
	w.webBackground(v)
	// Scripts apply to pages loaded from now on: load again.
	if first || scripts || c.Source != old.Source || !maps.Equal(c.Headers, old.Headers) {
		v.web.source = c.Source
		w.loadWeb(v)
	}
}

// webBackground shows the view's background behind the page while it
// loads and behind pages without one of their own, when it is opaque,
// as on Windows; else WebKit's own.
func (w *window) webBackground(v *view) {
	wv := v.web
	if wv == nil || wv.widget == 0 {
		return
	}
	bg := wv.background
	if c := v.style.Background; c&0xff == 0xff {
		bg = [4]float32{float32(c>>24) / 255, float32(c>>16&0xff) / 255, float32(c>>8&0xff) / 255, 1}
	}
	webkitWebViewSetBackgroundColor(wv.widget, &bg)
}

// webPlaceholder is what a web view shows without WebKitGTK, as NSH's
// does: gray, which reads on light and dark apps alike.
func webPlaceholder() uintptr {
	label := gtkLabelNew(nil)
	gtkLabelSetMarkup(label, cstr(`<span foreground="#8c8c8c">WebView is not available on Linux</span>`))
	gtkLabelSetWrap(label, 1)
	// Small boxes clip the text rather than overflow.
	gtkLabelSetEllipsize(label, pangoEllipsizeEnd)
	gtkLabelSetLines(label, 3)
	gtkLabelSetJustify(label, gtkJustifyCenter)
	gtkWidgetSetCanTarget(label, 0)
	return label
}

func addScript(content uintptr, source string, at int32) {
	addScriptIn(content, source, webkitTopFrame, at)
}

func addScriptIn(content uintptr, source string, frames, at int32) {
	if source == "" {
		return
	}
	script := webkitUserScriptNew(cstr(source), frames, at, 0, 0)
	webkitContentAddScript(content, script)
	webkitUserScriptUnref(script)
}

// makeWeb makes v's page in the app's network session, or an ephemeral one of its own for Incognito, and hosts it.
func (w *window) makeWeb(v *view) {
	wv := v.web
	var session uintptr
	if wv.config.Incognito {
		// The page holds its own reference; wv.session is released when the view closes.
		wv.session = webkitNetworkSessionNewEphemeral()
		gSignalConnect(wv.session, "download-started", cbWebDownloadStarted, 0)
		session = wv.session
	} else {
		session = w.webSession()
	}
	wv.widget = gObjectNew(webkitWebViewGetType(), cstr("network-session"), session, uintptr(0))
	wv.content = webkitWebViewGetUserContentManager(wv.widget)
	var token [16]byte
	_, _ = rand.Read(token[:])
	wv.token, wv.page = hex.EncodeToString(token[:])+":", ""
	webPages[wv.widget] = webPage{window: w, view: v}
	registerWebSchemes(wv)
	webkitContentRegisterHandler(wv.content, cstr(webHandler), nil)
	webkitWebViewGetBackgroundColor(wv.widget, &wv.background)
	w.host(v, &wv.hosted, wv.widget, true)
	w.connect(v, wv.widget, "load-changed", cbWebLoadChanged)
	w.connect(v, wv.widget, "load-failed", cbWebLoadFailed)
	w.connect(v, wv.widget, "notify::uri", cbWebNavigationChanged)
	w.connect(v, wv.widget, "notify::title", cbWebNavigationChanged)
	w.connect(v, webkitWebViewGetBackForwardList(wv.widget), "changed", cbWebHistoryChanged)
	w.connect(v, wv.widget, "resource-load-started", cbWebResourceStarted)
	w.connect(v, wv.widget, "decide-policy", cbWebPolicy)
	w.connect(v, wv.widget, "permission-request", cbWebPermission)
	w.connect(v, wv.widget, "map", cbWebMap)
	w.connect(v, wv.content, "script-message-received::"+webHandler, cbWebMessage)
	// Focus can belong to a widget inside WebKit's page, so follow the whole subtree rather than only the WebView's has-focus property.
	focus := gtkEventControllerFocusNew()
	w.connect(v, focus, "enter", cbWebFocusIn)
	w.connect(v, focus, "leave", cbWebFocusOut)
	gtkWidgetAddController(wv.widget, focus)

	// History keys and mouse buttons, before WebKit sees them: it keeps
	// arrow keys, used or not, so Alt+Left would never reach the window.
	keys := gtkEventControllerKeyNew()
	gtkEventControllerSetPropagationPhase(keys, propagationPhaseCapture)
	w.connect(v, keys, "key-pressed", cbWebKey)
	gtkWidgetAddController(wv.widget, keys)
	buttons := gtkGestureClickNew()
	// Any button: only the back (8) and forward (9) ones are taken.
	gtkGestureSingleSetButton(buttons, 0)
	gtkEventControllerSetPropagationPhase(buttons, propagationPhaseCapture)
	w.connect(v, buttons, "pressed", cbWebButton)
	gtkWidgetAddController(wv.widget, buttons)

	motion := gtkEventControllerMotionNew()
	w.connect(v, motion, "motion", cbWebMotion)
	gtkWidgetAddController(wv.widget, motion)
	scroll := gtkEventControllerScrollNew(3) // both axes
	gtkEventControllerSetPropagationPhase(scroll, propagationPhaseCapture)
	w.connect(v, scroll, "scroll", cbWebScroll)
	gtkWidgetAddController(wv.widget, scroll)
}

// registerWebSchemes registers the schemes wv's app serves on its page's WebKitWebContext, which every page of the process shares, so a scheme is registered once for every view that serves it.
func registerWebSchemes(wv *web) {
	if len(wv.config.SchemeHandlers) == 0 {
		return
	}
	context := webkitWebViewGetContext(wv.widget)
	if context == 0 {
		return
	}
	registered := webSchemes[context]
	if registered == nil {
		registered = map[string]bool{}
		webSchemes[context] = registered
	}
	for name := range wv.config.SchemeHandlers {
		if !registered[name] {
			webkitWebContextRegisterURIScheme(context, cstr(name), cbWebScheme, 0, 0)
			registered[name] = true
		}
	}
	// A registered scheme may answer cross-origin fetches only when WebKit
	// takes it as CORS-enabled, which unlike WKWebView's URL scheme handlers
	// WebKitGTK does not do on its own: an Access-Control-Allow-Origin header
	// on the answer then still fails the page's check.
	if manager := webkitWebContextGetSecurityManager(context); manager != 0 {
		for name := range wv.config.SchemeHandlers {
			webkitSecurityManagerRegisterCORSScheme(manager, cstr(name))
		}
	}
}

// webSessions are the network sessions by app ID: one network process, and one set of cookies and storage, serves every web view of an app that is not Incognito. A session lives as long as the process, so views mounted again find it.
var webSessions = map[string]uintptr{}

// webSession returns the network session of w's app, which keeps
// website data in $XDG_DATA_HOME/<app id>/webkit and its cache in
// $XDG_CACHE_HOME/<app id>/webkit, with cookies in cookies.sqlite there.
func (w *window) webSession() uintptr {
	if s := webSessions[w.appID]; s != 0 {
		return s
	}
	data, cache := webFolders(w.appID)
	if err := os.MkdirAll(data, 0o755); err != nil {
		log.Printf("nsgo: web view: %v", err)
	}
	s := webkitNetworkSessionNew(cstr(data), cstr(cache))
	gSignalConnect(s, "download-started", cbWebDownloadStarted, 0)
	// Cookies outlive the app, as other website data does.
	if cookies := webkitNetworkSessionGetCookieManager(s); cookies != 0 {
		webkitCookieManagerSetPersistent(cookies, cstr(filepath.Join(data, "cookies.sqlite")), webkitCookieSQLite)
	}
	webSessions[w.appID] = s
	return s
}

// webSchemeCallback answers a request for an app-owned scheme from the handler of the view whose page made it. Every request is finished, with an error when no live view serves its scheme, so no page waits on one.
func webSchemeCallback(request, _ uintptr) {
	_, v := webPageOf(webkitURISchemeRequestGetWebView(request))
	raw := goString(webkitURISchemeRequestGetURI(request))
	u, err := url.Parse(raw)
	if err != nil {
		finishWebSchemeError(request, webkitNetworkFailed, err.Error())
		return
	}
	var handler driver.WebSchemeHandler
	if v != nil {
		handler = v.web.config.SchemeHandlers[strings.ToLower(u.Scheme)]
	}
	if handler == nil {
		finishWebSchemeError(request, webkitNetworkUnknown, "nsgo: no handler serves "+u.Scheme+": URLs in this web view")
		return
	}
	schemeRequest := &driver.WebSchemeRequest{
		URL:     raw,
		Method:  goString(webkitURISchemeRequestGetHTTPMethod(request)),
		Headers: webSchemeHeaders(webkitURISchemeRequestGetHTTPHeaders(request)),
	}
	resp := handler(schemeRequest)
	// A nil answer is 404, an empty body.
	status := int32(404)
	var body []byte
	if resp != nil {
		status = int32(resp.Status)
		if status == 0 {
			status = 200
		}
		body = resp.Body
	}
	bytes := gBytesNew(unsafe.Pointer(nil), 0)
	if len(body) > 0 {
		bytes = gBytesNew(unsafe.Pointer(&body[0]), uintptr(len(body)))
	}
	stream := gMemoryInputStreamNewFromBytes(bytes)
	gBytesUnref(bytes)
	// A request is answered as HTTP: the response's status and headers are
	// the page's fetch's, and its content type is the response's
	// Content-Type header, else what the URL and body imply.
	response := webkitURISchemeResponseNew(stream, int64(len(body)))
	webkitURISchemeResponseSetStatus(response, status, nil) // WebKit supplies the standard reason phrase.
	contentType := resp.Header("Content-Type")
	if contentType == "" {
		contentType = driver.WebMIME(u.Path, body)
	}
	webkitURISchemeResponseSetContentType(response, cstr(contentType))
	if resp != nil {
		const soupHeadersResponse = 1
		var headers uintptr
		for name, value := range resp.Headers {
			if !strings.EqualFold(name, "Content-Type") {
				if headers == 0 {
					headers = soupMessageHeadersNew(soupHeadersResponse)
				}
				webkitMessageHeadersAppend(headers, cstr(name), cstr(value))
			}
		}
		if headers != 0 {
			// set_http_headers takes ownership of the SoupMessageHeaders.
			webkitURISchemeResponseSetHTTPHeaders(response, headers)
		}
	}
	webkitURISchemeRequestFinishWithResponse(request, response)
	gObjectUnref(response)
	gObjectUnref(stream)
}

// webSchemeHeaders is a request's SoupMessageHeaders as a map, nil when it
// has none.
func webSchemeHeaders(headers uintptr) map[string]string {
	if headers == 0 {
		return nil
	}
	var iter [4]uintptr // SoupMessageHeadersIter is four pointers
	var name, value uintptr
	soupMessageHeadersIterInit(uintptr(unsafe.Pointer(&iter)), headers)
	var out map[string]string
	for soupMessageHeadersIterNext(uintptr(unsafe.Pointer(&iter)), uintptr(unsafe.Pointer(&name)), uintptr(unsafe.Pointer(&value))) != 0 {
		if out == nil {
			out = map[string]string{}
		}
		out[goString(name)] = goString(value)
	}
	return out
}

// finishWebSchemeError ends request with a WebKit network error of code and message.
func finishWebSchemeError(request uintptr, code int32, message string) {
	err := gErrorNewLiteral(webkitNetworkErrorQuark(), code, cstr(message))
	webkitURISchemeRequestFinishError(request, err)
	gErrorFree(err)
}

// webDownloadStartedCallback takes a download a network session started for the view whose page asked for it, when the view saves downloads, and cancels it otherwise.
func webDownloadStartedCallback(_, download, _ uintptr) {
	w, v := webPageOf(webkitDownloadGetWebView(download))
	if v == nil || v.web.config.DownloadDir == "" {
		webkitDownloadCancel(download)
		return
	}
	webDownloadSerial++
	id := strconv.FormatInt(time.Now().UnixMilli(), 36) + "-" + strconv.FormatUint(webDownloadSerial, 36)
	urlText := ""
	if request := webkitDownloadGetRequest(download); request != 0 {
		urlText = goString(webkitURIRequestGetURI(request))
	}
	dl := &webDownload{window: w, view: v, item: driver.WebDownload{State: driver.WebDownloadStarted, ID: id, URL: urlText}}
	webDownloads[gObjectRef(download)] = dl
	// Downloads own these temporary signal connections; attaching them to the view's hosted-signal list would leave stale object pointers after WebKit releases a completed download. The handlers find the download in webDownloads, which a closing view leaves.
	gSignalConnect(download, "decide-destination", cbWebDownloadDestination, 0)
	gSignalConnect(download, "received-data", cbWebDownloadReceived, 0)
	gSignalConnect(download, "finished", cbWebDownloadFinished, 0)
	gSignalConnect(download, "failed", cbWebDownloadFailed, 0)
}

// forgetDownload stops following download and releases the reference webDownloads held.
func forgetDownload(download uintptr) {
	if _, ok := webDownloads[download]; ok {
		delete(webDownloads, download)
		gObjectUnref(download)
	}
}

// cancelDownload cancels download and forgets it first, so the failed signal the cancellation emits finds nothing to report.
func cancelDownload(download uintptr) {
	_, followed := webDownloads[download]
	delete(webDownloads, download)
	webkitDownloadCancel(download)
	if followed {
		gObjectUnref(download)
	}
}

// cancelDownloads cancels and forgets the downloads v saves, as its page closes.
func cancelDownloads(v *view) {
	for download, dl := range webDownloads {
		if dl.view == v {
			cancelDownload(download)
		}
	}
}

func webDownloadDestinationCallback(download, suggested, _ uintptr) int32 {
	dl := webDownloads[download]
	if dl == nil || dl.view.web == nil {
		// A download no view saves any more is cancelled, never left to WebKit's default folder.
		cancelDownload(download)
		return 1
	}
	name := webDownloadFilename(goString(suggested))
	dir := filepath.Join(dl.view.web.config.DownloadDir, dl.item.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		dl.item.State, dl.item.Message = driver.WebDownloadFailed, err.Error()
		dl.report()
		cancelDownload(download)
		return 1
	}
	dl.item.Filename = name
	dl.item.Path = filepath.Join(dir, name)
	webkitDownloadSetDestination(download, cstr(dl.item.Path))
	dl.report()
	return 1
}

func webDownloadFilename(suggested string) string {
	name := filepath.Base(suggested)
	if name == "." || name == ".." || name == string(filepath.Separator) || name == "" {
		return "download"
	}
	return name
}

func webDownloadReceivedCallback(download, _ uint64, _ uintptr) {
	// WebKit reports progress through received-data; the public event model
	// reports only started, finished and failed, as the Apple backend does.
}

func webDownloadFinishedCallback(download, _ uintptr) {
	dl := webDownloads[download]
	if dl == nil {
		return
	}
	dl.item.State = driver.WebDownloadFinished
	dl.item.Bytes = int64(webkitDownloadGetReceivedDataLength(download))
	if dest := webkitDownloadGetDestination(download); dest != 0 {
		dl.item.Path = goString(dest)
	}
	dl.report()
	forgetDownload(download)
}

func webDownloadFailedCallback(download, gerr, _ uintptr) {
	dl := webDownloads[download]
	if dl == nil {
		return
	}
	dl.item.State = driver.WebDownloadFailed
	if gerr != 0 {
		dl.item.Message = goString((*gError)(cPointer(gerr)).message)
	}
	dl.report()
	forgetDownload(download)
}

// webFolders are where web views of app id keep their data and cache.
func webFolders(id string) (data, cache string) {
	data, cache, _ = engine.AppDirs(id)
	return filepath.Join(data, "webkit"), filepath.Join(cache, "webkit")
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
	wv.ownStep = ""
	src := wv.source
	switch {
	case src.HTML != "":
		wv.ownNavigation = true
		if wv.htmlStart == nil && webkitBackForwardListGetCurrentItem(webkitWebViewGetBackForwardList(wv.widget)) == 0 {
			saved := src
			wv.htmlStart = &saved
		}
		wv.htmlShown = wv.htmlStart != nil
		var base *byte
		if src.BaseURL != "" {
			base = cstr(src.BaseURL)
		}
		webkitWebViewLoadHTML(wv.widget, cstr(src.HTML), base)
	case src.Src != "":
		wv.htmlShown = false
		wv.ownNavigation = true
		if len(wv.config.Headers) == 0 {
			webkitWebViewLoadURI(wv.widget, cstr(src.Src))
			return
		}
		request := webkitURIRequestNew(cstr(src.Src))
		if request == 0 {
			webkitWebViewLoadURI(wv.widget, cstr(src.Src))
			return
		}
		headers := webkitURIRequestGetHTTPHeaders(request)
		if headers != 0 {
			for name, value := range wv.config.Headers {
				webkitMessageHeadersAppend(headers, cstr(name), cstr(value))
			}
		}
		webkitWebViewLoadRequest(wv.widget, request)
		gObjectUnref(request)
	}
}

// webCommand has v's page do c, with value for messages and scripts.
func (w *window) webCommand(v *view, c driver.WebCommand, value string, data any) {
	if v.web == nil || v.web.widget == 0 {
		return // the placeholder has no page
	}
	page := v.web.widget
	switch c {
	case driver.WebReload:
		if v.web.ownPage && v.web.source.HTML != "" {
			w.loadWeb(v)
			return
		}
		v.web.ownNavigation, v.web.ownStep = v.web.ownPage, ""
		webkitWebViewReload(page)
	case driver.WebGoBack:
		if v.web.htmlStart != nil && !v.web.htmlShown && webkitBackForwardListGetCurrentItem(webkitWebViewGetBackForwardList(page)) == v.web.htmlAnchor {
			v.web.source = *v.web.htmlStart
			w.loadWeb(v)
			return
		}
		v.web.useHistory(webkitBackForwardListGetBackItem(webkitWebViewGetBackForwardList(page)))
		webkitWebViewGoBack(page)
	case driver.WebGoForward:
		if v.web.htmlShown && v.web.htmlAnchor != 0 {
			v.web.htmlShown = false
			v.web.ownNavigation, v.web.ownStep = false, ""
			webkitWebViewGoToHistory(page, v.web.htmlAnchor)
			return
		}
		v.web.useHistory(webkitBackForwardListGetForwardItem(webkitWebViewGetBackForwardList(page)))
		webkitWebViewGoForward(page)
	case driver.WebStopLoading:
		webkitWebViewStopLoading(page)
	case driver.WebPostMessage:
		evaluate(page, driver.WebMessageScript(value))
	case driver.WebInjectJavaScript:
		evaluate(page, value)
	case driver.WebLoad:
		if src, ok := data.(*driver.WebSource); ok && src != nil {
			v.web.source = *src
			w.loadWeb(v)
		}
	}
}

func evaluate(page uintptr, script string) {
	webkitWebViewEvaluateJavascript(page, cstr(script), -1, nil, nil, 0, 0, 0)
}

// closeWeb stops v's page, cancels its downloads and lets it go.
func (w *window) closeWeb(v *view) {
	wv := v.web
	if wv.htmlAnchor != 0 {
		gObjectUnref(wv.htmlAnchor)
	}
	for item := range wv.ownHistory {
		gObjectUnref(item)
	}
	v.web = nil
	cancelDownloads(v)
	if wv.widget != 0 {
		delete(webPages, wv.widget)
		webkitWebViewStopLoading(wv.widget)
		// Before unhosting, which lets the page and its content manager go.
		webkitContentUnregisterHandler(wv.content, cstr(webHandler), nil)
	}
	w.unhost(v, &wv.hosted)
	if wv.session != 0 {
		// The Incognito page's own session, whose website data goes with its last reference.
		gObjectUnref(wv.session)
	}
}

func (wv *web) ownsHistory(item uintptr) bool {
	_, ok := wv.ownHistory[item]
	return item != 0 && ok
}

// useHistory grants only the app-created native entry selected by a host command. Frame navigation actions never set this flag.
func (wv *web) useHistory(item uintptr) {
	src, own := wv.ownHistory[item]
	wv.ownNavigation, wv.ownStep = item != 0 && own, ""
	if wv.ownNavigation {
		wv.source = src
		wv.ownStep = goString(webkitBackForwardListItemGetURI(item))
	}
}

func (wv *web) rememberHistory() {
	item := webkitBackForwardListGetCurrentItem(webkitWebViewGetBackForwardList(wv.widget))
	if item == 0 || wv.htmlShown && wv.htmlAnchor != 0 || wv.ownsHistory(item) {
		return
	}
	if wv.ownHistory == nil {
		wv.ownHistory = make(map[uintptr]driver.WebSource)
	}
	gObjectRef(item)
	wv.ownHistory[item] = wv.source
}

// pruneHistory releases entries WebKit dropped, including a discarded forward branch. Retaining live entries keeps recycled native pointers from inheriting an app source's exemption.
func (wv *web) pruneHistory() {
	list := webkitWebViewGetBackForwardList(wv.widget)
	live := map[uintptr]bool{webkitBackForwardListGetCurrentItem(list): true}
	for _, step := range []int32{-1, 1} {
		for i := step; ; i += step {
			item := webkitBackForwardListGetNthItem(list, i)
			if item == 0 {
				break
			}
			live[item] = true
		}
	}
	for item := range wv.ownHistory {
		if !live[item] {
			delete(wv.ownHistory, item)
			gObjectUnref(item)
		}
	}
	if wv.htmlAnchor != 0 && !live[wv.htmlAnchor] {
		gObjectUnref(wv.htmlAnchor)
		wv.htmlStart, wv.htmlAnchor, wv.htmlShown = nil, 0, false
	}
}

// focusedWeb returns the web view whose page has keyboard focus, or nil.
func (w *window) focusedWeb() *view {
	if len(w.natives) == 0 {
		return nil
	}
	for f := gtkWindowGetFocus(w.win); f != 0; f = gtkWidgetGetParent(f) {
		if v := w.natives[f]; v != nil && v.web != nil && v.web.widget == f {
			return v
		}
	}
	return nil
}

// webEdit runs an editing menu role in the focused page, if any.
func (w *window) webEdit(role driver.MenuRole) bool {
	v := w.focusedWeb()
	command := map[driver.MenuRole]string{
		driver.MenuUndo: "Undo", driver.MenuRedo: "Redo", driver.MenuCut: "Cut",
		driver.MenuCopy: "Copy", driver.MenuPaste: "Paste", driver.MenuPasteAndMatchStyle: "PasteAsPlainText",
		driver.MenuDelete: "Delete", driver.MenuSelectAll: "SelectAll",
	}[role]
	if v == nil || command == "" {
		return false
	}
	webkitWebViewExecuteEditingCommand(v.web.widget, cstr(command))
	return true
}

// webOf returns the live web view whose page p belongs to.
func webOf(data, p uintptr) (*window, *view) {
	w, v := nativeOf(data, p)
	if v == nil || v.web == nil || v.web.widget == 0 {
		return nil, nil
	}
	return w, v
}

// navigated reports the page's state as kind and as a navigation state
// change.
func (w *window) navigated(v *view, loading bool, kind driver.EventKind) {
	w.navigatedTo(v, goString(webkitWebViewGetURI(v.web.widget)), loading, kind)
}

// webNavigation reads the displayed page's state, or a provisional URL the load reports.
func webNavigation(v *view, uri string, loading bool) driver.WebNavigation {
	page := v.web.widget
	nav := driver.WebNavigation{
		URL:          uri,
		Title:        goString(webkitWebViewGetTitle(page)),
		Loading:      loading,
		CanGoBack:    webkitWebViewCanGoBack(page) != 0,
		CanGoForward: webkitWebViewCanGoForward(page) != 0,
	}
	if v.web.htmlAnchor != 0 {
		if v.web.htmlShown {
			nav.CanGoBack, nav.CanGoForward = false, true
		} else if webkitBackForwardListGetCurrentItem(webkitWebViewGetBackForwardList(page)) == v.web.htmlAnchor {
			nav.CanGoBack = true
		}
	}
	return nav
}

// navigatedTo also reports provisional loads that failed before their URI committed.
func (w *window) navigatedTo(v *view, uri string, loading bool, kind driver.EventKind) {
	nav := webNavigation(v, uri, loading)
	if !loading {
		v.web.lastNavigation = nav
	}
	w.report(v, driver.Event{Kind: kind, ID: v.id, Navigation: &nav})
	state := nav
	w.report(v, driver.Event{Kind: driver.EventNavigationStateChange, ID: v.id, Navigation: &state})
}

func webHistoryChangedCallback(list, _, _, data uintptr) {
	webNavigationChangedCallback(list, 0, data)
}

// Read URI, title and history together after WebKit applies its property
// notifications. Full loads report their state through load-changed.
func webNavigationChangedCallback(instance, _, data uintptr) {
	w, v := webOf(data, instance)
	if v == nil || v.web.navigationQueued || !v.events.Has(driver.EventNavigationStateChange) {
		return
	}
	wv := v.web
	wv.navigationQueued = true
	w.post(func() {
		wv.navigationQueued = false
		if w.closing || v.deleted || v.web != wv || wv.page == "" || wv.refused || wv.restoring || webkitWebViewIsLoading(wv.widget) != 0 {
			return
		}
		nav := webNavigation(v, goString(webkitWebViewGetURI(wv.widget)), false)
		if nav != wv.lastNavigation {
			wv.lastNavigation = nav
			w.report(v, driver.Event{Kind: driver.EventNavigationStateChange, ID: v.id, Navigation: &nav})
		}
	})
}

func webLoadChangedCallback(page uintptr, event uint32, data uintptr) {
	w, v := webOf(data, page)
	if v == nil {
		return
	}
	switch event {
	case webkitLoadStarted:
		v.web.failed = false
		uri := goString(webkitWebViewGetURI(page))
		// At load start the URI of a refused about: page can still name the page before it. The last navigation action being a refused about: one, a frame's perhaps, as WebKitGTK does not say, holds this start until the top level commits, so a refused page reports none.
		v.web.startPending = v.web.config.Restricted && !v.web.ownNavigation &&
			(v.web.aboutPending || aboutScheme(uri) && !v.web.config.Navigable(uri))
		v.web.aboutPending = false
		// A page-made load can still expose the old URI here, so its start waits for its main resource request (webResourceStartedCallback), which names the destination without mistaking a frame's navigation action for the top level's.
		v.web.uriPending = !v.web.ownNavigation
		if !v.web.startPending && !v.web.uriPending {
			w.navigated(v, true, driver.EventLoadStart)
		}
	case webkitLoadCommitted:
		uri := goString(webkitWebViewGetURI(page))
		own, restoring := v.web.ownNavigation && (v.web.ownStep == "" || uri == v.web.ownStep), v.web.restoring
		start := v.web.startPending || v.web.uriPending
		v.web.aboutPending, v.web.startPending, v.web.uriPending = false, false, false
		v.web.ownNavigation, v.web.ownStep, v.web.refused, v.web.restoring = false, "", false, false
		// An about: page has no response for webPolicyCallback to hold, and WebKit has made its document by the time the top level's load starts, so the top level's is held as it commits, load-changed being only the top level's: it posts nothing and reports no load, and the view goes back to the page before it.
		if v.web.config.Restricted && !own && aboutScheme(uri) && !v.web.config.Navigable(uri) {
			v.web.failed, v.web.refused = true, true
			w.report(v, driver.Event{Kind: driver.EventError, ID: v.id, Text: driver.RefusedNavigation(uri), Navigation: &driver.WebNavigation{URL: uri}})
			w.restore(v)
			return
		}
		if start {
			w.navigated(v, true, driver.EventLoadStart)
		}
		if !own && v.web.htmlStart != nil {
			v.web.htmlShown = false
			if v.web.htmlAnchor == 0 {
				v.web.htmlAnchor = webkitBackForwardListGetCurrentItem(webkitWebViewGetBackForwardList(page))
				if v.web.htmlAnchor != 0 {
					gObjectRef(v.web.htmlAnchor)
				}
			}
		}
		v.web.ownPage, v.web.restored = own, time.Time{}
		v.web.pruneHistory()
		if own {
			v.web.rememberHistory()
		}
		if restoring {
			v.web.restored = time.Now()
		}
		// From here the view's URI is the page that posts; until a load commits, the page before it still does.
		v.web.page = uri
	case webkitLoadFinished:
		if v.web.ownPage && !v.web.failed {
			v.web.rememberHistory()
		}
		// A failed or refused navigation finishes too; its error was already reported.
		if v.web.failed {
			// WebKit can notify URI and history while rolling back a provisional load.
			v.web.lastNavigation = webNavigation(v, goString(webkitWebViewGetURI(page)), false)
			return
		}
		w.navigated(v, false, driver.EventLoad)
	}
}

// webResourceStartedCallback reports a page-made load's start as its main resource request starts, before the response or commit. WebKitGTK sets the main resource before this signal, so frame and subresource requests never take the pending start. Loads without a resource (about: pages or cached history) fall back to their committed URI in webLoadChangedCallback. A restricted view's page of a page's own making that its config refuses, such as a blob: page, is refused at its main-frame response (webPolicyCallback), so its start stays held and is never reported, as on the other platforms, which refuse it before it starts.
func webResourceStartedCallback(page, resource, request, data uintptr) {
	w, v := webOf(data, page)
	if v == nil || !v.web.uriPending || v.web.startPending || resource != webkitWebViewGetMainResource(page) {
		return
	}
	uri := goString(webkitURIRequestGetURI(request))
	if !v.web.config.Navigable(uri) {
		return
	}
	v.web.uriPending = false
	w.navigatedTo(v, uri, true, driver.EventLoadStart)
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
	// A start awaiting the request or commit is still reported before its failure, but never for a refused page.
	if (v.web.startPending || v.web.uriPending) && v.web.config.Navigable(goString(uri)) {
		w.navigatedTo(v, goString(uri), true, driver.EventLoadStart)
	}
	v.web.failed = true
	v.web.ownNavigation, v.web.ownStep = false, ""
	v.web.aboutPending, v.web.startPending, v.web.uriPending = false, false, false
	if gerr != 0 {
		e := (*gError)(cPointer(gerr))
		// Stopped or replaced by another navigation: not errors.
		cancelled := e.domain == webkitNetworkErrorQuark() && e.code == webkitNetworkCancelled ||
			e.domain == webkitPolicyErrorQuark() && e.code == webkitPolicyInterrupt
		if !cancelled {
			domain := ""
			if name := gQuarkToString(e.domain); name != 0 {
				domain = goString(name)
			}
			w.report(v, driver.Event{Kind: driver.EventError, ID: v.id, Text: goString(e.message),
				Data: &driver.WebFailure{Domain: domain, Code: int(e.code)}, Navigation: &driver.WebNavigation{URL: goString(uri)}})
		}
	}
	return 1 // the app shows failures itself, without WebKit's error page
}

// restore takes the top level of v back from the about: page it was refused to the page it showed before: through the history, which keeps a page WebKit cached as it was, or by loading it again where the about: page replaced it in the history, and always for the app's HTML, which WebKit's history does not keep. A page restore brought back that leaves for such a page again within a second is not brought back again, so a page that leaves as it loads cannot have the view load it over and over; the view keeps the blank about: page, which posts nothing.
func (w *window) restore(v *view) {
	page, prev := v.web.widget, v.web.page
	if prev == "" || !v.web.restored.IsZero() && time.Since(v.web.restored) < time.Second {
		return
	}
	v.web.restoring = true
	// The page before is the app's source again, which the restoring navigation may show as loadWeb's could.
	v.web.ownNavigation, v.web.ownStep = v.web.ownPage, ""
	list := webkitWebViewGetBackForwardList(page)
	is := func(item uintptr) bool { return item != 0 && goString(webkitBackForwardListItemGetURI(item)) == prev }
	switch {
	case v.web.ownPage && v.web.source.HTML != "":
		w.loadWeb(v)
	case is(webkitBackForwardListGetBackItem(list)):
		webkitWebViewGoBack(page)
	case is(webkitBackForwardListGetForwardItem(list)):
		webkitWebViewGoForward(page)
	case v.web.ownPage:
		w.loadWeb(v)
	default:
		webkitWebViewLoadURI(page, cstr(prev))
	}
}

// webPolicyCallback refuses the navigations a restricted view's config does not allow: the top level's and new windows'. WebKitGTK asks about every frame's navigation actions without saying which frame's an action is, so a page of the page's own making (driver.PageSchemes), which a frame within the page may load too, is held where the top level's can be told: a data: or blob: page at its main-frame response, and an about: page, which has none, as the top level commits it (webLoadChangedCallback).
func webPolicyCallback(page, decision uintptr, kind uint32, data uintptr) int32 {
	w, v := webOf(data, page)
	if v == nil || kind > webkitResponse {
		return 0
	}
	var url string
	allowed := v.web.config.Navigable
	switch kind {
	case webkitResponse:
		if !v.web.config.Restricted || !webkitResponseDecisionIsMainFrame(decision) {
			return 0
		}
		url = goString(webkitURIRequestGetURI(webkitResponseDecisionGetRequest(decision)))
		if !pageScheme(url) {
			return 0
		}
		if v.web.ownNavigation && (v.web.ownStep == "" || url == v.web.ownStep) {
			allowed = v.web.config.SourceNavigable
		}
	case webkitNewWindowAction:
		url = goString(webkitURIRequestGetURI(webkitNavigationActionGetRequest(webkitNavigationDecisionGetAction(decision))))
	default:
		action := webkitNavigationDecisionGetAction(decision)
		url = goString(webkitURIRequestGetURI(webkitNavigationActionGetRequest(action)))
		// A history step within the page starts no navigation and leaves the step's grant set: the back-forward action to its entry takes it, as a load's navigation does, and any other action, a page's own navigation, drops it.
		if v.web.ownStep != "" {
			if webkitNavigationActionGetType(action) != webkitBackForward || url != v.web.ownStep {
				v.web.ownNavigation = false
			}
			v.web.ownStep = ""
		}
		// Only the last action can be the one the next top-level load starts for, so a frame's about: action followed by the top level's own navigation, which WebKitGTK checks as an action of its own, does not hold that navigation's start. A frame acting between the top level's refused about: action and its load start clears the hint, so that start is reported if its URI still names the page before.
		v.web.aboutPending = aboutScheme(url) && !allowed(url)
		if pageScheme(url) {
			return 0
		}
	}
	if allowed(url) {
		return 0
	}
	if kind != webkitNewWindowAction {
		// A refused load of the app's source neither commits nor fails, so the next navigation is not its.
		v.web.ownNavigation, v.web.ownStep = false, ""
	}
	webkitPolicyDecisionIgnore(decision)
	w.report(v, driver.Event{Kind: driver.EventError, ID: v.id, Text: driver.RefusedNavigation(url),
		Navigation: &driver.WebNavigation{URL: url}})
	return 1
}

// pageScheme reports whether url is a page of a page's own making.
func pageScheme(url string) bool {
	scheme, _, ok := strings.Cut(url, ":")
	return ok && slices.ContainsFunc(driver.PageSchemes, func(s string) bool { return strings.EqualFold(s, scheme) })
}

// aboutScheme reports whether url is an about: page.
func aboutScheme(url string) bool {
	scheme, _, ok := strings.Cut(url, ":")
	return ok && strings.EqualFold(scheme, "about")
}

// webPermissionCallback answers getUserMedia with what the config's
// Media grants, without WebKit's prompt, and refuses screens; other
// permissions keep WebKit's handling, which denies them.
func webPermissionCallback(page, request uintptr, data uintptr) int32 {
	_, v := webOf(data, page)
	if v == nil || gTypeIsA(request, webkitUserMediaRequestGetType()) == 0 {
		return 0
	}
	var want driver.WebMedia
	if webkitUserMediaIsForAudio(request) != 0 {
		want |= driver.WebMicrophone
	}
	if webkitUserMediaIsForVideo(request) != 0 {
		want |= driver.WebCamera
	}
	if webkitUserMediaIsForDisplay(request) == 0 && v.web.config.Media.Allows(want) {
		webkitPermissionRequestAllow(request)
	} else {
		webkitPermissionRequestDeny(request)
	}
	return 1
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
	text, ok := strings.CutPrefix(text, v.web.token)
	if !ok || v.web.refused {
		return
	}
	// The page's top frame posted it: the page last committed, not one the view only started to load.
	w.report(v, driver.Event{Kind: driver.EventMessage, ID: v.id, Text: text, Origin: driver.WebOrigin(v.web.page), URL: v.web.page, MainFrame: true})
}

func webMapCallback(page uintptr, data uintptr) {
	w, v := webOf(data, page)
	if v != nil && v.web.pending {
		w.loadWeb(v)
	}
}

func webFocusInCallback(controller, data uintptr) {
	if w, v := webOf(data, controller); v != nil {
		w.report(v, driver.Event{Kind: driver.EventFocus, ID: v.id})
	}
}

func webFocusOutCallback(controller, data uintptr) {
	if w, v := webOf(data, controller); v != nil {
		w.report(v, driver.Event{Kind: driver.EventBlur, ID: v.id})
	}
}

// The keys that step through a page's history, beside the arrows.
const (
	gdkKPLeft  = 0xff96
	gdkKPRight = 0xff98
	gdkBack    = 0x1008ff26
	gdkForward = 0x1008ff27

	gestureClaimed = 1 // GTK_EVENT_SEQUENCE_CLAIMED
)

// historyKey reports which way keyval with state steps through history:
// -1 back, 1 forward, 0 neither. Lock keys and held buttons do not count.
func historyKey(keyval, state uint32) int {
	mods := state & (gdkControl | gdkAlt | gdkShift | gdkSuper)
	switch {
	case keyval == gdkBack && mods == 0:
		return -1
	case keyval == gdkForward && mods == 0:
		return 1
	case (keyval == gdkLeft || keyval == gdkKPLeft) && mods == gdkAlt:
		return -1
	case (keyval == gdkRight || keyval == gdkKPRight) && mods == gdkAlt:
		return 1
	}
	return 0
}

// goHistory steps the page way through its history, or back in the app
// when the page has nowhere to go back to, and reports whether something
// took it.
func (w *window) goHistory(v *view, way int) bool {
	page := v.web.widget
	switch {
	case way < 0 && (webkitWebViewCanGoBack(page) != 0 || v.web.htmlAnchor != 0 && !v.web.htmlShown):
		w.webCommand(v, driver.WebGoBack, "", nil)
	case way > 0 && (webkitWebViewCanGoForward(page) != 0 || v.web.htmlAnchor != 0 && v.web.htmlShown):
		w.webCommand(v, driver.WebGoForward, "", nil)
	case way < 0:
		w.back()
	default:
		return false
	}
	return true
}

func webKeyCallback(controller uintptr, keyval, keycode, state uint32, data uintptr) int32 {
	w, v := webOf(data, controller)
	if v == nil {
		return 0
	}
	if way := historyKey(keyval, state); way != 0 && w.goHistory(v, way) {
		return 1
	}
	return 0
}

func webButtonCallback(gesture uintptr, presses int32, x, y float64, data uintptr) {
	w, v := webOf(data, gesture)
	if v == nil {
		return
	}
	way := 0
	switch gtkGestureSingleGetCurrentButton(gesture) {
	case 8:
		way = -1
	case 9:
		way = 1
	}
	if way != 0 && w.goHistory(v, way) {
		gtkGestureSetState(gesture, gestureClaimed)
	}
}

func webMotionCallback(controller uintptr, x, y float64, data uintptr) {
	if _, v := webOf(data, controller); v != nil {
		v.web.pointer = driver.Point{X: driver.Points(x), Y: driver.Points(y)}
	}
}

// webScrollCallback leaves the wheels over a page that does not scroll
// to the scroll views around it.
func webScrollCallback(controller uintptr, dx, dy float64, data uintptr) int32 {
	w, v := webOf(data, controller)
	if v == nil || !v.web.config.NoScroll {
		return 0
	}
	at := w.frameAt(v)
	at.X += v.web.pointer.X
	at.Y += v.web.pointer.Y
	w.wheelEvent(gtkEventControllerGetCurrentEventState(controller), at, dx, dy, controllerPoints(controller))
	return 1
}
