//go:build darwin && !ios

package apple

import (
	"math"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Constructed Objective-C providers exercise native message and block ABIs; they never query a user's position or trigger an authorization prompt.
var extraFixture struct {
	block    objc.Block
	stopped  int
	grant    int
	pressure float64
}
var extraFixtureClass = sync.OnceValue(func() objc.Class {
	number := func(v float64) objc.ID { return objc.ID(Class("NSNumber")).Send(sel("numberWithDouble:"), v) }
	capture := func(_ objc.ID, _ objc.SEL, _ objc.ID, b objc.Block) { extraFixture.block = b.Copy() }
	stop := func(objc.ID, objc.SEL) {
		extraFixture.stopped++
		if extraFixture.block != 0 {
			extraFixture.block.Release()
			extraFixture.block = 0
		}
	}
	c := RegisterClass("NsgoExtendedSensorFixture", Class("NSObject"), nil, []objc.MethodDef{
		{Cmd: sel("numberOfSteps"), Fn: func(objc.ID, objc.SEL) objc.ID { return number(27) }},
		{Cmd: sel("startDate"), Fn: func(objc.ID, objc.SEL) objc.ID { return sensorDate(time.Unix(1000, 0)) }},
		{Cmd: sel("endDate"), Fn: func(objc.ID, objc.SEL) objc.ID { return sensorDate(time.Unix(1002, 0)) }},
		{Cmd: sel("distance"), Fn: func(objc.ID, objc.SEL) objc.ID { return number(0) }},
		{Cmd: sel("floorsAscended"), Fn: func(objc.ID, objc.SEL) objc.ID { return 0 }},
		{Cmd: sel("floorsDescended"), Fn: func(objc.ID, objc.SEL) objc.ID { return number(2) }},
		{Cmd: sel("stationary"), Fn: func(objc.ID, objc.SEL) bool { return true }},
		{Cmd: sel("automotive"), Fn: func(objc.ID, objc.SEL) bool { return true }},
		{Cmd: sel("walking"), Fn: func(objc.ID, objc.SEL) bool { return false }},
		{Cmd: sel("running"), Fn: func(objc.ID, objc.SEL) bool { return false }},
		{Cmd: sel("cycling"), Fn: func(objc.ID, objc.SEL) bool { return false }},
		{Cmd: sel("unknown"), Fn: func(objc.ID, objc.SEL) bool { return false }},
		{Cmd: sel("confidence"), Fn: func(objc.ID, objc.SEL) int { return 2 }},
		{Cmd: sel("pressure"), Fn: func(objc.ID, objc.SEL) objc.ID { return number(extraFixture.pressure) }},
		{Cmd: sel("relativeAltitude"), Fn: func(objc.ID, objc.SEL) objc.ID { return number(0) }},
		// Native forwarding supplies the floating-point return ABI that Go's callback trampoline cannot return directly.
		{Cmd: sel("methodSignatureForSelector:"), Fn: func(self objc.ID, _ objc.SEL, name objc.SEL) objc.ID {
			if name == sel("timestamp") {
				return objc.ID(Class("NSMethodSignature")).Send(sel("signatureWithObjCTypes:"), "d@:")
			}
			return SendSuper[objc.ID](self, sel("methodSignatureForSelector:"), name)
		}},
		{Cmd: sel("forwardInvocation:"), Fn: func(_ objc.ID, _ objc.SEL, invocation objc.ID) {
			value := 12.5
			invocation.Send(sel("setReturnValue:"), &value)
		}},
		{Cmd: sel("startUpdatesFromDate:withHandler:"), Fn: capture},
		{Cmd: sel("startRelativeAltitudeUpdatesToQueue:withHandler:"), Fn: capture},
		{Cmd: sel("startActivityUpdatesToQueue:withHandler:"), Fn: capture},
		{Cmd: sel("stopUpdates"), Fn: stop},
		{Cmd: sel("stopRelativeAltitudeUpdates"), Fn: stop},
		{Cmd: sel("stopActivityUpdates"), Fn: stop},
	})
	meta := objc.ID(c).Class()
	meta.AddMethod(sel("authorizationStatus"), objc.NewIMP(func(objc.ID, objc.SEL) int { return extraFixture.grant }), "q@:")
	for _, name := range []string{"isStepCountingAvailable", "isActivityAvailable", "isRelativeAltitudeAvailable"} {
		meta.AddMethod(sel(name), objc.NewIMP(func(objc.ID, objc.SEL) bool { return true }), "B@:")
	}
	return c
})

func extraBlockPointer(b objc.Block) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&b))
}

