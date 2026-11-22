package dev.oneo.nsgo;

import android.content.BroadcastReceiver;
import android.app.PendingIntent;
import android.os.Build;
import android.content.Context;
import android.content.Intent;
import android.location.LocationManager;
import java.util.HashMap;
import java.util.function.Consumer;

/** A non-exported, explicit PendingIntent target. Only live scoped watches are
 * registered; delivery after process death starts no engine and does nothing. */
public final class SensorRegionReceiver extends BroadcastReceiver {
    private static final HashMap<String, Consumer<Boolean>> listeners = new HashMap<>();
    static void add(String key, Consumer<Boolean> listener) { listeners.put(key, listener); }
    static void remove(String key) { listeners.remove(key); }
    @Override public void onReceive(Context context, Intent intent) {
        if (intent == null || intent.getData() == null || !intent.hasExtra(LocationManager.KEY_PROXIMITY_ENTERING)) return;
        Consumer<Boolean> listener = listeners.get(intent.getData().toString());
        if (listener != null) listener.accept(intent.getBooleanExtra(LocationManager.KEY_PROXIMITY_ENTERING, false));
        else if(intent.getData().toString().startsWith("nsgo-sensor-region:")) {
            // A queued callback must not resurrect a dead subscription or keep waking the app after its original process has ended.
            int flags=PendingIntent.FLAG_NO_CREATE|(Build.VERSION.SDK_INT>=31?PendingIntent.FLAG_MUTABLE:0);
            PendingIntent pending=PendingIntent.getBroadcast(context,0,new Intent(context,SensorRegionReceiver.class).setData(intent.getData()),flags);
            if(pending!=null){try{context.getSystemService(LocationManager.class).removeProximityAlert(pending);}catch(SecurityException ignored){}finally{pending.cancel();}}
        }
    }
}
