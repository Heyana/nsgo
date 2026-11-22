//go:build darwin && !ios

package macos

import (
	"bytes"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

func TestDownloadUsesTheNativeAppHost(t *testing.T) {
	if os.Getenv("NSGO_TEST_DOWNLOAD_PROCESS") != t.Name() {
		root, home := t.TempDir(), t.TempDir()
		for _, phase := range []string{"transfer", "replay"} {
			command := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
			command.Env = append(os.Environ(), "NSGO_TEST_DOWNLOAD_PROCESS="+t.Name(), "HOME="+home, "NSGO_TEST_DOWNLOAD_ROOT="+root, "NSGO_TEST_DOWNLOAD_PHASE="+phase)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("%s: %v\n%s", phase, err, output)
			}
		}
		return
	}
	payload := bytes.Repeat([]byte("native app transfer"), 1024)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Download") != "app" {
			http.Error(w, "missing header", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/slow" {
			for range 64 {
				if _, err := w.Write(bytes.Repeat([]byte{7}, 64<<10)); err != nil {
					return
				}
				w.(http.Flusher).Flush()
				time.Sleep(10 * time.Millisecond)
			}
			return
		}
		w.Write(payload)
	}))
	defer server.Close()
	root := os.Getenv("NSGO_TEST_DOWNLOAD_ROOT")
	replay := os.Getenv("NSGO_TEST_DOWNLOAD_PHASE") == "replay"
	pin := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	var completed atomic.Bool
	var rejected atomic.Int32
	var canceled atomic.Bool
	app := func(cx *core.Scope) core.Node {
		labels := make(map[core.DownloadTask]string)
		cancelIssued := false
		maybeQuit := func() {
			if completed.Load() && rejected.Load() == 3 && canceled.Load() {
				cx.Post(cx.Quit)
			}
		}
		checkPrevious := func(path string) {
			actual, err := os.ReadFile(path)
			if err != nil || string(actual) != "previous file" {
				t.Errorf("a refused or canceled download changed %s: %v", filepath.Base(path), err)
			}
		}
		cx.OnDownloadUpdate(func(progress core.DownloadProgress) {
			label := labels[progress.Task]
			if replay {
				label = filepath.Base(progress.Path)
			}
			if label == "cancel" && progress.State == core.DownloadRunning && progress.Loaded > 0 && !cancelIssued {
				cancelIssued = true
				progress.Task.Cancel()
			}
			switch progress.State {
			case core.DownloadFailed, core.DownloadInterrupted:
				if label == "system" || label == "hostname" || label == "pin" {
					checkPrevious(progress.Path)
					rejected.Add(1)
				} else {
					t.Errorf("native app download: %+v", progress)
					cx.Post(cx.Quit)
				}
			case core.DownloadCanceled:
				if label != "cancel" {
					t.Errorf("unexpected cancellation of %s", label)
				}
				checkPrevious(progress.Path)
				canceled.Store(true)
			case core.DownloadCompleted:
				if label != "valid" {
					t.Errorf("the %s download was accepted", label)
					cx.Post(cx.Quit)
					return
				}
				actual, err := os.ReadFile(progress.Path)
				if err != nil || !bytes.Equal(actual, payload) {
					t.Errorf("downloaded %d/%d bytes: %v", len(actual), len(payload), err)
				}
				completed.Store(true)
			}
			maybeQuit()
		})
		if !replay {
			for _, request := range []struct{ label, url, certificate string }{
				{"valid", server.URL + "/file", pin},
				{"system", server.URL + "/file", ""},
				{"hostname", strings.Replace(server.URL, "127.0.0.1", "localhost", 1) + "/file", pin},
				{"pin", server.URL + "/file", "invalid"},
				{"cancel", server.URL + "/slow", pin},
			} {
				destination := filepath.Join(cx.DataDir(), request.label)
				if err := os.WriteFile(destination, []byte("previous file"), 0o600); err != nil {
					t.Error(err)
					cx.Post(cx.Quit)
					break
				}
				task, err := cx.Download(core.DownloadRequest{URL: request.url, Headers: map[string]string{"X-Download": "app"}}, destination, core.DownloadOptions{CertificatePEM: request.certificate})
				if err != nil {
					t.Error(err)
					cx.Post(cx.Quit)
					break
				}
				labels[task] = request.label
			}
		}

		return core.Text{Content: "Download"}
	}
	timeout := time.AfterFunc(30*time.Second, func() { panic("the native app download did not finish") })
	defer timeout.Stop()
	onMain(func() {
		Run(app, Options{ID: "dev.nsgo.test.download", Title: "Download", Width: 200, Height: 100, Host: engine.HostOptions{DataDir: root}})
	})
	if !completed.Load() || rejected.Load() != 3 || !canceled.Load() {
		t.Fatalf("completed %v, rejected %d, canceled %v", completed.Load(), rejected.Load(), canceled.Load())
	}
	records, err := filepath.Glob(filepath.Join(root, "nsgo-downloads", "*", "*.json"))
	if err != nil || len(records) != 5 {
		t.Fatalf("journals in the app's data directory = %d, error %v", len(records), err)
	}
}
