package android

import (
	"encoding/json"
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func writeSensorRequest(w *writer, id uint32, r driver.SensorRequest) {
	w.u32(id)
	w.byte(byte(r.Command))
	w.byte(byte(r.Kind))
	w.u32(r.Generation)
	w.f64(r.Location.Accuracy)
	w.f64(r.Location.Distance)
	w.u64(uint64(r.Location.Interval))
	w.u64(uint64(r.Location.MaxAge))
	w.flags(r.Location.Background)
	w.str(r.Location.NotificationTitle)
	w.str(r.Location.NotificationBody)
	w.byte(byte(r.Motion.Sensors))
	w.u64(uint64(r.Motion.Interval))
	if r.Command >= driver.SensorWatchSteps {
		data, _ := json.Marshal(struct {
			Geofences        driver.GeofenceOptions
			BackgroundMotion driver.BackgroundMotionOptions
			Purpose          string
		}{r.Geofences, r.BackgroundMotion, r.Purpose})
		w.str(string(data))
	}
}

// sensorReports coalesces pipe input before Engine.Dispatch. The Java
// writer separately coalesces while the pipe itself is backed up.
type sensorReports struct{ streams map[uint32]*sensorPackets }
type sensorPackets struct {
	generation uint32
	packets    [5]driver.SensorPacket
	extra      map[string]*driver.SensorPacket
}

func (s *sensorReports) receive(payload []byte, dispatch func(driver.Event)) {
	var r struct {
		ID uint32 `json:"id"`
		driver.SensorReport
	}
	if json.Unmarshal(payload, &r) != nil {
		return
	}
	if r.End || r.Grant != nil || r.Precise != nil || r.Error != "" {
		delete(s.streams, r.ID)
		dispatch(driver.Event{Kind: driver.EventSensor, Count: r.ID, Data: &r.SensorReport})
		return
	}
	key := 0
	if r.Motion != nil {
		switch r.Motion.Sensor {
		case driver.SensorAccelerometer:
			key = 1
		case driver.SensorGyroscope:
			key = 2
		case driver.SensorMagnetometer:
			key = 3
		case driver.SensorDeviceMotion:
			key = 4
		default:
			return
		}
	} else if r.Location == nil && r.Steps == nil && r.Activity == nil && r.Pressure == nil && r.Geofence == nil {
		return
	}
	if s.streams == nil {
		s.streams = map[uint32]*sensorPackets{}
	}
	stream := s.streams[r.ID]
	if stream == nil || stream.generation != r.Generation {
		stream = &sensorPackets{generation: r.Generation}
		s.streams[r.ID] = stream
	}
	p := &stream.packets[key]
	if r.Steps != nil || r.Activity != nil || r.Pressure != nil || r.Geofence != nil {
		name := "steps"
		if r.Activity != nil {
			name = "activity"
		}
		if r.Pressure != nil {
			name = "pressure"
		}
		if r.Geofence != nil {
			name = "region:" + r.Geofence.Region
		}
		if stream.extra == nil {
			stream.extra = map[string]*driver.SensorPacket{}
		}
		if stream.extra[name] == nil {
			stream.extra[name] = &driver.SensorPacket{}
		}
		p = stream.extra[name]
	}
	if p.Offer(r.SensorReport) {
		dispatch(driver.Event{Kind: driver.EventSensor, Count: r.ID, Data: p})
	}
}

// filterExtendedSensors rejects advanced requests before encoding when the Java runtime predates their decoder. In particular fitness authorization must never be misread by an older runtime as a location permission request.
func filterExtendedSensors(capable bool, b *driver.MountBatch, dispatch func(driver.Event)) *driver.MountBatch {
	if capable {
		return b
	}
	advanced := func(it driver.MountItem) bool {
		return it.Kind == driver.ItemSensor && it.Sensor != nil && (it.Sensor.Kind >= driver.SensorSteps || it.Sensor.Command >= driver.SensorWatchSteps)
	}
	if !slices.ContainsFunc(b.Items, advanced) {
		return b
	}
	out := *b
	out.Items = slices.Clone(b.Items)
	out.Items = slices.DeleteFunc(out.Items, func(it driver.MountItem) bool {
		if !advanced(it) {
			return false
		}
		if it.Sensor.Command != driver.SensorStop {
			dispatch(driver.NoSensor(&it))
		}
		return true
	})
	return &out
}
