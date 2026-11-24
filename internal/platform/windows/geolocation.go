//go:build windows

package windows

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Location comes from WinRT's Geolocator. A one-shot fix awaits GetGeopositionAsync, a stream listens to PositionChanged, and a stop cancels the operation or removes the handler. The permission reads as denied while a switch of location's in the consent store, or a locator's status, says location is off for the app, as granted once RequestAccessAsync allowed it in this process, and otherwise as not asked. Background location is unavailable: a desktop app has no background state to keep a stream going in.

type windowsGeoRun struct {
	w         *window
	id        uint32
	gen       uint32
	locator   *comObject
	op        *comObject
	asyncInfo *comObject
	handler   *comHandler
	token     eventToken
	packet    driver.SensorPacket
}

var windowsGeoRuns = struct {
	sync.Mutex
	m map[*windowsGeoRun]struct{}
}{m: map[*windowsGeoRun]struct{}{}}

var activateGeolocator = func() *comObject { return activate(geolocatorClass, &iidGeolocator) }

// windowsGeoGrant is the last GeolocationAccessStatus RequestAccessAsync answered in this process as a SensorAuthorization, SensorNotAsked before one did.
var windowsGeoGrant atomic.Uint32

var (
	iidGeolocator          = parseGUID("a9c3bf62-4524-4989-8aa9-de019d2e551f")
	iidGeolocationComplete = parseGUID("7668a704-244e-5e12-8dcb-92a3299eba26")
	iidGeocoordinate       = parseGUID("ee21a3aa-976a-4c70-803d-083ea55bcbc4")
	iidPositionHandler     = parseGUID("df3c6164-4e7b-5e8e-9a7e-13da059dec1e")
	iidGeolocatorStatics   = parseGUID("9a8e7571-2df5-4591-9f87-eb5fd894e9b7")
	iidAccessComplete      = parseGUID("f3524c93-e5c7-5b88-bedb-d3e637cff271")
)

const (
	geolocatorClass             = "Windows.Devices.Geolocation.Geolocator"
	geolocatorPutAccuracy       = 7
	geolocatorPutMovement       = 9
	geolocatorPutReportInterval = 11
	geolocatorLocationStatus    = 12
	geolocatorGetPosition       = 13
	geoAddPositionChanged       = 15
	geoRemovePositionChanged    = 16
	geoPositionCoordinate       = 6
	geoCoordinateLatitude       = 6
	geoCoordinateLongitude      = 7
	geoCoordinateAccuracy       = 9
	geoCoordinateTimestamp      = 13
	geoChangedPosition          = 6
	geoAsyncCancel              = 9
	geoAsyncClose               = 10
	geoRequestAccess            = 6
	geoAsyncPutCompleted        = asyncPutCompleted
	geoAsyncGetResults          = asyncGetResults
	geoAsyncCompleted           = asyncStatusCompleted
	geoAsyncError               = 3

	// PositionStatus values a locator reports before it has a fix.
	geoStatusDisabled     = 3
	geoStatusNotAvailable = 5
	// GeolocationAccessStatus values.
	geoAccessAllowed = 1
	geoAccessDenied  = 2
)

// cancelWindowsLocation ends the run of stream id at generation, if it is still going. The runs leave the map under its lock and are released outside it, as IAsyncInfo.Cancel may call the completion handler, which takes the lock, on this thread.
func cancelWindowsLocation(w *window, id, generation uint32) {
	for _, run := range takeWindowsGeoRuns(func(r *windowsGeoRun) bool { return r.w == w && r.id == id && r.gen == generation }) {
		run.end()
	}
}

// endWindowLocation ends the window's runs as it is destroyed.
func endWindowLocation(w *window) {
	for _, run := range takeWindowsGeoRuns(func(r *windowsGeoRun) bool { return r.w == w }) {
		run.end()
	}
}

func takeWindowsGeoRuns(match func(*windowsGeoRun) bool) []*windowsGeoRun {
	windowsGeoRuns.Lock()
	defer windowsGeoRuns.Unlock()
	var runs []*windowsGeoRun
	for run := range windowsGeoRuns.m {
		if match(run) {
			delete(windowsGeoRuns.m, run)
			runs = append(runs, run)
		}
	}
	return runs
}

