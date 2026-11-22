package sensortest

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// Run on an owned emulator with its pressure sensor explicitly injected: adb -s <owned-serial> emu sensor set pressure 1013.25 go tool nsgo test -env NSGO_SENSOR_FIXTURE=1 -target android -device <serial> ./internal/sensortest -- -run TestAndroidExtendedSensors -count=1 The fixture never asks for a location or uses real device coordinates.
func TestAndroidExtendedSensors(t *testing.T) {
	if nsgotest.Target() != "android" || os.Getenv("NSGO_SENSOR_FIXTURE") != "1" {
		t.Skip("requires owned Android emulator and explicit sensor fixture opt-in")
	}
	var samples atomic.Int32
	var bad atomic.Bool
	var cx *core.Scope
	app := nsgotest.Start(t, func(c *core.Scope) core.Node {
		cx = c
		return core.View{Children: []core.Node{core.Text{TestID: "status", Content: "sensor fixture"}}}
	})
	app.Post(func() {
		_, err := cx.WatchPressure(func(v core.PressureSample) {
			if v.Pascals < 100000 || v.Pascals > 103000 || v.RelativeAltitude.Valid || v.Timestamp <= 0 {
				bad.Store(true)
			}
			samples.Add(1)
		}, func(err error) { t.Errorf("pressure: %v", err) })
		if err != nil {
			t.Error(err)
		}
	})
	app.WaitUntil(5*time.Second, func() bool { return samples.Load() > 0 })
	if bad.Load() {
		t.Fatal("pressure units/absence/monotonic timestamp invalid")
	}
	var expiry atomic.Bool
	var motion atomic.Int32
	var firstMotion, lastMotion atomic.Int64
	app.Post(func() {
		_, err := cx.WatchBackgroundMotion(core.BackgroundMotionOptions{Duration: 3 * time.Second, NotificationTitle: "Owned sensor fixture"}, func(v core.MotionSample) {
			motion.Add(1)
			firstMotion.CompareAndSwap(0, int64(v.Timestamp))
			lastMotion.Store(int64(v.Timestamp))
		}, func(err error) {
			if !errors.Is(err, core.SensorTimedOut) {
				t.Errorf("background motion: %v", err)
			}
			expiry.Store(true)
		})
		if err != nil {
			t.Error(err)
		}
	})
	app.WaitUntil(5*time.Second, func() bool { return expiry.Load() })
	if motion.Load() == 0 || lastMotion.Load()-firstMotion.Load() < int64(2*time.Second) {
		t.Fatal("bounded stream did not deliver SensorManager samples across its execution window")
	}
	var child *core.Scope
	var stopped *core.SensorStream
	app.Post(func() {
		child = cx.Child()
		var err error
		stopped, err = child.WatchBackgroundMotion(core.BackgroundMotionOptions{Duration: time.Minute, NotificationTitle: "Owned disposal fixture"}, nil, func(err error) { t.Errorf("disposed provider called back: %v", err) })
		if err != nil {
			t.Error(err)
		}
	})
	app.Settle()
	app.Post(func() {
		child.Dispose()
		if !stopped.Stopped() {
			t.Error("disposed stream retained")
		}
	})
	app.Settle()
	t.Log(fmt.Sprintf("native pressure samples=%d bounded raw motion=%d", samples.Load(), motion.Load()))
}
