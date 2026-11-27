package harmony

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/nsgo-dev/goja"
)

// TestWebViewConfigLogic runs the actual ArkTS WebViewConfig.ets helpers in goja
// to verify user-agent composition, reload/recreation semantics, navigation
// restrictions, injected script ordering, media mapping, and download sanitization.
func TestWebViewConfigLogic(t *testing.T) {
	source, err := templates.ReadFile("templates/entry/src/main/ets/pages/WebViewConfig.ets")
	if err != nil {
		t.Fatal(err)
	}
	js := api.Transform(string(source), api.TransformOptions{Loader: api.LoaderTS, Format: api.FormatIIFE, GlobalName: "webcfg"})
	if len(js.Errors) > 0 {
		t.Fatal(js.Errors)
	}
	cases := []struct {
		name, expr string
	}{
		{"default ua unchanged", `webcfg.resolveWebUserAgent('ArkWeb/1.0', '', '') === 'ArkWeb/1.0'`},
		{"custom ua overrides default", `webcfg.resolveWebUserAgent('ArkWeb/1.0', 'Custom/2.0', '') === 'Custom/2.0'`},
		{"suffix appends once to default", `webcfg.resolveWebUserAgent('ArkWeb/1.0', '', 'MyApp/3.0') === 'ArkWeb/1.0 MyApp/3.0' && webcfg.resolveWebUserAgent('ArkWeb/1.0', '', 'MyApp/3.0') === 'ArkWeb/1.0 MyApp/3.0'`},
		{"suffix appends to custom ua", `webcfg.resolveWebUserAgent('ArkWeb/1.0', 'Custom/2.0', 'MyApp/3.0') === 'Custom/2.0 MyApp/3.0'`},
		{"clearing custom restores default", `webcfg.resolveWebUserAgent('ArkWeb/1.0', '', '') === 'ArkWeb/1.0'`},
		{"reload key changes only on source or headers", `webcfg.webReloadKey('https://a.test', '', '', [{k:'X-A',v:'1'}]) !== webcfg.webReloadKey('https://a.test', '', '', [{k:'X-A',v:'2'}]) && webcfg.webReloadKey('https://a.test', '', '', []) === webcfg.webReloadKey('https://a.test', '', '', [])`},
		{"hooks key changes on scripts or noJS", `webcfg.webHooksKey(false, 'a', '') !== webcfg.webHooksKey(false, 'b', '') && webcfg.webHooksKey(false, 'a', 'x') !== webcfg.webHooksKey(false, 'a', 'y') && webcfg.webHooksKey(false, 'a', '') !== webcfg.webHooksKey(true, 'a', '')`},
		{"document start injects bridge before user script", `var s = webcfg.webDocumentStartScripts(false, 'tok1', 'window.x=1;'); s.length === 2 && s[0].indexOf('window.nsgo') >= 0 && s[0].indexOf('tok1') >= 0 && s[1].indexOf('window.x=1;') >= 0 && s[1].indexOf('window === window.top') >= 0`},
		{"document end wraps user script in top frame", `var s = webcfg.webDocumentEndScripts(false, 'window.y=2;'); s.length === 1 && s[0].indexOf('window.y=2;') >= 0 && s[0].indexOf('window === window.top') >= 0`},
		{"noJS suppresses start and end scripts", `webcfg.webDocumentStartScripts(true, 'tok1', 'window.x=1;').length === 0 && webcfg.webDocumentEndScripts(true, 'window.y=2;').length === 0`},
		{"unrestricted allows any url", `webcfg.webNavigable(false, [], [], [], 'https://any.example/path', false) === true`},
		{"restricted host matching", `webcfg.webNavigable(true, ['example.test', '*.cdn.test'], [], [], 'https://EXAMPLE.TEST:443/p', false) === true && webcfg.webNavigable(true, ['example.test', '*.cdn.test'], [], [], 'https://a.cdn.test/img', false) === true && webcfg.webNavigable(true, ['example.test', '*.cdn.test'], [], [], 'https://cdn.test/', false) === false && webcfg.webNavigable(true, ['example.test'], [], [], 'https://evil.test/', false) === false`},
		{"restricted page schemes vs app source", `webcfg.webNavigable(true, ['example.test'], [], [], 'data:text/html,hi', false) === false && webcfg.webNavigable(true, ['example.test'], [], [], 'data:text/html,hi', true) === true && webcfg.webNavigable(true, ['example.test'], ['data'], [], 'data:text/html,hi', false) === true && webcfg.webNavigable(true, ['example.test'], [], [], 'about:blank', true) === true && webcfg.webNavigable(true, ['example.test'], [], [], 'file:///etc/passwd', true) === false`},
		{"restricted app schemes", `webcfg.webNavigable(true, [], [], ['myapp'], 'myapp://home', false) === true && webcfg.webNavigable(true, [], [], ['myapp'], 'other://home', false) === false`},
		{"file access detection", `webcfg.webSourceNeedsFileAccess('file:///a.html', '', '') === true && webcfg.webSourceNeedsFileAccess('https://a.test', '', '') === false && webcfg.webSourceNeedsFileAccess('', '<p>hi</p>', 'file:///dir/') === true`},
		{"media resource mapping", `var m = webcfg.webMediaWant(['TYPE_AUDIO_CAPTURE', 'TYPE_VIDEO_CAPTURE']); m.want === 3 && m.perms.length === 2 && m.resources.length === 2`},
		{"download name sanitization", `webcfg.sanitizeDownloadName('../../evil/report.pdf') === 'report.pdf' && webcfg.sanitizeDownloadName('') === 'download' && webcfg.sanitizeDownloadName('..') === 'download'`},
		{"html without base normalizes data url to about:blank", `webcfg.normalizeReportedWebURL('data:text/html;charset=UTF-8,hi', true) === 'about:blank' && webcfg.normalizeReportedWebURL('data:text/html;charset=UTF-8,hi', false) === 'data:text/html;charset=UTF-8,hi'`},
		{"error filter ignores subframes and ERR_ABORTED", `webcfg.webShouldReportError(true, -105) === true && webcfg.webShouldReportError(false, -105) === false && webcfg.webShouldReportError(true, -3) === false`},
		{"cookie request parsing", `webcfg.parseWebCookiesReq('{"op":1,"url":"https://example.test/"}').op === 1 && webcfg.parseWebCookiesReq('') === undefined && webcfg.parseWebCookiesReq('{bad') === undefined`},
		{"cookie url validation", `webcfg.isValidWebCookieURL('https://example.test/path') === true && webcfg.isValidWebCookieURL('http://127.0.0.1:8080/') === true && webcfg.isValidWebCookieURL('file:///etc/passwd') === false && webcfg.isValidWebCookieURL('javascript:alert(1)') === false && webcfg.isValidWebCookieURL('') === false && webcfg.isValidWebCookieURL('https://') === false`},
		{"cookie batch validation never leaks value", `webcfg.validateWebCookieSetItems([{u:'https://example.test/',v:'sid=secret123; Path=/',n:'sid'}]) === '' && webcfg.validateWebCookieSetItems([{u:'javascript:bad',v:'sid=secret123',n:'sid'}]).indexOf('secret123') < 0 && webcfg.validateWebCookieSetItems([{u:'https://example.test/',v:'badvalue',n:'sid'}]).indexOf('sid') >= 0`},
		{"cookie sameSite mapping", `webcfg.mapWebCookieSameSite(0, true) === 3 && webcfg.mapWebCookieSameSite(0, false) === 0 && webcfg.mapWebCookieSameSite(1, false) === 1 && webcfg.mapWebCookieSameSite(2, true) === 2 && webcfg.mapWebCookieSameSite(99, true) === 0`},
		{"cookie refusal format", `webcfg.formatWebCookieRefused('SID', '17100005') === 'HarmonyOS WebCookieManager refused the cookie SID: 17100005' && webcfg.formatWebCookieRefused('', '') === 'HarmonyOS WebCookieManager refused the cookie unnamed'`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vm := goja.New()
			if _, err := vm.RunString(string(js.Code)); err != nil {
				t.Fatal(err)
			}
			res, err := vm.RunString(tc.expr)
			if err != nil || !res.ToBoolean() {
				t.Fatalf("%s: result %v, err %v", tc.expr, res, err)
			}
		})
	}
}

