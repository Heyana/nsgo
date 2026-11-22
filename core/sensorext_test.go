package core

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestExtendedSensorsAreScopedAndRejectOldGenerations(t *testing.T) {
	for _, kind := range []driver.SensorKind{driver.SensorSteps, driver.SensorActivity, driver.SensorPressure, driver.SensorGeofences} {
		t.Run(string(rune('0'+kind)), func(t *testing.T) {
			h, cx := sensorHost(t)
			child := cx.Child()
			calls := 0
			gap := false
			var stream *SensorStream
			var err error
			var report driver.SensorReport
			switch kind {
			case driver.SensorSteps:
				stream, err = child.WatchSteps(func(v StepSample) { calls++; gap = v.Gap }, nil)
				report.Steps = &StepSample{Count: 12}
			case driver.SensorActivity:
				stream, err = child.WatchActivity(func(v ActivitySample) { calls++; gap = v.Gap }, nil)
				report.Activity = &ActivitySample{Kinds: ActivityWalking}
			case driver.SensorPressure:
				stream, err = child.WatchPressure(func(v PressureSample) { calls++; gap = v.Gap }, nil)
				report.Pressure = &PressureSample{Pascals: 101325}
			case driver.SensorGeofences:
				stream, err = child.WatchGeofences(GeofenceOptions{Regions: []GeofenceRegion{{ID: "owned", Radius: 100}}}, func(v GeofenceSample) { calls++; gap = v.Gap }, nil)
				report.Geofence = &GeofenceSample{Region: "owned", State: GeofenceInside}
			}
			if err != nil {
				t.Fatal(err)
			}
			first := command(h.commit(), driver.ItemSensor)
			sensorAnswer(h, first, report)
			if calls != 1 || gap {
				t.Fatal("first sample missing or marked as resumed")
			}
			h.tree.SetEnvironment(Environment{Lifecycle: LifecycleBackground})
			if it := command(h.commit(), driver.ItemSensor); it == nil || it.Sensor.Command != driver.SensorStop {
				t.Fatal("foreground provider kept running")
			}
			h.tree.SetEnvironment(Environment{Lifecycle: LifecycleActive})
			next := command(h.commit(), driver.ItemSensor)
			sensorAnswer(h, first, report)
			if calls != 1 {
				t.Fatal("sample from an old generation escaped")
			}
			sensorAnswer(h, next, report)
			if calls != 2 || !gap {
				t.Fatal("resumed sample missing its gap")
			}
			child.Dispose()
			sensorAnswer(h, next, report)
			if calls != 2 || !stream.Stopped() {
				t.Fatal("disposed scope kept a provider or callback")
			}
		})
	}
}

