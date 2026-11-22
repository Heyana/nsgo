package dev.oneo.nsgo;

import android.Manifest;
import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.hardware.Sensor;
import android.hardware.SensorEvent;
import android.hardware.SensorEventListener;
import android.hardware.SensorManager;
import android.location.Location;
import android.location.LocationListener;
import android.location.LocationManager;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.CancellationSignal;
import android.os.Handler;
import android.os.Looper;
import android.provider.Settings;

import org.json.JSONObject;
import org.json.JSONException;

import java.io.DataInputStream;
import java.io.IOException;
import java.time.Instant;
import java.util.ArrayDeque;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.function.Supplier;

/** Framework providers only: no Play services, implicit prompts or unbounded sample queues. */
final class Sensors {
    static final int LOCATION = 0, BACKGROUND = 1, MOTION = 2, STEPS = 3, ACTIVITY = 4, PRESSURE = 5, GEOFENCES = 6, ACTIVITY_PERMISSION = 7;
    static final int READ = 0, ASK = 1, LOCATE = 2, WATCH_LOCATION = 3, WATCH_MOTION = 4, STOP = 5, WATCH_STEPS = 6, WATCH_ACTIVITY = 7, WATCH_PRESSURE = 8, WATCH_GEOFENCES = 9, PRECISE = 10, BACKGROUND_MOTION = 11;
    static final int NOT_ASKED = 0, GRANTED = 1, DENIED = 2, RESTRICTED = 3, UNAVAILABLE = 4;
    static final int PERMISSION_REQUEST = 0x6e7c;
    private final Context context;
    private final Supplier<Activity> activity;
    private final NsgoEngine engine;
    /** Runs once the last background stream stops, so a view kept off screen for it can end the app's process (NsgoView.keeperEnded). */
    private final Runnable backgroundEnded;
    private final Handler ui = new Handler(Looper.getMainLooper());
    private final HashMap<Long, Run> runs = new HashMap<>();
    private final ArrayDeque<Run> prompts = new ArrayDeque<>();
    private final LinkedHashMap<String, JSONObject> samples = new LinkedHashMap<>();
    private final ArrayDeque<JSONObject> answers = new ArrayDeque<>();
    private final ExecutorService writer = Executors.newSingleThreadExecutor(r -> new Thread(r, "nsgo-sensors"));
    private boolean writing, closed;
    private final SensorExtras extras;

    Sensors(Context context, Supplier<Activity> activity, NsgoEngine engine, Runnable backgroundEnded) {
        this.context = context.getApplicationContext(); this.activity = activity; this.engine = engine; this.backgroundEnded = backgroundEnded;
        extras=new SensorExtras(this.context,ui,new SensorExtras.Sink(){
            public boolean active(Run r){return !closed&&runs.get(r.id)==r;}
            public void sample(Run r,String field,JSONObject value,String key){if(active(r))send(objectWith(report(r),field,value),r.id+":"+key);}
            public void fail(Run r,String error){Sensors.this.fail(r,error);}
        });
    }

    static final class Run {
        long id, generation, interval, maxAge, startedMillis, startedNanos;
        int command, kind, mask;
        double accuracy, distance;
        boolean background;
        String title, body;
        LocationListener location;
        SensorEventListener motion;
        CancellationSignal cancellation;
        Object serviceToken;
        JSONObject extra=new JSONObject();
        SensorExtras.Watch extraWatch;
        Runnable deadline;
        long endNanos;
    }

