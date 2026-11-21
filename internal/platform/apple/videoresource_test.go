//go:build darwin

package apple

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestVideoResourceRedirectKeepsOnlyThePlayerRangeAcrossOrigins(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, header := range []string{"Authorization", "X-Stream-Key", "Referer", "Cookie"} {
			if r.Header.Get(header) != "" {
				t.Errorf("a cross-origin redirect received %s", header)
			}
		}
		if got := r.Header.Get("Range"); got != "bytes=3-5" {
			t.Errorf("range = %q, want bytes=3-5", got)
		}
		http.ServeContent(w, r, "video.mp4", time.Time{}, strings.NewReader("0123456789"))
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sample" || r.Header.Get("X-Stream-Key") != "sample" ||
			r.Header.Get("Referer") != "https://player.example/" || r.Header.Get("Cookie") != "session=sample" {
			t.Error("the original source did not receive the configured headers")
		}
		http.Redirect(w, r, target.URL+"/video.mp4", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	source, err := url.Parse(origin.URL + "/video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	resource := newVideoResource(source, driver.NewVideoHeaders(map[string]string{
		"Authorization": "Bearer sample",
		"X-Stream-Key":  "sample",
		"Referer":       "https://player.example/",
		"Cookie":        "session=sample",
		"Range":         "bytes=0-0",
	}))
	resp, length, err := resource.response(context.Background(), videoResourceRange{offset: 3, length: 3})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil || length != 10 || string(data) != "345" {
		t.Errorf("length = %d, range body = %q, error = %v", length, data, err)
	}
}
