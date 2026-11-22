//go:build nsgodev && !(js && wasm && nsgominiprogram)

package devtools

import (
	"io"
	"net/http"
)

// Fetch sends nsgo dev's server a request with the run's token (Token), and body as contentType when it has one.
func Fetch(method, url, contentType string, body io.Reader) (*Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-NSGo-Token", Token())
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	return &Response{Status: resp.Status, StatusCode: resp.StatusCode, Body: resp.Body}, nil
}
