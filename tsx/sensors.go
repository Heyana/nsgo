package tsx

import (
	"math"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx/internal/js"
)

func (r *runtime) sensors() js.Value {
	o := r.vm.NewObject()
	// permission tells, or with ask asks for, a sensor's core.Permission, resolving with its status's name; a sensor the package's capabilities leave out is "unavailable".
	permission := func(ask bool) func(js.FunctionCall) js.Value {
		return func(call js.FunctionCall) js.Value {
			var key core.Permission
			switch arg(call, 0).String() {
			case "location":
				key = core.PermissionLocation
			case "backgroundLocation":
				key = core.PermissionBackgroundLocation
			case "motion":
				key = core.PermissionMotion
			case "activityRecognition":
				key = core.PermissionActivityRecognition
			default:
				panic(r.vm.NewTypeError("Sensors: unknown permission"))
			}
			p, resolve, _ := r.newPromise()
			done := func(s core.PermissionStatus) { r.inJS(func() { _ = resolve(s.String()) }) }
			if ask {
				r.cx.RequestPermission(key, done)
			} else {
				r.cx.PermissionStatus(key, done)
			}
			return r.vm.ToValue(p)
		}
	}
	_ = o.Set("permission", r.function(permission(false)))
	_ = o.Set("requestPermission", r.function(permission(true)))
	_ = o.Set("locate", r.function(func(call js.FunctionCall) js.Value {
		p, resolve, reject := r.newPromise()
		_, err := r.cx.Locate(sensorLocationOptions(r.options(arg(call, 0))), func(f core.LocationFix, err error) {
			r.inJS(func() {
				if err != nil {
					_ = reject(r.sensorError(err))
				} else {
					_ = resolve(locationValue(f))
				}
			})
		})
		if err != nil {
			_ = reject(r.sensorError(err))
		}
		return r.vm.ToValue(p)
	}))
	watch := func(motion bool) func(js.FunctionCall) js.Value {
		return func(call js.FunctionCall) js.Value {
			sample, failed := arg(call, 1), arg(call, 2)
			if !isFunction(sample) || defined(failed) && !isFunction(failed) {
				panic(r.vm.NewTypeError("Sensors: callbacks must be functions"))
			}
			onError := func(err error) {
				if isFunction(failed) {
					r.call(failed, r.sensorError(err))
				}
			}
			var stream *core.SensorStream
			var err error
			op := r.options(arg(call, 0))
			if motion {
				mask := op.float("sensors", 1)
				if math.IsNaN(mask) || mask != math.Trunc(mask) || mask < 0 || mask > 15 {
					panic(r.vm.NewTypeError("Sensors: invalid sensor set"))
				}
				stream, err = r.cx.WatchMotion(core.MotionOptions{Sensors: core.MotionSensors(mask), Interval: sensorMilliseconds(op.float("interval", 20))},
					func(m core.MotionSample) { r.call(sample, r.vm.ToValue(motionValue(m))) }, onError)
			} else {
				stream, err = r.cx.WatchLocation(sensorLocationOptions(op), func(f core.LocationFix) { r.call(sample, r.vm.ToValue(locationValue(f))) }, onError)
			}
			if err != nil {
				panic(r.sensorError(err))
			}
			return r.function(func(js.FunctionCall) js.Value { stream.Stop(); return js.Undefined() })
		}
	}
	_ = o.Set("watchLocation", r.function(watch(false)))
	_ = o.Set("watchMotion", r.function(watch(true)))
	r.extendedSensors(o)
	return o
}

func sensorMilliseconds(v float64) time.Duration {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v >= float64(math.MaxInt64)/1e6 {
		return -1
	}
	return time.Duration(v * 1e6)
}

func sensorLocationOptions(o opts) core.LocationOptions {
	return core.LocationOptions{Accuracy: o.float("accuracy", 0), Distance: o.float("distance", 0),
		Interval: sensorMilliseconds(o.float("interval", 0)), MaxAge: sensorMilliseconds(o.float("maxAge", 0)), Timeout: sensorMilliseconds(o.float("timeout", 0)),
		Background: o.bool("background", false), NotificationTitle: o.str("notificationTitle", ""), NotificationBody: o.str("notificationBody", "")}
}

func sensorNumberValue(n core.SensorNumber) any {
	if n.Valid {
		return n.Value
	}
	return nil
}
func sensorVectorValue(v core.SensorVector) map[string]any {
	return map[string]any{"x": v.X, "y": v.Y, "z": v.Z}
}

func locationValue(f core.LocationFix) map[string]any {
	var monotonic any
	if f.HasMonotonic {
		monotonic = float64(f.Monotonic) / 1e6
	}
	return map[string]any{"latitude": f.Latitude, "longitude": f.Longitude, "timestamp": float64(f.Timestamp.UnixMilli()),
		"monotonic": monotonic, "altitude": sensorNumberValue(f.Altitude), "mslAltitude": sensorNumberValue(f.MSLAltitude),
		"horizontalAccuracy": sensorNumberValue(f.HorizontalAccuracy), "verticalAccuracy": sensorNumberValue(f.VerticalAccuracy),
		"speed": sensorNumberValue(f.Speed), "course": sensorNumberValue(f.Course), "age": float64(f.Age) / 1e6,
		"cached": f.Cached, "approximate": f.Approximate, "gap": f.Gap, "dropped": float64(f.Dropped)}
}

func motionValue(m core.MotionSample) map[string]any {
	q := m.Attitude
	return map[string]any{"sensor": int(m.Sensor), "vector": sensorVectorValue(m.Vector), "gravity": sensorVectorValue(m.Gravity),
		"userAcceleration": sensorVectorValue(m.UserAcceleration), "rotationRate": sensorVectorValue(m.RotationRate),
		"attitude": map[string]any{"x": q.X, "y": q.Y, "z": q.Z, "w": q.W}, "headingReferenced": m.HeadingReferenced,
		"timestamp": float64(m.Timestamp) / 1e6, "interval": float64(m.Interval) / 1e6, "accuracy": m.Accuracy, "gap": m.Gap, "dropped": float64(m.Dropped)}
}

func (r *runtime) sensorError(err error) *js.Object {
	o := r.goError(err)
	if code, ok := err.(core.SensorError); ok {
		_ = o.Set("code", string(code))
	}
	return o
}
