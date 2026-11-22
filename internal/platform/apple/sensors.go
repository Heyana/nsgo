//go:build darwin

package apple

import (
	"math"
	"runtime"
	"structs"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
	"github.com/nsgo-dev/nsgo/internal/platform/privacy"
)

// Sensors owns a surface's providers, on the main thread. Core Motion's
// handlers only copy samples into bounded packets; app code runs in core.
type Sensors struct{ runs map[uint32]*appleSensorRun }

type appleSensorRun struct {
	service                  *Sensors
	id                       uint32
	r                        driver.SensorRequest
	dispatch                 func(driver.Event)
	manager, delegate, queue objc.ID
	started                  time.Time
	closed                   bool
	packets                  [5]driver.SensorPacket
	motionFailure            atomic.Bool
	extraStop                func()
	regions                  map[string]*monitoredRegion
}

var locationRuns = map[objc.ID]*appleSensorRun{}
var sensorLocationClass = func() objc.Class { return Class("CLLocationManager") }

var loadLocation = sync.OnceValue(func() bool {
	_, err := purego.Dlopen("/System/Library/Frameworks/CoreLocation.framework/CoreLocation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	return err == nil && Class("CLLocationManager") != 0
})

var locationDelegate = sync.OnceValue(func() objc.Class {
	return RegisterClass("NsgoLocationDelegate", Class("NSObject"), []*objc.Protocol{objc.GetProtocol("CLLocationManagerDelegate")}, []objc.MethodDef{
		{Cmd: sel("locationManager:didUpdateLocations:"), Fn: func(self objc.ID, _ objc.SEL, _, locations objc.ID) {
			if run := locationRuns[self]; run != nil {
				run.locations(locations)
			}
		}},
		{Cmd: sel("locationManagerDidChangeAuthorization:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			if run := locationRuns[self]; run != nil {
				run.authorizationChanged()
			}
		}},
		// macOS 11 and older managers can use the older delegate method.
		{Cmd: sel("locationManager:didChangeAuthorizationStatus:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID, _ int32) {
			if run := locationRuns[self]; run != nil {
				run.authorizationChanged()
			}
		}},
		{Cmd: sel("locationManager:didEnterRegion:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID, region objc.ID) {
			if r := locationRuns[self]; r != nil {
				r.regionState(region, driver.GeofenceInside)
			}
		}},
		{Cmd: sel("locationManager:didExitRegion:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID, region objc.ID) {
			if r := locationRuns[self]; r != nil {
				r.regionState(region, driver.GeofenceOutside)
			}
		}},
		{Cmd: sel("locationManager:didDetermineState:forRegion:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID, state int, region objc.ID) {
			if r := locationRuns[self]; r != nil {
				switch state {
				case 1:
					r.regionState(region, driver.GeofenceInside)
				case 2:
					r.regionState(region, driver.GeofenceOutside)
				default:
					r.regionState(region, driver.GeofenceUnknown)
				}
			}
		}},
		{Cmd: sel("locationManager:didStartMonitoringForRegion:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID, region objc.ID) {
			if r := locationRuns[self]; r != nil && !r.closed {
				r.manager.Send(sel("requestStateForRegion:"), region)
			}
		}},
		{Cmd: sel("locationManager:monitoringDidFailForRegion:withError:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID, _ objc.ID, _ objc.ID) {
			if r := locationRuns[self]; r != nil && !r.closed {
				r.fail("failed")
			}
		}},
		{Cmd: sel("locationManager:didFailWithError:"), Fn: func(self objc.ID, _ objc.SEL, _, failure objc.ID) {
			if run := locationRuns[self]; run != nil {
				switch objc.Send[int](failure, sel("code")) {
				case 0: // kCLErrorLocationUnknown: temporary; keep waiting.
				case 1:
					run.fail("not-allowed")
				default:
					run.fail("failed")
				}
			}
		}},
	})
})

