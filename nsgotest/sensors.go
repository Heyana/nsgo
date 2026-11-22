package nsgotest

import (
	"cmp"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

type sensorRun struct {
	request      driver.SensorRequest
	packets      map[driver.MotionSensors]*driver.SensorPacket
	extraPackets map[string]*driver.SensorPacket
}

// sensorPermission are the sensors' permissions, which the model keeps in Permissions as it does every permission.
var sensorPermission = map[driver.SensorKind]core.Permission{
	driver.SensorLocation: core.PermissionLocation, driver.SensorBackgroundLocation: core.PermissionBackgroundLocation,
	driver.SensorMotion: core.PermissionMotion, driver.SensorSteps: core.PermissionActivityRecognition,
	driver.SensorActivity: core.PermissionActivityRecognition, driver.SensorPressure: core.PermissionMotion,
	driver.SensorGeofences: core.PermissionLocation, driver.SensorActivityPermission: core.PermissionActivityRecognition,
}

// sensorAuthorization is how a platform's sensor service tells the permission status s.
func sensorAuthorization(s core.PermissionStatus) driver.SensorAuthorization {
	switch s {
	case core.PermissionNotAsked:
		return driver.SensorNotAsked
	case core.PermissionGranted, core.PermissionNotGated:
		return driver.SensorGranted
	case core.PermissionDenied:
		return driver.SensorDenied
	}
	return driver.SensorUnavailable
}

// sensorRefusal is the error a provider fails with for want of permission p, "" when the app has it: a stream never asks.
func (a *App) sensorRefusal(p core.Permission) string {
	switch s := a.Permissions[p]; {
	case s.Allowed():
		return ""
	case s == core.PermissionUnavailable:
		return string(core.SensorNotAvailable)
	}
	return string(core.SensorNotAllowed)
}

// sensor answers an ItemSensor from Permissions, LocationCache and the fixes and samples a test feeds.
func (a *App) sensor(id uint32, r driver.SensorRequest) {
	if r.Command == driver.SensorStop {
		delete(a.sensorRuns, id)
		return
	}
	report := func(e driver.SensorReport) {
		e.Generation = r.Generation
		a.post(driver.Event{Kind: driver.EventSensor, Count: id, Data: &e})
	}
	p := sensorPermission[r.Kind]
	if r.Command <= driver.SensorRequestPermission {
		if r.Command == driver.SensorRequestPermission && a.Permissions[p] == core.PermissionNotAsked {
			a.Permissions[p] = cmp.Or(a.PermissionAnswers[p], core.PermissionGranted)
		}
		report(driver.SensorReport{Grant: &driver.SensorGrant{Authorization: sensorAuthorization(a.Permissions[p])}, End: true})
		return
	}
	if code := a.sensorRefusal(p); code != "" {
		report(driver.SensorReport{Error: code, End: true})
		return
	}
	if r.Location.Background || r.Geofences.Background {
		if code := a.sensorRefusal(core.PermissionBackgroundLocation); code != "" {
			report(driver.SensorReport{Error: code, End: true})
			return
		}
	}
	if r.Command == driver.SensorRequestPreciseLocation {
		report(driver.SensorReport{Precise: &driver.PreciseLocationAnswer{Full: a.PreciseLocationAnswer}, End: true})
		return
	}
	if r.Command == driver.SensorLocate && r.Location.MaxAge > 0 && a.LocationCache != nil {
		f := *a.LocationCache
		f.Age = a.Clock.Now().Sub(f.Timestamp)
		if f.Age >= 0 && f.Age <= r.Location.MaxAge {
			f.Cached = true
			report(driver.SensorReport{Location: &f, End: true})
			return
		}
	}
	if a.sensorRuns == nil {
		a.sensorRuns = map[uint32]*sensorRun{}
	}
	a.sensorRuns[id] = &sensorRun{request: r, packets: map[driver.MotionSensors]*driver.SensorPacket{}}
}

func (a *App) sensorSample(id uint32, run *sensorRun, key driver.MotionSensors, r driver.SensorReport) {
	p := run.packets[key]
	if p == nil {
		p = &driver.SensorPacket{}
		run.packets[key] = p
	}
	r.Generation = run.request.Generation
	if p.Offer(r) {
		a.post(driver.Event{Kind: driver.EventSensor, Count: id, Data: p})
	}
}

// FeedLocation delivers a copied fix to active location requests. Call
// several times before Settle to test latest-value back-pressure.
func (a *App) FeedLocation(f core.LocationFix) {
	a.requireSensorModel()
	for id, run := range a.sensorRuns {
		if run.request.Kind == driver.SensorLocation {
			copy := f
			a.sensorSample(id, run, 0, driver.SensorReport{Location: &copy})
		}
	}
}

// FeedMotion delivers a copied reading to streams requesting that sensor.
func (a *App) FeedMotion(m core.MotionSample) {
	a.requireSensorModel()
	for id, run := range a.sensorRuns {
		if run.request.Kind == driver.SensorMotion && run.request.Motion.Sensors&m.Sensor != 0 {
			copy := m
			copy.Interval = run.request.Motion.Interval
			a.sensorSample(id, run, m.Sensor, driver.SensorReport{Motion: &copy})
		}
	}
}

// ActiveSensorStreams is the number of native providers the model holds.
func (a *App) ActiveSensorStreams() int { a.requireSensorModel(); return len(a.sensorRuns) }

func (a *App) requireSensorModel() {
	a.t.Helper()
	if a.target != nil {
		a.t.Skip("nsgotest: synthetic sensors run headless only")
	}
}
