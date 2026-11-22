package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestIOSDocumentTypesEndToEnd packages and installs a receiver, then lets a
// separate native test app send documents through UIKit and Launch Services.
// UIKit creates the real scene contexts; the sender uses a test-only native
// default-recipient action because Open In's picker lives out of process.
//
//	NSGO_SIMULATOR_E2E=1 go tool nsgo test -run '^TestIOSDocumentTypesEndToEnd$' ./cmd/nsgo
func TestIOSDocumentTypesEndToEnd(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" || os.Getenv("NSGO_SIMULATOR_E2E") != "1" {
		t.Skip("set NSGO_SIMULATOR_E2E=1 on an Apple silicon Mac to test native document delivery")
	}
	ctx := t.Context()
	p, err := load(filepath.Join("..", "..", "testdata", "iosdocuments"))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := buildIOSApp(ctx, p, iOS, t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	udid, err := bootSimulator(ctx, iOS, os.Getenv("NSGO_SIMULATOR_DEVICE"))
	if err != nil {
		t.Fatal(err)
	}
	// This fixed ID belongs only to the opt-in fixture. A reinstall keeps
	// its Inbox, where UIKit otherwise renames a repeated document.
	exec.Command("xcrun", "simctl", "terminate", udid, p.ID).Run()
	exec.Command("xcrun", "simctl", "uninstall", udid, p.ID).Run()
	if err := run(ctx, "xcrun", "simctl", "install", udid, bundle); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		exec.Command("xcrun", "simctl", "terminate", udid, p.ID).Run()
		exec.Command("xcrun", "simctl", "uninstall", udid, p.ID).Run()
	})
	container, err := exec.CommandContext(ctx, "xcrun", "simctl", "get_app_container", udid, p.ID, "data").Output()
	if err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(strings.TrimSpace(string(container)), "Library", "Application Support", p.ID, "opened.jsonl")
	type event struct {
		Kind, Path, Body, SHA256 string
		PID                      int
	}
	readEvents := func(want int) []event {
		t.Helper()
		for deadline := time.Now().Add(15 * time.Second); ; time.Sleep(50 * time.Millisecond) {
			data, err := os.ReadFile(report)
			if err == nil {
				var events []event
				for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
					var e event
					if err := json.Unmarshal([]byte(line), &e); err != nil {
						continue // The app may still be writing its final line.
					}
					events = append(events, e)
				}
				if len(events) >= want {
					return events
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("the receiver did not record %d events: %s (%v)", want, data, err)
			}
		}
	}
	send := func(which string) {
		t.Helper()
		sender, err := load(filepath.Join("..", "..", "internal", "platform", "ios"))
		if err != nil {
			t.Fatal(err)
		}
		if err := runTests(ctx, sender, testOptions{
			target: "ios_simulator", device: udid, timeout: time.Minute,
			run: "^TestSystemDocumentOpenInRoutesToRegisteredApp$",
			env: []string{"NSGO_IOS_DOCUMENT_CASE=" + which},
		}); err != nil {
			t.Fatal(err)
		}
	}
	pid := 0
	for i, which := range []string{"cold", "warm"} {
		send(which)
		events := readEvents(i + 2)
		if i == 0 {
			if events[0].Kind != "started" || events[0].PID == 0 {
				t.Fatalf("the document did not cold-launch the receiver: %+v", events)
			}
			pid = events[0].PID
		}
		e := events[i+1]
		body := "NSGo system document " + which + "\n"
		sum := sha256.Sum256([]byte(body))
		if len(events) != i+2 || e.Kind != "file" || e.PID != pid || filepath.Base(e.Path) != which+" #日本.nsgo1385doc" || e.Body != body || e.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("%s document handoff: %+v", which, events)
		}
		if data, err := os.ReadFile(e.Path); err != nil || string(data) != body {
			t.Fatalf("the imported copy did not survive its sender: %q, %v", data, err)
		}
	}
	const link = "nsgo1385documents://separate-link"
	// simctl openurl presents a SpringBoard confirmation on recent iOS.
	// An actual sender app uses UIApplication's public openURL instead.
	send("link")
	events := readEvents(4)
	if len(events) != 4 || events[3].Kind != "link" || events[3].Path != link || events[3].PID != pid {
		t.Fatalf("the custom link was mixed with document opens: %+v", events)
	}
}
