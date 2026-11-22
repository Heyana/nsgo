//go:build darwin

package apple

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
	"github.com/nsgo-dev/nsgo/internal/platform/privacy"
)

var extraSensorClass = nativeExtraSensorClass
var extraMotionUsage = func() bool { return privacy.DeclaresUsage("NSMotionUsageDescription") }

func nativeExtraSensorClass(kind driver.SensorKind) objc.Class {
	if !loadMotion() {
		return 0
	}
	name := ""
	switch kind {
	case driver.SensorSteps, driver.SensorActivityPermission:
		name = "CMPedometer"
	case driver.SensorActivity:
		name = "CMMotionActivityManager"
	case driver.SensorPressure:
		name = "CMAltimeter"
	}
	return Class(name)
}

func extraMotionGrant(class objc.Class) driver.SensorGrant {
	g := driver.SensorGrant{Authorization: driver.SensorUnavailable}
	if class == 0 || !responds(objc.ID(class), "authorizationStatus") {
		return g
	}
	switch objc.Send[int](objc.ID(class), sel("authorizationStatus")) {
	case 0:
		g.Authorization = driver.SensorNotAsked
	case 1:
		g.Authorization = driver.SensorRestricted
	case 2:
		g.Authorization = driver.SensorDenied
	case 3:
		g.Authorization = driver.SensorGranted
	}
	return g
}

func (r *appleSensorRun) advancedMotion() {
	class := extraSensorClass(r.r.Kind)
	g := extraMotionGrant(class)
	if r.r.Command <= driver.SensorRequestPermission {
		if r.r.Command == driver.SensorReadPermission || g.Authorization != driver.SensorNotAsked {
			r.grant(g)
			return
		}
		if !extraMotionUsage() {
			r.grant(driver.SensorGrant{Authorization: driver.SensorUnavailable})
			return
		}
		// Only an explicit permission request starts the system's authorization operation. Starting any of the streams below never prompts.
		if !objc.Send[bool](objc.ID(class), sel("isStepCountingAvailable")) {
			r.unavailable()
			return
		}
		r.manager = objc.ID(class).Send(sel("new"))
		block := objcblock.New(func(objc.Block, objc.ID, objc.ID) {})
		r.manager.Send(sel("startUpdatesFromDate:withHandler:"), sensorDate(time.Now()), block)
		block.Release()
		r.extraStop = func() { r.manager.Send(sel("stopUpdates")) }
		r.pollExtraGrant(class)
		return
	}
	if g.Authorization != driver.SensorGranted {
		r.fail(grantError(g))
		return
	}
	if !extraMotionUsage() {
		r.fail("not-configured")
		return
	}
	var available string
	switch r.r.Kind {
	case driver.SensorSteps:
		available = "isStepCountingAvailable"
	case driver.SensorActivity:
		available = "isActivityAvailable"
	case driver.SensorPressure:
		available = "isRelativeAltitudeAvailable"
	default:
		r.unavailable()
		return
	}
	if !objc.Send[bool](objc.ID(class), sel(available)) {
		r.unavailable()
		return
	}
	r.manager = objc.ID(class).Send(sel("new"))
	packet := &driver.SensorPacket{}
	deliver := func(report driver.SensorReport) {
		report.Generation = r.r.Generation
		if packet.Offer(report) {
			r.dispatch(driver.Event{Kind: driver.EventSensor, Count: r.id, Data: packet})
		}
	}
	failed := func(failure objc.ID) bool {
		if failure == 0 {
			return false
		}
		OnMain(func() {
			if !r.closed {
				g := extraMotionGrant(class)
				if g.Authorization != driver.SensorGranted {
					r.fail(grantError(g))
				} else {
					r.fail("failed")
				}
			}
		})
		return true
	}
	switch r.r.Kind {
	case driver.SensorSteps:
		block := objcblock.New(func(_ objc.Block, data, failure objc.ID) {
			if failed(failure) || data == 0 {
				return
			}
			v := readStepSample(data)
			deliver(driver.SensorReport{Steps: &v})
		})
		r.extraStop = func() { r.manager.Send(sel("stopUpdates")) }
		r.manager.Send(sel("startUpdatesFromDate:withHandler:"), sensorDate(r.started), block)
		block.Release()
	case driver.SensorActivity:
		r.queue = objc.ID(Class("NSOperationQueue")).Send(sel("new"))
		r.queue.Send(sel("setMaxConcurrentOperationCount:"), 1)
		block := objcblock.New(func(_ objc.Block, data objc.ID) {
			if data == 0 {
				return
			}
			v := readActivitySample(data)
			deliver(driver.SensorReport{Activity: &v})
		})
		r.extraStop = func() { r.manager.Send(sel("stopActivityUpdates")) }
		r.manager.Send(sel("startActivityUpdatesToQueue:withHandler:"), r.queue, block)
		block.Release()
	case driver.SensorPressure:
		r.queue = objc.ID(Class("NSOperationQueue")).Send(sel("new"))
		r.queue.Send(sel("setMaxConcurrentOperationCount:"), 1)
		var previous time.Duration
		block := objcblock.New(func(_ objc.Block, data, failure objc.ID) {
			if failed(failure) || data == 0 {
				return
			}
			v := readPressureSample(data)
			if math.IsNaN(v.Pascals) || math.IsInf(v.Pascals, 0) || v.Pascals <= 0 || v.Timestamp < 0 {
				return
			}
			if previous > 0 {
				v.Interval = v.Timestamp - previous
			}
			previous = v.Timestamp
			deliver(driver.SensorReport{Pressure: &v})
		})
		r.extraStop = func() { r.manager.Send(sel("stopRelativeAltitudeUpdates")) }
		r.manager.Send(sel("startRelativeAltitudeUpdatesToQueue:withHandler:"), r.queue, block)
		block.Release()
	}
	r.pollExtraAuthorization(class)
}

