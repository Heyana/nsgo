package tsx

import (
	"math"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx/internal/js"
)

func (r *runtime) extendedSensors(object *js.Object) {
	watch := func(name string, options bool, start func(opts, js.Value, func(error)) (*core.SensorStream, error)) {
		_ = object.Set(name, r.function(func(call js.FunctionCall) js.Value {
			index := 0
			var op opts
			if options {
				op = r.options(arg(call, 0))
				index = 1
			}
			sample, failed := arg(call, index), arg(call, index+1)
			if !isFunction(sample) || defined(failed) && !isFunction(failed) {
				panic(r.vm.NewTypeError("Sensors: callbacks must be functions"))
			}
			onError := func(err error) {
				if isFunction(failed) {
					r.call(failed, r.sensorError(err))
				}
			}
			stream, err := start(op, sample, onError)
			if err != nil {
				panic(r.sensorError(err))
			}
			return r.function(func(js.FunctionCall) js.Value { stream.Stop(); return js.Undefined() })
		}))
	}
	watch("watchSteps", false, func(_ opts, sample js.Value, failed func(error)) (*core.SensorStream, error) {
		return r.cx.WatchSteps(func(v core.StepSample) { r.call(sample, r.vm.ToValue(stepValue(v))) }, failed)
	})
	watch("watchActivity", false, func(_ opts, sample js.Value, failed func(error)) (*core.SensorStream, error) {
		return r.cx.WatchActivity(func(v core.ActivitySample) { r.call(sample, r.vm.ToValue(activityValue(v))) }, failed)
	})
	watch("watchPressure", false, func(_ opts, sample js.Value, failed func(error)) (*core.SensorStream, error) {
		return r.cx.WatchPressure(func(v core.PressureSample) { r.call(sample, r.vm.ToValue(pressureValue(v))) }, failed)
	})
	watch("watchGeofences", true, func(o opts, sample js.Value, failed func(error)) (*core.SensorStream, error) {
		v := o.get("regions")
		if !r.isArray(v) {
			panic(r.vm.NewTypeError("Sensors: regions must be an array"))
		}
		length := v.(*js.Object).Get("length").ToInteger()
		if length < 1 || length > 20 {
			panic(r.vm.NewTypeError("Sensors: one to twenty regions required"))
		}
		regions := make([]core.GeofenceRegion, int(length))
		for i, item := range r.arrayItems(v) {
			region := r.options(item)
			regions[i] = core.GeofenceRegion{ID: region.str("id", ""), Latitude: region.float("latitude", math.NaN()), Longitude: region.float("longitude", math.NaN()), Radius: region.float("radius", 0)}
		}
		op := core.GeofenceOptions{Regions: regions, Background: o.bool("background", false), NotificationTitle: o.str("notificationTitle", ""), NotificationBody: o.str("notificationBody", "")}
		return r.cx.WatchGeofences(op, func(v core.GeofenceSample) { r.call(sample, r.vm.ToValue(geofenceValue(v))) }, failed)
	})
	watch("watchBackgroundMotion", true, func(o opts, sample js.Value, failed func(error)) (*core.SensorStream, error) {
		mask := o.float("sensors", 1)
		if math.IsNaN(mask) || mask != math.Trunc(mask) || mask < 0 || mask > 15 {
			panic(r.vm.NewTypeError("Sensors: invalid sensor set"))
		}
		op := core.BackgroundMotionOptions{Motion: core.MotionOptions{Sensors: core.MotionSensors(mask), Interval: sensorMilliseconds(o.float("interval", 20))}, Duration: sensorMilliseconds(o.float("duration", 0)), NotificationTitle: o.str("notificationTitle", ""), NotificationBody: o.str("notificationBody", "")}
		return r.cx.WatchBackgroundMotion(op, func(v core.MotionSample) { r.call(sample, r.vm.ToValue(motionValue(v))) }, failed)
	})
	_ = object.Set("requestPreciseLocation", r.function(func(call js.FunctionCall) js.Value {
		purpose := arg(call, 0)
		if !defined(purpose) {
			panic(r.vm.NewTypeError("Sensors: purpose must be a string"))
		}
		key, ok := purpose.Export().(string)
		if !ok {
			panic(r.vm.NewTypeError("Sensors: purpose must be a string"))
		}
		promise, resolve, reject := r.newPromise()
		_, err := r.cx.RequestPreciseLocation(key, func(full bool, err error) {
			r.inJS(func() {
				if err != nil {
					_ = reject(r.sensorError(err))
				} else {
					_ = resolve(full)
				}
			})
		})
		if err != nil {
			_ = reject(r.sensorError(err))
		}
		return r.vm.ToValue(promise)
	}))
}

func stepValue(v core.StepSample) map[string]any {
	return map[string]any{"count": float64(v.Count), "since": float64(v.Since.UnixMilli()), "timestamp": float64(v.Timestamp.UnixMilli()), "distance": sensorNumberValue(v.Distance), "floorsAscended": sensorNumberValue(v.FloorsAscended), "floorsDescended": sensorNumberValue(v.FloorsDescended), "gap": v.Gap, "dropped": float64(v.Dropped)}
}
func activityValue(v core.ActivitySample) map[string]any {
	return map[string]any{"kinds": int(v.Kinds), "confidence": int(v.Confidence), "timestamp": float64(v.Timestamp.UnixMilli()), "gap": v.Gap, "dropped": float64(v.Dropped)}
}
func pressureValue(v core.PressureSample) map[string]any {
	return map[string]any{"pascals": v.Pascals, "relativeAltitude": sensorNumberValue(v.RelativeAltitude), "timestamp": float64(v.Timestamp) / 1e6, "interval": float64(v.Interval) / 1e6, "gap": v.Gap, "dropped": float64(v.Dropped)}
}
func geofenceValue(v core.GeofenceSample) map[string]any {
	state := "unknown"
	if v.State == core.GeofenceInside {
		state = "inside"
	}
	if v.State == core.GeofenceOutside {
		state = "outside"
	}
	return map[string]any{"region": v.Region, "state": state, "timestamp": float64(v.Timestamp.UnixMilli()), "gap": v.Gap, "dropped": float64(v.Dropped)}
}