func locationGrant(manager objc.ID, background bool) driver.SensorGrant {
	g := driver.SensorGrant{Authorization: driver.SensorNotAsked}
	status := objc.Send[int](manager, sel("authorizationStatus"))
	switch status {
	case 1:
		g.Authorization = driver.SensorRestricted
	case 2:
		g.Authorization = driver.SensorDenied
	case 3, 4:
		g.Authorization = driver.SensorGranted
	}
	if !objc.Send[bool](objc.ID(sensorLocationClass()), sel("locationServicesEnabled")) {
		g.Authorization = driver.SensorUnavailable
	}
	if background && !backgroundLocationDeclared() {
		g.Authorization = driver.SensorUnavailable
	}
	return g
}

func backgroundLocationDeclared() bool {
	if runtime.GOOS != "ios" {
		return true
	}
	info := objc.ID(Class("NSBundle")).Send(sel("mainBundle")).Send(sel("infoDictionary"))
	modes := info.Send(selObjectForKey, NSString("UIBackgroundModes"))
	return modes != 0 && objc.Send[bool](modes, sel("isKindOfClass:"), Class("NSArray")) &&
		objc.Send[bool](modes, sel("containsObject:"), NSString("location"))
}

func responds(object objc.ID, method string) bool {
	return objc.Send[bool](object, sel("respondsToSelector:"), sel(method))
}

// Handle applies ItemSensor on the main thread. Motion and continuous
// location are separately disabled on platforms whose SDK omits them.
func (s *Sensors) Handle(it *driver.MountItem, dispatch func(driver.Event), motion, continuous bool) {
	if s.runs == nil {
		s.runs = map[uint32]*appleSensorRun{}
	}
	if previous := s.runs[it.Count]; previous != nil {
		previous.close()
	}
	r := *it.Sensor
	if r.Command == driver.SensorStop {
		return
	}
	run := &appleSensorRun{service: s, id: it.Count, r: r, dispatch: dispatch, started: time.Now()}
	s.runs[it.Count] = run
	if r.Kind == driver.SensorMotion {
		if !motion || !loadMotion() {
			run.unavailable()
			return
		}
		if r.Command == driver.SensorWatchBackgroundMotion && !run.beginMotionLease() {
			return
		}
		run.motion()
		return
	}
	if r.Kind == driver.SensorSteps || r.Kind == driver.SensorActivity || r.Kind == driver.SensorPressure || r.Kind == driver.SensorActivityPermission {
		if !motion {
			run.unavailable()
			return
		}
		run.advancedMotion()
		return
	}
	if r.Kind != driver.SensorLocation && r.Kind != driver.SensorBackgroundLocation && r.Kind != driver.SensorGeofences || r.Kind == driver.SensorGeofences && !continuous {
		run.unavailable()
		return
	}
	if !loadLocation() {
		run.unavailable()
		return
	}
	run.manager = objc.ID(sensorLocationClass()).Send(sel("new"))
	if r.Command == driver.SensorRequestPreciseLocation {
		run.requestAccuracy()
		return
	}
	if r.Kind == driver.SensorGeofences {
		run.geofences()
		return
	}
	if r.Command <= driver.SensorRequestPermission {
		grant := locationGrant(run.manager, r.Kind == driver.SensorBackgroundLocation)
		upgrade := r.Command == driver.SensorRequestPermission && r.Kind == driver.SensorBackgroundLocation && privacy.DeclaresUsage("NSLocationAlwaysAndWhenInUseUsageDescription") && objc.Send[int](run.manager, sel("authorizationStatus")) == 4 && responds(run.manager, "requestAlwaysAuthorization")
		if r.Command == driver.SensorReadPermission || grant.Authorization != driver.SensorNotAsked && !upgrade {
			run.grant(grant)
			return
		}
		if !privacy.DeclaresUsage("NSLocationWhenInUseUsageDescription") && !privacy.DeclaresUsage("NSLocationUsageDescription") {
			run.grant(driver.SensorGrant{Authorization: driver.SensorUnavailable})
			return
		}
		run.attachDelegate()
		if r.Kind == driver.SensorBackgroundLocation && privacy.DeclaresUsage("NSLocationAlwaysAndWhenInUseUsageDescription") && responds(run.manager, "requestAlwaysAuthorization") {
			run.manager.Send(sel("requestAlwaysAuthorization"))
		} else {
			run.manager.Send(sel("requestWhenInUseAuthorization"))
		}
		return
	}
	grant := locationGrant(run.manager, false)
	if grant.Authorization != driver.SensorGranted {
		run.fail(grantError(grant))
		return
	}
	if r.Command == driver.SensorWatchLocation && !continuous {
		run.fail("unavailable")
		return
	}
	if r.Location.Background && runtime.GOOS == "ios" {
		if !backgroundLocationDeclared() || !responds(run.manager, "setAllowsBackgroundLocationUpdates:") {
			run.fail("not-configured")
			return
		}
		run.manager.Send(sel("setAllowsBackgroundLocationUpdates:"), true)
		if responds(run.manager, "setShowsBackgroundLocationIndicator:") {
			run.manager.Send(sel("setShowsBackgroundLocationIndicator:"), true)
		}
	}
	accuracy := r.Location.Accuracy
	if accuracy == 0 {
		accuracy = 100
	}
	run.manager.Send(sel("setDesiredAccuracy:"), accuracy)
	run.manager.Send(sel("setDistanceFilter:"), r.Location.Distance)
	run.attachDelegate()
	if r.Command == driver.SensorLocate {
		if r.Location.MaxAge > 0 {
			if fix, ok := run.location(run.manager.Send(sel("location"))); ok {
				run.deliverLocation(fix)
				return
			}
		}
		run.manager.Send(sel("requestLocation"))
	} else {
		run.manager.Send(sel("startUpdatingLocation"))
	}
}

