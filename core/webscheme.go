package core

import (
	"fmt"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// ServeScheme registers an app-owned URL scheme for web views created under cx: cx and what it owns, where the nearest scope serving a scheme answers it, and no other subtree or window. A web view serves the schemes registered before its native view is made; the handler runs on the platform's UI thread and returns the response body, which every request answers with status 200 whatever its method, as ServeSchemeRequest's handler of WebSchemeResponse{Body: handler(url)}. The response's content type is inferred from the URL and bytes by the native backend. It refuses the schemes web views handle themselves, such as http, ws or javascript.
func (cx *Scope) ServeScheme(scheme string, handler func(url string) []byte) error {
	if handler == nil {
		// ServeSchemeRequest reports the scope, the scheme's name and the
		// missing handler, in that order.
		return cx.ServeSchemeRequest(scheme, nil)
	}
	return cx.ServeSchemeRequest(scheme, func(r *WebSchemeRequest) *WebSchemeResponse {
		return &WebSchemeResponse{Body: handler(r.URL)}
	})
}

// ServeSchemeRequest registers an app-owned URL scheme for web views created under cx, as ServeScheme does, whose handler sees the request — its URL, its method where the platform reports one and its headers where it gives them — and answers its status, headers and body: status as HTTP's, 405 for a method the scheme refuses among them, with 0 for 200 and a nil response for 404, an empty body. The response's content type is the response's Content-Type header, else what the native backend infers from the URL and body. As WebKit checks them, a scheme's cross-origin fetches also need the response's Access-Control-Allow-Origin header; WebView2 lets no page of another scheme request it.
func (cx *Scope) ServeSchemeRequest(scheme string, handler func(*WebSchemeRequest) *WebSchemeResponse) error {
	if cx == nil || cx.rt == nil {
		return fmt.Errorf("core: no scope")
	}
	name, err := validWebScheme(scheme)
	if err != nil {
		return err
	}
	if handler == nil {
		return fmt.Errorf("core: web scheme %q has no handler", name)
	}
	schemes, _ := cx.values[webSchemesKey{}].(map[string]driver.WebSchemeHandler)
	if schemes == nil {
		schemes = map[string]driver.WebSchemeHandler{}
		if cx.values == nil {
			cx.values = map[any]any{}
		}
		cx.values[webSchemesKey{}] = schemes
	}
	schemes[name] = handler
	return nil
}

// webSchemesKey keys the schemes a scope serves in its values.
type webSchemesKey struct{}

// webSchemesOf returns the schemes the web views of owner o serve, each the nearest scope's, nil for none.
func webSchemesOf(o *Scope) map[string]driver.WebSchemeHandler {
	var out map[string]driver.WebSchemeHandler
	for s := o; s != nil; s = s.parent {
		schemes, _ := s.values[webSchemesKey{}].(map[string]driver.WebSchemeHandler)
		for name, handler := range schemes {
			if _, nearer := out[name]; !nearer {
				if out == nil {
					out = map[string]driver.WebSchemeHandler{}
				}
				out[name] = handler
			}
		}
	}
	return out
}

// servesWebScheme reports whether a scope at or above o serves scheme, written in lower case.
func servesWebScheme(o *Scope, scheme string) bool {
	for s := o; s != nil; s = s.parent {
		if schemes, ok := s.values[webSchemesKey{}].(map[string]driver.WebSchemeHandler); ok && schemes[scheme] != nil {
			return true
		}
	}
	return false
}

func validWebScheme(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSuffix(raw, ":"))
	if name == "" {
		return "", fmt.Errorf("core: web scheme is empty")
	}
	if webBuiltinSchemes[name] {
		return "", fmt.Errorf("core: web scheme %q is reserved: web views handle it themselves", name)
	}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (i > 0 && ((r >= '0' && r <= '9') || r == '+' || r == '-' || r == '.')) {
			continue
		}
		return "", fmt.Errorf("core: invalid web scheme %q", raw)
	}
	return name, nil
}

// webBuiltinSchemes are the schemes web views handle themselves, which an app cannot serve: WebKit's built-in schemes (LegacySchemeRegistry::isBuiltinScheme, for which WKWebView's setURLSchemeHandler:forURLScheme: raises an exception that ends the app) and the URL standard's special schemes, which WebView2 and WebKitGTK keep too.
var webBuiltinSchemes = map[string]bool{
	"about": true, "applewebdata": true, "blob": true, "data": true, "file": true, "ftp": true,
	"http": true, "https": true, "javascript": true, "resource": true, "safari-extension": true,
	"webkit-fake-url": true, "webkit-pdfjs-viewer": true, "ws": true, "wss": true,
	"x-apple-content-filter": true, "x-apple-ql-id": true,
}
