//go:build darwin && !ios

package apple

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

func TestDownloadPublishesAndRecoversItsStagedFile(t *testing.T) {
	root := t.TempDir()
	staged := filepath.Join(root, "staged")
	destination := filepath.Join(root, "destination")
	payload := []byte("finished download")
	if err := os.WriteFile(staged, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old destination"), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := publishDownload(staged, destination, "recovered"); err != nil {
			t.Fatal(err)
		}
		actual, err := os.ReadFile(destination)
		if err != nil || !bytes.Equal(actual, payload) {
			t.Fatalf("published %q: %v", actual, err)
		}
	}
	if _, err := os.Stat(staged); err != nil {
		t.Fatal("the staged original must survive until its result is journaled", err)
	}
	missing := filepath.Join(root, "absent")
	if err := publishDownload(missing, destination, "missing"); err == nil {
		t.Fatal("a missing staged file was published")
	}
	actual, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(actual, payload) {
		t.Fatal("a failed publication changed the destination")
	}
}

func TestNativeDownloadPausesResumesAndReplays(t *testing.T) {
	release := holdMainLoop()
	defer release()
	payload := bytes.Repeat([]byte("background transfer"), 512<<10)
	var ranged atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Download-Test") != "sample" {
			http.Error(w, "missing header", http.StatusForbidden)
			return
		}
		start := 0
		if value := r.Header.Get("Range"); value != "" {
			start, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(value, "bytes="), "-"))
			if start < 0 || start >= len(payload) {
				http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
				return
			}
			ranged.Store(true)
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("ETag", `"download-test"`)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)-start))
		if start > 0 {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
			w.WriteHeader(http.StatusPartialContent)
		}
		for remaining := payload[start:]; len(remaining) > 0; {
			size := min(len(remaining), 64<<10)
			if _, err := w.Write(remaining[:size]); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			remaining = remaining[size:]
			time.Sleep(8 * time.Millisecond)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	directory := filepath.Join(root, "journal")
	destination := filepath.Join(root, "result")
	updates := make(chan driver.DownloadProgress, 1024)
	stop := driver.HearDownloads(func(update driver.DownloadProgress) {
		if update.ID == "nativeTest" {
			updates <- update
		}
	})
	defer stop()
	var host *downloadHost
	open := func() {
		host = &downloadHost{directory: directory, identifier: "dev.nsgo.test.download"}
		if err := host.open(); err != nil {
			t.Error(err)
		}
	}
	closeHost := func() {
		host.session.Send(sel("invalidateAndCancel"))
		host.session.Send(sel("release"))
		delete(downloadDelegates, host.delegate)
		host.delegate.Send(sel("release"))
	}
	onMain(open)
	defer func() { onMain(closeHost) }()
	pin := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	onMain(func() {
		host.command(driver.DownloadCommand{Op: "start", ID: "nativeTest", Path: destination, URL: server.URL + "/file", Headers: map[string]string{"X-Download-Test": "sample"}, CertificatePEM: pin})
	})
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()
	pausing, resumed := false, false
	started := time.Now()
	reports := 0
	for {
		select {
		case update := <-updates:
			reports++
			if update.State == "failed" || update.State == "interrupted" {
				t.Fatalf("native download: %+v", update)
			}
			if update.State == "running" && update.Loaded > 128<<10 && !pausing {
				pausing = true
				onMain(func() { host.command(driver.DownloadCommand{Op: "pause", ID: update.ID}) })
			}
			if update.State == "paused" && !resumed {
				resumed = true
				onMain(func() { closeHost(); open(); host.command(driver.DownloadCommand{Op: "resume", ID: update.ID}) })
			}
			if update.State == "completed" {
				// Allow state transitions and scheduling delays while catching a report for every native network chunk sent to the engine.
				if limit := 10 + int(time.Since(started)/(100*time.Millisecond)); reports > limit {
					t.Errorf("%d progress reports exceeded the %d report budget", reports, limit)
				}
				if !pausing || !resumed || !ranged.Load() {
					t.Fatalf("pause %v, resume %v, range %v", pausing, resumed, ranged.Load())
				}
				actual, err := os.ReadFile(destination)
				if err != nil || !bytes.Equal(actual, payload) {
					t.Fatalf("published %d/%d bytes: %v", len(actual), len(payload), err)
				}
				onMain(func() { closeHost(); open(); host.command(driver.DownloadCommand{Op: "list"}) })
				for {
					select {
					case replay := <-updates:
						if replay.State == "completed" {
							return
						}
					case <-timeout.C:
						t.Fatal("no journaled completion replay")
					}
				}
			}
		case <-timeout.C:
			t.Fatal("the native download did not finish")
		}
	}
}

