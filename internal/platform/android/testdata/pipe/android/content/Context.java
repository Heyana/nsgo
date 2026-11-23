package android.content;

import java.io.File;

/** Only the application-context calls the engine makes; no Android UI. */
public class Context {
    public Context getApplicationContext() { return this; }
    public Resources getResources() { return new Resources(); }
    public File getFilesDir() { return new File("."); }
    public Intent registerReceiver(BroadcastReceiver receiver, IntentFilter filter) { return null; }
    public void unregisterReceiver(BroadcastReceiver receiver) {}

    public static final class Resources {
        public Metrics getDisplayMetrics() { return new Metrics(); }
    }

    public static final class Metrics {
        public float density = 1;
    }
}
