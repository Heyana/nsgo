package android

import (
	"bytes"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func TestBackgroundTransferStreamsAndResumes(t *testing.T) {
	testhost.RequireSource(t)
	javac, err := exec.LookPath("javac")
	if err != nil || exec.Command(javac, "-version").Run() != nil {
		t.Skip("no JDK")
	}
	dir := t.TempDir()
	if out, err := exec.CommandContext(t.Context(), javac, "-d", dir,
		"java/dev/oneo/nsgo/BackgroundTransfer.java", "testdata/BackgroundTransferCheck.java").CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	payload := bytes.Repeat([]byte{1, 2, 3, 4, 5, 6, 7, 8}, 128<<10)
	var resumed atomic.Bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "download" {
			http.Error(w, "missing header", http.StatusBadRequest)
			return
		}
		start := 0
		if value := r.Header.Get("Range"); value != "" {
			start, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(value, "bytes="), "-"))
			if start <= 0 || start >= len(payload) || r.Header.Get("If-Range") != `"test"` {
				http.Error(w, "invalid resume", http.StatusBadRequest)
				return
			}
			resumed.Store(true)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
		}
		w.Header().Set("ETag", `"test"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)-start))
		if start > 0 {
			w.WriteHeader(http.StatusPartialContent)
		}
		for data := payload[start:]; len(data) > 0; {
			n := min(len(data), 64<<10)
			if _, err := w.Write(data[:n]); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			data = data[n:]
			if start == 0 && r.TLS == nil {
				time.Sleep(55 * time.Millisecond)
			}
		}
	})
	java := filepath.Join(filepath.Dir(javac), "java")
	run := func(rawURL, name string, cert ...string) ([]byte, error) {
		args := []string{"-cp", dir, "dev.oneo.nsgo.BackgroundTransferCheck", rawURL, filepath.Join(dir, name)}
		return exec.CommandContext(t.Context(), java, append(args, cert...)...).CombinedOutput()
	}
	verify := func(name string) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || !bytes.Equal(got, payload) {
			t.Fatalf("%s has %d of %d bytes: %v", name, len(got), len(payload), err)
		}
	}
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()
	if out, err := run(httpServer.URL, "range.part"); err != nil {
		t.Fatalf("range: %v\n%s", err, out)
	}
	if !resumed.Load() {
		t.Fatal("the paused request never used Range")
	}
	verify("range.part")

	tlsServer := httptest.NewTLSServer(handler)
	defer tlsServer.Close()
	cert := filepath.Join(dir, "pin.pem")
	if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tlsServer.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := run(tlsServer.URL, "pinned.part", cert); err != nil {
		t.Fatalf("pinned: %v\n%s", err, out)
	}
	verify("pinned.part")
	if out, err := run(tlsServer.URL, "unpinned.part"); err == nil || !bytes.Contains(out, []byte("SSLHandshakeException")) {
		t.Fatalf("unpinned: %v\n%s", err, out)
	}
	wrongHost := strings.Replace(tlsServer.URL, "127.0.0.1", "localhost", 1)
	if out, err := run(wrongHost, "wrong-host.part", cert); err == nil || !bytes.Contains(out, []byte("SSLHandshakeException")) {
		t.Fatalf("pinned with wrong hostname: %v\n%s", err, out)
	}
}
