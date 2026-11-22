//go:build darwin

package apple

import (
	"log"
	"maps"
	neturl "net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/privacy"
)

// Web is a web view's WKWebView, filling the view that holds it. WebKit
// loads on the first web view, so apps without one do not pay for it.
// The WKWebView is made on the first Configure.
type Web struct {
	// Dispatch reports the web view's events.
	Dispatch func(driver.Event)

	holder        objc.ID
	web           objc.ID
	handler       objc.ID
	config        driver.WebConfig
	fileDir       string                        // the directory a file: page, or HTML with a file: base URL, may navigate in; "" for other sources
	ownNavigation bool                          // load started the app's source, whose top-level navigation comes next
	ownStep       string                        // the URL of the entry a history command granted ownNavigation for, which only a back-forward action to it takes; "" for a load or reload
	committingOwn bool                          // the policy accepted an app-owned navigation, until commit
	ownPage       bool                          // the document last committed came from the app
	ownHistory    map[objc.ID]*driver.WebSource // retained native app-created entries
	pageSource    driver.WebSource              // the app source reload replays, including HTML bytes
	loading       objc.ID                       // the main-frame navigation whose start the delegate reported
	lastState     driver.WebNavigation
	stateQueued   bool
	htmlStart     *driver.WebSource // initial HTML if WebKit omitted its history entry
	htmlAnchor    objc.ID           // retained first native entry after that HTML
	htmlShown     bool              // initial HTML is shown in place of its anchor
	made          bool
}

var (
	classWebHandler objc.Class
	webHandlers     = map[objc.ID]*Web{}
	schemeHandlers  = map[objc.ID]*Web{}
)

// messageHandler is the name pages post to, as window.webkit.messageHandlers.nsgo.
const messageHandler = "nsgo"

var webStateKeys = [...]string{"URL", "title", "canGoBack", "canGoForward"}

