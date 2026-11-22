package core

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// StepSample counts steps in the provider's explicit observation window.
type StepSample = driver.StepSample

// ActivityKinds is a set of native activity classifications.
type ActivityKinds = driver.ActivityKinds

// ActivityConfidence is the provider's unknown, low, medium or high confidence.
type ActivityConfidence = driver.ActivityConfidence

// ActivitySample reports native activity flags and confidence.
type ActivitySample = driver.ActivitySample

// PressureSample reports pascals and optional relative altitude in meters.
type PressureSample = driver.PressureSample

// GeofenceRegion is a WGS 84 circle with a stream-local identifier.
type GeofenceRegion = driver.GeofenceRegion

// GeofenceState is unknown, inside or outside.
type GeofenceState = driver.GeofenceState

// GeofenceSample is the latest state for one monitored region.
type GeofenceSample = driver.GeofenceSample

// GeofenceOptions chooses circles and an explicit background policy.
type GeofenceOptions = driver.GeofenceOptions

// BackgroundMotionOptions bounds a live motion stream and its execution lease.
type BackgroundMotionOptions = driver.BackgroundMotionOptions

const (
	ActivityStationary        = driver.ActivityStationary
	ActivityWalking           = driver.ActivityWalking
	ActivityRunning           = driver.ActivityRunning
	ActivityCycling           = driver.ActivityCycling
	ActivityAutomotive        = driver.ActivityAutomotive
	ActivityUnknown           = driver.ActivityUnknown
	ActivityConfidenceUnknown = driver.ActivityConfidenceUnknown
	ActivityConfidenceLow     = driver.ActivityConfidenceLow
	ActivityConfidenceMedium  = driver.ActivityConfidenceMedium
	ActivityConfidenceHigh    = driver.ActivityConfidenceHigh
	GeofenceUnknown           = driver.GeofenceUnknown
	GeofenceInside            = driver.GeofenceInside
	GeofenceOutside           = driver.GeofenceOutside
)

// WatchSteps follows the provider's cumulative steps in an explicit observation window, under AccessMotion and PermissionActivityRecognition. It never prompts. A lifecycle restart starts a new Since window and marks its first sample Gap.
func (cx *Scope) WatchSteps(sample func(StepSample), failed func(error)) (*SensorStream, error) {
	s, err := cx.newSensor(driver.SensorRequest{Kind: driver.SensorSteps, Command: driver.SensorWatchSteps})
	if err != nil {
		return nil, err
	}
	s.steps, s.onError = sample, failed
	s.followLifecycle(cx)
	return s, nil
}

// WatchActivity follows native fitness classifications under AccessMotion and PermissionActivityRecognition. It does not infer walking or driving from raw acceleration; a platform with no classifier reports SensorNotAvailable.
func (cx *Scope) WatchActivity(sample func(ActivitySample), failed func(error)) (*SensorStream, error) {
	s, err := cx.newSensor(driver.SensorRequest{Kind: driver.SensorActivity, Command: driver.SensorWatchActivity})
	if err != nil {
		return nil, err
	}
	s.activity, s.onError = sample, failed
	s.followLifecycle(cx)
	return s, nil
}

// WatchPressure follows barometric pressure in pascals and optional relative altitude under AccessMotion. Apple requires the Motion & Fitness grant (PermissionActivityRecognition); Android needs no runtime permission. It never invents altitude from pressure and never prompts for authorization.
func (cx *Scope) WatchPressure(sample func(PressureSample), failed func(error)) (*SensorStream, error) {
	s, err := cx.newSensor(driver.SensorRequest{Kind: driver.SensorPressure, Command: driver.SensorWatchPressure})
	if err != nil {
		return nil, err
	}
	s.pressure, s.onError = sample, failed
	s.followLifecycle(cx)
	return s, nil
}

