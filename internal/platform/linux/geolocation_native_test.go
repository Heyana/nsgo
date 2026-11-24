//go:build linux && !android

package linux

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// startGeoClueFixture runs testdata/geoclue_fixture.py on a private bus of its own and points the provider at it; it never reaches the host's GeoClue. It returns the fixture's call log and the file whose presence makes the fixture refuse clients.
func startGeoClueFixture(t *testing.T) (logPath, denyPath string) {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("needs dbus-daemon")
	}
	if err := exec.Command("python3", "-c", "import dbus, dbus.service, dbus.mainloop.glib; from gi.repository import GLib").Run(); err != nil {
		t.Skip("needs python3 with dbus-python and PyGObject")
	}
	bus := exec.Command(daemon, "--session", "--nofork", "--print-address=1")
	out, err := bus.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := bus.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bus.Process.Kill(); _ = bus.Wait() })
	address, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	address = strings.TrimSpace(address)

	dir := t.TempDir()
	logPath, denyPath = filepath.Join(dir, "calls.log"), filepath.Join(dir, "deny")
	fixture := exec.Command("python3", "testdata/geoclue_fixture.py")
	fixture.Env = append(os.Environ(), "DBUS_SESSION_BUS_ADDRESS="+address, "GEOCLUE_FIXTURE_LOG="+logPath, "GEOCLUE_FIXTURE_DENY="+denyPath)
	fixture.Stderr = os.Stderr
	ready, err := fixture.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.Process.Kill(); _ = fixture.Wait() })
	if line, err := bufio.NewReader(ready).ReadString('\n'); err != nil || strings.TrimSpace(line) != "ready" {
		t.Fatalf("GeoClue fixture did not start: %q, %v", line, err)
	}
	// geoClueBus connects once per process, so every GeoClue test shares this fixture.
	t.Setenv("NSGO_GEOCLUE_BUS_ADDRESS", address)
	return logPath, denyPath
}

func waitForGeoClueLog(t *testing.T, logPath, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, _ := os.ReadFile(logPath)
		if strings.Contains(string(data), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("GeoClue fixture log lacks %q:\n%s", want, data)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestGeoClueLocation has an app read and request location permission, take a one-shot fix and watch fixes from the private GeoClue fixture, then stop the stream; and has a refusal reach the app as denied and not allowed. Every client the provider starts is stopped.
func TestGeoClueLocation(t *testing.T) {
	logPath, denyPath := startGeoClueFixture(t)
	scopes := make(chan *core.Scope, 1)
	startTestWindow(t, func(cx *core.Scope) core.Node {
		select {
		case scopes <- cx:
		default:
		}
		return core.View{Style: core.Style{Width: core.Pt(40), Height: core.Pt(40)}}
	}, Options{ID: "dev.nsgo.geoclue-test", Title: "geoclue", Width: 60, Height: 60})
	var cx *core.Scope
	select {
	case cx = <-scopes:
	case <-time.After(10 * time.Second):
		t.Fatal("the app did not render")
	}
	run := func(fn func()) { cx.Post(fn) }

	status := func(ask bool) core.PermissionStatus {
		t.Helper()
		got := make(chan core.PermissionStatus, 1)
		run(func() {
			if ask {
				cx.RequestPermission(core.PermissionLocation, func(s core.PermissionStatus) { got <- s })
			} else {
				cx.PermissionStatus(core.PermissionLocation, func(s core.PermissionStatus) { got <- s })
			}
		})
		select {
		case s := <-got:
			return s
		case <-time.After(10 * time.Second):
			t.Fatal("no permission answer")
		}
		return 0
	}
	if s := status(false); s != core.PermissionNotAsked {
		t.Fatalf("status before a request = %v, want not asked", s)
	}
	if s := status(true); s != core.PermissionGranted {
		t.Fatalf("request = %v, want granted", s)
	}
	waitForGeoClueLog(t, logPath, "start /org/freedesktop/GeoClue2/Client/1 dev.nsgo.geoclue-test 8")
	waitForGeoClueLog(t, logPath, "stop /org/freedesktop/GeoClue2/Client/1")
	if s := status(false); s != core.PermissionGranted {
		t.Fatalf("status after a grant = %v, want granted", s)
	}

	type result struct {
		fix core.LocationFix
		err error
	}
	located := make(chan result, 1)
	run(func() {
		if _, err := cx.Locate(core.LocationOptions{Accuracy: 500}, func(f core.LocationFix, err error) { located <- result{f, err} }); err != nil {
			located <- result{err: err}
		}
	})
	select {
	case r := <-located:
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.fix.Longitude != 139.7671 || r.fix.Latitude < 35.68 || !r.fix.HorizontalAccuracy.Valid || r.fix.HorizontalAccuracy.Value != 8 || r.fix.Speed.Valid || !r.fix.Course.Valid || r.fix.Course.Value != 90 || time.Since(r.fix.Timestamp) > time.Minute {
			t.Errorf("fix = %+v", r.fix)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Locate did not answer")
	}
	waitForGeoClueLog(t, logPath, "start /org/freedesktop/GeoClue2/Client/2 dev.nsgo.geoclue-test 6")
	waitForGeoClueLog(t, logPath, "stop /org/freedesktop/GeoClue2/Client/2")

	fixes := make(chan core.LocationFix, 16)
	failures := make(chan error, 1)
	streams := make(chan *core.SensorStream, 1)
	run(func() {
		s, err := cx.WatchLocation(core.LocationOptions{}, func(f core.LocationFix) {
			select {
			case fixes <- f:
			default:
			}
		}, func(err error) { failures <- err })
		if err != nil {
			failures <- err
		}
		streams <- s
	})
	stream := <-streams
	var lats []float64
	for len(lats) < 2 {
		select {
		case f := <-fixes:
			if len(lats) == 0 || f.Latitude != lats[len(lats)-1] {
				lats = append(lats, f.Latitude)
			}
		case err := <-failures:
			t.Fatal(err)
		case <-time.After(10 * time.Second):
			t.Fatalf("WatchLocation delivered %v", lats)
		}
	}
	if lats[1] <= lats[0] {
		t.Errorf("watched latitudes = %v, want the fixture's later fix", lats)
	}
	run(stream.Stop)
	waitForGeoClueLog(t, logPath, "stop /org/freedesktop/GeoClue2/Client/3")

	if err := os.WriteFile(denyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if s := status(true); s != core.PermissionDenied {
		t.Fatalf("refused request = %v, want denied", s)
	}
	if s := status(false); s != core.PermissionDenied {
		t.Fatalf("status after a refusal = %v, want denied", s)
	}
	run(func() {
		if _, err := cx.Locate(core.LocationOptions{}, func(_ core.LocationFix, err error) { located <- result{err: err} }); err != nil {
			located <- result{err: err}
		}
	})
	select {
	case r := <-located:
		if r.err != core.SensorNotAllowed {
			t.Fatalf("refused Locate = %v, want not allowed", r.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("refused Locate did not answer")
	}
	run(func() {
		if _, err := cx.WatchLocation(core.LocationOptions{Background: true}, func(core.LocationFix) {}, func(err error) { failures <- err }); err != nil {
			failures <- err
		}
	})
	select {
	case err := <-failures:
		if err != core.SensorNotAvailable {
			t.Fatalf("background stream = %v, want unavailable", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("background stream did not fail")
	}
}