func invokeExtraBlock(block objc.Block, data objc.ID, activity bool) {
	header := (*struct {
		Isa             uintptr
		Flags, Reserved int32
		Invoke          uintptr
	})(extraBlockPointer(block))
	if activity {
		var f func(objc.Block, objc.ID)
		purego.RegisterFunc(&f, header.Invoke)
		f(block, data)
	} else {
		var f func(objc.Block, objc.ID, objc.ID)
		purego.RegisterFunc(&f, header.Invoke)
		f(block, data, 0)
	}
}

func TestExtendedAppleNativeMeasurements(t *testing.T) {
	onMain(func() {
		withPool(func() {
			extraFixture.pressure = 101.325
			data := objc.ID(extraFixtureClass()).Send(sel("new"))
			defer data.Send(selRelease)
			steps := readStepSample(data)
			if steps.Count != 27 || steps.Since.Unix() != 1000 || steps.Timestamp.Unix() != 1002 || !steps.Distance.Valid || steps.Distance.Value != 0 || steps.FloorsAscended.Valid || steps.FloorsDescended.Value != 2 {
				t.Errorf("steps %+v", steps)
			}
			activity := readActivitySample(data)
			if activity.Kinds != driver.ActivityStationary|driver.ActivityAutomotive || activity.Confidence != driver.ActivityConfidenceHigh {
				t.Errorf("activity %+v", activity)
			}
			pressure := readPressureSample(data)
			if math.Abs(pressure.Pascals-101325) > 1e-6 || pressure.Timestamp != 12500*time.Millisecond || !pressure.RelativeAltitude.Valid || pressure.RelativeAltitude.Value != 0 {
				t.Errorf("pressure %+v", pressure)
			}
		})
	})
}

func TestExtendedAppleNativeProviderBlocksAndStop(t *testing.T) {
	for _, kind := range []driver.SensorKind{driver.SensorSteps, driver.SensorActivity, driver.SensorPressure} {
		t.Run(string(rune('0'+kind)), func(t *testing.T) {
			onMain(func() {
				withPool(func() {
					oldClass, oldUsage := extraSensorClass, extraMotionUsage
					defer func() { extraSensorClass = oldClass; extraMotionUsage = oldUsage }()
					extraSensorClass = func(driver.SensorKind) objc.Class { return extraFixtureClass() }
					extraMotionUsage = func() bool { return true }
					extraFixture.grant = 3
					extraFixture.stopped = 0
					extraFixture.pressure = 101.325
					service := &Sensors{runs: map[uint32]*appleSensorRun{}}
					var packets []*driver.SensorPacket
					r := &appleSensorRun{service: service, id: 19, r: driver.SensorRequest{Kind: kind, Command: driver.SensorWatchSteps, Generation: 7}, started: time.Now(), dispatch: func(e driver.Event) {
						if p, ok := e.Data.(*driver.SensorPacket); ok {
							packets = append(packets, p)
						} else {
							t.Errorf("unexpected report %+v", e.Data)
						}
					}}
					service.runs[r.id] = r
					r.advancedMotion()
					if extraFixture.block == 0 {
						t.Error("provider did not copy callback")
						return
					}
					retained := extraFixture.block.Copy()
					defer retained.Release()
					runtime.GC()
					data := objc.ID(extraFixtureClass()).Send(sel("new"))
					defer data.Send(selRelease)
					if kind == driver.SensorPressure {
						extraFixture.pressure = math.NaN()
						invokeExtraBlock(retained, data, false)
						if len(packets) != 0 {
							t.Error("nonfinite pressure delivered")
						}
						extraFixture.pressure = 101.325
					}
					invokeExtraBlock(retained, data, kind == driver.SensorActivity)
					if len(packets) != 1 {
						t.Errorf("native callback queued %d packets", len(packets))
						r.close()
						return
					}
					report, ok := packets[0].Take()
					if !ok || report.Generation != 7 || report.Steps == nil && report.Activity == nil && report.Pressure == nil {
						t.Errorf("callback %+v", report)
					}
					r.close()
					r.close()
					if extraFixture.stopped != 1 || extraFixture.block != 0 || len(service.runs) != 0 {
						t.Errorf("stop count=%d block=%v runs=%d", extraFixture.stopped, extraFixture.block, len(service.runs))
					}
				})
			})
		})
	}
}

