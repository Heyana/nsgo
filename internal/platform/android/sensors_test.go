package android

import (
	"encoding/json"
	"math"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestSensorJavaProviderFixtures(t *testing.T) {
	javac, java := androidJDK(t)
	dir := t.TempDir()
	if out, err := exec.Command(javac, "-d", dir, filepath.Join("java", "dev", "oneo", "nsgo", "SensorPolicy.java"), filepath.Join("testdata", "SensorCheck.java")).CombinedOutput(); err != nil {
		t.Fatalf("compile fixtures: %v\n%s", err, out)
	}
	if out, err := exec.Command(java, "-cp", dir, "dev.oneo.nsgo.SensorCheck").CombinedOutput(); err != nil {
		t.Fatalf("sensor fixtures: %v\n%s", err, out)
	}
}

func TestSensorPipeKeepsNewestPerSensor(t *testing.T) {
	var receiver sensorReports
	var events []driver.Event
	dispatch := func(ev driver.Event) { events = append(events, ev) }
	for i := 0; i < 1000; i++ {
		for _, sensor := range []driver.MotionSensors{driver.SensorAccelerometer, driver.SensorGyroscope} {
			payload, _ := json.Marshal(struct {
				ID uint32 `json:"id"`
				driver.SensorReport
			}{9, driver.SensorReport{
				Generation: 3, Motion: &driver.MotionSample{Sensor: sensor, Timestamp: time.Duration(i), Vector: driver.SensorVector{Z: float64(i)}, Dropped: 2}}})
			receiver.receive(payload, dispatch)
		}
	}
	if len(events) != 2 {
		t.Fatalf("queued %d events for two sensors", len(events))
	}
	for _, ev := range events {
		r, ok := ev.Data.(*driver.SensorPacket).Take()
		if !ok || r.Generation != 3 || r.Motion.Timestamp != 999 || r.Motion.Dropped != 2999 {
			t.Fatalf("sample %+v", r.Motion)
		}
	}
	receiver.receive([]byte(`{"id":9,"Generation":3,"Error":"not-allowed","End":true}`), dispatch)
	if len(receiver.streams) != 0 || len(events) != 3 {
		t.Fatal("terminal answer lost or packets leaked")
	}
}

func TestSensorPipePreservesLocationFields(t *testing.T) {
	var receiver sensorReports
	var event driver.Event
	receiver.receive([]byte(`{"id":17,"Generation":2,"End":true,"Location":{"Latitude":51.5,"Longitude":-0.12,"Timestamp":"2026-11-15T01:02:03Z","Monotonic":12000000000,"HasMonotonic":true,"Altitude":{"Value":0,"Valid":true},"VerticalAccuracy":{"Value":0,"Valid":false}}}`), func(e driver.Event) { event = e })
	f := event.Data.(*driver.SensorReport).Location
	if event.Count != 17 || f.Latitude != 51.5 || f.Longitude != -0.12 || !f.Altitude.Valid || f.VerticalAccuracy.Valid || f.Monotonic != 12*time.Second || f.Timestamp.UTC().Hour() != 1 {
		t.Fatalf("fix %+v", f)
	}
}

func TestSensorRequestWireIncludesBackgroundAndRate(t *testing.T) {
	var w writer
	writeSensorRequest(&w, 12, driver.SensorRequest{Command: driver.SensorWatchLocation, Kind: driver.SensorLocation, Generation: 4,
		Location: driver.LocationOptions{Accuracy: 5, Distance: 10, Interval: time.Second, MaxAge: 2 * time.Second, Background: true, NotificationTitle: "Route", NotificationBody: "Recording"},
		Motion:   driver.MotionOptions{Sensors: driver.SensorAccelerometer | driver.SensorDeviceMotion, Interval: 20 * time.Millisecond}})
	r := reader{buf: w.buf}
	if r.u32() != 12 || r.byte() != byte(driver.SensorWatchLocation) || r.byte() != byte(driver.SensorLocation) || r.u32() != 4 {
		t.Fatal("session header changed")
	}
	if math.Float64frombits(r.u64()) != 5 || math.Float64frombits(r.u64()) != 10 || r.u64() != uint64(time.Second) || r.u64() != uint64(2*time.Second) || r.byte() != 1 || r.str() != "Route" || r.str() != "Recording" || r.byte() != 9 || r.u64() != uint64(20*time.Millisecond) {
		t.Fatal("provider request changed")
	}
}
