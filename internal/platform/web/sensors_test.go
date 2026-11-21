//go:build js && wasm && nsgotest

package web

import (
	"errors"
	"math"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestWebMotionDeliversAndDetaches(t *testing.T) {
	g := js.Global()
	nav := g.Get("navigator")
	motionTestProperty(t, nav, "userAgent", "Chrome (Android)")
	motionTestProperty(t, g.Get("DeviceMotionEvent"), "requestPermission", js.Undefined())
	permissions := g.Get("Object").New()
	query := js.FuncOf(func(js.Value, []js.Value) any {
		return g.Get("Promise").Call("resolve", map[string]any{"state": "granted"})
	})
	defer query.Release()
	permissions.Set("query", query)
	motionTestProperty(t, nav, "permissions", permissions)
	var simulated listeners
	for _, name := range []string{"devicemotion", "deviceorientation"} {
		simulated.on(g, name, func(event js.Value) {
			if event.Get("isTrusted").Bool() {
				event.Call("stopImmediatePropagation")
			}
		})
	}
	defer simulated.release()
	samples := make(chan core.MotionSample, 3)
	failures := make(chan error, 1)
	type started struct {
		scope  *core.Scope
		stream *core.SensorStream
	}
	streams := make(chan started, 1)
	s := open(func(cx *core.Scope) core.Node {
		stream, err := cx.WatchMotion(core.MotionOptions{Sensors: core.SensorAccelerometer | core.SensorGyroscope | core.SensorDeviceMotion},
			func(sample core.MotionSample) { samples <- sample }, func(err error) { failures <- err })
		if err != nil {
			failures <- err
		}
		streams <- started{cx, stream}
		return core.View{}
	}, Options{}, nil)
	defer s.engine.Close()
	active := motionTestWait(t, streams)
	if active.stream == nil {
		t.Fatal(motionTestWait(t, failures))
	}
	for deadline := time.Now().Add(3 * time.Second); len(s.motion.orientation) == 0; {
		select {
		case err := <-failures:
			t.Fatal(err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("motion and orientation listeners were never installed")
		}
		time.Sleep(time.Millisecond)
	}
	orientation := g.Get("DeviceOrientationEvent").New("deviceorientation", map[string]any{
		"alpha": 90, "beta": 0, "gamma": 0, "absolute": true,
	})
	g.Call("dispatchEvent", orientation)
	motion := g.Get("DeviceMotionEvent").New("devicemotion", map[string]any{
		"acceleration":                 map[string]any{"x": 1, "y": 2, "z": 0},
		"accelerationIncludingGravity": map[string]any{"x": 1, "y": 2, "z": 9.81},
		"rotationRate":                 map[string]any{"alpha": 180, "beta": 90, "gamma": -180},
		"interval":                     20,
	})
	g.Call("dispatchEvent", motion)
	got := make(map[core.MotionSensors]core.MotionSample)
	for range 3 {
		sample := motionTestWait(t, samples)
		got[sample.Sensor] = sample
	}
	if v := got[core.SensorAccelerometer].Vector; v != (core.SensorVector{X: 1, Y: 2, Z: 9.81}) {
		t.Errorf("acceleration = %+v", v)
	}
	if v := got[core.SensorGyroscope].Vector; v != (core.SensorVector{X: math.Pi, Y: math.Pi / 2, Z: -math.Pi}) {
		t.Errorf("rotation = %+v", v)
	}
	fused := got[core.SensorDeviceMotion]
	if fused.Gravity != (core.SensorVector{Z: 9.81}) || fused.UserAcceleration != (core.SensorVector{X: 1, Y: 2}) ||
		!fused.HeadingReferenced || math.Abs(fused.Attitude.Z-math.Sqrt(0.5)) > 1e-12 ||
		math.Abs(fused.Attitude.W-math.Sqrt(0.5)) > 1e-12 || fused.Interval != 20*time.Millisecond || fused.Accuracy != -1 {
		t.Errorf("fused sample = %+v", fused)
	}
	active.scope.Post(active.stream.Stop)
	s.engine.Sync()
	if len(s.motion.motion) != 0 || len(s.motion.orientation) != 0 {
		t.Fatal("the last stopped stream left a listener")
	}
}

func TestWebMotionPermissionRequiresActivation(t *testing.T) {
	g := js.Global()
	nav := g.Get("navigator")
	motionTestProperty(t, nav, "userAgent", "iPhone; AppleWebKit/617")
	activation := g.Get("Object").New()
	activation.Set("isActive", false)
	motionTestProperty(t, nav, "userActivation", activation)
	calls := 0
	ask := js.FuncOf(func(js.Value, []js.Value) any {
		calls++
		return g.Get("Promise").Call("resolve", "denied")
	})
	defer ask.Release()
	motionTestProperty(t, g.Get("DeviceMotionEvent"), "requestPermission", ask)

	permissions := make(chan core.PermissionStatus, 2)
	failures := make(chan error, 1)
	scopes := make(chan *core.Scope, 1)
	s := open(func(cx *core.Scope) core.Node {
		cx.RequestPermission(core.PermissionMotion, func(status core.PermissionStatus) { permissions <- status })
		scopes <- cx
		return core.View{}
	}, Options{}, nil)
	defer s.engine.Close()
	cx := motionTestWait(t, scopes)
	if got := motionTestWait(t, permissions); got != core.PermissionNotAsked || calls != 0 {
		t.Fatalf("without activation: permission = %v, prompts = %d", got, calls)
	}
	activation.Set("isActive", true)
	cx.Post(func() {
		cx.RequestPermission(core.PermissionMotion, func(status core.PermissionStatus) {
			permissions <- status
			_, err := cx.WatchMotion(core.MotionOptions{}, nil, func(err error) { failures <- err })
			if err != nil {
				failures <- err
			}
		})
	})
	if got := motionTestWait(t, permissions); got != core.PermissionDenied || calls != 1 {
		t.Fatalf("with activation: permission = %v, prompts = %d", got, calls)
	}
	if err := motionTestWait(t, failures); !errors.Is(err, core.SensorNotAllowed) {
		t.Fatalf("denied stream = %v", err)
	}
	if len(s.motion.motion) != 0 {
		t.Fatal("a denied watcher installed a motion listener")
	}
}

func TestWebMotionIsUnavailableOnDesktop(t *testing.T) {
	permission := make(chan core.PermissionStatus, 1)
	failures := make(chan error, 1)
	s := open(func(cx *core.Scope) core.Node {
		cx.PermissionStatus(core.PermissionMotion, func(status core.PermissionStatus) { permission <- status })
		_, err := cx.WatchMotion(core.MotionOptions{}, nil, func(err error) { failures <- err })
		if err != nil {
			failures <- err
		}
		return core.View{}
	}, Options{}, nil)
	defer s.engine.Close()
	if status := motionTestWait(t, permission); status != core.PermissionUnavailable {
		t.Errorf("desktop permission = %v", status)
	}
	if err := motionTestWait(t, failures); !errors.Is(err, core.SensorNotAvailable) {
		t.Errorf("desktop motion = %v", err)
	}
}

func motionTestProperty(t *testing.T, target js.Value, name string, value any) {
	t.Helper()
	g := js.Global()
	object := g.Get("Object")
	previous := object.Call("getOwnPropertyDescriptor", target, name)
	object.Call("defineProperty", target, name, map[string]any{"value": value, "configurable": true})
	t.Cleanup(func() {
		if previous.IsUndefined() {
			g.Get("Reflect").Call("deleteProperty", target, name)
		} else {
			object.Call("defineProperty", target, name, previous)
		}
	})
}

func motionTestWait[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(4 * time.Second):
		t.Fatal("motion provider never answered")
		var zero T
		return zero
	}
}
