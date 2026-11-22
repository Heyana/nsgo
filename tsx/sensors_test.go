package tsx_test

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestSensorsTSXSubscriptionsAndPermissionPromises(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("synthetic sensor fixtures run headless only")
	}
	app := run(t, `import { Sensors, View, Text, mount, signal } from "nsgo";
function App() {
 const state = signal("idle"); let stop = () => {};
 return <View>
  <View testID="permission" style={{height:20}} onPress={() => Sensors.permission("location").then(s => state.set("permission " + s))} />
  <View testID="ask" style={{height:20}} onPress={() => Sensors.requestPermission("motion").then(s => state.set("asked " + s))} />
  <View testID="locate" style={{height:20}} onPress={() => Sensors.locate().then(f => state.set("fix " + f.latitude + " " + f.altitude + " " + f.cached + " " + f.approximate))} />
  <View testID="motion" style={{height:20}} onPress={() => stop = Sensors.watchMotion({sensors:1, interval:1}, m => state.set("motion " + m.vector.z + " " + m.interval + " " + m.dropped))} />
  <View testID="stop" style={{height:20}} onPress={() => stop()} />
  <Text testID="state">{state}</Text>
 </View>;
}
mount(App);`)
	app.Permissions[core.PermissionLocation] = core.PermissionGranted
	app.PermissionAnswers[core.PermissionMotion] = core.PermissionGranted
	app.Press("id:permission")
	app.WaitFor("text:permission granted", time.Second)
	app.Press("id:ask")
	app.WaitFor("text:asked granted", time.Second)
	app.Press("id:locate")
	app.FeedLocation(core.LocationFix{Latitude: 42, Timestamp: app.Clock.Now(), Approximate: true})
	app.Settle()
	app.WaitFor("text:fix 42 null false true", time.Second)
	app.Press("id:motion")
	app.FeedMotion(core.MotionSample{Sensor: core.SensorAccelerometer, Vector: core.SensorVector{Z: 9.81}})
	app.Settle()
	app.WaitFor("text:motion 9.81 10 0", time.Second)
	app.Press("id:stop")
	if app.ActiveSensorStreams() != 0 {
		t.Fatal("TSX stop retained a provider")
	}
}

func TestExtendedSensorsTSXValuesStopsAndAccuracy(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("synthetic sensor fixtures run headless only")
	}
	app := run(t, `import {Sensors,View,Text,mount,signal} from "nsgo";
 function App(){const state=signal("idle");let stops=[];
 return <View>
 <View testID="start" style={{height:20}} onPress={()=>{stops=[
 Sensors.watchSteps(v=>state.set("steps "+v.count+" "+v.distance+" "+v.floorsAscended)),
 Sensors.watchActivity(v=>state.set("activity "+v.kinds+" "+v.confidence)),
 Sensors.watchPressure(v=>state.set("pressure "+v.pascals+" "+v.relativeAltitude)),
 Sensors.watchGeofences({regions:[{id:"owned",latitude:0,longitude:0,radius:100}]},v=>state.set("region "+v.region+" "+v.state)),
 Sensors.watchBackgroundMotion({duration:5000},v=>state.set("motion "+v.vector.z))];}} />
 <View testID="precise" style={{height:20}} onPress={()=>Sensors.requestPreciseLocation("purpose").then(v=>state.set("precise "+v))}/>
 <View testID="invalid" style={{height:20}} onPress={()=>{let n=0;for(const v of [undefined,null,23,{}]){try{Sensors.requestPreciseLocation(v)}catch(e){if(e.name==="TypeError")n++}}state.set("invalid "+n)}}/>
 <View testID="stop" style={{height:20}} onPress={()=>stops.forEach(s=>s())}/>
 <Text>{state}</Text></View>}mount(App);`)
	for _, p := range []core.Permission{core.PermissionLocation, core.PermissionMotion, core.PermissionActivityRecognition} {
		app.Permissions[p] = core.PermissionGranted
	}
	app.Press("id:start")
	if app.ActiveSensorStreams() != 5 {
		t.Fatalf("started %d streams", app.ActiveSensorStreams())
	}
	app.FeedSteps(core.StepSample{Count: 27, Distance: core.SensorNumber{Value: 0, Valid: true}})
	app.Settle()
	app.WaitFor("text:steps 27 0 null", time.Second)
	app.FeedActivity(core.ActivitySample{Kinds: core.ActivityWalking, Confidence: core.ActivityConfidenceHigh})
	app.Settle()
	app.WaitFor("text:activity 2 3", time.Second)
	app.FeedPressure(core.PressureSample{Pascals: 101325})
	app.Settle()
	app.WaitFor("text:pressure 101325 null", time.Second)
	app.FeedGeofence(core.GeofenceSample{Region: "owned", State: core.GeofenceInside})
	app.Settle()
	app.WaitFor("text:region owned inside", time.Second)
	app.PreciseLocationAnswer = true
	app.Press("id:precise")
	app.WaitFor("text:precise true", time.Second)
	app.Press("id:invalid")
	app.WaitFor("text:invalid 4", time.Second)
	app.Press("id:stop")
	if app.ActiveSensorStreams() != 0 {
		t.Fatal("extended TSX subscriptions leaked")
	}
}
