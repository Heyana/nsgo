package nsgotest

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHTTPTransportRoutesMethodsAndWildcards(t *testing.T) {
	transport := NewHTTPTransport(map[string]HTTPHandler{
		"POST https://api.example.com/todos*": func(req *http.Request) (HTTPResponse, error) {
			if got := req.Header.Get("X-Test"); got != "yes" {
				t.Fatalf("request header = %q, want yes", got)
			}
			return HTTPResponse{Status: http.StatusCreated, Header: http.Header{"X-Reply": {"ok"}}, Body: []byte(`[1,2]`)}, nil
		},
	})
	req, err := http.NewRequest(http.MethodPost, "https://api.example.com/todos?page=1", strings.NewReader("new"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Test", "yes")
	res, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusCreated || string(body) != `[1,2]` || res.Header.Get("X-Reply") != "ok" {
		t.Fatalf("response = %d %q %q", res.StatusCode, body, res.Header.Get("X-Reply"))
	}
}

func TestHTTPTransportRejectsMissingRoutes(t *testing.T) {
	transport := NewHTTPTransport(nil)
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	if _, err := transport.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "no HTTP route") {
		t.Fatalf("RoundTrip error = %v", err)
	}
}