// TestWebConfigKeysMatchHost checks that every JSON key serialized by
// internal/platform/harmonyos/web.go is declared in WebViewConfig.ets's WebConfigOp.
func TestWebConfigKeysMatchHost(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	goSrc, err := os.ReadFile(filepath.Join(root, "internal", "platform", "harmonyos", "web.go"))
	if err != nil {
		t.Fatal(err)
	}
	etsSrc, err := templates.ReadFile("templates/entry/src/main/ets/pages/WebViewConfig.ets")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(goSrc), "type harmonyWebConfig struct {")
	if start < 0 {
		t.Fatal("harmonyWebConfig not found")
	}
	end := strings.Index(string(goSrc)[start:], "\n}")
	iface := regexp.MustCompile(`(?s)export interface WebConfigOp \{(.*?)\n\}`).FindStringSubmatch(string(etsSrc))
	if end < 0 || iface == nil {
		t.Fatal("harmonyWebConfig or WebConfigOp not found")
	}
	keys := regexp.MustCompile(`json:"([A-Za-z0-9]+)`).FindAllStringSubmatch(string(goSrc)[start:start+end], -1)
	if len(keys) == 0 {
		t.Fatal("harmonyWebConfig has no json keys")
	}
	for _, k := range keys {
		if !regexp.MustCompile(`(?m)^\s*` + k[1] + `\??:`).MatchString(iface[1]) {
			t.Errorf("WebConfigOp does not declare %q", k[1])
		}
	}
}

