package android.content;

import android.content.res.AssetManager;
import java.io.File;

public class Context {
    public static final String AUDIO_SERVICE = "audio";
    public static final int RECEIVER_NOT_EXPORTED = 4;

    public Context getApplicationContext() { return this; }
    public Object getSystemService(String name) { return null; }
    public Intent registerReceiver(BroadcastReceiver receiver, IntentFilter filter) { return null; }
    public Intent registerReceiver(BroadcastReceiver receiver, IntentFilter filter, int flags) { return null; }
    public void unregisterReceiver(BroadcastReceiver receiver) {}
    public AssetManager getAssets() { return new AssetManager(); }
    public File getCacheDir() { return new File(System.getProperty("java.io.tmpdir")); }
}
