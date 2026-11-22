//go:build linux && !android

package linux

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInotifyTimeZoneWatcherDetectsAtomicSymlinkReplaceAndStopsCleanly(t *testing.T) {
	root := t.TempDir()
	zoneinfo := filepath.Join(root, "usr", "share", "zoneinfo")
	tokyo := filepath.Join(zoneinfo, "Asia", "Tokyo")
	berlin := filepath.Join(zoneinfo, "Europe", "Berlin")
	for _, p := range []string{tokyo, berlin} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(tokyo, makeTestTZif("JST", 9*3600), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(berlin, makeTestTZif("CET", 3600), 0o644); err != nil {
		t.Fatal(err)
	}

	etcDir := filepath.Join(root, "etc")
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	localtime := filepath.Join(etcDir, "localtime")
	if err := os.Symlink(tokyo, localtime); err != nil {
		t.Fatal(err)
	}

	updated := make(chan *time.Location, 4)
	tracker := &linuxZoneTracker{
		localtimePath: localtime,
		timezonePath:  filepath.Join(etcDir, "timezone"),
		lookupEnv:     func(string) (string, bool) { return "", false },
		loadLocation:  time.LoadLocation,
		apply:         func(loc *time.Location) { updated <- loc },
		nowYear:       func() int { return 2026 },
	}
	tracker.seed()

	stop, err := startInotifyTimeZoneWatcher(localtime, []string{"localtime", "timezone"}, func() {
		tracker.refresh()
	})
	if err != nil {
		t.Fatalf("startInotifyTimeZoneWatcher: %v", err)
	}
	t.Cleanup(stop)

	tmpLink := filepath.Join(etcDir, ".#localtime9999")
	if err := os.Symlink(berlin, tmpLink); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmpLink, localtime); err != nil {
		t.Fatal(err)
	}

	select {
	case loc := <-updated:
		if loc.String() != "Europe/Berlin" {
			t.Fatalf("updated location = %q, want Europe/Berlin", loc)
		}
		if _, off := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC).In(loc).Zone(); off != 3600 {
			t.Fatalf("offset = %d, want 3600", off)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for inotify time zone update")
	}

	// Stopping twice must be safe and return promptly without hanging.
	stopped := make(chan struct{})
	go func() {
		stop()
		stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inotify watcher to stop")
	}
}