func grantError(g driver.SensorGrant) string {
	if g.Authorization == driver.SensorUnavailable {
		return "unavailable"
	}
	return "not-allowed"
}

func (r *appleSensorRun) attachDelegate() {
	r.delegate = objc.ID(locationDelegate()).Send(sel("new"))
	locationRuns[r.delegate] = r
	r.manager.Send(sel("setDelegate:"), r.delegate)
}

func (r *appleSensorRun) authorizationChanged() {
	g := locationGrant(r.manager, r.r.Kind == driver.SensorBackgroundLocation)
	if r.r.Kind == driver.SensorGeofences && r.r.Geofences.Background && objc.Send[int](r.manager, sel("authorizationStatus")) != 3 {
		r.fail("not-allowed")
		return
	}
	if r.r.Command <= driver.SensorRequestPermission {
		if g.Authorization != driver.SensorNotAsked {
			r.grant(g)
		}
	} else if g.Authorization != driver.SensorGranted {
		r.fail(grantError(g))
	}
}

func (r *appleSensorRun) grant(g driver.SensorGrant) {
	r.dispatch(driver.Event{Kind: driver.EventSensor, Count: r.id, Data: &driver.SensorReport{Generation: r.r.Generation, Grant: &g, End: true}})
	r.close()
}

func (r *appleSensorRun) unavailable() {
	if r.r.Command <= driver.SensorRequestPermission {
		r.grant(driver.SensorGrant{Authorization: driver.SensorUnavailable})
	} else {
		r.fail("unavailable")
	}
}

func (r *appleSensorRun) fail(code string) {
	r.dispatch(driver.Event{Kind: driver.EventSensor, Count: r.id, Data: &driver.SensorReport{Generation: r.r.Generation, Error: code, End: true}})
	r.close()
}

func (r *appleSensorRun) locations(locations objc.ID) {
	n := objc.Send[uint](locations, selCount)
	for i := uint(0); i < n && !r.closed; i++ {
		if f, ok := r.location(locations.Send(selObjectAtIndex, i)); ok {
			r.deliverLocation(f)
		}
	}
	if !r.closed && r.r.Command == driver.SensorLocate {
		// A provider may answer requestLocation with only a cached fix.
		// Ask again later rather than delivering it as a new position.
		time.AfterFunc(200*time.Millisecond, func() {
			OnMain(func() {
				if !r.closed {
					r.manager.Send(sel("requestLocation"))
				}
			})
		})
	}
}

func sensorNumber(v float64, valid bool) driver.SensorNumber {
	return driver.SensorNumber{Value: v, Valid: valid && !math.IsNaN(v) && !math.IsInf(v, 0)}
}

type locationCoordinate struct {
	_                   structs.HostLayout
	Latitude, Longitude float64
}

