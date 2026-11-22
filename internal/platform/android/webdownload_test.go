package android

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestWebDownloadJava compiles and runs WebDownload.java on the host JVM against two local HTTP servers, verifying filename sanitization, data:/base64 decoding, same-origin and cross-origin header/Referer/cookie isolation across redirects, HTTP error handling and cancellation when a web view closes.
func TestWebDownloadJava(t *testing.T) {
	javac, java := androidJDK(t)
	classes := t.TempDir()
	if out, err := exec.CommandContext(t.Context(), javac, "-d", classes,
		filepath.Join("java", "dev", "oneo", "nsgo", "WebDownload.java"),
		filepath.Join("testdata", "WebDownloadCheck.java")).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}

	writeEcho := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "auth=%s\ncookie=%s\nua=%s\nreferer=%s\n",
			r.Header.Get("X-App-Token"), r.Header.Get("Cookie"), r.Header.Get("User-Agent"), r.Header.Get("Referer"))
	}

	originB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeEcho(w, r)
	}))
	t.Cleanup(originB.Close)

	var originA *httptest.Server
	originA = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file":
			writeEcho(w, r)
		case "/redirect-same":
			http.Redirect(w, r, "/file", http.StatusFound)
		case "/redirect-cross":
			http.Redirect(w, r, originB.URL+"/target", http.StatusFound)
		case "/redirect-file":
			w.Header().Set("Location", "file:///etc/passwd")
			w.WriteHeader(http.StatusFound)
		case "/missing":
			http.Error(w, "not found", http.StatusNotFound)
		case "/slow":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			flusher, _ := w.(http.Flusher)
			chunk := make([]byte, 1024)
			for range 200 {
				if _, err := w.Write(chunk); err != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
				select {
				case <-r.Context().Done():
					return
				case <-time.After(50 * time.Millisecond):
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(originA.Close)

	dir := t.TempDir()
	if out, err := exec.CommandContext(t.Context(), java, "-cp", classes, "dev.oneo.nsgo.WebDownloadCheck",
		dir, originA.URL, originB.URL).CombinedOutput(); err != nil {
		t.Fatalf("WebDownloadCheck: %v\n%s", err, out)
	}
}
