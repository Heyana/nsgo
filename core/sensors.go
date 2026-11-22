package core

import (
	"fmt"
	"math"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// SensorNumber is a measurement with an explicit availability flag.
type SensorNumber = driver.SensorNumber

// LocationFix is a WGS 84 fix, with optional measurements and provider times.
type LocationFix = driver.LocationFix

// LocationOptions chooses accuracy, distance, caching and background policy.
type LocationOptions = driver.LocationOptions

// MotionOptions chooses a sensor set and sample interval.
type MotionOptions = driver.MotionOptions

// MotionSensors is a set of raw and fused device sensors.
type MotionSensors = driver.MotionSensors

const (
	SensorAccelerometer = driver.SensorAccelerometer
	SensorGyroscope     = driver.SensorGyroscope
	SensorMagnetometer  = driver.SensorMagnetometer
	SensorDeviceMotion  = driver.SensorDeviceMotion
)

// SensorVector is a vector in the natural device frame, in SI units (µT for magnetism).
type SensorVector = driver.SensorVector

// SensorQuaternion rotates device axes into world axes.
type SensorQuaternion = driver.SensorQuaternion

// MotionSample is one sensor's latest sample and its dropped-sample count.
type MotionSample = driver.MotionSample

// SensorError is a stable provider failure; it never contains location data.
type SensorError string

const (
	SensorNotAllowed    SensorError = "not-allowed"
	SensorNotAvailable  SensorError = "unavailable"
	SensorTimedOut      SensorError = "timeout"
	SensorFailed        SensorError = "failed"
	SensorNotConfigured SensorError = "not-configured"
)

func (e SensorError) Error() string { return "nsgo: sensor: " + string(e) }

// SensorStream owns a pending location or a live stream. Its callbacks run on the engine goroutine only while its scope is alive.
type SensorStream struct {
	tree                 *Tree
	owner                *Scope
	id                   uint32
	request              driver.SensorRequest
	location             func(LocationFix)
	motion               func(MotionSample)
	steps                func(StepSample)
	activity             func(ActivitySample)
	pressure             func(PressureSample)
	geofence             func(GeofenceSample)
	precise              func(bool)
	permission           func(driver.SensorGrant)
	onError              func(error)
	stopped, paused, gap bool
	stopEffect           func()
	stopTimer            func() bool
	remaining            time.Duration
	started              time.Time
}

// sensorPermissions are the sensors' kinds by their permissions, which PermissionStatus and RequestPermission ask the sensor service for.
var sensorPermissions = map[Permission]driver.SensorKind{
	PermissionLocation:            driver.SensorLocation,
	PermissionBackgroundLocation:  driver.SensorBackgroundLocation,
	PermissionMotion:              driver.SensorMotion,
	PermissionActivityRecognition: driver.SensorActivityPermission,
}

// sensorGate is gate's refusal of the sensor of kind, held to AccessLocation, AccessBackgroundLocation or AccessMotion.
func (cx *Scope) sensorGate(kind driver.SensorKind) error {
	switch kind {
	case driver.SensorBackgroundLocation:
		return cx.gate(serviceBackgroundLocation, "")
	case driver.SensorMotion, driver.SensorSteps, driver.SensorActivity, driver.SensorPressure, driver.SensorActivityPermission:
		return cx.gate(serviceMotion, "")
	}
	return cx.gate(serviceLocation, "")
}

// sensorStatus is the PermissionStatus of a sensor's authorization: a restricted one is denied, as an administrator refused it.
func sensorStatus(a driver.SensorAuthorization) PermissionStatus {
	switch a {
	case driver.SensorNotAsked:
		return PermissionNotAsked
	case driver.SensorGranted:
		return PermissionGranted
	case driver.SensorDenied, driver.SensorRestricted:
		return PermissionDenied
	}
	return PermissionUnavailable
}

// sensorPermission reads, or with ask asks for, the sensor permission of kind through the sensor service, and answers done with its status; it reports false, sending nothing, where the current owner's Access refuses the sensor or no tree serves it. The request ends with the requesting owner.
func (cx *Scope) sensorPermission(kind driver.SensorKind, ask bool, done func(PermissionStatus)) bool {
	command := driver.SensorReadPermission
	if ask {
		command = driver.SensorRequestPermission
	}
	s, err := cx.newSensor(driver.SensorRequest{Kind: kind, Command: command})
	if err != nil {
		return false
	}
	s.permission = func(g driver.SensorGrant) { done(sensorStatus(g.Authorization)) }
	s.send()
	return true
}

// Locate waits for a new provider fix unless MaxAge explicitly allows a cached one, under PermissionLocation, which it never asks for. It waits through background pauses and stops after one fix. The default foreground timeout is 30 seconds; Stop cancels silently.
func (cx *Scope) Locate(opts LocationOptions, done func(LocationFix, error)) (*SensorStream, error) {
	s, err := cx.locationSensor(opts, driver.SensorLocate)
	if err != nil {
		return nil, err
	}
	s.location = func(f LocationFix) {
		if done != nil {
			done(f, nil)
		}
	}
	s.onError = func(err error) {
		if done != nil {
			done(LocationFix{}, err)
		}
	}
	s.remaining = opts.Timeout
	if s.remaining == 0 {
		s.remaining = 30 * time.Second
	}
	s.followLifecycle(cx)
	return s, nil
}

// WatchLocation starts a stream under PermissionLocation, which it never asks for. Foreground streams pause in background and mark their first resumed fix with Gap. Background streams require AccessLocation and AccessBackgroundLocation, the platform's declaration and PermissionBackgroundLocation; they never restart a killed process.
func (cx *Scope) WatchLocation(opts LocationOptions, sample func(LocationFix), failed func(error)) (*SensorStream, error) {
	s, err := cx.locationSensor(opts, driver.SensorWatchLocation)
	if err != nil {
		return nil, err
	}
	s.location, s.onError = sample, failed
	s.followLifecycle(cx)
	return s, nil
}

func (cx *Scope) locationSensor(opts LocationOptions, command driver.SensorCommand) (*SensorStream, error) {
	if math.IsNaN(opts.Accuracy) || math.IsInf(opts.Accuracy, 0) || opts.Accuracy < 0 ||
		math.IsNaN(opts.Distance) || math.IsInf(opts.Distance, 0) || opts.Distance < 0 ||
		opts.Interval < 0 || opts.MaxAge < 0 || opts.Timeout < 0 {
		return nil, fmt.Errorf("nsgo: invalid location options")
	}
	if opts.Background {
		if err := cx.sensorGate(driver.SensorBackgroundLocation); err != nil {
			return nil, err
		}
	}
	return cx.newSensor(driver.SensorRequest{Kind: driver.SensorLocation, Command: command, Location: opts})
}

// WatchMotion starts raw or fused sensors at the requested interval, clamped to 10 ms–1 s, under PermissionMotion, which it never asks for. Zero requests accelerometer at 20 ms. Samples use natural device axes, with +9.80665 m/s² on z face-up, and report dropped samples. Motion always pauses in background; it never keeps a process alive.
func (cx *Scope) WatchMotion(opts MotionOptions, sample func(MotionSample), failed func(error)) (*SensorStream, error) {
	opts, err := normalizeMotionOptions(opts)
	if err != nil {
		return nil, err
	}
	s, err := cx.newSensor(driver.SensorRequest{Kind: driver.SensorMotion, Command: driver.SensorWatchMotion, Motion: opts})
	if err != nil {
		return nil, err
	}
	s.motion, s.onError = sample, failed
	s.followLifecycle(cx)
	return s, nil
}

// newSensor makes the stream of r, held to its sensor's capability.
func (cx *Scope) newSensor(r driver.SensorRequest) (*SensorStream, error) {
	if err := cx.sensorGate(r.Kind); err != nil {
		return nil, err
	}
	t := cx.tree()
	if t == nil || cx.disposed {
		return nil, SensorNotAvailable
	}
	t.alertID++
	s := &SensorStream{tree: t, owner: cx, id: t.alertID, request: r}
	if t.sensors == nil {
		t.sensors = map[uint32]*SensorStream{}
	}
	t.sensors[s.id] = s
	s.owner.addCleanup(s.Stop)
	return s, nil
}

func (s *SensorStream) followLifecycle(cx *Scope) {
	s.paused = true
	s.tree.within(s.owner, func() {
		s.stopEffect = cx.Effect(func() {
			background := s.request.Location.Background || s.request.Geofences.Background || s.request.Command == driver.SensorWatchBackgroundMotion
			pause := cx.Env().Lifecycle() == LifecycleBackground && !background
			if s.stopped || pause == s.paused {
				return
			}
			s.paused = pause
			if pause {
				s.gap = true
				if s.stopTimer != nil {
					s.stopTimer()
					s.remaining = max(0, s.remaining-s.owner.Now().Sub(s.started))
				}
				s.command(driver.SensorStop)
			} else {
				s.request.Generation++
				s.send()
				if s.request.Command == driver.SensorLocate {
					s.tree.within(s.owner, s.armTimeout)
				}
			}
		})
	})
}

func (s *SensorStream) armTimeout() {
	s.started = s.owner.Now()
	s.stopTimer = s.owner.AfterFunc(s.remaining, func() {
		if !s.stopped && !s.paused {
			s.fail(SensorTimedOut)
		}
	})
}

func (s *SensorStream) send() { s.command(s.request.Command) }

func (s *SensorStream) command(command driver.SensorCommand) {
	r := s.request
	r.Command = command
	s.tree.commands = append(s.tree.commands, driver.MountItem{Kind: driver.ItemSensor, Count: s.id, Sensor: &r})
	s.tree.changed = true
}

// Stop cancels this request and its provider; queued answers are ignored.
func (s *SensorStream) Stop() {
	if s == nil || s.stopped {
		return
	}
	s.stopped = true
	delete(s.tree.sensors, s.id)
	if s.stopEffect != nil {
		s.stopEffect()
	}
	if s.stopTimer != nil {
		s.stopTimer()
	}
	s.command(driver.SensorStop)
}

// Stopped reports cancellation, completion or a terminal provider failure.
func (s *SensorStream) Stopped() bool { return s == nil || s.stopped }

func (s *SensorStream) fail(err error) {
	s.Stop()
	if !s.owner.disposed && s.onError != nil {
		s.tree.within(s.owner, func() { s.onError(err) })
	}
}

func (t *Tree) sensorEvent(ev driver.Event) {
	var r driver.SensorReport
	switch v := ev.Data.(type) {
	case *driver.SensorReport:
		r = *v
	case *driver.SensorPacket:
		var ok bool
		r, ok = v.Take()
		if !ok {
			return
		}
	default:
		return
	}
	s := t.sensors[ev.Count]
	if s == nil || s.stopped || s.paused || s.owner.disposed || r.Generation != s.request.Generation {
		return
	}
	if r.Error != "" {
		s.fail(SensorError(r.Error))
		return
	}
	if r.Grant != nil {
		s.Stop()
		if s.permission != nil {
			t.within(s.owner, func() { s.permission(*r.Grant) })
		}
		return
	}
	if r.Location != nil && s.location != nil {
		f := *r.Location
		f.Gap, s.gap = f.Gap || s.gap, false
		if s.request.Command == driver.SensorLocate {
			s.Stop()
		}
		t.within(s.owner, func() { s.location(f) })
	}
	if r.Motion != nil && s.motion != nil {
		m := *r.Motion
		m.Gap, s.gap = m.Gap || s.gap, false
		t.within(s.owner, func() { s.motion(m) })
	}
	if r.Steps != nil && s.steps != nil {
		v := *r.Steps
		v.Gap, s.gap = v.Gap || s.gap, false
		t.within(s.owner, func() { s.steps(v) })
	}
	if r.Activity != nil && s.activity != nil {
		v := *r.Activity
		v.Gap, s.gap = v.Gap || s.gap, false
		t.within(s.owner, func() { s.activity(v) })
	}
	if r.Pressure != nil && s.pressure != nil {
		v := *r.Pressure
		v.Gap, s.gap = v.Gap || s.gap, false
		t.within(s.owner, func() { s.pressure(v) })
	}
	if r.Geofence != nil && s.geofence != nil {
		v := *r.Geofence
		v.Gap, s.gap = v.Gap || s.gap, false
		t.within(s.owner, func() { s.geofence(v) })
	}
	if r.Precise != nil && s.precise != nil {
		s.Stop()
		t.within(s.owner, func() { s.precise(r.Precise.Full) })
	}
	if r.End {
		s.Stop()
	}
}