func (r *appleSensorRun) location(location objc.ID) (driver.LocationFix, bool) {
	if location == 0 {
		return driver.LocationFix{}, false
	}
	timestamp := objc.Send[float64](location.Send(sel("timestamp")), sel("timeIntervalSince1970"))
	t := time.Unix(0, int64(timestamp*1e9))
	age := max(time.Duration(0), time.Since(t))
	cached := t.Before(r.started)
	if r.r.Command == driver.SensorLocate && cached && (r.r.Location.MaxAge <= 0 || age > r.r.Location.MaxAge) {
		return driver.LocationFix{}, false
	}
	c := objc.Send[locationCoordinate](location, sel("coordinate"))
	h := objc.Send[float64](location, sel("horizontalAccuracy"))
	v := objc.Send[float64](location, sel("verticalAccuracy"))
	if h < 0 {
		return driver.LocationFix{}, false
	}
	f := driver.LocationFix{Latitude: c.Latitude, Longitude: c.Longitude, Timestamp: t, Age: age, Cached: cached,
		HorizontalAccuracy: sensorNumber(h, h >= 0), VerticalAccuracy: sensorNumber(v, v >= 0),
		MSLAltitude: sensorNumber(objc.Send[float64](location, sel("altitude")), v >= 0)}
	if responds(location, "ellipsoidalAltitude") {
		f.Altitude = sensorNumber(objc.Send[float64](location, sel("ellipsoidalAltitude")), v >= 0)
	}
	f.Speed = sensorNumber(objc.Send[float64](location, sel("speed")), true)
	f.Speed.Valid = f.Speed.Valid && f.Speed.Value >= 0
	f.Course = sensorNumber(objc.Send[float64](location, sel("course")), true)
	f.Course.Valid = f.Course.Valid && f.Course.Value >= 0
	// CLAccuracyAuthorizationReducedAccuracy is 1; before macOS 11 and iOS 14 every fix is precise.
	f.Approximate = responds(r.manager, "accuracyAuthorization") && objc.Send[int](r.manager, sel("accuracyAuthorization")) == 1
	return f, true
}

func (r *appleSensorRun) deliverLocation(f driver.LocationFix) {
	copy := f
	report := driver.SensorReport{Generation: r.r.Generation, Location: &copy}
	if r.r.Command == driver.SensorLocate {
		report.End = true
		r.dispatch(driver.Event{Kind: driver.EventSensor, Count: r.id, Data: &report})
		r.close()
	} else if r.packets[0].Offer(report) {
		r.dispatch(driver.Event{Kind: driver.EventSensor, Count: r.id, Data: &r.packets[0]})
	}
}

// Close releases every provider when a surface closes, even without a last batch.
func (s *Sensors) Close() {
	for _, r := range s.runs {
		r.close()
	}
}

func (r *appleSensorRun) close() {
	if r.closed {
		return
	}
	r.closed = true
	if r.extraStop != nil {
		r.extraStop()
		r.extraStop = nil
	}
	delete(r.service.runs, r.id)
	if r.delegate != 0 {
		delete(locationRuns, r.delegate)
		r.manager.Send(sel("setDelegate:"), objc.ID(0))
		if responds(r.manager, "stopUpdatingLocation") {
			r.manager.Send(sel("stopUpdatingLocation"))
		}
		r.delegate.Send(selRelease)
	} else if r.manager != 0 && r.r.Kind == driver.SensorMotion {
		for _, name := range []string{"stopAccelerometerUpdates", "stopGyroUpdates", "stopMagnetometerUpdates", "stopDeviceMotionUpdates"} {
			r.manager.Send(sel(name))
		}
	}
	if r.manager != 0 {
		r.manager.Send(selRelease)
	}
	if r.queue != 0 {
		r.queue.Send(selRelease)
	}
}

