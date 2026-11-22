package android.content;
import android.app.*;
import android.content.pm.*;
public class Context {
    public static boolean permission = true, refuseService;
    public Context getApplicationContext() { return this; }
    public int checkSelfPermission(String name) { return permission ? PackageManager.PERMISSION_GRANTED : -1; }
    public PackageManager getPackageManager() { return new PackageManager(); }
    public String getPackageName() { return "dev.nsgo.recording.test"; }
    public ApplicationInfo getApplicationInfo() { return new ApplicationInfo(); }
    public <T> T getSystemService(Class<T> type) { return type.cast(NotificationManager.instance); }
    public ComponentName startForegroundService(Intent intent) {
        if (refuseService) throw new IllegalStateException("foreground refused");
        Service.start(intent); return new ComponentName(this, dev.oneo.nsgo.RecordingService.class);
    }
}
