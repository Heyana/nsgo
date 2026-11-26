package nsgotest

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// HTTPResponse describes one response returned by an HTTPHandler.
type HTTPResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

// HTTPHandler answers one request made through an HTTPTransport.
type HTTPHandler func(*http.Request) (HTTPResponse, error)

// HTTPTransport is an in-process HTTP fake for tests that must run in a
// browser, where httptest.Server cannot listen on a loopback socket. Routes
// use the same format as the TSX test network, such as
// "POST https://api.example.com/todos*". An asterisk matches any substring.
type HTTPTransport struct {
	routes []httpRoute
}

type httpRoute struct {
	pattern string
	handler HTTPHandler
}

// NewHTTPTransport returns a RoundTripper that answers requests from routes.
// The first matching route is selected in map iteration order only when route
// keys overlap, so callers should avoid overlapping patterns.
func NewHTTPTransport(routes map[string]HTTPHandler) *HTTPTransport {
	keys := make([]string, 0, len(routes))
	for route := range routes {
		keys = append(keys, route)
	}
	sort.Strings(keys)
	ordered := make([]httpRoute, 0, len(keys))
	for _, route := range keys {
		ordered = append(ordered, httpRoute{pattern: route, handler: routes[route]})
	}
	return &HTTPTransport{routes: ordered}
}

// RoundTrip implements http.RoundTripper.
func (t *HTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("nsgotest: HTTP request has no URL")
	}
	key := req.Method + " " + req.URL.String()
	for _, route := range t.routes {
		method, pattern, ok := strings.Cut(route.pattern, " ")
		if !ok || method != req.Method || !wildcardMatch(pattern, req.URL.String()) {
			continue
		}
		if route.handler == nil {
			return nil, fmt.Errorf("nsgotest: HTTP route %q has no handler", route.pattern)
		}
		out, err := route.handler(req)
		if err != nil {
			return nil, err
		}
		status := out.Status
		if status == 0 {
			status = http.StatusOK
		}
		header := out.Header.Clone()
		if header == nil {
			header = make(http.Header)
		}
		return &http.Response{
			StatusCode:    status,
			Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
			Header:        header,
			Body:          io.NopCloser(bytes.NewReader(out.Body)),
			ContentLength: int64(len(out.Body)),
			Request:       req,
		}, nil
	}
	return nil, fmt.Errorf("nsgotest: no HTTP route for %s", key)
}

func wildcardMatch(pattern, value string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == value
	}
	if !strings.HasPrefix(value, parts[0]) {
		return false
	}
	value = value[len(parts[0]):]
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		i := strings.Index(value, part)
		if i < 0 {
			return false
		}
		value = value[i+len(part):]
	}
	return strings.HasSuffix(pattern, "*") || value == ""
}