var regionFixture struct {
	regions        []objc.ID
	stopped        []string
	delegate       objc.ID
	precisionCalls int
}
var regionFixtureClass = sync.OnceValue(func() objc.Class {
	c := RegisterClass("NsgoScopedRegionProviderFixture", Class("NSObject"), nil, []objc.MethodDef{
		{Cmd: sel("authorizationStatus"), Fn: func(objc.ID, objc.SEL) int { return 4 }},
		{Cmd: sel("accuracyAuthorization"), Fn: func(objc.ID, objc.SEL) int { return 0 }},
		{Cmd: sel("setDelegate:"), Fn: func(_ objc.ID, _ objc.SEL, d objc.ID) { regionFixture.delegate = d }},
		{Cmd: sel("monitoredRegions"), Fn: func(objc.ID, objc.SEL) objc.ID {
			array := objc.ID(Class("NSMutableArray")).Send(sel("array"))
			for _, v := range regionFixture.regions {
				array.Send(sel("addObject:"), v)
			}
			return objc.ID(Class("NSSet")).Send(sel("setWithArray:"), array)
		}},
		{Cmd: sel("startMonitoringForRegion:"), Fn: func(_ objc.ID, _ objc.SEL, v objc.ID) { regionFixture.regions = append(regionFixture.regions, v) }},
		{Cmd: sel("stopMonitoringForRegion:"), Fn: func(_ objc.ID, _ objc.SEL, v objc.ID) {
			regionFixture.stopped = append(regionFixture.stopped, GoString(v.Send(sel("identifier"))))
		}},
		{Cmd: sel("requestTemporaryFullAccuracyAuthorizationWithPurposeKey:completion:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID, b objc.Block) {
			regionFixture.precisionCalls++
			extraFixture.block = b.Copy()
		}},
		{Cmd: sel("methodSignatureForSelector:"), Fn: func(self objc.ID, _ objc.SEL, name objc.SEL) objc.ID {
			if name == sel("maximumRegionMonitoringDistance") {
				return objc.ID(Class("NSMethodSignature")).Send(sel("signatureWithObjCTypes:"), "d@:")
			}
			return SendSuper[objc.ID](self, sel("methodSignatureForSelector:"), name)
		}},
		{Cmd: sel("forwardInvocation:"), Fn: func(_ objc.ID, _ objc.SEL, i objc.ID) { v := 10000.0; i.Send(sel("setReturnValue:"), &v) }},
	})
	meta := objc.ID(c).Class()
	meta.AddMethod(sel("locationServicesEnabled"), objc.NewIMP(func(objc.ID, objc.SEL) bool { return true }), "B@:")
	meta.AddMethod(sel("isMonitoringAvailableForClass:"), objc.NewIMP(func(objc.ID, objc.SEL, objc.Class) bool { return true }), "B@:#")
	return c
})