var bridge = driver.WebBridge("function (d) { window.webkit.messageHandlers." + messageHandler + ".postMessage(d); }")

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
	failed := func(self objc.ID, _ objc.SEL, _, navigation, err objc.ID) {
		if w := webHandlers[self]; w != nil {
			if w.loading == navigation {
				w.loading = 0
			}
			domain := GoString(err.Send(sel("domain")))
			code := objc.Send[int](err, sel("code"))
			// WebKit reports an intentional stop or policy interruption as a failed navigation. They are control flow, not page errors.
			if appleWebCancellation(domain, code) {
				return
			}
			info := err.Send(sel("userInfo"))
			// NSURLErrorFailingURLStringErrorKey, else NSURLErrorFailingURLErrorKey.
			url := GoString(info.Send(sel("objectForKey:"), NSString("NSErrorFailingURLStringKey")))
			if url == "" {
				url = GoString(info.Send(sel("objectForKey:"), NSString("NSErrorFailingURLKey")).Send(sel("absoluteString")))
			}
			w.Dispatch(driver.Event{Kind: driver.EventError, Text: GoString(err.Send(sel("localizedDescription"))),
				Data: &driver.WebFailure{Domain: domain, Code: code}, Navigation: &driver.WebNavigation{URL: url}})
		}
	}
	var protocols []*objc.Protocol
	for _, name := range []string{"WKNavigationDelegate", "WKUIDelegate", "WKScriptMessageHandler", "WKDownloadDelegate", "WKURLSchemeHandler"} {
		if p := objc.GetProtocol(name); p != nil {
			protocols = append(protocols, p)
		}
	}
	classWebHandler = RegisterClass("NsgoWebHandler", Class("NSObject"), protocols,
		[]objc.MethodDef{
			{Cmd: sel("webView:didStartProvisionalNavigation:"), Fn: func(self objc.ID, _ objc.SEL, _, navigation objc.ID) {
				if w := webHandlers[self]; w != nil {
					w.loading = navigation
					w.reportNavigation(w.navigation(true), driver.EventLoadStart, driver.EventNavigationStateChange)
				}
			}},
			{Cmd: sel("webView:didFinishNavigation:"), Fn: func(self objc.ID, _ objc.SEL, _, navigation objc.ID) {
				if w := webHandlers[self]; w != nil {
					if w.loading == navigation {
						w.loading = 0
					}
					w.reportNavigation(w.navigation(false), driver.EventLoad, driver.EventNavigationStateChange)
				}
			}},
			{Cmd: sel("webView:didCommitNavigation:"), Fn: func(self objc.ID, _ objc.SEL, _, _ objc.ID) {
				if w := webHandlers[self]; w != nil {
					w.committed()
				}
			}},
			{Cmd: sel("webView:didFailNavigation:withError:"), Fn: failed},
			{Cmd: sel("webView:didFailProvisionalNavigation:withError:"), Fn: failed},
			{Cmd: sel("observeValueForKeyPath:ofObject:change:context:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID, web objc.ID, _ objc.ID, _ uintptr) {
				if w := webHandlers[self]; w != nil && w.web == web {
					w.queueStateChange()
				}
			}},
			{Cmd: sel("userContentController:didReceiveScriptMessage:"), Fn: func(self objc.ID, _ objc.SEL, _, message objc.ID) {
				if w := webHandlers[self]; w != nil {
					body := message.Send(sel("body"))
					frame := message.Send(sel("frameInfo"))
					w.Dispatch(driver.Event{Kind: driver.EventMessage, Text: GoString(body.Send(sel("description"))),
						Origin: frameOrigin(frame), URL: frameURL(frame), MainFrame: objc.Send[bool](frame, sel("isMainFrame"))})
				}
			}},
			{Cmd: sel("webView:startURLSchemeTask:"), Fn: func(self objc.ID, _ objc.SEL, _, task objc.ID) {
				w := schemeHandlers[self]
				if w == nil || task == 0 {
					return
				}
				u := task.Send(sel("request")).Send(sel("URL"))
				raw := GoString(u.Send(sel("absoluteString")))
				name := strings.ToLower(GoString(u.Send(sel("scheme"))))
				handler := w.config.SchemeHandlers[name]
				if handler == nil {
					task.Send(sel("didFinish"))
					return
				}
				request := &driver.WebSchemeRequest{URL: raw}
				req := task.Send(sel("request"))
				request.Method = GoString(req.Send(sel("HTTPMethod")))
				if fields := req.Send(sel("allHTTPHeaderFields")); fields != 0 {
					keys := fields.Send(sel("allKeys"))
					for i := range objc.Send[uint](keys, sel("count")) {
						k := keys.Send(sel("objectAtIndex:"), i)
						if request.Headers == nil {
							request.Headers = map[string]string{}
						}
						request.Headers[GoString(k)] = GoString(fields.Send(sel("objectForKey:"), k))
					}
				}
				resp := handler(request)
				// A nil answer is 404, an empty body.
				status := int64(404)
				var body []byte
				if resp != nil {
					status = int64(resp.Status)
					if status == 0 {
						status = 200
					}
					body = resp.Body
				}
				// A request is answered as HTTP (NSHTTPURLResponse), whose
				// status and headers the page's fetches see.
				fields := objc.ID(Class("NSMutableDictionary")).Send(sel("dictionary"))
				contentType := resp.Header("Content-Type")
				if contentType == "" {
					contentType = driver.WebMIME(GoString(u.Send(sel("path"))), body)
				}
				fields.Send(sel("setObject:forKey:"), NSString(contentType), NSString("Content-Type"))
				if resp != nil {
					for k, v := range resp.Headers {
						if !strings.EqualFold(k, "Content-Type") {
							fields.Send(sel("setObject:forKey:"), NSString(v), NSString(k))
						}
					}
				}
				response := objc.ID(Class("NSHTTPURLResponse")).Send(sel("alloc")).Send(
					sel("initWithURL:statusCode:HTTPVersion:headerFields:"), u,
					status, NSString("HTTP/1.1"), fields)
				task.Send(sel("didReceiveResponse:"), response)
				response.Send(sel("release"))
				var ptr unsafe.Pointer
				if len(body) > 0 {
					ptr = unsafe.Pointer(&body[0])
				}
				data := objc.ID(Class("NSData")).Send(sel("dataWithBytes:length:"), ptr, uintptr(len(body)))
				task.Send(sel("didReceiveData:"), data)
				task.Send(sel("didFinish"))
			}},
			{Cmd: sel("webView:stopURLSchemeTask:"), Fn: func(objc.ID, objc.SEL, objc.ID, objc.ID) {}},
			{Cmd: sel("webView:createWebViewWithConfiguration:forNavigationAction:windowFeatures:"), Fn: appleWebOpenWindow},
			// getUserMedia (macOS 12, iOS 15): what the config's Media
			// grants, which skips WebKit's own prompt; the system still
			// asks the user for each device.
			{Cmd: sel("webView:requestMediaCapturePermissionForOrigin:initiatedByFrame:type:decisionHandler:"),
				Fn: func(self objc.ID, _ objc.SEL, _, _, _ objc.ID, kind int, decide objc.ID) {
					const grant, deny = 1, 2 // WKPermissionDecision
					w := webHandlers[self]
					if w == nil || !w.mayCapture(captureMedia(kind)) {
						CallBlock(decide, deny)
						return
					}
					CallBlock(decide, grant)
				}},
			// Downloads (WKDownload, macOS 11.3 and iOS 14.5), only for a
			// view that saves them.
			{Cmd: sel("webView:decidePolicyForNavigationAction:decisionHandler:"), Fn: func(self objc.ID, _ objc.SEL, _, action, decide objc.ID) {
				const cancel, allow, download = 0, 1, 2 // WKNavigationActionPolicy
				policy := uintptr(allow)
				// A restricted view's page goes only where its config
				// allows at the top level; a new window's has no frame.
				if w := webHandlers[self]; w != nil {
					frame := action.Send(sel("targetFrame"))
					if frame == 0 || objc.Send[bool](frame, sel("isMainFrame")) {
						url := GoString(action.Send(sel("request")).Send(sel("URL")).Send(sel("absoluteString")))
						// The app's source loads in the main frame, so a new window's action is the page's.
						kind := objc.Send[int](action, sel("navigationType"))
						const backForward, reload = 2, 3 // WKNavigationType
						// A history step within the page starts no navigation, so the step's grant may still be set: only a back-forward action to the entry it was for takes it, never a page's navigation of another kind.
						own := frame != 0 && w.ownNavigation && (w.ownStep == "" || kind == backForward && url == w.ownStep)
						if frame != 0 && !own {
							item := w.currentHistory()
							// A reload, the page's own included, loads the current entry again; which entry a page's back-forward action goes to WebKit does not say, so only the app's history commands grant those.
							own = kind == reload && w.ownHistory[item] != nil &&
								url == GoString(item.Send(sel("URL")).Send(sel("absoluteString")))
							if own {
								w.pageSource = *w.ownHistory[item]
							}
							if kind == reload && w.ownPage && w.pageSource.HTML != "" {
								CallBlock(decide, cancel)
								w.load(w.pageSource)
								return
							}
						}
						if frame != 0 {
							w.ownNavigation, w.ownStep = false, ""
						}
						allowed := fileNavigable(url, w.fileDir, w.config.Restricted)
						if allowed && own {
							allowed = w.config.SourceNavigable(url)
						} else if allowed {
							allowed = w.config.Navigable(url)
						}
						if !allowed {
							CallBlock(decide, cancel)
							w.Dispatch(driver.Event{Kind: driver.EventError, Text: driver.RefusedNavigation(url),
								Navigation: &driver.WebNavigation{URL: url}})
							return
						}
						if frame != 0 {
							w.committingOwn = own
						}
					}
				}
				if w := webHandlers[self]; w != nil && w.config.DownloadDir != "" &&
					objc.Send[bool](action, sel("respondsToSelector:"), sel("shouldPerformDownload")) && objc.Send[bool](action, sel("shouldPerformDownload")) {
					policy = download
				}
				CallBlock(decide, policy)
			}},
			{Cmd: sel("webView:decidePolicyForNavigationResponse:decisionHandler:"), Fn: func(self objc.ID, _ objc.SEL, _, response, decide objc.ID) {
				const allow, download = 1, 2 // WKNavigationResponsePolicy
				policy := uintptr(allow)
				if w := webHandlers[self]; w != nil && w.config.DownloadDir != "" && objc.Send[bool](response, sel("isForMainFrame")) &&
					(!objc.Send[bool](response, sel("canShowMIMEType")) || attachment(response.Send(sel("response")))) {
					policy = download
				}
				CallBlock(decide, policy)
			}},
			{Cmd: sel("webView:navigationAction:didBecomeDownload:"), Fn: becameDownload},
			{Cmd: sel("webView:navigationResponse:didBecomeDownload:"), Fn: becameDownload},
			{Cmd: sel("download:decideDestinationUsingResponse:suggestedFilename:completionHandler:"),
				Fn: func(self objc.ID, _ objc.SEL, d, _, name, done objc.ID) {
					w, dl := webHandlers[self], downloads[d]
					if w == nil || dl == nil {
						CallBlock(done, 0)
						return
					}
					dl.Filename = filepath.Base(GoString(name))
					if dl.Filename == "." || dl.Filename == string(filepath.Separator) {
						dl.Filename = "download"
					}
					dir := filepath.Join(w.config.DownloadDir, dl.ID)
					if err := os.MkdirAll(dir, 0o755); err != nil {
						dl.State, dl.Message = driver.WebDownloadFailed, err.Error()
						w.download(d, dl)
						CallBlock(done, 0)
						return
					}
					dl.Path = filepath.Join(dir, dl.Filename)
					w.download(d, &driver.WebDownload{State: driver.WebDownloadStarted, ID: dl.ID, URL: dl.URL, Filename: dl.Filename, Path: dl.Path})
					CallBlock(done, uintptr(objc.ID(Class("NSURL")).Send(sel("fileURLWithPath:"), NSString(dl.Path))))
				}},
			{Cmd: sel("downloadDidFinish:"), Fn: func(self objc.ID, _ objc.SEL, d objc.ID) {
				if w, dl := webHandlers[self], downloads[d]; w != nil && dl != nil {
					dl.State = driver.WebDownloadFinished
					if info, err := os.Stat(dl.Path); err == nil {
						dl.Bytes = info.Size()
					}
					w.download(d, dl)
				}
			}},
			{Cmd: sel("download:didFailWithError:resumeData:"), Fn: func(self objc.ID, _ objc.SEL, d, err, _ objc.ID) {
				if w, dl := webHandlers[self], downloads[d]; w != nil && dl != nil {
					dl.State, dl.Message = driver.WebDownloadFailed, GoString(err.Send(sel("localizedDescription")))
					w.download(d, dl)
				}
			}},
		})
}