// claim removes run from the map and reports whether it was there: whoever removes a run releases it.
func (run *windowsGeoRun) claim() bool {
	windowsGeoRuns.Lock()
	defer windowsGeoRuns.Unlock()
	_, ok := windowsGeoRuns.m[run]
	delete(windowsGeoRuns.m, run)
	return ok
}

func (run *windowsGeoRun) active() bool {
	windowsGeoRuns.Lock()
	defer windowsGeoRuns.Unlock()
	_, ok := windowsGeoRuns.m[run]
	return ok
}

// end cancels a run taken from the map: it removes a stream's handler, or cancels and closes a pending operation, and releases the locator.
func (run *windowsGeoRun) end() {
	if run.handler != nil {
		if run.locator != nil {
			run.locator.call(geoRemovePositionChanged, uintptr(run.token.value))
		}
		run.handler.release()
	}
	if run.op != nil {
		if run.asyncInfo != nil {
			run.asyncInfo.call(geoAsyncCancel)
			run.asyncInfo.call(geoAsyncClose)
			run.asyncInfo.release()
		}
		run.op.release()
	}
	if run.locator != nil {
		run.locator.release()
	}
}

func (w *window) sensorAnswer(id uint32, r *driver.SensorReport) {
	w.engine.Dispatch(driver.Event{Kind: driver.EventSensor, Count: id, Data: r})
}

// sensorLocation serves an ItemSensor other than a stop on the window's thread. WinRT completes operations and raises events off it; only copied fixes cross back.
func (w *window) sensorLocation(id uint32, req driver.SensorRequest) {
	if req.Kind != driver.SensorLocation || req.Location.Background {
		w.engine.Dispatch(driver.NoSensor(&driver.MountItem{Count: id, Sensor: &req}))
		return
	}
	switch req.Command {
	case driver.SensorReadPermission:
		w.readWindowsLocationPermission(id, req)
	case driver.SensorRequestPermission:
		w.requestWindowsLocationPermission(id, req)
	case driver.SensorLocate:
		w.locateWindows(id, req)
	case driver.SensorWatchLocation:
		w.watchWindowsLocation(id, req)
	default:
		w.engine.Dispatch(driver.NoSensor(&driver.MountItem{Count: id, Sensor: &req}))
	}
}

// windowsLocationStatus is a new locator's PositionStatus, which says whether location is off for the app without asking, or false without a locator.
func windowsLocationStatus(locator *comObject) (int32, bool) {
	var status int32
	if failed(locator.call(geolocatorLocationStatus, uintptr(unsafe.Pointer(&status)))) {
		return 0, false
	}
	return status, true
}

func (w *window) readWindowsLocationPermission(id uint32, req driver.SensorRequest) {
	comApartment()
	auth := driver.SensorUnavailable
	if locator := activateGeolocator(); locator != nil {
		status, ok := windowsLocationStatus(locator)
		locator.release()
		switch {
		case !ok || status == geoStatusNotAvailable:
		case status == geoStatusDisabled:
			auth = driver.SensorDenied
		default:
			auth = windowsLocationConsent()
		}
	}
	w.sensorAnswer(id, &driver.SensorReport{Generation: req.Generation, Grant: &driver.SensorGrant{Authorization: auth}, End: true})
}