func TestExtendedAppleRegionsKeepOwnershipAndSeparateDelivery(t *testing.T) {
	onMain(func() {
		withPool(func() {
			if !loadLocation() {
				t.Skip("Core Location unavailable")
			}
			old := sensorLocationClass
			defer func() { sensorLocationClass = old }()
			sensorLocationClass = regionFixtureClass
			regionFixture.regions = nil
			regionFixture.stopped = nil
			region := func(id string) objc.ID {
				return objc.ID(Class("CLCircularRegion")).Send(sel("alloc")).Send(sel("initWithCenter:radius:identifier:"), locationCoordinate{Latitude: 0, Longitude: 0}, float64(100), NSString(id))
			}
			foreign, stale := region("embedded-host-region"), region("dev.nsgo.scoped-region:old-process:1:0")
			defer foreign.Send(selRelease)
			defer stale.Send(selRelease)
			regionFixture.regions = []objc.ID{foreign, stale}
			service := &Sensors{runs: map[uint32]*appleSensorRun{}}
			var packets []*driver.SensorPacket
			r := &appleSensorRun{service: service, id: 23, r: driver.SensorRequest{Kind: driver.SensorGeofences, Generation: 4, Geofences: driver.GeofenceOptions{Regions: []driver.GeofenceRegion{{ID: "a", Radius: 100}, {ID: "b", Radius: 100}}}}, manager: objc.ID(regionFixtureClass()).Send(sel("new")), dispatch: func(e driver.Event) {
				if p, ok := e.Data.(*driver.SensorPacket); ok {
					packets = append(packets, p)
				} else {
					t.Errorf("unexpected report %+v", e.Data)
				}
			}}
			service.runs[r.id] = r
			r.geofences()
			if len(r.regions) != 2 || len(regionFixture.stopped) != 1 || regionFixture.stopped[0] != "dev.nsgo.scoped-region:old-process:1:0" {
				t.Errorf("regions=%d removed=%v", len(r.regions), regionFixture.stopped)
			}
			for _, v := range r.regions {
				for range 10 {
					r.regionState(v.native, driver.GeofenceInside)
				}
			}
			r.regionState(foreign, driver.GeofenceOutside)
			if len(packets) != 2 {
				t.Errorf("two regions queued %d packets", len(packets))
			}
			for _, p := range packets {
				report, ok := p.Take()
				if !ok || report.Generation != 4 || report.Geofence.Dropped != 9 || report.Geofence.State != driver.GeofenceInside {
					t.Errorf("region report %+v", report)
				}
			}
			r.close()
			r.close()
			if len(regionFixture.stopped) != 3 || regionFixture.delegate != 0 || len(service.runs) != 0 {
				t.Errorf("cleanup %+v", regionFixture.stopped)
			}
			for _, id := range regionFixture.stopped {
				if id == "embedded-host-region" {
					t.Error("removed host's region")
				}
			}
			regionFixture.regions = nil
		})
	})
}

func TestExtendedAppleAccuracyValidatesPurposeAndIgnoresDisposedAnswer(t *testing.T) {
	release := holdMainLoop()
	defer release()
	var run *appleSensorRun
	var retained objc.Block
	answers := 0
	onMain(func() {
		withPool(func() {
			oldClass, oldPurpose := sensorLocationClass, accuracyPurposeDeclared
			defer func() { sensorLocationClass = oldClass; accuracyPurposeDeclared = oldPurpose }()
			sensorLocationClass = regionFixtureClass
			regionFixture.precisionCalls = 0
			service := &Sensors{runs: map[uint32]*appleSensorRun{}}
			start := func() *appleSensorRun {
				r := &appleSensorRun{service: service, id: 31, r: driver.SensorRequest{Kind: driver.SensorLocation, Command: driver.SensorRequestPreciseLocation, Purpose: "fixture", Generation: 2}, manager: objc.ID(regionFixtureClass()).Send(sel("new")), dispatch: func(e driver.Event) {
					report := e.Data.(*driver.SensorReport)
					if report.Error == "not-configured" {
						return
					}
					answers++
					if report.Precise == nil || !report.Precise.Full || !report.End {
						t.Errorf("accuracy %+v", report)
					}
				}}
				service.runs[r.id] = r
				return r
			}
			accuracyPurposeDeclared = func(string) bool { return false }
			r := start()
			r.requestAccuracy()
			if regionFixture.precisionCalls != 0 || !r.closed {
				t.Error("undeclared purpose reached native request")
			}
			accuracyPurposeDeclared = func(string) bool { return true }
			run = start()
			run.requestAccuracy()
			if regionFixture.precisionCalls != 1 || extraFixture.block == 0 {
				t.Error("declared purpose did not request accuracy")
				return
			}
			retained = extraFixture.block
			extraFixture.block = 0
			header := (*struct {
				Isa             uintptr
				Flags, Reserved int32
				Invoke          uintptr
			})(extraBlockPointer(retained))
			var invoke func(objc.Block, objc.ID)
			purego.RegisterFunc(&invoke, header.Invoke)
			invoke(retained, 0)
		})
	})
	onMain(func() {
		if answers != 1 || !run.closed {
			t.Errorf("accuracy completion: answers=%d closed=%v", answers, run.closed)
		}
		if retained != 0 {
			header := (*struct {
				Isa             uintptr
				Flags, Reserved int32
				Invoke          uintptr
			})(extraBlockPointer(retained))
			var invoke func(objc.Block, objc.ID)
			purego.RegisterFunc(&invoke, header.Invoke)
			invoke(retained, 0)
			retained.Release()
		}
	})
	onMain(func() {
		if answers != 1 {
			t.Error("disposed accuracy answer escaped")
		}
	})
}