func appleWebCancellation(domain string, code int) bool {
	return domain == "NSURLErrorDomain" && code == -999 || domain == "WebKitErrorDomain" && code == 102
}

// captureMedia is the devices a WKMediaCaptureType names.
func captureMedia(kind int) driver.WebMedia {
	switch kind {
	case 0: // WKMediaCaptureTypeCamera
		return driver.WebCamera
	case 1: // WKMediaCaptureTypeMicrophone
		return driver.WebMicrophone
	}
	return driver.WebCamera | driver.WebMicrophone
}

// usageKeys are the Info.plist keys each device needs, without which the
// system would end the app as WebKit asks for it.
var usageKeys = [...]struct {
	media driver.WebMedia
	key   string
}{{driver.WebMicrophone, "NSMicrophoneUsageDescription"}, {driver.WebCamera, "NSCameraUsageDescription"}}

// mayCapture reports whether the page may capture want: the config
// grants it and the app declares why, logging what is missing.
func (w *Web) mayCapture(want driver.WebMedia) bool {
	if !w.config.Media.Allows(want) {
		log.Print("nsgo: a web view's page asked for a device its WebView's Media leaves out")
		return false
	}
	for _, u := range usageKeys {
		if want&u.media != 0 && !privacy.DeclaresUsage(u.key) {
			log.Printf("nsgo: a web view's page needs %s in the app's Info.plist (nsgo.yaml's apple.info)", u.key)
			return false
		}
	}
	return true
}