func TestDownloadCertificatePinKeepsHostnameAndValidityChecks(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	certificate := server.Certificate()
	pin := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}))
	for _, test := range []struct {
		name, pin, host string
		now             time.Time
		want            bool
	}{
		{"exact pin", pin, "127.0.0.1", time.Now(), true},
		{"wrong host", pin, "wrong.example", time.Now(), false},
		{"expired", pin, "127.0.0.1", certificate.NotAfter.Add(time.Second), false},
		{"not valid yet", pin, "127.0.0.1", certificate.NotBefore.Add(-time.Second), false},
		{"different pin", "invalid", "127.0.0.1", time.Now(), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := downloadCertificateMatches(test.pin, certificate.Raw, test.host, test.now); got != test.want {
				t.Fatalf("pin match = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDownloadJournalPreservesPausedRequests(t *testing.T) {
	host := &downloadHost{directory: t.TempDir(), tasks: make(map[string]*downloadTask)}
	task := &downloadTask{DownloadProgress: driver.DownloadProgress{ID: "paused", Path: filepath.Join(t.TempDir(), "result"), State: "paused", Loaded: 123, Total: 456}, Request: driver.DownloadCommand{URL: "https://example.test/file", Headers: map[string]string{"Authorization": "Bearer sample"}}, ResumeData: []byte("opaque resume data")}
	if err := host.save(task); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(host.record(task.ID))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("journal permissions %v: %v", info, err)
	}
	restored := &downloadHost{directory: host.directory, tasks: make(map[string]*downloadTask)}
	if err := restored.restore(); err != nil {
		t.Fatal(err)
	}
	actual := restored.tasks[task.ID]
	before, _ := json.Marshal(task)
	after, _ := json.Marshal(actual)
	if !bytes.Equal(before, after) {
		t.Fatalf("restored task differs\n%s\n%s", before, after)
	}
}

func TestNativeDownloadRejectsUntrustedServers(t *testing.T) {
	release := holdMainLoop()
	defer release()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("protected file")) }))
	defer server.Close()
	pin := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	for _, test := range []struct{ name, url, pin string }{
		{"system trust", server.URL, ""},
		{"wrong hostname", strings.Replace(server.URL, "127.0.0.1", "localhost", 1), pin},
		{"wrong pin", server.URL, "invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			destination := filepath.Join(root, "destination")
			previous := []byte("previous file")
			if err := os.WriteFile(destination, previous, 0o600); err != nil {
				t.Fatal(err)
			}
			updates := make(chan driver.DownloadProgress, 32)
			stop := driver.HearDownloads(func(update driver.DownloadProgress) {
				if update.ID == "deniedTest" {
					updates <- update
				}
			})
			defer stop()
			host := &downloadHost{directory: filepath.Join(root, "journal"), identifier: "dev.nsgo.test.denied"}
			onMain(func() {
				if err := host.open(); err != nil {
					t.Error(err)
					return
				}
				host.command(driver.DownloadCommand{Op: "start", ID: "deniedTest", Path: destination, URL: test.url, CertificatePEM: test.pin})
			})
			defer func() {
				onMain(func() {
					host.session.Send(sel("invalidateAndCancel"))
					host.session.Send(sel("release"))
					delete(downloadDelegates, host.delegate)
					host.delegate.Send(sel("release"))
				})
			}()
			timeout := time.NewTimer(10 * time.Second)
			defer timeout.Stop()
			for {
				select {
				case update := <-updates:
					if update.State == "completed" {
						t.Fatal("the untrusted server was accepted")
					}
					if update.State == "failed" {
						actual, err := os.ReadFile(destination)
						if err != nil || !bytes.Equal(actual, previous) {
							t.Fatal("failed trust changed the destination", err)
						}
						return
					}
				case <-timeout.C:
					t.Fatal("no trust failure was reported")
				}
			}
		})
	}
}

func TestDownloadBackgroundCompletionWaitsForPublication(t *testing.T) {
	var calls atomic.Int32
	done := objcblock.New(func(_ objc.Block) { calls.Add(1) })
	defer done.Release()
	host := &downloadHost{finishedEvents: true, publishing: 1}
	onMain(func() {
		host.completion = objc.ID(done).Send(sel("copy"))
		host.finishEvents()
	})
	if calls.Load() != 0 {
		t.Fatal("UIKit was finished before the file publication")
	}
	onMain(func() { host.publishing = 0; host.finishEvents(); host.finishEvents() })
	if calls.Load() != 1 {
		t.Fatalf("completion called %d times", calls.Load())
	}
}

