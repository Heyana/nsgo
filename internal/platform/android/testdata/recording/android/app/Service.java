package android.app;
import android.content.*;
import android.os.IBinder;
public class Service extends Context {
    public static final int START_NOT_STICKY = 2, STOP_FOREGROUND_REMOVE = 1;
    public static Service current;
    public static boolean foreground, refuseForeground;
    public static int type;
    public static void start(Intent intent) {
        if (current == null) { current = new dev.oneo.nsgo.RecordingService(); current.onCreate(); }
        current.onStartCommand(intent, 0, 1);
    }
    public void onCreate() {}
    public int onStartCommand(Intent i, int flags, int id) { return START_NOT_STICKY; }
    public void onDestroy() {}
    public IBinder onBind(Intent intent) { return null; }
    public void startForeground(int id, Notification n) { startForeground(id, n, 0); }
    public void startForeground(int id, Notification n, int t) {
        if (refuseForeground) throw new IllegalStateException("type refused"); foreground = true; type = t;
    }
    public void stopForeground(int flags) { foreground = false; }
    public void stopSelf(int id) { if (current == this) current = null; onDestroy(); }
}