// downloads are the downloads running, by their WKDownload, each
// retained until it ends or its web view closes.
var (
	downloads   = map[objc.ID]*driver.WebDownload{}
	downloadSeq int
)

// becameDownload follows a navigation that became download d, which the
// web view's handler then delegates.
func becameDownload(self objc.ID, _ objc.SEL, _, _, d objc.ID) {
	w := webHandlers[self]
	if w == nil {
		return
	}
	downloadSeq++
	dl := &driver.WebDownload{ID: strconv.FormatInt(time.Now().UnixMilli(), 36) + "-" + strconv.Itoa(downloadSeq)}
	if request := d.Send(sel("originalRequest")); request != 0 {
		dl.URL = GoString(request.Send(sel("URL")).Send(sel("absoluteString")))
	}
	downloads[d.Send(sel("retain"))] = dl
	d.Send(sel("setDelegate:"), w.handler)
}

// attachment reports whether an HTTP response asks to be saved
// (Content-Disposition: attachment).
func attachment(response objc.ID) bool {
	if response == 0 || !objc.Send[bool](response, sel("isKindOfClass:"), Class("NSHTTPURLResponse")) {
		return false
	}
	v := GoString(response.Send(sel("valueForHTTPHeaderField:"), NSString("Content-Disposition")))
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(v)), "attachment")
}