func (r *appleSensorRun) pollExtraGrant(class objc.Class) {
	if r.closed {
		return
	}
	g := extraMotionGrant(class)
	if g.Authorization != driver.SensorNotAsked || time.Since(r.started) >= 30*time.Second {
		r.grant(g)
		return
	}
	time.AfterFunc(100*time.Millisecond, func() { OnMain(func() { r.pollExtraGrant(class) }) })
}

func (r *appleSensorRun) pollExtraAuthorization(class objc.Class) {
	time.AfterFunc(time.Second, func() {
		OnMain(func() {
			if r.closed {
				return
			}
			g := extraMotionGrant(class)
			if g.Authorization != driver.SensorGranted {
				r.fail(grantError(g))
				return
			}
			r.pollExtraAuthorization(class)
		})
	})
}

func sensorDate(t time.Time) objc.ID {
	return objc.ID(Class("NSDate")).Send(sel("dateWithTimeIntervalSince1970:"), float64(t.UnixNano())/1e9)
}
func sensorTime(date objc.ID) time.Time {
	if date == 0 {
		return time.Time{}
	}
	v := objc.Send[float64](date, sel("timeIntervalSince1970"))
	seconds, fraction := math.Modf(v)
	return time.Unix(int64(seconds), int64(fraction*1e9))
}
func optionalSensorNumber(data objc.ID, name string) driver.SensorNumber {
	n := data.Send(sel(name))
	if n == 0 {
		return driver.SensorNumber{}
	}
	v := objc.Send[float64](n, sel("doubleValue"))
	return sensorNumber(v, !math.IsNaN(v) && !math.IsInf(v, 0))
}

func readStepSample(data objc.ID) driver.StepSample {
	return driver.StepSample{Count: objc.Send[uint64](data.Send(sel("numberOfSteps")), sel("unsignedLongLongValue")), Since: sensorTime(data.Send(sel("startDate"))), Timestamp: sensorTime(data.Send(sel("endDate"))), Distance: optionalSensorNumber(data, "distance"), FloorsAscended: optionalSensorNumber(data, "floorsAscended"), FloorsDescended: optionalSensorNumber(data, "floorsDescended")}
}

