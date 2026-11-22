//go:build !js || !wasm

package diagram

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func pictureFixture(t *testing.T, data []byte) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/picture.png"
}