// download reports dl, letting download d go once it ended.
func (w *Web) download(d objc.ID, dl *driver.WebDownload) {
	report := *dl
	w.Dispatch(driver.Event{Kind: driver.EventDownload, Data: &report})
	if dl.State != driver.WebDownloadStarted {
		delete(downloads, d)
		d.Send(sel("release"))
	}
}

// cancelDownloads cancels and lets go the downloads w's handler
// delegates, as w closes: once the handler goes, nothing would end them.
func (w *Web) cancelDownloads() {
	for d := range downloads {
		if d.Send(sel("delegate")) == w.handler {
			delete(downloads, d)
			d.Send(sel("setDelegate:"), objc.ID(0))
			d.Send(sel("cancel:"), objc.ID(0))
			d.Send(sel("release"))
		}
	}
}

// NewWeb returns the web view of holder, empty until configured.
func NewWeb(holder objc.ID) *Web {
	return &Web{holder: holder}
}

// View returns the WKWebView, 0 before the first Configure.
func (w *Web) View() objc.ID { return w.web }

// Configure applies c, loading the source again when it or the scripts
// changed.
func (w *Web) Configure(c *driver.WebConfig) {
	old, first := w.config, !w.made
	w.config = *c
	if first {
		w.make()
	}
	scripts := first || c.InjectedJavaScript != old.InjectedJavaScript ||
		c.InjectedJavaScriptBeforeContent != old.InjectedJavaScriptBeforeContent || c.Mobile != old.Mobile
	// NoScroll's script (macOS) applies from the next page on.
	if scripts || c.NoScroll != old.NoScroll {
		w.addScripts()
	}
	ua := c.UserAgent
	if ua == "" && w.mobile() {
		ua = driver.WebMobileUserAgentIPhone
	}
	if ua == "" {
		// WebKit's own, which ends in the suffix make gave the configuration.
		w.web.Send(sel("setCustomUserAgent:"), objc.ID(0))
	} else {
		w.web.Send(sel("setCustomUserAgent:"), NSString(driver.WebUserAgent(ua, c.UserAgentSuffix)))
	}
	w.web.Send(sel("setAllowsBackForwardNavigationGestures:"), c.BackForwardGestures)
	if runtime.GOOS != "ios" {
		w.web.Send(sel("setAllowsMagnification:"), c.PinchZoom)
	}
	w.web.Send(sel("configuration")).Send(sel("defaultWebpagePreferences")).
		Send(sel("setAllowsContentJavaScript:"), !c.NoJavaScript)
	if runtime.GOOS == "ios" {
		w.web.Send(sel("scrollView")).Send(sel("setScrollEnabled:"), !c.NoScroll)
	}
	const suspend = "setAllMediaPlaybackSuspended:completionHandler:"
	if c.Muted != old.Muted && responds(w.web, suspend) {
		w.web.Send(sel(suspend), c.Muted, objc.ID(0))
	}
	if first || scripts || c.Source != old.Source || !maps.Equal(c.Headers, old.Headers) {
		w.load(c.Source)
	}
}

