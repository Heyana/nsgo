package driver

import "time"

// StepSample counts steps in the provider's observation window. A restarted provider starts another window; Since and Gap make that boundary explicit.
type StepSample struct {
	Count                                     uint64
	Since, Timestamp                          time.Time
	Distance, FloorsAscended, FloorsDescended SensorNumber
	Gap                                       bool
	Dropped                                   uint64
}

// ActivityKinds is a set: providers can report stationary and automotive together.
type ActivityKinds uint16

const (
	ActivityStationary ActivityKinds = 1 << iota
	ActivityWalking
	ActivityRunning
	ActivityCycling
	ActivityAutomotive
	ActivityUnknown
)

// ActivityConfidence is unknown, low, medium or high, in that order.
type ActivityConfidence uint8

const (
	ActivityConfidenceUnknown ActivityConfidence = iota
	ActivityConfidenceLow
	ActivityConfidenceMedium
	ActivityConfidenceHigh
)

// ActivitySample reports the provider's classification without inferring one from acceleration.
type ActivitySample struct {
	Kinds      ActivityKinds
	Confidence ActivityConfidence
	Timestamp  time.Time
	Gap        bool
	Dropped    uint64
}

// PressureSample reports absolute pressure in pascals. RelativeAltitude, when available, is meters from the provider's start, not an absolute elevation.
type PressureSample struct {
	Pascals             float64
	RelativeAltitude    SensorNumber
	Timestamp, Interval time.Duration
	Gap                 bool
	Dropped             uint64
}

// GeofenceRegion is a WGS 84 circle whose identifier is local to its stream.
type GeofenceRegion struct {
	ID                          string
	Latitude, Longitude, Radius float64
}

type GeofenceState uint8

const (
	GeofenceUnknown GeofenceState = iota
	GeofenceInside
	GeofenceOutside
)

// GeofenceSample is the latest observed state of one region. Dropped counts replaced deliveries for that region; Unknown never means Outside.
type GeofenceSample struct {
	Region    string
	State     GeofenceState
	Timestamp time.Time
	Gap       bool
	Dropped   uint64
}

// GeofenceOptions chooses at most twenty regions and explicit background policy.
type GeofenceOptions struct {
	Regions                             []GeofenceRegion
	Background                          bool
	NotificationTitle, NotificationBody string
}

// BackgroundMotionOptions requests a finite live stream. The OS may expire its execution lease earlier; it never records after this app terminates.
type BackgroundMotionOptions struct {
	Motion                              MotionOptions
	Duration                            time.Duration
	NotificationTitle, NotificationBody string
}

// PreciseLocationAnswer reports the authorization after an explicit accuracy request; it contains no position. Full=false is an allowed approximate grant.
type PreciseLocationAnswer struct {
	Full bool
}