// WatchGeofences monitors up to twenty WGS 84 circles under AccessLocation and PermissionLocation. Background also requires AccessBackgroundLocation and the platform's background authorization/declarations. Stop removes only its regions; no subscription is restored after process death. The OS can wake an app for a registered region; callbacks belong only to the scope that created it. Samples are latest state per region.
func (cx *Scope) WatchGeofences(opts GeofenceOptions, sample func(GeofenceSample), failed func(error)) (*SensorStream, error) {
	if len(opts.Regions) == 0 || len(opts.Regions) > 20 {
		return nil, fmt.Errorf("nsgo: geofencing needs one to twenty regions")
	}
	ids := make(map[string]bool, len(opts.Regions))
	for _, r := range opts.Regions {
		if r.ID == "" || !utf8.ValidString(r.ID) || strings.IndexByte(r.ID, 0) >= 0 || ids[r.ID] ||
			math.IsNaN(r.Latitude) || math.IsInf(r.Latitude, 0) || math.Abs(r.Latitude) > 90 ||
			math.IsNaN(r.Longitude) || math.IsInf(r.Longitude, 0) || math.Abs(r.Longitude) > 180 ||
			math.IsNaN(r.Radius) || math.IsInf(r.Radius, 0) || r.Radius <= 0 {
			return nil, fmt.Errorf("nsgo: invalid geofence region")
		}
		ids[r.ID] = true
	}
	if opts.Background {
		if err := cx.sensorGate(driver.SensorBackgroundLocation); err != nil {
			return nil, err
		}
	}
	// Keep the request independent from subsequent app edits to its slice.
	opts.Regions = append([]GeofenceRegion(nil), opts.Regions...)
	s, err := cx.newSensor(driver.SensorRequest{Kind: driver.SensorGeofences, Command: driver.SensorWatchGeofences, Geofences: opts})
	if err != nil {
		return nil, err
	}
	s.geofence, s.onError = sample, failed
	s.followLifecycle(cx)
	return s, nil
}

// RequestPreciseLocation explicitly asks for a temporary full-accuracy location grant. purpose is an Apple NSLocationTemporaryUsageDescriptionDictionary key; Android asks for its declared fine-location permission. It returns authorization, not a position, and a denied accuracy upgrade may answer full=false without revoking approximate location. Disposal suppresses the pending answer.
func (cx *Scope) RequestPreciseLocation(purpose string, done func(full bool, err error)) (*SensorStream, error) {
	if purpose == "" || !utf8.ValidString(purpose) || strings.IndexByte(purpose, 0) >= 0 {
		return nil, fmt.Errorf("nsgo: precise location needs a purpose key")
	}
	s, err := cx.newSensor(driver.SensorRequest{Kind: driver.SensorLocation, Command: driver.SensorRequestPreciseLocation, Purpose: purpose})
	if err != nil {
		return nil, err
	}
	s.precise = func(full bool) {
		if done != nil {
			done(full, nil)
		}
	}
	s.onError = func(err error) {
		if done != nil {
			done(false, err)
		}
	}
	s.send()
	return s, nil
}

// WatchBackgroundMotion requests a live motion stream with an explicit duration, held separately by AccessMotion and AccessBackgroundMotion. Apple may expire its execution lease sooner. Android requires an explicitly declared foreground service and a notification title. It stops with its scope and never records after process termination. Duration or early OS lease expiry answers SensorTimedOut; it is not CMSensorRecorder's uncancellable recording.
func (cx *Scope) WatchBackgroundMotion(opts BackgroundMotionOptions, sample func(MotionSample), failed func(error)) (*SensorStream, error) {
	if err := cx.gate(serviceBackgroundMotion, ""); err != nil {
		return nil, err
	}
	if opts.Duration <= 0 {
		return nil, fmt.Errorf("nsgo: background motion needs a positive duration")
	}
	m, err := normalizeMotionOptions(opts.Motion)
	if err != nil {
		return nil, err
	}
	opts.Motion = m
	s, err := cx.newSensor(driver.SensorRequest{Kind: driver.SensorMotion, Command: driver.SensorWatchBackgroundMotion, Motion: m, BackgroundMotion: opts})
	if err != nil {
		return nil, err
	}
	s.motion, s.onError = sample, failed
	s.followLifecycle(cx)
	s.tree.within(s.owner, func() {
		s.stopTimer = s.owner.AfterFunc(opts.Duration, func() {
			if !s.stopped {
				s.fail(SensorTimedOut)
			}
		})
	})
	return s, nil
}

func normalizeMotionOptions(opts MotionOptions) (MotionOptions, error) {
	if opts.Sensors == 0 {
		opts.Sensors = SensorAccelerometer
	}
	if opts.Sensors & ^(SensorAccelerometer|SensorGyroscope|SensorMagnetometer|SensorDeviceMotion) != 0 || opts.Interval < 0 {
		return opts, fmt.Errorf("nsgo: invalid motion options")
	}
	if opts.Interval == 0 {
		opts.Interval = 20 * time.Millisecond
	}
	opts.Interval = min(max(opts.Interval, 10*time.Millisecond), time.Second)
	return opts, nil
}