func (w *Web) make() {
	loadWebKit()
	config := objc.ID(Class("WKWebViewConfiguration")).Send(sel("new"))
	// On macOS, only a script with a WebKit user gesture may request a
	// new window. iOS already uses this pop-up policy.
	config.Send(sel("preferences")).Send(sel("setJavaScriptCanOpenWindowsAutomatically:"), false)
	// WebKit appends the application name to its default user agent after a space, and a view reads it only as it is made; iOS's default name, such as Mobile/15E148, stays before the suffix.
	if suffix := w.config.UserAgentSuffix; suffix != "" {
		name := GoString(config.Send(sel("applicationNameForUserAgent")))
		config.Send(sel("setApplicationNameForUserAgent:"), NSString(driver.WebUserAgent(name, suffix)))
	}
	if w.config.Incognito {
		store := objc.ID(Class("WKWebsiteDataStore")).Send(sel("nonPersistentDataStore"))
		config.Send(sel("setWebsiteDataStore:"), store)
	}
	w.handler = objc.ID(classWebHandler).Send(sel("new"))
	webHandlers[w.handler] = w
	config.Send(sel("userContentController")).Send(sel("addScriptMessageHandler:name:"), w.handler, NSString(messageHandler))
	for name := range w.config.SchemeHandlers {
		// WebKit raises NSInvalidArgumentException, which ends the app, for a scheme it handles itself: core refuses those, and this keeps a later WebKit's own from ending it.
		if objc.Send[bool](objc.ID(Class("WKWebView")), sel("handlesURLScheme:"), NSString(name)) {
			log.Printf("nsgo: WebKit handles the %q scheme itself: the web view does not serve it", name)
			continue
		}
		schemeHandlers[w.handler] = w
		config.Send(sel("setURLSchemeHandler:forURLScheme:"), w.handler, NSString(name))
	}
	bounds := objc.Send[CGRect](w.holder, sel("bounds"))
	w.web = objc.ID(Class("WKWebView")).Send(sel("alloc")).Send(sel("initWithFrame:configuration:"), bounds, config)
	config.Send(sel("release"))
	w.web.Send(sel("setNavigationDelegate:"), w.handler)
	w.web.Send(sel("setUIDelegate:"), w.handler)
	for _, key := range webStateKeys {
		w.web.Send(sel("addObserver:forKeyPath:options:context:"), w.handler, NSString(key), uint(0), uintptr(0))
	}
	const flexibleSize = 2 | 16 // width and height, in AppKit and UIKit alike
	w.web.Send(sel("setAutoresizingMask:"), uint(flexibleSize))
	// WebKit paints white otherwise; on macOS a page without a
	// background shows the view's own, as its style expects. iOS keeps
	// WebKit's, as NSH does.
	if runtime.GOOS != "ios" {
		no := objc.ID(Class("NSNumber")).Send(sel("numberWithBool:"), false)
		w.web.Send(sel("setValue:forKey:"), no, NSString("drawsBackground"))
	}
	w.holder.Send(sel("addSubview:"), w.web)
	w.made = true
}

// mobile reports whether the page is presented as a phone's (WebConfig.Mobile), which only macOS does: iOS shows pages as they are.
func (w *Web) mobile() bool { return w.config.Mobile && runtime.GOOS != "ios" }

func (w *Web) addScripts() {
	content := w.web.Send(sel("configuration")).Send(sel("userContentController"))
	content.Send(sel("removeAllUserScripts"))
	addIn := func(source string, at int, mainFrameOnly bool) {
		if source == "" {
			return
		}
		script := objc.ID(Class("WKUserScript")).Send(sel("alloc")).Send(
			sel("initWithSource:injectionTime:forMainFrameOnly:"), NSString(source), at, mainFrameOnly)
		content.Send(sel("addUserScript:"), script)
		script.Send(sel("release"))
	}
	add := func(source string, at int) { addIn(source, at, true) }
	const atStart, atEnd = 0, 1 // WKUserScriptInjectionTime
	if w.mobile() {
		addIn(driver.WebMobileScript("iPhone", true), atStart, false)
	}
	add(bridge, atStart)
	add(w.config.InjectedJavaScriptBeforeContent, atStart)
	if w.config.NoScroll && runtime.GOOS != "ios" {
		add(noScroll, atEnd)
	}
	add(w.config.InjectedJavaScript, atEnd)
}

func (w *Web) load(src driver.WebSource) {
	w.pageSource = src
	w.ownNavigation, w.ownStep = false, ""
	w.fileDir = ""
	switch {
	case src.HTML != "":
		w.ownNavigation = true
		if w.htmlStart == nil && w.currentHistory() == 0 {
			saved := src
			w.htmlStart = &saved
		}
		w.htmlShown = w.htmlStart != nil
		base := objc.ID(0)
		if src.BaseURL != "" {
			base = nsURL(src.BaseURL)
			w.fileDir, _ = fileBaseDir(src.BaseURL)
		}
		w.web.Send(sel("loadHTMLString:baseURL:"), NSString(src.HTML), base)
	case src.Src != "":
		w.htmlShown = false
		// A URL NSURL cannot parse loads nothing.
		if url := nsURL(src.Src); url != 0 {
			if path, ok := fileURLPath(src.Src); ok {
				w.fileDir = filepath.Dir(path)
				access := url.Send(sel("URLByDeletingLastPathComponent"))
				w.web.Send(sel("loadFileURL:allowingReadAccessToURL:"), url, access)
				return
			}
			w.ownNavigation = true
			request := objc.ID(Class("NSMutableURLRequest")).Send(sel("requestWithURL:"), url)
			for name, value := range w.config.Headers {
				request.Send(sel("setValue:forHTTPHeaderField:"), NSString(value), NSString(name))
			}
			w.web.Send(sel("loadRequest:"), request)
		}
	}
}