func readActivitySample(data objc.ID) driver.ActivitySample {
	v := driver.ActivitySample{Timestamp: sensorTime(data.Send(sel("startDate")))}
	for _, flag := range []struct {
		name string
		kind driver.ActivityKinds
	}{{"stationary", driver.ActivityStationary}, {"walking", driver.ActivityWalking}, {"running", driver.ActivityRunning}, {"cycling", driver.ActivityCycling}, {"automotive", driver.ActivityAutomotive}, {"unknown", driver.ActivityUnknown}} {
		if objc.Send[bool](data, sel(flag.name)) {
			v.Kinds |= flag.kind
		}
	}
	confidence := objc.Send[int](data, sel("confidence"))
	if confidence >= 0 && confidence <= 2 {
		v.Confidence = driver.ActivityConfidence(confidence + 1)
	}
	return v
}

func readPressureSample(data objc.ID) driver.PressureSample {
	return driver.PressureSample{Pascals: objc.Send[float64](data.Send(sel("pressure")), sel("doubleValue")) * 1000, RelativeAltitude: optionalSensorNumber(data, "relativeAltitude"), Timestamp: time.Duration(objc.Send[float64](data, sel("timestamp")) * 1e9)}
}

// requestAccuracy does not ask for a location fix. The purpose dictionary is checked before invoking an API that would otherwise terminate an app.
func (r *appleSensorRun) requestAccuracy() {
	g := locationGrant(r.manager, false)
	if g.Authorization != driver.SensorGranted {
		r.fail(grantError(g))
		return
	}
	if !responds(r.manager, "requestTemporaryFullAccuracyAuthorizationWithPurposeKey:completion:") {
		r.unavailable()
		return
	}
	if !accuracyPurposeDeclared(r.r.Purpose) {
		r.fail("not-configured")
		return
	}
	block := objcblock.New(func(_ objc.Block, failure objc.ID) {
		OnMain(func() {
			if r.closed {
				return
			}
			if failure != 0 {
				r.fail("failed")
				return
			}
			full := objc.Send[int](r.manager, sel("accuracyAuthorization")) == 0
			report := driver.SensorReport{Generation: r.r.Generation, Precise: &driver.PreciseLocationAnswer{Full: full}, End: true}
			r.dispatch(driver.Event{Kind: driver.EventSensor, Count: r.id, Data: &report})
			r.close()
		})
	})
	r.manager.Send(sel("requestTemporaryFullAccuracyAuthorizationWithPurposeKey:completion:"), NSString(r.r.Purpose), block)
	block.Release()
}

type monitoredRegion struct {
	id     string
	native objc.ID
	packet driver.SensorPacket
}

var regionSequence atomic.Uint64
var regionProcess = sync.OnceValue(func() string { return GoString(objc.ID(Class("NSUUID")).Send(sel("UUID")).Send(sel("UUIDString"))) })