// windowsLocationConsent reads location's switches in the consent store, as permissionStatus does the microphone's: any switch off denies it, as does a refusal this process was answered. An app whose own switch is on reads as granted, as does one allowed when it asked in this process; any other as not asked, since Windows may still ask it as it first locates.
func windowsLocationConsent() driver.SensorAuthorization {
	path := consentStore + "location"
	family := packageFamilyName()
	app := family
	if family == "" {
		app = "NonPackaged"
	}
	machine := regString(syscall.HKEY_LOCAL_MACHINE, 0, path, "Value")
	user := regString(syscall.HKEY_CURRENT_USER, 0, path, "Value")
	own := regString(syscall.HKEY_CURRENT_USER, 0, path+`\`+app, "Value")
	grant := driver.SensorAuthorization(windowsGeoGrant.Load())
	if consentStatus(machine, user, own, family != "") == driver.PermissionDenied || grant == driver.SensorDenied {
		return driver.SensorDenied
	}
	if strings.EqualFold(own, "Allow") {
		return driver.SensorGranted
	}
	return grant
}

func (w *window) requestWindowsLocationPermission(id uint32, req driver.SensorRequest) {
	comApartment()
	statics := activationFactory(geolocatorClass, &iidGeolocatorStatics)
	if statics == nil {
		w.engine.Dispatch(driver.NoSensor(&driver.MountItem{Count: id, Sensor: &req}))
		return
	}
	var operation *comObject
	if failed(statics.call(geoRequestAccess, uintptr(unsafe.Pointer(&operation)))) || operation == nil {
		statics.release()
		w.engine.Dispatch(driver.NoSensor(&driver.MountItem{Count: id, Sensor: &req}))
		return
	}
	completed := newHandler(&iidAccessComplete, func(_ uintptr, status unsafe.Pointer) {
		var value int32
		if uintptr(status) != geoAsyncCompleted || failed(operation.call(geoAsyncGetResults, uintptr(unsafe.Pointer(&value)))) {
			value = 0
		}
		operation.release()
		statics.release()
		auth := driver.SensorUnavailable
		switch value {
		case geoAccessAllowed:
			auth = driver.SensorGranted
		case geoAccessDenied:
			auth = driver.SensorDenied
		}
		if auth != driver.SensorUnavailable {
			windowsGeoGrant.Store(uint32(auth))
		}
		w.post(func() {
			if !w.destroying {
				w.sensorAnswer(id, &driver.SensorReport{Generation: req.Generation, Grant: &driver.SensorGrant{Authorization: auth}, End: true})
			}
		})
	})
	completed.agile = true
	hr := operation.call(geoAsyncPutCompleted, completed.ptr())
	completed.release()
	if failed(hr) {
		operation.release()
		statics.release()
		w.engine.Dispatch(driver.NoSensor(&driver.MountItem{Count: id, Sensor: &req}))
	}
}

// configureGeolocator applies the request's accuracy, distance and interval: high accuracy for 50 m or better, as the web asks.
func configureGeolocator(locator *comObject, opts driver.LocationOptions) {
	if opts.Accuracy > 0 && opts.Accuracy <= 50 {
		locator.call(geolocatorPutAccuracy, 1)
	}
	if opts.Distance > 0 {
		locator.callDouble(geolocatorPutMovement, opts.Distance)
	}
	if ms := opts.Interval.Milliseconds(); ms > 0 {
		locator.call(geolocatorPutReportInterval, uintptr(uint32(min(ms, int64(^uint32(0))))))
	}
}

func (w *window) locateWindows(id uint32, req driver.SensorRequest) {
	comApartment()
	locator := activateGeolocator()
	if locator == nil {
		w.sensorAnswer(id, &driver.SensorReport{Generation: req.Generation, Error: "unavailable", End: true})
		return
	}
	configureGeolocator(locator, req.Location)
	var operation *comObject
	if failed(locator.call(geolocatorGetPosition, uintptr(unsafe.Pointer(&operation)))) || operation == nil {
		locator.release()
		w.sensorAnswer(id, &driver.SensorReport{Generation: req.Generation, Error: "unavailable", End: true})
		return
	}
	run := &windowsGeoRun{w: w, id: id, gen: req.Generation, locator: locator, op: operation, asyncInfo: operation.query(&iidAsyncInfo)}
	windowsGeoRuns.Lock()
	windowsGeoRuns.m[run] = struct{}{}
	windowsGeoRuns.Unlock()
	completed := newHandler(&iidGeolocationComplete, func(_ uintptr, status unsafe.Pointer) {
		// A stopped run was released by its stop, which also cancelled the operation that calls this.
		if !run.claim() {
			return
		}
		report := &driver.SensorReport{Generation: req.Generation, End: true}
		switch uintptr(status) {
		case geoAsyncCompleted:
			var position *comObject
			if failed(operation.call(geoAsyncGetResults, uintptr(unsafe.Pointer(&position)))) || position == nil {
				report.Error = "unavailable"
				break
			}
			fix, err := windowsGeoPosition(position)
			position.release()
			if err != nil {
				report.Error = "unavailable"
			} else {
				report.Location = &fix
			}
		case geoAsyncError:
			report.Error = "unavailable"
			if asyncErrorCode(operation) == hrAccessDenied {
				report.Error = "not-allowed"
			}
		default:
			report.Error = "unavailable"
		}
		if run.asyncInfo != nil {
			run.asyncInfo.call(geoAsyncClose)
			run.asyncInfo.release()
		}
		operation.release()
		locator.release()
		w.post(func() {
			if !w.destroying {
				w.sensorAnswer(id, report)
			}
		})
	})
	completed.agile = true
	hr := operation.call(geoAsyncPutCompleted, completed.ptr())
	completed.release()
	if failed(hr) && run.claim() {
		run.end()
		w.sensorAnswer(id, &driver.SensorReport{Generation: req.Generation, Error: "unavailable", End: true})
	}
}

func (w *window) watchWindowsLocation(id uint32, req driver.SensorRequest) {
	comApartment()
	locator := activateGeolocator()
	if locator == nil {
		w.sensorAnswer(id, &driver.SensorReport{Generation: req.Generation, Error: "unavailable", End: true})
		return
	}
	// PositionChanged raises nothing while location is off for the app, so a stream that would never deliver fails at once instead.
	if status, ok := windowsLocationStatus(locator); ok && (status == geoStatusDisabled || status == geoStatusNotAvailable) {
		locator.release()
		code := "unavailable"
		if status == geoStatusDisabled {
			code = "not-allowed"
		}
		w.sensorAnswer(id, &driver.SensorReport{Generation: req.Generation, Error: code, End: true})
		return
	}
	configureGeolocator(locator, req.Location)
	run := &windowsGeoRun{w: w, id: id, gen: req.Generation, locator: locator}
	run.handler = newHandler(&iidPositionHandler, func(_ uintptr, args unsafe.Pointer) {
		if args == nil || !run.active() {
			return
		}
		var position *comObject
		if failed((*comObject)(args).call(geoChangedPosition, uintptr(unsafe.Pointer(&position)))) || position == nil {
			return
		}
		fix, err := windowsGeoPosition(position)
		position.release()
		if err != nil {
			return
		}
		if run.packet.Offer(driver.SensorReport{Generation: req.Generation, Location: &fix}) {
			w.post(func() {
				if !w.destroying {
					w.engine.Dispatch(driver.Event{Kind: driver.EventSensor, Count: id, Data: &run.packet})
				}
			})
		}
	})
	run.handler.agile = true
	windowsGeoRuns.Lock()
	windowsGeoRuns.m[run] = struct{}{}
	windowsGeoRuns.Unlock()
	if failed(locator.call(geoAddPositionChanged, run.handler.ptr(), uintptr(unsafe.Pointer(&run.token)))) {
		if run.claim() {
			run.handler.release()
			locator.release()
		}
		w.sensorAnswer(id, &driver.SensorReport{Generation: req.Generation, Error: "unavailable", End: true})
	}
}

// windowsGeoPosition copies a Geoposition's coordinate. Its timestamp is a DateTime, 100 ns ticks since 1601.
func windowsGeoPosition(position *comObject) (driver.LocationFix, error) {
	var coordinate *comObject
	if failed(position.call(geoPositionCoordinate, uintptr(unsafe.Pointer(&coordinate)))) || coordinate == nil {
		return driver.LocationFix{}, errors.New("geolocation returned no coordinate")
	}
	defer coordinate.release()
	var lat, lon, accuracy float64
	if failed(coordinate.call(geoCoordinateLatitude, uintptr(unsafe.Pointer(&lat)))) || failed(coordinate.call(geoCoordinateLongitude, uintptr(unsafe.Pointer(&lon)))) {
		return driver.LocationFix{}, errors.New("geolocation coordinate is invalid")
	}
	accuracyValid := !failed(coordinate.call(geoCoordinateAccuracy, uintptr(unsafe.Pointer(&accuracy))))
	var rawTimestamp int64
	when := time.Now()
	if !failed(coordinate.call(geoCoordinateTimestamp, uintptr(unsafe.Pointer(&rawTimestamp)))) {
		when = time.Unix(0, (rawTimestamp-116444736000000000)*100)
	}
	return driver.LocationFix{Latitude: lat, Longitude: lon, HorizontalAccuracy: driver.SensorNumber{Value: accuracy, Valid: accuracyValid && accuracy >= 0}, Timestamp: when, Age: max(0, time.Since(when))}, nil
}