func (w *Web) currentHistory() objc.ID {
	return w.web.Send(sel("backForwardList")).Send(sel("currentItem"))
}

func (w *Web) committed() {
	w.ownPage, w.committingOwn = w.committingOwn, false
	w.pruneHistory()
	item := w.currentHistory()
	if !w.ownPage && w.htmlStart != nil {
		w.htmlShown = false
		if w.htmlAnchor == 0 && item != 0 {
			w.htmlAnchor = item.Send(sel("retain"))
		}
	}
	if w.ownPage && item != 0 && !(w.htmlShown && w.htmlAnchor != 0) && w.ownHistory[item] == nil {
		if w.ownHistory == nil {
			w.ownHistory = make(map[objc.ID]*driver.WebSource)
		}
		item.Send(sel("retain"))
		src := w.pageSource
		w.ownHistory[item] = &src
	}
}

func (w *Web) pruneHistory() {
	list := w.web.Send(sel("backForwardList"))
	live := map[objc.ID]bool{w.currentHistory(): true}
	for _, name := range []string{"backList", "forwardList"} {
		items := list.Send(sel(name))
		for i, n := uint(0), objc.Send[uint](items, sel("count")); i < n; i++ {
			live[items.Send(sel("objectAtIndex:"), i)] = true
		}
	}
	for item := range w.ownHistory {
		if !live[item] {
			delete(w.ownHistory, item)
			item.Send(sel("release"))
		}
	}
	if w.htmlAnchor != 0 && !live[w.htmlAnchor] {
		w.htmlAnchor.Send(sel("release"))
		w.htmlStart, w.htmlAnchor, w.htmlShown = nil, 0, false
	}
}

func fileURLPath(raw string) (string, bool) {
	u, err := neturl.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "file") || (u.Host != "" && !strings.EqualFold(u.Host, "localhost")) || u.Path == "" {
		return "", false
	}
	p, err := neturl.PathUnescape(u.EscapedPath())
	if err != nil || p == "" {
		return "", false
	}
	return filepath.Clean(p), true
}

// fileBaseDir returns the directory a file: base URL names: itself when it ends in a slash, else the one holding it.
func fileBaseDir(raw string) (string, bool) {
	path, ok := fileURLPath(raw)
	if !ok {
		return "", false
	}
	if u, err := neturl.Parse(raw); err == nil && strings.HasSuffix(u.Path, "/") {
		return path, true
	}
	return filepath.Dir(path), true
}

// fileNavigable reports whether a page may go to url as far as file: URLs go: a page from a file, or HTML with a file: base URL, only within fileDir, and a restricted view's page to no other file. Other views leave file: URLs to WebKit, which keeps web pages from them.
func fileNavigable(url, fileDir string, restricted bool) bool {
	if !strings.EqualFold(strings.SplitN(url, ":", 2)[0], "file") || fileDir == "" && !restricted {
		return true
	}
	return fileDir != "" && fileURLInDir(url, fileDir)
}

