package nsgotest

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func (a *App) extraSensorSample(id uint32, run *sensorRun, key string, report driver.SensorReport) {
	if run.extraPackets == nil {
		run.extraPackets = map[string]*driver.SensorPacket{}
	}
	p := run.extraPackets[key]
	if p == nil {
		p = &driver.SensorPacket{}
		run.extraPackets[key] = p
	}
	report.Generation = run.request.Generation
	if p.Offer(report) {
		a.post(driver.Event{Kind: driver.EventSensor, Count: id, Data: p})
	}
}

// FeedSteps delivers a copied counter observation to active step streams.
func (a *App) FeedSteps(sample core.StepSample) {
	a.requireSensorModel()
	for id, run := range a.sensorRuns {
		if run.request.Kind == driver.SensorSteps {
			copy := sample
			a.extraSensorSample(id, run, "steps", driver.SensorReport{Steps: &copy})
		}
	}
}

// FeedActivity delivers a copied native classification to active activity streams.
func (a *App) FeedActivity(sample core.ActivitySample) {
	a.requireSensorModel()
	for id, run := range a.sensorRuns {
		if run.request.Kind == driver.SensorActivity {
			copy := sample
			a.extraSensorSample(id, run, "activity", driver.SensorReport{Activity: &copy})
		}
	}
}

// FeedPressure delivers a copied pressure observation in pascals.
func (a *App) FeedPressure(sample core.PressureSample) {
	a.requireSensorModel()
	for id, run := range a.sensorRuns {
		if run.request.Kind == driver.SensorPressure {
			copy := sample
			a.extraSensorSample(id, run, "pressure", driver.SensorReport{Pressure: &copy})
		}
	}
}

// FeedGeofence delivers the latest observed state only to streams monitoring that region. Back-pressure is separate for each region identifier.
func (a *App) FeedGeofence(sample core.GeofenceSample) {
	a.requireSensorModel()
	for id, run := range a.sensorRuns {
		if run.request.Kind != driver.SensorGeofences {
			continue
		}
		for _, region := range run.request.Geofences.Regions {
			if region.ID == sample.Region {
				copy := sample
				a.extraSensorSample(id, run, "region:"+sample.Region, driver.SensorReport{Geofence: &copy})
				break
			}
		}
	}
}