func (r *appleSensorRun) geofences() {
	g := locationGrant(r.manager, r.r.Geofences.Background)
	if g.Authorization != driver.SensorGranted {
		r.fail(grantError(g))
		return
	}
	if r.r.Geofences.Background && objc.Send[int](r.manager, sel("authorizationStatus")) != 3 {
		r.fail("not-allowed")
		return
	}
	class := Class("CLCircularRegion")
	if class == 0 || !objc.Send[bool](objc.ID(sensorLocationClass()), sel("isMonitoringAvailableForClass:"), class) {
		r.unavailable()
		return
	}
	// A new process has no old scopes. Remove only this runtime's stale registrations, preserving other region users in an embedded host.
	currentPrefix := "dev.nsgo.scoped-region:" + regionProcess() + ":"
	old := r.manager.Send(sel("monitoredRegions")).Send(sel("allObjects"))
	for i, n := uint(0), objc.Send[uint](old, sel("count")); i < n; i++ {
		region := old.Send(sel("objectAtIndex:"), i)
		id := GoString(region.Send(sel("identifier")))
		if strings.HasPrefix(id, "dev.nsgo.scoped-region:") && !strings.HasPrefix(id, currentPrefix) {
			r.manager.Send(sel("stopMonitoringForRegion:"), region)
		}
	}
	r.attachDelegate()
	r.regions = map[string]*monitoredRegion{}
	r.extraStop = func() {
		for _, region := range r.regions {
			r.manager.Send(sel("stopMonitoringForRegion:"), region.native)
			region.native.Send(selRelease)
		}
		r.regions = nil
	}
	maximum := objc.Send[float64](r.manager, sel("maximumRegionMonitoringDistance"))
	owner := regionSequence.Add(1)
	for i, region := range r.r.Geofences.Regions {
		if maximum > 0 && region.Radius > maximum {
			r.fail("unavailable")
			return
		}
		key := fmt.Sprintf("dev.nsgo.scoped-region:%s:%d:%d", regionProcess(), owner, i)
		native := objc.ID(class).Send(sel("alloc")).Send(sel("initWithCenter:radius:identifier:"), locationCoordinate{Latitude: region.Latitude, Longitude: region.Longitude}, region.Radius, NSString(key))
		if native == 0 {
			r.fail("failed")
			return
		}
		r.regions[key] = &monitoredRegion{id: region.ID, native: native}
		r.manager.Send(sel("startMonitoringForRegion:"), native)
	}
}

func (r *appleSensorRun) regionState(region objc.ID, state driver.GeofenceState) {
	if r.closed {
		return
	}
	monitored := r.regions[GoString(region.Send(sel("identifier")))]
	if monitored == nil {
		return
	}
	v := driver.GeofenceSample{Region: monitored.id, State: state, Timestamp: time.Now()}
	if monitored.packet.Offer(driver.SensorReport{Generation: r.r.Generation, Geofence: &v}) {
		r.dispatch(driver.Event{Kind: driver.EventSensor, Count: r.id, Data: &monitored.packet})
	}
}

var motionLeaseApplication = func() objc.ID {
	c := Class("UIApplication")
	if c == 0 {
		return 0
	}
	return objc.ID(c).Send(sel("sharedApplication"))
}

func (r *appleSensorRun) beginMotionLease() bool {
	app := motionLeaseApplication()
	if !responds(app, "beginBackgroundTaskWithName:expirationHandler:") {
		r.unavailable()
		return false
	}
	if g := motionGrant(); g.Authorization != driver.SensorGranted {
		r.fail(grantError(g))
		return false
	}
	expired := objcblock.New(func(objc.Block) {
		OnMain(func() {
			if !r.closed {
				r.fail("timeout")
			}
		})
	})
	task := objc.Send[uintptr](app, sel("beginBackgroundTaskWithName:expirationHandler:"), NSString("NSGo bounded motion"), expired)
	expired.Release()
	if task == ^uintptr(0) {
		r.unavailable()
		return false
	}
	deadline := time.AfterFunc(r.r.BackgroundMotion.Duration, func() {
		OnMain(func() {
			if !r.closed {
				r.fail("timeout")
			}
		})
	})
	r.extraStop = func() { deadline.Stop(); app.Send(sel("endBackgroundTask:"), task) }
	return true
}

var accuracyPurposeDeclared = declaredAccuracyPurpose

func declaredAccuracyPurpose(purpose string) bool {
	info := objc.ID(Class("NSBundle")).Send(sel("mainBundle")).Send(sel("infoDictionary"))
	purposes := info.Send(selObjectForKey, NSString("NSLocationTemporaryUsageDescriptionDictionary"))
	if purposes == 0 || !objc.Send[bool](purposes, sel("isKindOfClass:"), Class("NSDictionary")) {
		return false
	}
	reason := purposes.Send(selObjectForKey, NSString(purpose))
	return reason != 0 && objc.Send[bool](reason, sel("isKindOfClass:"), Class("NSString")) && GoString(reason) != ""
}