    void handle(DataInputStream in) throws IOException {
        Run r = new Run();
        r.id = in.readInt() & 0xffffffffL;
        r.command = in.readUnsignedByte(); r.kind = in.readUnsignedByte(); r.generation = in.readInt() & 0xffffffffL;
        r.accuracy = in.readDouble(); r.distance = in.readDouble();
        r.interval = in.readLong(); r.maxAge = in.readLong();
        r.background = in.readBoolean(); r.title = Protocol.readString(in); r.body = Protocol.readString(in);
        r.mask = in.readUnsignedByte(); long motionInterval = in.readLong();
        if (r.kind == MOTION) r.interval = motionInterval;
        if(r.command>=WATCH_STEPS){try{r.extra=new JSONObject(Protocol.readString(in));}catch(JSONException e){throw new IOException("invalid extended sensor options",e);}}
        if(r.kind==GEOFENCES){JSONObject o=r.extra.optJSONObject("Geofences");if(o!=null){r.background=o.optBoolean("Background");r.title=o.optString("NotificationTitle");r.body=o.optString("NotificationBody");}}

        stop(r.id);
        if (closed) return;
        if (r.command == STOP) { send(objectWith(report(r), "End", true), null); return; }
        runs.put(r.id, r);
        if(!SensorPolicy.validRequest(r.command,r.kind)){fail(r,"unavailable");return;}

        if(r.command==PRECISE){
            if(!declared(Manifest.permission.ACCESS_FINE_LOCATION)||!declared(Manifest.permission.ACCESS_COARSE_LOCATION)){fail(r,"not-configured");return;}
            if(authorization(LOCATION)!=GRANTED){fail(r,"not-allowed");return;}
            if(allowed(Manifest.permission.ACCESS_FINE_LOCATION)){grant(r);return;}
            prompts.add(r);if(prompts.size()==1)prompt(r);return;
        }
        if (r.command <= ASK) {
            if (r.command == READ || r.kind == MOTION || authorization(r.kind) == GRANTED || authorization(r.kind) == UNAVAILABLE) { grant(r); return; }
            prompts.add(r);
            if (prompts.size() == 1) prompt(r);
            return;
        }
        if (authorization(r.kind) != GRANTED) { fail(r, authorization(r.kind) == UNAVAILABLE ? "unavailable" : "not-allowed"); return; }

        if(r.command==BACKGROUND_MOTION){startBackgroundMotion(r);return;}
        if(r.kind==GEOFENCES&&r.background){
            if(authorization(BACKGROUND)!=GRANTED){fail(r,"not-allowed");return;}
            if(r.title.isEmpty()){fail(r,"not-configured");return;}
            try{r.serviceToken=new Object();LocationService.add(context,r.serviceToken,r.title,r.body,()->fail(r,"failed"));}catch(RuntimeException e){fail(r,"failed");return;}
        }
        if(r.kind>=STEPS){extras.start(r);return;}
        if (r.kind == MOTION) startMotion(r); else startLocation(r);
    }

    private boolean allowed(String permission) { return context.checkSelfPermission(permission) == PackageManager.PERMISSION_GRANTED; }

    private boolean declared(String permission) {
        try {
            String[] p = context.getPackageManager().getPackageInfo(context.getPackageName(), PackageManager.GET_PERMISSIONS).requestedPermissions;
            return p != null && java.util.Arrays.asList(p).contains(permission);
        } catch (PackageManager.NameNotFoundException e) { return false; }
    }

    private int authorization(int kind) {

        if(kind==ACTIVITY)return UNAVAILABLE;
        if(kind==STEPS||kind==ACTIVITY_PERMISSION){
            if(context.getSystemService(SensorManager.class)==null)return UNAVAILABLE;
            if(Build.VERSION.SDK_INT<29)return GRANTED;
            if(!declared(Manifest.permission.ACTIVITY_RECOGNITION))return UNAVAILABLE;
            if(allowed(Manifest.permission.ACTIVITY_RECOGNITION))return GRANTED;
            return context.getSharedPreferences("nsgo.sensors",Context.MODE_PRIVATE).getBoolean("asked."+ACTIVITY_PERMISSION,false)?DENIED:NOT_ASKED;
        }
        if(kind==PRESSURE)return context.getSystemService(SensorManager.class)==null?UNAVAILABLE:GRANTED;
        if (kind == MOTION) return context.getSystemService(SensorManager.class) == null ? UNAVAILABLE : GRANTED;
        if (context.getSystemService(LocationManager.class) == null) return UNAVAILABLE;
        if (kind == BACKGROUND) {
            // nsgo build declares LocationService only for an app that lists FOREGROUND_SERVICE_LOCATION, on every API level.
            if (!declared(Manifest.permission.FOREGROUND_SERVICE) || !declared(Manifest.permission.FOREGROUND_SERVICE_LOCATION)) return UNAVAILABLE;
            // An explicitly started location foreground service continues a
            // foreground grant. ACCESS_BACKGROUND_LOCATION is for starting
            // location while already in background and is requested separately.
            if (Build.VERSION.SDK_INT < 29 || !declared(Manifest.permission.ACCESS_BACKGROUND_LOCATION)) return authorization(LOCATION);
            if (allowed(Manifest.permission.ACCESS_BACKGROUND_LOCATION)) return GRANTED;
        } else {
            if (allowed(Manifest.permission.ACCESS_FINE_LOCATION) || allowed(Manifest.permission.ACCESS_COARSE_LOCATION)) return GRANTED;
            if (!declared(Manifest.permission.ACCESS_COARSE_LOCATION)) return UNAVAILABLE;
        }
        return context.getSharedPreferences("nsgo.sensors", Context.MODE_PRIVATE).getBoolean("asked." + kind, false) ? DENIED : NOT_ASKED;
    }

