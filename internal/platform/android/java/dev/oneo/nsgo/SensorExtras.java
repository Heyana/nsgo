package dev.oneo.nsgo;

import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.hardware.Sensor;
import android.hardware.SensorEvent;
import android.hardware.SensorEventListener;
import android.hardware.SensorManager;
import android.location.LocationManager;
import android.net.Uri;
import android.os.Build;
import android.os.Handler;
import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;
import java.time.Instant;
import java.util.ArrayList;
import java.util.UUID;

/** Framework step counters, barometers and proximity alerts; no Play services,
 * inferred activities, unbounded queues or implicit authorization. */
final class SensorExtras {
    interface Watch { void close(); }
    interface Sink {
        boolean active(Sensors.Run run);
        void sample(Sensors.Run run, String field, JSONObject value, String key);
        void fail(Sensors.Run run, String error);
    }
    private final Context context;
    private final Handler ui;
    private final Sink sink;
    SensorExtras(Context context, Handler ui, Sink sink) { this.context=context;this.ui=ui;this.sink=sink; }

    void start(Sensors.Run run) {
        try {
            switch (run.kind) {
                case Sensors.STEPS: startCounter(run); return;
                case Sensors.PRESSURE: startPressure(run); return;
                case Sensors.GEOFENCES: startRegions(run); return;
                // The Android framework has no CMMotionActivity-style classifier. Do not substitute significant-motion triggers for walking/driving.
                default: sink.fail(run,"unavailable");
            }
        } catch (SecurityException e) { sink.fail(run,"not-allowed"); }
        catch (RuntimeException | JSONException e) { sink.fail(run,"failed"); }
    }

    private void startCounter(Sensors.Run run) {
        SensorManager manager=context.getSystemService(SensorManager.class);
        Sensor sensor=manager==null?null:manager.getDefaultSensor(Sensor.TYPE_STEP_COUNTER);
        if(sensor==null){sink.fail(run,"unavailable");return;}
        SensorEventListener listener=new SensorEventListener(){
            double baseline=Double.NaN,last;
            String since;
            public void onAccuracyChanged(Sensor sensor,int accuracy) {}
            public void onSensorChanged(SensorEvent event){
                if(!sink.active(run)||event.values.length==0)return;
                double total=event.values[0];
                if(!Double.isFinite(total)||total<0)return;
                String stamp=wallTimestamp(event.timestamp);
                boolean reset=!Double.isNaN(baseline)&&total<last;
                if(Double.isNaN(baseline)||reset){baseline=total;since=stamp;}
                last=total;
                JSONObject value=Sensors.object("Count",Math.round(total-baseline),"Since",since,"Timestamp",stamp,"Gap",reset,"Dropped",0);
                sink.sample(run,"Steps",value,"steps");
            }
        };
        run.extraWatch=()->manager.unregisterListener(listener);
        if(!manager.registerListener(listener,sensor,SensorManager.SENSOR_DELAY_NORMAL,ui))sink.fail(run,"unavailable");
    }

    private void startPressure(Sensors.Run run) {
        SensorManager manager=context.getSystemService(SensorManager.class);
        Sensor sensor=manager==null?null:manager.getDefaultSensor(Sensor.TYPE_PRESSURE);
        if(sensor==null){sink.fail(run,"unavailable");return;}
        SensorEventListener listener=new SensorEventListener(){
            long previous;
            public void onAccuracyChanged(Sensor sensor,int accuracy) {}
            public void onSensorChanged(SensorEvent event){
                if(!sink.active(run)||event.values.length==0)return;
                double pascals=event.values[0]*100.0;
                if(!Double.isFinite(pascals)||pascals<=0)return;
                long interval=previous==0?0:Math.max(0,event.timestamp-previous);previous=event.timestamp;
                sink.sample(run,"Pressure",Sensors.object("Pascals",pascals,"Timestamp",event.timestamp,"Interval",interval,"Gap",false,"Dropped",0),"pressure");
            }
        };
        run.extraWatch=()->manager.unregisterListener(listener);
        if(!manager.registerListener(listener,sensor,SensorManager.SENSOR_DELAY_NORMAL,ui))sink.fail(run,"unavailable");
    }

    private void startRegions(Sensors.Run run) throws JSONException {
        LocationManager manager=context.getSystemService(LocationManager.class);
        if(manager==null){sink.fail(run,"unavailable");return;}
        JSONObject options=run.extra.optJSONObject("Geofences");
        JSONArray regions=options==null?null:options.optJSONArray("Regions");
        if(regions==null||regions.length()==0||regions.length()>20){sink.fail(run,"failed");return;}
        ArrayList<PendingIntent> intents=new ArrayList<>();
        ArrayList<String> keys=new ArrayList<>();
        run.extraWatch=()->{
            for(String key:keys)SensorRegionReceiver.remove(key);
            for(PendingIntent intent:intents){try{manager.removeProximityAlert(intent);}catch(SecurityException ignored){}finally{intent.cancel();}}
        };
        for(int i=0;i<regions.length();i++){
            JSONObject region=regions.getJSONObject(i);
            String id=region.getString("ID");
            String key="nsgo-sensor-region:"+UUID.randomUUID();
            SensorRegionReceiver.add(key,entering->{
                if(sink.active(run))sink.sample(run,"Geofence",Sensors.object("Region",id,"State",entering?1:2,"Timestamp",Instant.now().toString(),"Gap",false,"Dropped",0),"region:"+id);
            });
            keys.add(key);
            Intent intent=new Intent(context,SensorRegionReceiver.class).setData(Uri.parse(key));
            int flags=PendingIntent.FLAG_UPDATE_CURRENT;
            if(Build.VERSION.SDK_INT>=31)flags|=PendingIntent.FLAG_MUTABLE;
            PendingIntent pending=PendingIntent.getBroadcast(context,0,intent,flags);
            intents.add(pending);
            manager.addProximityAlert(region.getDouble("Latitude"),region.getDouble("Longitude"),(float)region.getDouble("Radius"),-1,pending);
        }
    }

    private static String wallTimestamp(long elapsedNanos) {
        long age=Math.max(0,android.os.SystemClock.elapsedRealtimeNanos()-elapsedNanos);
        return Instant.ofEpochMilli(System.currentTimeMillis()-age/1000000).toString();
    }
}
