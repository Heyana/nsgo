package driver

import (
	"sync"
	"testing"
)

func TestSensorPacketKeepsNewestWithDropCount(t *testing.T) {
	var p SensorPacket
	for i := 0; i < 10000; i++ {
		m := &MotionSample{Vector: SensorVector{Z: float64(i)}, Dropped: 2}
		first := p.Offer(SensorReport{Generation: 7, Motion: m})
		if first != (i == 0) {
			t.Fatalf("sample %d woke engine %v", i, first)
		}
	}
	r, ok := p.Take()
	if !ok || r.Generation != 7 || r.Motion.Vector.Z != 9999 || r.Motion.Dropped != 29999 {
		t.Fatalf("report %+v", r.Motion)
	}
	if _, ok := p.Take(); ok {
		t.Fatal("sample delivered twice")
	}
	if !p.Offer(SensorReport{Location: &LocationFix{Latitude: 3}}) {
		t.Fatal("next delivery did not wake")
	}
	r, _ = p.Take()
	if r.Location.Dropped != 0 {
		t.Fatal("drop count crossed deliveries")
	}
}

func TestSensorPacketConcurrentProducerAndConsumer(t *testing.T) {
	var p SensorPacket
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Go(func() {
			for i := 0; i < 1000; i++ {
				p.Offer(SensorReport{Motion: &MotionSample{}})
			}
		})
	}
	wg.Wait()
	r, ok := p.Take()
	if !ok || r.Motion.Dropped != 3999 {
		t.Fatalf("lost drop count: %+v", r.Motion)
	}
}

func TestExtendedSensorPacketsAccumulateNativeDropsAcrossReplacement(t *testing.T) {
	for _, kind := range []SensorKind{SensorSteps, SensorActivity, SensorPressure, SensorGeofences} {
		t.Run(string(rune('0'+kind)), func(t *testing.T) {
			var p SensorPacket
			offer := func(dropped uint64) {
				r := SensorReport{Generation: 9}
				switch kind {
				case SensorSteps:
					r.Steps = &StepSample{Count: 27, Dropped: dropped}
				case SensorActivity:
					r.Activity = &ActivitySample{Kinds: ActivityWalking, Dropped: dropped}
				case SensorPressure:
					r.Pressure = &PressureSample{Pascals: 101325, Dropped: dropped}
				case SensorGeofences:
					r.Geofence = &GeofenceSample{Region: "owned", State: GeofenceInside, Dropped: dropped}
				}
				p.Offer(r)
			}
			offer(2)
			offer(3)
			offer(4)
			report, ok := p.Take()
			if !ok || report.Generation != 9 {
				t.Fatal("lost generation or delivery")
			}
			var dropped uint64
			switch kind {
			case SensorSteps:
				dropped = report.Steps.Dropped
			case SensorActivity:
				dropped = report.Activity.Dropped
			case SensorPressure:
				dropped = report.Pressure.Dropped
			case SensorGeofences:
				dropped = report.Geofence.Dropped
			}
			if dropped != 11 {
				t.Fatalf("native drops plus replaced deliveries=%d, want 11", dropped)
			}
			if _, ok := p.Take(); ok {
				t.Fatal("same sensor observation delivered twice")
			}
		})
	}
}