// TestHostWiresArkWebSettings verifies Index.ets hooks all WebConfig settings,
// controller calls, and callbacks onto ArkWeb's Web component.
func TestHostWiresArkWebSettings(t *testing.T) {
	srcBytes, err := templates.ReadFile("templates/entry/src/main/ets/pages/Index.ets")
	if err != nil {
		t.Fatal(err)
	}
	src := string(srcBytes)
	for _, want := range []string{
		"incognitoMode: this.v.webIncognito",
		".javaScriptAccess(!this.v.noJS)",
		".domStorageAccess(true)",
		".fileAccess(this.fileAccess)",
		".zoomAccess(this.pinchZoom)",
		".verticalScrollBarAccess(!this.noScroll)",
		".horizontalScrollBarAccess(!this.noScroll)",
		"name: '__nsgoWebBridge'",
		".onControllerAttached(",
		".onLoadIntercept(",
		".onPageBegin(",
		".onPageEnd(",
		".onTitleReceive(",
		".onNavigationEntryCommitted(",
		".onErrorReceive(",
		".onWindowNew(",
		".onPermissionRequest(",
		"setCustomUserAgent(",
		"setAudioMuted(",
		"setScrollable(",
		"setDownloadDelegate(",
		"loadData(",
		"loadUrl(",
		".javaScriptOnDocumentStart(",
		".javaScriptOnDocumentEnd(",
		"WebCookieManager.configCookieSync(",
		"WebCookieManager.saveCookieAsync(",
		"WebCookieManager.fetchAllCookies(",
		"WebCookieManager.fetchCookieSync(",
		"WebCookieManager.clearAllCookiesSync(",
		"WebStorage.deleteAllData(",
		"WebviewController.removeAllCache(",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("Index.ets missing ArkWeb wiring %q", want)
		}
	}
}

// TestWebCookieKeysMatchHost checks that every JSON key serialized or decoded
// for Bridge.webcookies in internal/platform/harmonyos/web.go is declared in
// WebViewConfig.ets.
func TestWebCookieKeysMatchHost(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	goSrc, err := os.ReadFile(filepath.Join(root, "internal", "platform", "harmonyos", "web.go"))
	if err != nil {
		t.Fatal(err)
	}
	etsSrc, err := templates.ReadFile("templates/entry/src/main/ets/pages/WebViewConfig.ets")
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		goType, tsIface string
	}{
		{"harmonyWebCookieEntry", "WebCookieSetItem"},
		{"harmonyWebCookiesRequest", "WebCookiesReq"},
		{"harmonyWebCookieWire", "WebCookieWire"},
	} {
		start := strings.Index(string(goSrc), "type "+pair.goType+" struct {")
		if start < 0 {
			t.Fatalf("%s not found", pair.goType)
		}
		end := strings.Index(string(goSrc)[start:], "\n}")
		iface := regexp.MustCompile(`(?s)export interface ` + pair.tsIface + ` \{(.*?)\n\}`).FindStringSubmatch(string(etsSrc))
		if end < 0 || iface == nil {
			t.Fatalf("%s or %s not found", pair.goType, pair.tsIface)
		}
		keys := regexp.MustCompile(`json:"([A-Za-z0-9]+)`).FindAllStringSubmatch(string(goSrc)[start:start+end], -1)
		if len(keys) == 0 {
			t.Fatalf("%s has no json keys", pair.goType)
		}
		for _, k := range keys {
			if !regexp.MustCompile(`(?m)^\s*` + k[1] + `\??:`).MatchString(iface[1]) {
				t.Errorf("%s does not declare %q", pair.tsIface, k[1])
			}
		}
	}
}