func TestGeofencesCopyRegionsAndValidateWithoutPublishing(t *testing.T) {
	h, cx := sensorHost(t)
	regions := []GeofenceRegion{{ID: "owned", Latitude: 51.5, Longitude: -0.12, Radius: 100}}
	_, err := cx.WatchGeofences(GeofenceOptions{Regions: regions}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	regions[0].ID = "changed"
	it := command(h.commit(), driver.ItemSensor)
	if it.Sensor.Geofences.Regions[0].ID != "owned" {
		t.Fatal("app mutation changed a published provider request")
	}
	for _, regions := range [][]GeofenceRegion{
		nil, {{ID: "", Radius: 1}}, {{ID: "x", Radius: 0}}, {{ID: "x", Radius: math.NaN()}},
		{{ID: "x", Radius: 1, Latitude: 91}}, {{ID: "x", Radius: 1, Longitude: 181}},
		{{ID: "x", Radius: 1}, {ID: "x", Radius: 2}}, make([]GeofenceRegion, 21),
	} {
		if _, err := cx.WatchGeofences(GeofenceOptions{Regions: regions}, nil, nil); err == nil {
			t.Fatal("invalid region accepted")
		}
	}
	if command(h.commit(), driver.ItemSensor) != nil {
		t.Fatal("invalid regions reached a provider")
	}
}

func TestBackgroundMotionIsExplicitFiniteAndStopsOnDisposal(t *testing.T) {
	h, cx := sensorHost(t)
	denied := cx.Child()
	denied.ProvideAccess(sensorAccess{AccessMotion: true})
	if _, err := denied.WatchBackgroundMotion(BackgroundMotionOptions{Duration: time.Second}, nil, nil); err == nil {
		t.Fatal("raw motion capability allowed background execution")
	}
	if command(h.commit(), driver.ItemSensor) != nil {
		t.Fatal("refused request reached the platform")
	}
	child := cx.Child()
	child.ProvideAccess(sensorAccess{AccessMotion: true, AccessBackgroundMotion: true})
	var got error
	stream, err := child.WatchBackgroundMotion(BackgroundMotionOptions{Duration: time.Second}, nil, func(err error) { got = err })
	if err != nil {
		t.Fatal(err)
	}
	it := command(h.commit(), driver.ItemSensor)
	if it.Sensor.Motion.Interval != 20*time.Millisecond || it.Sensor.Command != driver.SensorWatchBackgroundMotion {
		t.Fatal("background stream lost its rate or command")
	}
	h.tree.SetEnvironment(Environment{Lifecycle: LifecycleBackground})
	h.commit()
	if stream.paused {
		t.Fatal("explicit background stream paused before its bound")
	}
	h.clock.Advance(time.Second)
	h.drain()
	if !stream.Stopped() || !errors.Is(got, SensorTimedOut) {
		t.Fatalf("duration did not end stream: stopped=%v err=%v", stream.Stopped(), got)
	}
	h.tree.SetEnvironment(Environment{Lifecycle: LifecycleActive})
	h.commit()
	next, _ := child.WatchBackgroundMotion(BackgroundMotionOptions{Duration: time.Hour}, nil, nil)
	h.commit()
	child.Dispose()
	h.commit()
	if !next.Stopped() {
		t.Fatal("scope disposal did not release background provider")
	}
}

func TestPreciseLocationAnswerIsOneShotAndScopeOwned(t *testing.T) {
	h, cx := sensorHost(t)
	calls := 0
	s, err := cx.RequestPreciseLocation("owned-purpose", func(full bool, err error) {
		calls++
		if !full || err != nil {
			t.Error("accuracy grant lost")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	it := command(h.commit(), driver.ItemSensor)
	if it.Sensor.Purpose != "owned-purpose" || it.Sensor.Command != driver.SensorRequestPreciseLocation {
		t.Fatal("wrong accuracy request")
	}
	r := driver.SensorReport{Precise: &driver.PreciseLocationAnswer{Full: true}, End: true}
	sensorAnswer(h, it, r)
	sensorAnswer(h, it, r)
	if calls != 1 || !s.Stopped() {
		t.Fatal("accuracy completion was not one-shot")
	}
	child := cx.Child()
	child.RequestPreciseLocation("owned-purpose", func(bool, error) { calls++ })
	it = command(h.commit(), driver.ItemSensor)
	child.Dispose()
	sensorAnswer(h, it, r)
	if calls != 1 {
		t.Fatal("disposed scope heard an accuracy answer")
	}
	if _, err := cx.RequestPreciseLocation("", nil); err == nil {
		t.Fatal("empty purpose accepted")
	}
}

func TestSensorServicesBelongToTheirExplicitScopeInsideHandlers(t *testing.T) {
	h, cx := sensorHost(t)
	owner, handler := cx.Child(), cx.Child()
	var owned, parent *SensorStream
	h.tree.within(handler, func() {
		var err error
		owned, err = owner.WatchPressure(nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		parent, err = cx.WatchBackgroundMotion(BackgroundMotionOptions{Duration: time.Hour}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	})
	h.commit()
	handler.Dispose()
	h.commit()
	if owned.Stopped() || parent.Stopped() {
		t.Fatal("handler disposal ended another scope's sensors")
	}
	owner.Dispose()
	h.commit()
	if !owned.Stopped() || parent.Stopped() {
		t.Fatal("explicit owner did not stop only its own provider")
	}
	cx.Dispose()
	if !parent.Stopped() {
		t.Fatal("parent disposal retained background motion")
	}
}