var loadMotion = sync.OnceValue(func() bool {
	if runtime.GOOS != "ios" {
		return false
	}
	_, err := purego.Dlopen("/System/Library/Frameworks/CoreMotion.framework/CoreMotion", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	return err == nil && Class("CMMotionManager") != 0
})

func motionGrant() driver.SensorGrant {
	g := driver.SensorGrant{Authorization: driver.SensorGranted}
	class := objc.ID(Class("CMMotionManager"))
	if responds(class, "authorizationStatus") {
		switch objc.Send[int](class, sel("authorizationStatus")) {
		case 0:
			g.Authorization = driver.SensorNotAsked
		case 1:
			g.Authorization = driver.SensorRestricted
		case 2:
			g.Authorization = driver.SensorDenied
		}
	}
	return g
}

func (r *appleSensorRun) motion() {
	g := motionGrant()
	if r.r.Command <= driver.SensorRequestPermission {
		if r.r.Command == driver.SensorReadPermission || g.Authorization != driver.SensorNotAsked {
			r.grant(g)
			return
		}
		if !privacy.DeclaresUsage("NSMotionUsageDescription") {
			r.grant(driver.SensorGrant{Authorization: driver.SensorUnavailable})
			return
		}
		r.manager = objc.ID(Class("CMMotionManager")).Send(sel("new"))
		r.manager.Send(sel("startAccelerometerUpdates"))
		r.pollMotionGrant()
		return
	}
	if g.Authorization != driver.SensorGranted {
		r.fail(grantError(g))
		return
	}
	if !privacy.DeclaresUsage("NSMotionUsageDescription") {
		r.fail("not-configured")
		return
	}
	r.manager = objc.ID(Class("CMMotionManager")).Send(sel("new"))
	types := []struct {
		mask                       driver.MotionSensors
		available, interval, start string
	}{
		{driver.SensorAccelerometer, "isAccelerometerAvailable", "setAccelerometerUpdateInterval:", "startAccelerometerUpdatesToQueue:withHandler:"},
		{driver.SensorGyroscope, "isGyroAvailable", "setGyroUpdateInterval:", "startGyroUpdatesToQueue:withHandler:"},
		{driver.SensorMagnetometer, "isMagnetometerAvailable", "setMagnetometerUpdateInterval:", "startMagnetometerUpdatesToQueue:withHandler:"},
		{driver.SensorDeviceMotion, "isDeviceMotionAvailable", "setDeviceMotionUpdateInterval:", "startDeviceMotionUpdatesUsingReferenceFrame:toQueue:withHandler:"},
	}
	for _, t := range types {
		if r.r.Motion.Sensors&t.mask != 0 && !objc.Send[bool](r.manager, sel(t.available)) {
			r.fail("unavailable")
			return
		}
	}
	r.queue = objc.ID(Class("NSOperationQueue")).Send(sel("new"))
	r.queue.Send(sel("setMaxConcurrentOperationCount:"), 1)
	for i, t := range types {
		if r.r.Motion.Sensors&t.mask == 0 {
			continue
		}
		r.manager.Send(sel(t.interval), r.r.Motion.Interval.Seconds())
		frame := uint(1) // arbitrary horizontal heading, z vertical
		if t.mask == driver.SensorDeviceMotion {
			available := objc.Send[uint](objc.ID(Class("CMMotionManager")), sel("availableAttitudeReferenceFrames"))
			if available&4 != 0 {
				frame = 4
			} // magnetic north
		}
		packet := &r.packets[i+1]
		var previous time.Duration
		block := objcblock.New(func(_ objc.Block, data, failure objc.ID) {
			if failure != 0 {
				if !r.motionFailure.CompareAndSwap(false, true) {
					return
				}
				code := "failed"
				if native := objc.Send[int](failure, sel("code")); native == 105 || native == 111 {
					code = "not-allowed"
				}
				OnMain(func() {
					if !r.closed {
						if g := motionGrant(); g.Authorization != driver.SensorGranted {
							r.fail(grantError(g))
						} else {
							r.fail(code)
						}
					}
				})
				return
			}
			if data == 0 {
				return
			}
			m := readMotionSample(data, t.mask, frame, r.r.Motion.Interval)
			if previous > 0 {
				m.Interval = m.Timestamp - previous
			}
			previous = m.Timestamp
			if packet.Offer(driver.SensorReport{Generation: r.r.Generation, Motion: &m}) {
				r.dispatch(driver.Event{Kind: driver.EventSensor, Count: r.id, Data: packet})
			}
		})
		if t.mask == driver.SensorDeviceMotion {
			r.manager.Send(sel(t.start), frame, r.queue, block)
		} else {
			r.manager.Send(sel(t.start), r.queue, block)
		}
		block.Release()
	}
}

func (r *appleSensorRun) pollMotionGrant() {
	if r.closed {
		return
	}
	g := motionGrant()
	if g.Authorization != driver.SensorNotAsked {
		r.grant(g)
		return
	}
	// Core Motion tells no answer: a prompt still unanswered after 30 seconds answers as it stands, not asked, rather than as a refusal the user never gave.
	if time.Since(r.started) > 30*time.Second {
		r.grant(g)
		return
	}
	time.AfterFunc(100*time.Millisecond, func() { OnMain(r.pollMotionGrant) })
}

type cmVector struct {
	_       structs.HostLayout
	X, Y, Z float64
}
type cmQuaternion struct {
	_          structs.HostLayout
	X, Y, Z, W float64
}
type cmMagnetic struct {
	_        structs.HostLayout
	Field    cmVector
	Accuracy int32
}

// These large return values use objc_msgSend_stret on Intel simulators.
var motionReaders = sync.OnceValue(func() struct {
	vector     func(objc.ID, objc.SEL) cmVector
	quaternion func(objc.ID, objc.SEL) cmQuaternion
	magnetic   func(objc.ID, objc.SEL) cmMagnetic
} {
	var f struct {
		vector     func(objc.ID, objc.SEL) cmVector
		quaternion func(objc.ID, objc.SEL) cmQuaternion
		magnetic   func(objc.ID, objc.SEL) cmMagnetic
	}
	symbol := "objc_msgSend"
	if runtime.GOARCH == "amd64" {
		symbol = "objc_msgSend_stret"
	}
	lib := load("/usr/lib/libobjc.A.dylib")
	purego.RegisterLibFunc(&f.vector, lib, symbol)
	purego.RegisterLibFunc(&f.quaternion, lib, symbol)
	purego.RegisterLibFunc(&f.magnetic, lib, symbol)
	return f
})

func motionVector(v cmVector, scale float64) driver.SensorVector {
	return driver.SensorVector{X: v.X * scale, Y: v.Y * scale, Z: v.Z * scale}
}

func readMotionSample(data objc.ID, sensor driver.MotionSensors, frame uint, interval time.Duration) driver.MotionSample {
	f := motionReaders()
	m := driver.MotionSample{Sensor: sensor, Timestamp: time.Duration(objc.Send[float64](data, sel("timestamp")) * 1e9), Interval: interval, Accuracy: -1}
	switch sensor {
	case driver.SensorAccelerometer:
		m.Vector = motionVector(f.vector(data, sel("acceleration")), -9.80665)
	case driver.SensorGyroscope:
		m.Vector = motionVector(f.vector(data, sel("rotationRate")), 1)
	case driver.SensorMagnetometer:
		m.Vector = motionVector(f.vector(data, sel("magneticField")), 1)
	case driver.SensorDeviceMotion:
		m.Gravity = motionVector(f.vector(data, sel("gravity")), -9.80665)
		m.UserAcceleration = motionVector(f.vector(data, sel("userAcceleration")), -9.80665)
		m.RotationRate = motionVector(f.vector(data, sel("rotationRate")), 1)
		magnetic := f.magnetic(data, sel("magneticField"))
		m.Accuracy = int(magnetic.Accuracy)
		q := f.quaternion(data.Send(sel("attitude")), sel("quaternion"))
		m.Attitude = driver.SensorQuaternion{X: q.X, Y: q.Y, Z: q.Z, W: q.W}
		m.HeadingReferenced = frame == 4 || frame == 8
		if m.HeadingReferenced {
			m.Attitude = northToENU(m.Attitude)
		}
	}
	return m
}

func northToENU(q driver.SensorQuaternion) driver.SensorQuaternion {
	const s = math.Sqrt2 / 2
	return driver.SensorQuaternion{X: s * (q.X - q.Y), Y: s * (q.X + q.Y), Z: s * (q.Z + q.W), W: s * (q.W - q.Z)}
}
