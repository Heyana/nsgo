//go:build js && wasm

package web

import (
	"math"
	"strings"
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type motionService struct {
	streams             map[uint32]*motionStream
	motion              listeners
	orientation         listeners
	attitude            driver.SensorQuaternion
	absolute            bool
	oriented            bool
	orientationSeen     bool
	authorization       driver.SensorAuthorization
	reverseAcceleration bool
}

type motionStream struct {
	req     driver.SensorRequest
	active  bool
	packets [3]driver.SensorPacket
	last    [3]time.Duration
	sent    [3]bool
}

// motionBrowser reports hardware-class availability. iPadOS can identify
// itself as a Mac in Safari's desktop browsing mode.
func motionBrowser(g js.Value) (available, reverseAcceleration bool) {
	if !g.Get("isSecureContext").Truthy() || g.Get("DeviceMotionEvent").Type() != js.TypeFunction {
		return false, false
	}
	nav := g.Get("navigator")
	agent := nav.Get("userAgent").String()
	touch := nav.Get("maxTouchPoints")
	iPad := nav.Get("platform").String() == "MacIntel" && touch.Type() == js.TypeNumber && touch.Int() > 1
	ios := iPad || strings.Contains(agent, "iPhone") || strings.Contains(agent, "iPad") || strings.Contains(agent, "iPod")
	return ios || strings.Contains(agent, "Android"), ios && strings.Contains(agent, "AppleWebKit")
}

func (s *surface) sensor(it *driver.MountItem) {
	id, req := it.Count, *it.Sensor
	if req.Command == driver.SensorStop {
		if stream := s.motion.streams[id]; stream != nil && stream.req.Generation == req.Generation {
			delete(s.motion.streams, id)
			s.syncMotionListeners()
		}
		return
	}
	g := js.Global()
	available, reverse := motionBrowser(g)
	if req.Kind != driver.SensorMotion || !available {
		s.engine.Dispatch(driver.NoSensor(it))
		return
	}

	switch req.Command {
	case driver.SensorReadPermission, driver.SensorRequestPermission:
		s.motionPermission(req.Command == driver.SensorRequestPermission, func(status driver.SensorAuthorization) {
			s.engine.Dispatch(driver.Event{Kind: driver.EventSensor, Count: id, Data: &driver.SensorReport{
				Generation: req.Generation, Grant: &driver.SensorGrant{Authorization: status}, End: true,
			}})
		})
	case driver.SensorWatchMotion:
		if req.Motion.Sensors&driver.SensorMagnetometer != 0 ||
			req.Motion.Sensors&driver.SensorDeviceMotion != 0 && g.Get("DeviceOrientationEvent").Type() != js.TypeFunction {
			s.engine.Dispatch(driver.NoSensor(it))
			return
		}
		s.motion.reverseAcceleration = reverse
		if s.motion.streams == nil {
			s.motion.streams = make(map[uint32]*motionStream)
		}
		stream := &motionStream{req: req}
		s.motion.streams[id] = stream
		s.motionPermission(false, func(status driver.SensorAuthorization) {
			if s.motion.streams[id] != stream {
				return
			}
			if status != driver.SensorGranted {
				s.endMotion(id, stream, "not-allowed")
				return
			}
			stream.active = true
			s.syncMotionListeners()
		})
	}
}

func (s *surface) motionPermission(ask bool, answer func(driver.SensorAuthorization)) {
	g := js.Global()
	class := g.Get("DeviceMotionEvent")
	if class.Get("requestPermission").Type() != js.TypeFunction {
		motionPermissionStatus(g.Get("navigator").Get("permissions"), answer)
		return
	}
	activation := g.Get("navigator").Get("userActivation")
	if !ask || s.motion.authorization != driver.SensorNotAsked || activation.Truthy() && !activation.Get("isActive").Truthy() {
		answer(s.motion.authorization)
		return
	}
	request, ok := tryCall(class, "requestPermission")
	if !ok {
		answer(driver.SensorDenied)
		return
	}
	then(request, func(result js.Value) {
		status := driver.SensorDenied
		if result.String() == "granted" {
			status = driver.SensorGranted
		}
		s.motion.authorization = status
		answer(status)
	}, func(js.Value) {
		// WebKit rejects only while the grant is still undecided and the call came outside a gesture, so no prompt was shown.
		answer(driver.SensorNotAsked)
	})
}

// Browsers without the prompt method use their sensor site settings.
// Where the Permissions API cannot report those names, devices can
// still supply samples without an explicit grant.
func motionPermissionStatus(permissions js.Value, answer func(driver.SensorAuthorization)) {
	if !permissions.Truthy() {
		answer(driver.SensorGranted)
		return
	}
	query := func(name string, done func(driver.SensorAuthorization)) {
		result, ok := tryCall(permissions, "query", map[string]any{"name": name})
		if !ok {
			done(driver.SensorGranted)
			return
		}
		then(result, func(status js.Value) {
			switch status.Get("state").String() {
			case "denied":
				done(driver.SensorDenied)
			case "prompt":
				done(driver.SensorNotAsked)
			default:
				done(driver.SensorGranted)
			}
		}, func(js.Value) { done(driver.SensorGranted) })
	}
	query("accelerometer", func(acceleration driver.SensorAuthorization) {
		query("gyroscope", func(rotation driver.SensorAuthorization) {
			if acceleration == driver.SensorDenied || rotation == driver.SensorDenied {
				answer(driver.SensorDenied)
			} else if acceleration == driver.SensorNotAsked || rotation == driver.SensorNotAsked {
				answer(driver.SensorNotAsked)
			} else {
				answer(driver.SensorGranted)
			}
		})
	})
}

func (s *surface) syncMotionListeners() {
	m := &s.motion
	var active, fused bool
	for _, stream := range m.streams {
		if stream.active {
			active = true
			fused = fused || stream.req.Motion.Sensors&driver.SensorDeviceMotion != 0
		}
	}
	if active && len(m.motion) == 0 {
		m.motion.on(js.Global(), "devicemotion", s.onMotion)
	} else if !active {
		m.motion.release()
	}
	if fused && len(m.orientation) == 0 {
		m.orientation.on(js.Global(), "deviceorientation", m.onOrientation)
	} else if !fused {
		m.orientation.release()
		m.oriented, m.orientationSeen = false, false
	}
}

func (m *motionService) onOrientation(event js.Value) {
	m.orientationSeen = true
	angles, ok := motionVector(event, "beta", "gamma", "alpha", math.Pi/180)
	m.oriented = ok
	if !ok {
		return
	}
	// Intrinsic Z-X-Y rotations compose as qz * qx * qy.
	sx, cx := math.Sincos(angles.X / 2)
	sy, cy := math.Sincos(angles.Y / 2)
	sz, cz := math.Sincos(angles.Z / 2)
	m.attitude = driver.SensorQuaternion{
		X: sx*cy*cz - cx*sy*sz,
		Y: cx*sy*cz + sx*cy*sz,
		Z: cx*cy*sz + sx*sy*cz,
		W: cx*cy*cz - sx*sy*sz,
	}
	m.absolute = event.Get("absolute").Truthy()
}

func (s *surface) onMotion(event js.Value) {
	if s.doc.Get("visibilityState").String() == "hidden" || !event.InstanceOf(js.Global().Get("DeviceMotionEvent")) {
		return
	}
	m := &s.motion
	sign := 1.0
	// WebKit on iOS exposes Core Motion's acceleration sign. NSGo uses
	// the accelerometer convention with positive Z when face up.
	if m.reverseAcceleration {
		sign = -1
	}
	acceleration, hasAcceleration := motionVector(event.Get("accelerationIncludingGravity"), "x", "y", "z", sign)
	user, hasUser := motionVector(event.Get("acceleration"), "x", "y", "z", sign)
	rotation, hasRotation := motionVector(event.Get("rotationRate"), "alpha", "beta", "gamma", math.Pi/180)
	now := time.Duration(event.Get("timeStamp").Float() * float64(time.Millisecond))
	interval := time.Duration(event.Get("interval").Float() * float64(time.Millisecond))
	readings := [3]driver.MotionSample{
		{Sensor: driver.SensorAccelerometer, Vector: acceleration},
		{Sensor: driver.SensorGyroscope, Vector: rotation},
		{Sensor: driver.SensorDeviceMotion,
			Gravity:          driver.SensorVector{X: acceleration.X - user.X, Y: acceleration.Y - user.Y, Z: acceleration.Z - user.Z},
			UserAcceleration: user, RotationRate: rotation, Attitude: m.attitude, HeadingReferenced: m.absolute},
	}
	for id, stream := range m.streams {
		if !stream.active {
			continue
		}
		sensors := stream.req.Motion.Sensors
		if sensors&driver.SensorAccelerometer != 0 && !hasAcceleration ||
			sensors&driver.SensorGyroscope != 0 && !hasRotation ||
			sensors&driver.SensorDeviceMotion != 0 && (!hasAcceleration || !hasUser || !hasRotation || m.orientationSeen && !m.oriented) {
			s.endMotion(id, stream, "unavailable")
			continue
		}
		for i, sample := range readings {
			if sensors&sample.Sensor == 0 || sample.Sensor == driver.SensorDeviceMotion && !m.oriented {
				continue
			}
			sample.Timestamp, sample.Interval, sample.Accuracy = now, interval, -1
			if stream.sent[i] {
				sample.Interval = now - stream.last[i]
				if sample.Interval < stream.req.Motion.Interval {
					continue
				}
			}
			stream.last[i], stream.sent[i] = now, true
			packet := &stream.packets[i]
			if packet.Offer(driver.SensorReport{Generation: stream.req.Generation, Motion: &sample}) {
				s.engine.Dispatch(driver.Event{Kind: driver.EventSensor, Count: id, Data: packet})
			}
		}
	}
}

func (s *surface) endMotion(id uint32, stream *motionStream, code string) {
	delete(s.motion.streams, id)
	s.syncMotionListeners()
	s.engine.Dispatch(driver.Event{Kind: driver.EventSensor, Count: id, Data: &driver.SensorReport{
		Generation: stream.req.Generation, Error: code, End: true,
	}})
}

func motionVector(object js.Value, x, y, z string, scale float64) (driver.SensorVector, bool) {
	if object.Type() != js.TypeObject {
		return driver.SensorVector{}, false
	}
	values := [3]float64{}
	for i, name := range []string{x, y, z} {
		value := object.Get(name)
		if value.Type() != js.TypeNumber {
			return driver.SensorVector{}, false
		}
		values[i] = value.Float()
		if math.IsNaN(values[i]) || math.IsInf(values[i], 0) {
			return driver.SensorVector{}, false
		}
	}
	return driver.SensorVector{X: scale * values[0], Y: scale * values[1], Z: scale * values[2]}, true
}