    private void prompt(Run r) {
        if (runs.get(r.id) != r) { finishPrompt(); return; }
        Activity a = activity.get();
        if (a == null || r.kind == BACKGROUND && authorization(LOCATION) != GRANTED) { grant(r); finishPrompt(); return; }
        context.getSharedPreferences("nsgo.sensors", Context.MODE_PRIVATE).edit().putBoolean("asked." + ((r.kind==STEPS||r.kind==ACTIVITY_PERMISSION)?ACTIVITY_PERMISSION:r.kind), true).apply();
        if (r.kind == BACKGROUND && Build.VERSION.SDK_INT >= 30 && declared(Manifest.permission.ACCESS_BACKGROUND_LOCATION)) {
            // Android 11 removed "Allow all the time" from the runtime
            // prompt. Only this explicit API opens the app's settings.
            a.startActivity(new Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.parse("package:" + context.getPackageName())));
            awaitSettings(r);
            return;
        }
        String[] permissions;
        if(r.kind==STEPS||r.kind==ACTIVITY_PERMISSION){permissions=new String[]{Manifest.permission.ACTIVITY_RECOGNITION};a.requestPermissions(permissions,PERMISSION_REQUEST);return;}
        if (r.kind == BACKGROUND && Build.VERSION.SDK_INT >= 29 && declared(Manifest.permission.ACCESS_BACKGROUND_LOCATION)) {
            permissions = new String[]{Manifest.permission.ACCESS_BACKGROUND_LOCATION};
        } else if (declared(Manifest.permission.ACCESS_FINE_LOCATION)) {
            permissions = new String[]{Manifest.permission.ACCESS_COARSE_LOCATION, Manifest.permission.ACCESS_FINE_LOCATION};
        } else { permissions = new String[]{Manifest.permission.ACCESS_COARSE_LOCATION}; }
        a.requestPermissions(permissions, PERMISSION_REQUEST);
    }

    private boolean settingsLeft;
    private void awaitSettings(Run r) {
        settingsLeft = false;
        Runnable poll = new Runnable() { public void run() {
            if (closed || prompts.peek() != r || runs.get(r.id) != r) return;
            Activity a = activity.get();
            if (a != null && !a.hasWindowFocus()) settingsLeft = true;
            if (settingsLeft && a != null && a.hasWindowFocus()) { grant(r); finishPrompt(); return; }
            ui.postDelayed(this, 200);
        }};
        ui.postDelayed(poll, 200);
    }

    void permissionResult() {
        Run r = prompts.peek();
        if (r != null) grant(r);
        finishPrompt();
    }

    private void finishPrompt() {
        prompts.poll();
        if (!prompts.isEmpty()) ui.post(() -> { if (!prompts.isEmpty()) prompt(prompts.peek()); });
    }

    private JSONObject report(Run r) { return object("id", r.id, "Generation", r.generation); }
    private void grant(Run r) {

        if(runs.get(r.id)!=r)return;
        if(r.command==PRECISE){send(objectWith(report(r),"Precise",object("Full",allowed(Manifest.permission.ACCESS_FINE_LOCATION)),"End",true),null);stopProvider(r);return;}
        JSONObject g = object("Authorization", authorization(r.kind));
        send(objectWith(report(r), "Grant", g, "End", true), null);
        stopProvider(r);
    }
    private void fail(Run r, String code) {
        if (runs.get(r.id) != r) return;
        send(objectWith(report(r), "Error", code, "End", true), null);
        stopProvider(r);
    }

    private String provider(LocationManager manager, double accuracy) {
        boolean fused = Build.VERSION.SDK_INT >= 31 && manager.hasProvider(LocationManager.FUSED_PROVIDER) && manager.isProviderEnabled(LocationManager.FUSED_PROVIDER);
        return SensorPolicy.provider(Build.VERSION.SDK_INT, allowed(Manifest.permission.ACCESS_FINE_LOCATION), fused,
            manager.isProviderEnabled(LocationManager.GPS_PROVIDER), manager.isProviderEnabled(LocationManager.NETWORK_PROVIDER), accuracy);
    }

    private void startLocation(Run r) {
        LocationManager manager = context.getSystemService(LocationManager.class);
        try {
            String provider = provider(manager, r.accuracy);
            if (provider == null) { fail(r, "unavailable"); return; }
            if (r.background) {
                if (authorization(BACKGROUND) != GRANTED) { fail(r, "not-allowed"); return; }
                if (r.title.isEmpty()) { fail(r, "not-configured"); return; }
                r.serviceToken = new Object();
                LocationService.add(context, r.serviceToken, r.title, r.body, () -> fail(r, "failed"));
            }
            r.startedMillis = System.currentTimeMillis(); r.startedNanos = android.os.SystemClock.elapsedRealtimeNanos();
            if (r.command == LOCATE && r.maxAge > 0) {
                Location cached = manager.getLastKnownLocation(provider);
                if (acceptable(r, cached)) { location(r, cached); return; }
            }
            r.location = new LocationListener() {
                public void onLocationChanged(Location l) {
                    if (runs.get(r.id) != r) return;
                    if (acceptable(r, l)) location(r, l);
                    else if (r.command == LOCATE) ui.postDelayed(() -> { if (runs.get(r.id) == r) freshLocation(r, manager, provider); }, 200);
                }
                public void onProviderDisabled(String p) { if (runs.get(r.id) == r) { stopProvider(r); runs.put(r.id, r); startLocation(r); } }
                public void onProviderEnabled(String p) {}
                public void onStatusChanged(String p, int status, Bundle extras) {}
            };
            if (r.command == LOCATE) freshLocation(r, manager, provider);
            else if (Build.VERSION.SDK_INT >= 31) {
                android.location.LocationRequest request = new android.location.LocationRequest.Builder(Math.max(0, r.interval/1000000L))
                    .setMinUpdateDistanceMeters((float)r.distance)
                    .setQuality(r.accuracy > 0 && r.accuracy <= 100 ? android.location.LocationRequest.QUALITY_HIGH_ACCURACY : android.location.LocationRequest.QUALITY_BALANCED_POWER_ACCURACY).build();
                manager.requestLocationUpdates(provider, request, context.getMainExecutor(), r.location);
            } else manager.requestLocationUpdates(provider, Math.max(0, r.interval/1000000L), (float)r.distance, r.location, Looper.getMainLooper());
        } catch (SecurityException e) { fail(r, "not-allowed"); }
        catch (RuntimeException e) { fail(r, "failed"); }
    }

    private void freshLocation(Run r, LocationManager manager, String provider) {
        try {
            if (Build.VERSION.SDK_INT >= 30) {
                r.cancellation = new CancellationSignal();
                manager.getCurrentLocation(provider, r.cancellation, context.getMainExecutor(), l -> {
                    if (runs.get(r.id) != r) return;
                    if (l == null) fail(r, "unavailable"); else r.location.onLocationChanged(l);
                });
            } else manager.requestSingleUpdate(provider, r.location, Looper.getMainLooper());
        } catch (SecurityException e) { fail(r, "not-allowed"); }
        catch (RuntimeException e) { fail(r, "failed"); }
    }


    private boolean acceptable(Run r, Location l) {
        return l != null && l.hasAccuracy() && SensorPolicy.fresh(l.getElapsedRealtimeNanos(), r.startedNanos, android.os.SystemClock.elapsedRealtimeNanos(), r.maxAge);
    }

    private void location(Run r, Location l) {
        long age = Math.max(0, android.os.SystemClock.elapsedRealtimeNanos()-l.getElapsedRealtimeNanos());
        JSONObject f = object("Latitude", l.getLatitude(), "Longitude", l.getLongitude(),
            "Timestamp", Instant.ofEpochMilli(l.getTime()).toString(), "Monotonic", l.getElapsedRealtimeNanos(), "HasMonotonic", true,
            "Age", age, "Cached", l.getElapsedRealtimeNanos() < r.startedNanos, "Approximate", !allowed(Manifest.permission.ACCESS_FINE_LOCATION),
            "HorizontalAccuracy", number(l.getAccuracy(), l.hasAccuracy()), "Altitude", number(l.getAltitude(), l.hasAltitude()),
            "VerticalAccuracy", number(l.hasVerticalAccuracy() ? l.getVerticalAccuracyMeters() : 0, l.hasVerticalAccuracy()),
            "Speed", number(l.getSpeed(), l.hasSpeed()), "Course", number(l.getBearing(), l.hasBearing()));
        if (Build.VERSION.SDK_INT >= 34 && l.hasMslAltitude()) put(f, "MSLAltitude", number(l.getMslAltitudeMeters(), true));
        JSONObject out = objectWith(report(r), "Location", f);
        if (r.command == LOCATE) { put(out, "End", true); send(out, null); stopProvider(r); }
        else send(out, r.id + ":location");
    }

    private static int sensorType(int bit) {
        switch (bit) {
        case 1: return Sensor.TYPE_ACCELEROMETER;
        case 2: return Sensor.TYPE_GYROSCOPE;
        case 4: return Sensor.TYPE_MAGNETIC_FIELD;
        default: return Sensor.TYPE_ROTATION_VECTOR;
        }
    }

    private void startMotion(Run r) {
        SensorManager manager = context.getSystemService(SensorManager.class);
        for (int bit = 1; bit <= 8; bit <<= 1) if ((r.mask & bit) != 0 && manager.getDefaultSensor(sensorType(bit)) == null) { fail(r, "unavailable"); return; }
        // Fused readings keep fields absent until each actual sensor has
        // delivered. Require every promised axis rather than fabricate it.
        if ((r.mask & 8) != 0 && (manager.getDefaultSensor(Sensor.TYPE_GRAVITY) == null || manager.getDefaultSensor(Sensor.TYPE_LINEAR_ACCELERATION) == null || manager.getDefaultSensor(Sensor.TYPE_GYROSCOPE) == null)) { fail(r, "unavailable"); return; }
        r.motion = new SensorEventListener() {
            JSONObject gravity, acceleration, rotation;
            long[] last = new long[4];
            public void onAccuracyChanged(Sensor sensor, int accuracy) {}
            public void onSensorChanged(SensorEvent e) {
                if (runs.get(r.id) != r) return;
                if(r.command==BACKGROUND_MOTION&&android.os.SystemClock.elapsedRealtimeNanos()>=r.endNanos){fail(r,"timeout");return;}
                int type = e.sensor.getType(), bit;
                if (type == Sensor.TYPE_GRAVITY) { gravity = vector(e.values); return; }
                if (type == Sensor.TYPE_LINEAR_ACCELERATION) { acceleration = vector(e.values); return; }
                if (type == Sensor.TYPE_GYROSCOPE) { rotation = vector(e.values); bit = 2; }
                else if (type == Sensor.TYPE_ACCELEROMETER) bit = 1;
                else if (type == Sensor.TYPE_MAGNETIC_FIELD) bit = 4;
                else if (type == Sensor.TYPE_ROTATION_VECTOR) bit = 8;
                else return;
                if ((r.mask & bit) == 0) return;
                int index = Integer.numberOfTrailingZeros(bit);
                long interval = last[index] == 0 ? r.interval : e.timestamp-last[index]; last[index] = e.timestamp;
                JSONObject m = object("Sensor", bit, "Timestamp", e.timestamp, "Interval", interval, "Accuracy", e.accuracy);
                if (bit == 8) {
                    if (gravity == null || acceleration == null || rotation == null) return;
                    float[] q = new float[4]; SensorManager.getQuaternionFromVector(q, e.values);
                    put(m, "Attitude", object("X", q[1], "Y", q[2], "Z", q[3], "W", q[0]));
                    put(m, "HeadingReferenced", true); put(m, "Gravity", gravity); put(m, "UserAcceleration", acceleration); put(m, "RotationRate", rotation);
                } else put(m, "Vector", vector(e.values));
                send(objectWith(report(r), "Motion", m), r.id + ":motion:" + bit);
            }
        };
        try {
            int period = SensorPolicy.period(r.interval, 0);
            for (int bit = 1; bit <= 8; bit <<= 1) if ((r.mask & bit) != 0) {
                Sensor sensor = manager.getDefaultSensor(sensorType(bit));
                if (!manager.registerListener(r.motion, sensor, SensorPolicy.period(r.interval, sensor.getMinDelay()), ui)) { fail(r, "unavailable"); return; }
            }
            if ((r.mask & 8) != 0) for (int type : new int[]{Sensor.TYPE_GRAVITY, Sensor.TYPE_LINEAR_ACCELERATION, Sensor.TYPE_GYROSCOPE}) {
                if (type == Sensor.TYPE_GYROSCOPE && (r.mask & 2) != 0) continue;
                if (!manager.registerListener(r.motion, manager.getDefaultSensor(type), period, ui)) { fail(r, "unavailable"); return; }
            }
        } catch (SecurityException e) { fail(r, "not-allowed"); }
    }


    private void startBackgroundMotion(Run r){
        JSONObject o=r.extra.optJSONObject("BackgroundMotion");
        if(o==null||o.optLong("Duration")<=0){fail(r,"failed");return;}
        if(!declared(Manifest.permission.FOREGROUND_SERVICE)||o.optString("NotificationTitle").isEmpty()){fail(r,"not-configured");return;}
        try{
            context.getPackageManager().getServiceInfo(new android.content.ComponentName(context,MotionService.class),0);
            r.serviceToken=new Object();
            MotionService.add(context,r.serviceToken,o.optString("NotificationTitle"),o.optString("NotificationBody"),()->fail(r,"failed"),()->fail(r,"timeout"));
            long duration=Math.min(o.optLong("Duration"),180000000000L);
            r.endNanos=android.os.SystemClock.elapsedRealtimeNanos()+duration;
            r.deadline=()->{if(runs.get(r.id)==r)fail(r,"timeout");};ui.postDelayed(r.deadline,Math.max(1,duration/1000000));
            startMotion(r);
        }catch(android.content.pm.PackageManager.NameNotFoundException e){fail(r,"not-configured");}
        catch(RuntimeException e){fail(r,"failed");}
    }

    boolean hasBackground() { for (Run r : runs.values()) if (r.serviceToken != null) return true; return false; }

    private void stop(long id) { Run r = runs.get(id); if (r != null) stopProvider(r); }
    private void stopProvider(Run r) {
        runs.remove(r.id, r);
        if (r.cancellation != null) r.cancellation.cancel();
        if(r.deadline!=null)ui.removeCallbacks(r.deadline);
        if(r.extraWatch!=null){r.extraWatch.close();r.extraWatch=null;}
        if (r.location != null) context.getSystemService(LocationManager.class).removeUpdates(r.location);
        if (r.motion != null) context.getSystemService(SensorManager.class).unregisterListener(r.motion);
        if (r.serviceToken != null) {
            if(r.command==BACKGROUND_MOTION)MotionService.remove(context,r.serviceToken);else LocationService.remove(context, r.serviceToken);
            r.serviceToken = null;
            // Posted: the view may stop, which closes these sensors, and a closing Sensors ends nothing.
            if (!closed && !hasBackground()) ui.post(() -> { if (!closed) backgroundEnded.run(); });
        }
    }

    void close() {
        if (closed) return;
        closed = true;
        for (Run r : new java.util.ArrayList<>(runs.values())) stopProvider(r);
        prompts.clear();
        synchronized (this) { samples.clear(); answers.clear(); }
        writer.shutdownNow();
    }

    /** Native callbacks never wait on Go's pipe. Only the latest sample
     * per stream/sensor waits behind the one serialized writer. */
    private synchronized void send(JSONObject report, String key) {
        if (closed) return;
        if (key == null) answers.add(report);
        else {
            JSONObject old = samples.put(key, report);
            if (old != null) {
                String field="Location";for(String candidate:new String[]{"Motion","Steps","Activity","Pressure","Geofence"})if(report.has(candidate)){field=candidate;break;}
                JSONObject value = report.optJSONObject(field), previous = old.optJSONObject(field);
                put(value, "Dropped", value.optLong("Dropped") + (previous == null ? 0 : previous.optLong("Dropped")) + 1);
            }
        }
        if (!writing) { writing = true; writer.execute(this::drain); }
    }

    private void drain() {
        while (true) {
            JSONObject report;
            synchronized (this) {
                report = answers.poll();
                if (report == null && !samples.isEmpty()) { String key = samples.keySet().iterator().next(); report = samples.remove(key); }
                if (report == null || closed) { writing = false; return; }
            }
            engine.write(Protocol.SENSOR_REPORT, report.toString().getBytes(java.nio.charset.StandardCharsets.UTF_8));
        }
    }

    private static JSONObject vector(float[] v) { return object("X", v[0], "Y", v[1], "Z", v[2]); }
    static JSONObject number(double v, boolean valid) { return object("Value", v, "Valid", valid); }
    static void put(JSONObject object, String key, Object value) { try { object.put(key, value); } catch (JSONException e) { throw new IllegalArgumentException(e); } }
    static JSONObject objectWith(JSONObject out, Object... pairs) { for (int i = 0; i < pairs.length; i += 2) put(out, (String)pairs[i], pairs[i+1]); return out; }
    static JSONObject object(Object... pairs) { return objectWith(new JSONObject(), pairs); }
}