func fileURLInDir(raw, dir string) bool {
	path, ok := fileURLPath(raw)
	if !ok {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// frameOrigin is the origin of a WKFrameInfo's document, as driver.WebOrigin writes it: its security origin's, or for a document whose security origin is opaque, such as a sandboxed one, its URL's.
func frameOrigin(frame objc.ID) string {
	origin := frame.Send(sel("securityOrigin"))
	scheme := GoString(origin.Send(sel("protocol")))
	if scheme == "" {
		return driver.WebOrigin(frameURL(frame))
	}
	host := GoString(origin.Send(sel("host")))
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	if port := objc.Send[int](origin, sel("port")); port != 0 {
		host += ":" + strconv.Itoa(port)
	}
	return driver.WebOrigin(scheme + "://" + host + "/")
}

// frameURL is the URL of a WKFrameInfo's document.
func frameURL(frame objc.ID) string {
	return GoString(frame.Send(sel("request")).Send(sel("URL")).Send(sel("absoluteString")))
}

func nsURL(s string) objc.ID {
	return objc.ID(Class("NSURL")).Send(sel("URLWithString:"), NSString(s))
}

// navigation reads the WebKit state the app presents.
func (w *Web) navigation(loading bool) driver.WebNavigation {
	nav := driver.WebNavigation{
		URL:          GoString(w.web.Send(sel("URL")).Send(sel("absoluteString"))),
		Title:        GoString(w.web.Send(sel("title"))),
		Loading:      loading,
		CanGoBack:    objc.Send[bool](w.web, sel("canGoBack")),
		CanGoForward: objc.Send[bool](w.web, sel("canGoForward")),
	}
	if w.htmlAnchor != 0 {
		if w.htmlShown {
			nav.CanGoBack, nav.CanGoForward = false, true
		} else if w.currentHistory() == w.htmlAnchor {
			nav.CanGoBack = true
		}
	}
	return nav
}

func (w *Web) reportNavigation(nav driver.WebNavigation, kinds ...driver.EventKind) {
	w.lastState = nav
	for _, kind := range kinds {
		n := nav
		w.Dispatch(driver.Event{Kind: kind, Navigation: &n})
	}
}

// queueStateChange reads the URL, title and history together after a WebKit
// update. A full load reports its state through the navigation delegate.
func (w *Web) queueStateChange() {
	if w.stateQueued {
		return
	}
	w.stateQueued = true
	OnMain(func() {
		w.stateQueued = false
		if w.web == 0 || w.loading != 0 || objc.Send[bool](w.web, sel("isLoading")) {
			return
		}
		nav := w.navigation(false)
		if nav != w.lastState {
			w.reportNavigation(nav, driver.EventNavigationStateChange)
		}
	})
}

// Command does c, with value for messages and scripts.
func (w *Web) Command(c driver.WebCommand, value string, data any) {
	if w.web == 0 {
		return
	}
	// stopLoading may call the delegate back at once; the view outlives
	// the command.
	web := w.web.Send(sel("retain"))
	defer web.Send(sel("release"))
	switch c {
	case driver.WebReload:
		if w.ownPage && w.pageSource.HTML != "" {
			w.load(w.pageSource)
			return
		}
		w.ownNavigation, w.ownStep = w.ownPage, ""
		web.Send(sel("reload"))
	case driver.WebGoBack:
		if w.htmlStart != nil && !w.htmlShown && w.currentHistory() == w.htmlAnchor {
			w.load(*w.htmlStart)
			return
		}
		w.useHistory(web.Send(sel("backForwardList")).Send(sel("backItem")))
		web.Send(sel("goBack"))
	case driver.WebGoForward:
		if w.htmlShown && w.htmlAnchor != 0 {
			w.htmlShown = false
			w.useHistory(w.htmlAnchor)
			web.Send(sel("goToBackForwardListItem:"), w.htmlAnchor)
			return
		}
		w.useHistory(web.Send(sel("backForwardList")).Send(sel("forwardItem")))
		web.Send(sel("goForward"))
	case driver.WebStopLoading:
		web.Send(sel("stopLoading"))
	case driver.WebPostMessage:
		w.evaluate(driver.WebMessageScript(value))
	case driver.WebInjectJavaScript:
		w.evaluate(value)
	case driver.WebLoad:
		if src, ok := data.(*driver.WebSource); ok && src != nil {
			w.load(*src)
		}
	}
}

func (w *Web) useHistory(item objc.ID) {
	src := w.ownHistory[item]
	w.ownNavigation, w.ownStep = src != nil, ""
	if src != nil {
		w.pageSource = *src
		w.ownStep = GoString(item.Send(sel("URL")).Send(sel("absoluteString")))
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
	for _, key := range webStateKeys {
		w.web.Send(sel("removeObserver:forKeyPath:context:"), w.handler, NSString(key), uintptr(0))
	}
	w.web.Send(sel("stopLoading"))
	w.web.Send(sel("setNavigationDelegate:"), objc.ID(0))
	w.web.Send(sel("setUIDelegate:"), objc.ID(0))
	for item := range w.ownHistory {
		item.Send(sel("release"))
	}
	w.ownHistory = nil
	if w.htmlAnchor != 0 {
		w.htmlAnchor.Send(sel("release"))
		w.htmlAnchor = 0
	}
	// The content controller holds the handler, which would keep it alive.
	w.web.Send(sel("configuration")).Send(sel("userContentController")).
		Send(sel("removeScriptMessageHandlerForName:"), NSString(messageHandler))
	w.web.Send(sel("removeFromSuperview"))
	w.web.Send(sel("release"))
	w.cancelDownloads()
	delete(webHandlers, w.handler)
	delete(schemeHandlers, w.handler)
	w.handler.Send(sel("release"))
	w.web, w.handler = 0, 0
}