func TestDownloadBackgroundCompletionWaitsForItsOwnEventBatch(t *testing.T) {
	var calls atomic.Int32
	done := objcblock.New(func(_ objc.Block) { calls.Add(1) })
	defer done.Release()
	onMain(func() {
		previous := configuredDownloads
		defer func() { configuredDownloads = previous }()
		host := &downloadHost{identifier: "dev.nsgo.test.download.system", background: true, finishedEvents: true}
		// This test delivers the UIKit callback after an earlier batch drained. open only needs a live session marker, and performs no network work.
		host.session = objc.ID(Class("NSObject")).Send(sel("new"))
		defer host.session.Send(sel("release"))
		configuredDownloads = &downloadManager{identifier: "dev.nsgo.test.download", background: true, hosts: map[string]*downloadHost{"system": host}}
		if !HandleDownloadEvents(host.identifier, objc.ID(done)) {
			t.Error("the background session was not recognized")
		}
		host.finishEvents()
		if calls.Load() != 0 {
			t.Error("a stale event batch finished the new UIKit callback")
		}
		host.finishedEvents = true
		host.finishEvents()
	})
	if calls.Load() != 1 {
		t.Fatalf("completion called %d times", calls.Load())
	}
}

func TestDownloadRestoresMovedContainerPaths(t *testing.T) {
	root := t.TempDir()
	previousHome := filepath.Join(root, "previous-home")
	currentHome := filepath.Join(root, "current-home")
	journal := filepath.Join("Library", "Application Support", "app", "nsgo-downloads", "system")
	directory := filepath.Join(previousHome, journal)
	staged := filepath.Join(previousHome, "Library", "Caches", ".nsgo-download-moved-holding", "payload")
	destination := filepath.Join(previousHome, "Documents", "result")
	for _, path := range []string{directory, filepath.Dir(staged), filepath.Dir(destination)} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	payload := []byte("completed before app update")
	if err := os.WriteFile(staged, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	host := &downloadHost{directory: directory}
	task := &downloadTask{
		DownloadProgress: driver.DownloadProgress{ID: "moved", Path: destination, State: "publishing", Loaded: int64(len(payload)), Total: int64(len(payload))},
		Request:          driver.DownloadCommand{Path: destination}, Home: previousHome, Staged: staged,
	}
	if err := host.save(task); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "external-destination")
	if err := host.save(&downloadTask{DownloadProgress: driver.DownloadProgress{ID: "outside", Path: outside, State: "paused"}, Home: previousHome}); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(previousHome, currentHome); err != nil {
		t.Fatal(err)
	}
	restored := &downloadHost{directory: filepath.Join(currentHome, journal), home: currentHome, tasks: make(map[string]*downloadTask)}
	if err := restored.restore(); err != nil {
		t.Fatal(err)
	}
	actual := restored.tasks[task.ID]
	expected := filepath.Join(currentHome, "Documents", "result")
	if actual.Path != expected || actual.Request.Path != expected || actual.Home != currentHome {
		t.Fatalf("destination after container move = %q, request = %q, home = %q", actual.Path, actual.Request.Path, actual.Home)
	}
	if err := publishDownload(actual.Staged, actual.Path, actual.ID); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(expected)
	if err != nil || !bytes.Equal(contents, payload) {
		t.Fatalf("recovered file %q: %v", contents, err)
	}
	if restored.tasks["outside"].Path != outside {
		t.Fatal("a destination outside the app's home moved")
	}
}

func TestDownloadRestoresLegacyContainerPaths(t *testing.T) {
	previousHome := "/private/var/mobile/Containers/Data/Application/00112233-4455-6677-8899-aabbccddeeff"
	currentHome := "/var/mobile/Containers/Data/Application/11223344-5566-7788-99aa-bbccddeeff00"
	host := &downloadHost{directory: t.TempDir()}
	destination := previousHome + "/Documents/result"
	task := &downloadTask{DownloadProgress: driver.DownloadProgress{ID: "legacy", Path: destination, State: "paused"}, Request: driver.DownloadCommand{Path: destination}}
	if err := host.save(task); err != nil {
		t.Fatal(err)
	}
	restored := &downloadHost{directory: host.directory, home: currentHome, tasks: make(map[string]*downloadTask)}
	if err := restored.restore(); err != nil {
		t.Fatal(err)
	}
	if got := restored.tasks["legacy"].Path; got != currentHome+"/Documents/result" {
		t.Fatalf("legacy destination = %q", got)
	}
}
