package driver

import (
	"sync"
	"time"
)

// SensorKind identifies a provider and its separately requested authorization: core.PermissionLocation, PermissionBackgroundLocation and PermissionMotion.
type SensorKind uint8

const (
	SensorLocation SensorKind = iota
	SensorBackgroundLocation
	SensorMotion
)

// SensorAuthorization is a provider's current authorization, without prompting, which core answers as a core.PermissionStatus.
type SensorAuthorization uint8

const (
	SensorNotAsked SensorAuthorization = iota
	SensorGranted
	SensorDenied
	SensorRestricted
	SensorUnavailable
)

// SensorGrant answers SensorReadPermission and SensorRequestPermission.
type SensorGrant struct {
	Authorization SensorAuthorization
}

// SensorNumber distinguishes a missing measurement from a measured zero.
type SensorNumber struct {
	Value float64
	Valid bool
}

// LocationFix is a copied WGS 84 fix. Timestamp is wall time; Monotonic is time since boot where available. Altitude is ellipsoidal, MSLAltitude is above mean sea level. Distances are meters, speed m/s and course degrees. Approximate says the user allowed the app only an approximate location as the fix came: Apple's reduced accuracy, or Android's coarse location without the fine one.
type LocationFix struct {
	Latitude, Longitude                                         float64
	Altitude, MSLAltitude, HorizontalAccuracy, VerticalAccuracy SensorNumber
	Speed, Course                                               SensorNumber
	Timestamp                                                   time.Time
	Monotonic                                                   time.Duration
	HasMonotonic                                                bool
	Cached                                                      bool
	Approximate                                                 bool
	Age                                                         time.Duration
	Gap                                                         bool
	Dropped                                                     uint64
}

// LocationOptions are explicit provider settings. A zero MaxAge refuses
// cached fixes. Background requires a separately granted capability and a
// native background declaration; NotificationTitle and Body name Android's
// foreground-service notification. Timeout defaults to 30 seconds for Locate.
type LocationOptions struct {
	Accuracy, Distance                  float64
	Interval, MaxAge, Timeout           time.Duration
	Background                          bool
	NotificationTitle, NotificationBody string
}

// MotionSensors is a set of raw sensors and the fused device-motion sensor.
type MotionSensors uint8

const (
	SensorAccelerometer MotionSensors = 1 << iota
	SensorGyroscope
	SensorMagnetometer
	SensorDeviceMotion
)

// SensorVector is a measurement in the right-handed natural device frame:
// x right, y up, z out of the screen. Acceleration is m/s², rotation rad/s,
// magnetic field µT. A face-up device at rest reads +9.80665 on z.
type SensorVector struct{ X, Y, Z float64 }

// SensorQuaternion rotates the device frame into east-north-up world axes
// when HeadingReferenced, or an arbitrary horizontal frame otherwise.
type SensorQuaternion struct{ X, Y, Z, W float64 }

// MotionSample contains one sensor's latest reading, never fabricated axes.
// Timestamp is monotonic time in the provider's clock. Accuracy is -1 if unknown,
// otherwise 0 (unreliable) through 3 (high). Dropped counts replaced samples
// since the previous delivery, including native provider back-pressure.
type MotionSample struct {
	Sensor                                          MotionSensors
	Vector, Gravity, UserAcceleration, RotationRate SensorVector
	Attitude                                        SensorQuaternion
	HeadingReferenced                               bool
	Timestamp, Interval                             time.Duration
	Accuracy                                        int
	Gap                                             bool
	Dropped                                         uint64
}

// MotionOptions chooses sensors and a sample interval (10 ms to one second).
// Zero Sensors requests accelerometer; zero Interval requests 20 ms.
type MotionOptions struct {
	Sensors  MotionSensors
	Interval time.Duration
}

// SensorCommand is an ItemSensor's operation; starts never prompt.
type SensorCommand uint8

const (
	SensorReadPermission SensorCommand = iota
	SensorRequestPermission
	SensorLocate
	SensorWatchLocation
	SensorWatchMotion
	SensorStop
)

// SensorRequest is scoped by Count and Generation. A restart uses a new
// generation, so samples queued before a pause cannot enter a resumed stream.
type SensorRequest struct {
	Command    SensorCommand
	Kind       SensorKind
	Generation uint32
	Location   LocationOptions
	Motion     MotionOptions
}

// SensorReport is a provider result. Errors are stable codes without data.
type SensorReport struct {
	Generation uint32
	Grant      *SensorGrant
	Location   *LocationFix
	Motion     *MotionSample
	Error      string
	End        bool
}

// SensorPacket coalesces samples before they enter the engine's mailbox.
// Each stream has one packet and at most one queued wake-up. Errors and
// permission answers use separate events and cannot replace a sample.
type SensorPacket struct {
	mu      sync.Mutex
	latest  SensorReport
	pending bool
}

// Offer keeps the newest sample and says whether a wake-up must be queued.
func (p *SensorPacket) Offer(r SensorReport) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	first := !p.pending
	if p.pending {
		if r.Motion != nil && p.latest.Motion != nil {
			r.Motion.Dropped += p.latest.Motion.Dropped + 1
		}
		if r.Location != nil && p.latest.Location != nil {
			r.Location.Dropped += p.latest.Location.Dropped + 1
		}
	}
	p.latest, p.pending = r, true
	return first
}

// Take copies and consumes the pending sample.
func (p *SensorPacket) Take() (SensorReport, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.latest, p.pending
	p.latest, p.pending = SensorReport{}, false
	return r, ok
}

// NoSensor is the explicit answer of a backend with no sensor provider.
func NoSensor(it *MountItem) Event {
	r := &SensorReport{Generation: it.Sensor.Generation, Error: "unavailable", End: true}
	if it.Sensor.Command <= SensorRequestPermission {
		r.Grant = &SensorGrant{Authorization: SensorUnavailable}
		r.Error = ""
	}
	return Event{Kind: EventSensor, Count: it.Count, Data: r}
}
