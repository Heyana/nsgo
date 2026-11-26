package phonesensors_test

import (
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// expected reads NSGO_TEST_LOCATION, the "latitude,longitude" the runner injected into the simulator or emulator; the fixture never reads a user's live location.
func expected(t *testing.T) (lat, lon float64) {
	value := os.Getenv("NSGO_TEST_LOCATION")
	if value == "" {
		t.Skip("set NSGO_TEST_LOCATION to the injected latitude,longitude; see README.md")
	}
	parts := strings.Split(value, ",")
	if len(parts) != 2 {
		t.Fatalf("NSGO_TEST_LOCATION %q is not latitude,longitude", value)
	}
	lat, err1 := strconv.ParseFloat(parts[0], 64)
	lon, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil {
		t.Fatalf("NSGO_TEST_LOCATION %q is not latitude,longitude", value)
	}
	return lat, lon
}

// result is what an engine callback reports to the test goroutine.
type result[T any] struct {
	mu   sync.Mutex
	val  *T
	err  error
	done bool
}

func (r *result[T]) set(v *T, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.done {
		r.val, r.err, r.done = v, err, true
	}
}

func (r *result[T]) get() (*T, error, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.val, r.err, r.done
}

func (r *result[T]) ready() bool { _, _, done := r.get(); return done }

func phone(t *testing.T) {
	switch nsgotest.Target() {
	case "ios_simulator", "ios", "android":
	default:
		t.Skip("native sensor acceptance requires -target ios_simulator, ios or android")
	}
}

func status(t *testing.T, app *nsgotest.App, scope *core.Scope, p core.Permission) core.PermissionStatus {
	var got result[core.PermissionStatus]
	app.Post(func() { scope.PermissionStatus(p, func(s core.PermissionStatus) { got.set(&s, nil) }) })
	app.WaitUntil(10*time.Second, got.ready)
	s, _, _ := got.get()
	return *s
}

func near(f core.LocationFix, lat, lon float64) bool {
	return math.Abs(f.Latitude-lat) < 0.001 && math.Abs(f.Longitude-lon) < 0.001
}

// TestGrantedLocation reads a granted permission without a prompt, takes a fresh fix and a stream of the injected location, and stops the stream.
func TestGrantedLocation(t *testing.T) {
	phone(t)
	lat, lon := expected(t)
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{Style: core.Style{Width: core.Pt(100), Height: core.Pt(100)}}
	})
	switch got := status(t, app, scope, core.PermissionLocation); got {
	case core.PermissionGranted:
	case core.PermissionNotAsked:
		t.Log("asking for location: allow it in the system's prompt, or grant it as README.md says")
		var asked result[core.PermissionStatus]
		app.Post(func() {
			scope.RequestPermission(core.PermissionLocation, func(s core.PermissionStatus) { asked.set(&s, nil) })
		})
		app.WaitUntil(90*time.Second, asked.ready)
		if s, _, _ := asked.get(); *s != core.PermissionGranted {
			t.Fatalf("the location request answered %s", *s)
		}
	default:
		t.Skipf("location is %s; reset it as README.md says", got)
	}

	var located result[core.LocationFix]
	app.Post(func() {
		if _, err := scope.Locate(core.LocationOptions{Accuracy: 10}, func(f core.LocationFix, err error) { located.set(&f, err) }); err != nil {
			located.set(nil, err)
		}
	})
	app.WaitUntil(40*time.Second, located.ready)
	fix, fixErr, _ := located.get()
	if fixErr != nil {
		t.Fatalf("Locate: %v", fixErr)
	}
	if !near(*fix, lat, lon) || fix.Cached || !fix.HorizontalAccuracy.Valid || fix.Timestamp.IsZero() {
		t.Fatalf("Locate gave %+v, want a fresh fix at %v,%v with its accuracy", *fix, lat, lon)
	}
	t.Logf("fix %.5f,%.5f ±%.0f m approximate=%v", fix.Latitude, fix.Longitude, fix.HorizontalAccuracy.Value, fix.Approximate)

	var stream *core.SensorStream
	var watched result[core.LocationFix]
	app.Post(func() {
		var err error
		stream, err = scope.WatchLocation(core.LocationOptions{Accuracy: 10}, func(f core.LocationFix) {
			if near(f, lat, lon) {
				watched.set(&f, nil)
			}
		}, func(err error) { watched.set(nil, err) })
		if err != nil {
			watched.set(nil, err)
		}
	})
	app.WaitUntil(40*time.Second, watched.ready)
	if _, err, _ := watched.get(); err != nil {
		t.Fatalf("WatchLocation: %v", err)
	}
	var stopped result[bool]
	app.Post(func() { stream.Stop(); ok := stream.Stopped(); stopped.set(&ok, nil) })
	app.WaitUntil(10*time.Second, stopped.ready)
	if ok, _, _ := stopped.get(); !*ok {
		t.Fatal("the stream did not stop")
	}
}

// TestDeniedLocation reads a denied permission without a prompt and fails a fix as not allowed.
func TestDeniedLocation(t *testing.T) {
	phone(t)
	if os.Getenv("NSGO_TEST_LOCATION_DENIED") == "" {
		t.Skip("set NSGO_TEST_LOCATION_DENIED=1 after revoking location; see README.md")
	}
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{Style: core.Style{Width: core.Pt(100), Height: core.Pt(100)}}
	})
	if got := status(t, app, scope, core.PermissionLocation); got != core.PermissionDenied && got != core.PermissionNotAsked {
		t.Fatalf("revoked location read %s", got)
	}
	var located result[core.LocationFix]
	app.Post(func() {
		if _, err := scope.Locate(core.LocationOptions{}, func(f core.LocationFix, err error) { located.set(&f, err) }); err != nil {
			located.set(nil, err)
		}
	})
	app.WaitUntil(10*time.Second, located.ready)
	_, fixErr, _ := located.get()
	if !errors.Is(fixErr, core.SensorNotAllowed) {
		t.Fatalf("a fix without the permission failed with %v, want %v", fixErr, core.SensorNotAllowed)
	}
}

// TestMotion samples the accelerometer, or on a simulator, which has none, fails it as unavailable.
func TestMotion(t *testing.T) {
	phone(t)
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{Style: core.Style{Width: core.Pt(100), Height: core.Pt(100)}}
	})
	var stream *core.SensorStream
	var moved result[core.MotionSample]
	app.Post(func() {
		var err error
		stream, err = scope.WatchMotion(core.MotionOptions{Interval: 50 * time.Millisecond}, func(m core.MotionSample) { moved.set(&m, nil) }, func(err error) { moved.set(nil, err) })
		if err != nil {
			moved.set(nil, err)
		}
	})
	app.WaitUntil(10*time.Second, moved.ready)
	sample, motionErr, _ := moved.get()
	if nsgotest.Target() == "ios_simulator" {
		if !errors.Is(motionErr, core.SensorNotAvailable) {
			t.Fatalf("the simulator's accelerometer gave %v, %v; want %v", sample, motionErr, core.SensorNotAvailable)
		}
		return
	}
	if motionErr != nil {
		t.Fatalf("WatchMotion: %v", motionErr)
	}
	t.Logf("accelerometer %+v", sample.Vector)
	app.Post(func() { stream.Stop() })
	app.Settle()
}