var leaseFixture struct {
	block objc.Block
	ended int
}
var leaseFixtureClass = sync.OnceValue(func() objc.Class {
	return RegisterClass("NsgoMotionLeaseFixture", Class("NSObject"), nil, []objc.MethodDef{
		{Cmd: sel("beginBackgroundTaskWithName:expirationHandler:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID, b objc.Block) uintptr { leaseFixture.block = b.Copy(); return 7 }},
		{Cmd: sel("endBackgroundTask:"), Fn: func(_ objc.ID, _ objc.SEL, task uintptr) {
			if task == 7 {
				leaseFixture.ended++
			}
			if leaseFixture.block != 0 {
				leaseFixture.block.Release()
				leaseFixture.block = 0
			}
		}},
	})
})

func TestExtendedAppleMotionLeaseExpiryAndDisposal(t *testing.T) {
	release := holdMainLoop()
	defer release()
	for _, mode := range []string{"duration", "system", "dispose"} {
		t.Run(mode, func(t *testing.T) {
			done := make(chan string, 1)
			var run *appleSensorRun
			onMain(func() {
				old := motionLeaseApplication
				defer func() { motionLeaseApplication = old }()
				app := objc.ID(leaseFixtureClass()).Send(sel("new"))
				defer app.Send(selRelease)
				motionLeaseApplication = func() objc.ID { return app }
				leaseFixture.ended = 0
				service := &Sensors{runs: map[uint32]*appleSensorRun{}}
				duration := time.Hour
				if mode == "duration" {
					duration = 20 * time.Millisecond
				}
				run = &appleSensorRun{service: service, id: 42, r: driver.SensorRequest{Command: driver.SensorWatchBackgroundMotion, BackgroundMotion: driver.BackgroundMotionOptions{Duration: duration}}, dispatch: func(e driver.Event) { done <- e.Data.(*driver.SensorReport).Error }}
				service.runs[42] = run
				if !run.beginMotionLease() {
					t.Error("lease refused")
					return
				}
				// The lease owns this application for its lifetime in a real process. Keep the fixture alive until its asynchronous ending below.
				app.Send(sel("retain"))
				oldStop := run.extraStop
				run.extraStop = func() { oldStop(); app.Send(selRelease) }
				if mode == "system" {
					b := leaseFixture.block.Copy()
					header := (*struct {
						Isa             uintptr
						Flags, Reserved int32
						Invoke          uintptr
					})(extraBlockPointer(b))
					var invoke func(objc.Block)
					purego.RegisterFunc(&invoke, header.Invoke)
					invoke(b)
					b.Release()
				}
				if mode == "dispose" {
					run.close()
					done <- "disposed"
				}
			})
			select {
			case result := <-done:
				if mode != "dispose" && result != "timeout" {
					t.Errorf("expiry %s", result)
				}
			case <-time.After(time.Second):
				t.Fatal("lease did not end")
			}
			onMain(func() {
				if leaseFixture.ended != 1 || leaseFixture.block != 0 || !run.closed {
					t.Errorf("lease cleanup ended=%d block=%v closed=%v", leaseFixture.ended, leaseFixture.block, run.closed)
				}
			})
		})
	}
}
