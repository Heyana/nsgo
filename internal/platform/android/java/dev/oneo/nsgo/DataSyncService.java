package dev.oneo.nsgo;

import android.app.ForegroundServiceStartNotAllowedException;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.content.pm.ServiceInfo;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.os.PowerManager;
import android.util.Log;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import org.json.JSONException;
import org.json.JSONObject;

/**
 * Generic, transport-agnostic Android dataSync foreground service (mobile.StartDataSync),
 * keeping NsgoEngine / libnsgo.so alive across backgrounding and NsgoActivity destruction
 * while Go-owned transfers (downloads, uploads, or sync) run.
 *
 * Only apps whose manifest declares this service use it: nsgo.yaml's android.data_sync: true
 * declares DataSyncService with foregroundServiceType="dataSync" alongside FOREGROUND_SERVICE,
 * FOREGROUND_SERVICE_DATA_SYNC and POST_NOTIFICATIONS. The service returns START_NOT_STICKY so
 * Android never silently restarts it after a process kill, stops cleanly when the Go process
 * ends (ownerEnded), handles ForegroundServiceStartNotAllowedException with a structured
 * "not_allowed" report, and handles API 35+ Service.onTimeout(int, int) by notifying Go so it
 * can flush checkpoints before stopping.
 */
public final class DataSyncService extends Service {
    private static final String TAG = "nsgo";
    private static final String CHANNEL = "nsgo.datasync";
    private static final int NOTIFICATION_ID = 0x6e7364;
    private static final String WAKE_LOCK_PERMISSION = "android.permission.WAKE_LOCK";
    private static final long WAKE_LOCK_TIMEOUT_MS = 6L * 60 * 60 * 1000;
    /** Grace period after API 35+ onTimeout for Go to flush checkpoints and send stop before forcing stopSelf. */
    static long timeoutGraceMs = 3000;

    private static final ExecutorService reports = Executors.newSingleThreadExecutor();
    private static final Handler main = new Handler(Looper.getMainLooper());

    // Guarded by DataSyncService.class.
    private static Session session;
    private static DataSyncService service;

    // Main-thread owned.
    private int lastStart;
    private PowerManager.WakeLock wakeLock;
    private Runnable pendingTimeoutStop;

    /** Reports whether the app's manifest declares DataSyncService. */
    static boolean declared(Context context) {
        try {
            context.getPackageManager().getServiceInfo(new ComponentName(context, DataSyncService.class), 0);
            return true;
        } catch (PackageManager.NameNotFoundException e) {
            return false;
        }
    }

    /** Initial capability state sent to Go when the engine connects. */
    static byte[] state(Context context) {
        return json(new Json().put("type", "state").put("supported", declared(context)));
    }

    /** Keeps owner's engine alive across NsgoActivity.onDestroy while a session is active. */
    static synchronized boolean active(NsgoEngine owner) {
        return owner != null && session != null && session.owner == owner;
    }

    /** Takes an EXT_DATA_SYNC JSON command from Go on NsgoEngine's reader thread. */
    static void message(Context context, NsgoEngine owner, byte[] payload) {
        JSONObject cmd;
        try {
            cmd = new JSONObject(new String(payload, StandardCharsets.UTF_8));
        } catch (JSONException e) {
            Log.w(TAG, "invalid dataSync message from app");
            return;
        }
        switch (cmd.optString("op")) {
            case "start":
                start(context, owner, cmd);
                break;
            case "update":
                update(owner, cmd);
                break;
            case "stop":
                stop(owner, cmd.optLong("id"));
                break;
            default:
        }
    }

    private static void start(Context context, NsgoEngine owner, JSONObject cmd) {
        long id = cmd.optLong("id");
        if (!declared(context)) {
            sendReport(owner, endedJson(id, "unavailable", "the app declares no dataSync service (nsgo.yaml's android.data_sync)"));
            return;
        }
        Session next;
        DataSyncService running = null;
        boolean alreadyRunning = false;
        synchronized (DataSyncService.class) {
            if (session != null && session.owner != owner) {
                sendReport(owner, endedJson(id, "busy", "another engine owns the dataSync service"));
                return;
            }
            next = new Session(owner, id,
                    cmd.optString("title"),
                    cmd.optString("body"),
                    cmd.optLong("current"),
                    cmd.optLong("total"),
                    cmd.optBoolean("indeterminate"),
                    cmd.optBoolean("wakeLock"));
            if (session != null && session.owner == owner && !session.timedOut && service != null && session.started) {
                next.started = true;
                running = service;
                alreadyRunning = true;
            }
            session = next;
        }
        if (alreadyRunning) {
            final DataSyncService svc = running;
            main.post(svc::refresh);
            sendReport(owner, json(new Json().put("type", "started").put("id", id)));
            return;
        }
        try {
            if (context.startForegroundService(new Intent(context, DataSyncService.class)) == null) {
                failStart(next, "unavailable", "the app declares no dataSync service (nsgo.yaml's android.data_sync)");
            }
        } catch (RuntimeException e) {
            failStart(next, startFailureCode(e), detailOf(e));
        }
    }

    private static void update(NsgoEngine owner, JSONObject cmd) {
        long id = cmd.optLong("id");
        DataSyncService svc;
        synchronized (DataSyncService.class) {
            if (session == null || session.owner != owner || session.id != id) return;
            session.title = cmd.optString("title");
            session.body = cmd.optString("body");
            session.current = cmd.optLong("current");
            session.total = cmd.optLong("total");
            session.indeterminate = cmd.optBoolean("indeterminate");
            session.wakeLock = cmd.optBoolean("wakeLock");
            svc = service;
        }
        if (svc != null) main.post(svc::refresh);
    }

    private static void stop(NsgoEngine owner, long id) {
        Session ended;
        DataSyncService svc;
        synchronized (DataSyncService.class) {
            if (session == null || session.owner != owner || (id != 0 && session.id != id)) return;
            ended = session;
            session = null;
            svc = service;
        }
        ended.owner.dataSyncEnded();
        if (svc != null) main.post(svc::idle);
    }

    /** Cleans up when owner's Go process exits unexpectedly or its engine is destroyed. */
    static void ownerEnded(NsgoEngine owner) {
        DataSyncService svc;
        synchronized (DataSyncService.class) {
            if (session == null || session.owner != owner) return;
            session = null;
            svc = service;
        }
        if (svc != null) main.post(svc::idle);
    }

    private static void failStart(Session failed, String code, String detail) {
        boolean cleared;
        DataSyncService svc;
        synchronized (DataSyncService.class) {
            cleared = (session == failed);
            if (cleared) session = null;
            svc = service;
        }
        sendReport(failed.owner, endedJson(failed.id, code, detail));
        if (cleared) {
            failed.owner.dataSyncEnded();
            if (svc != null) main.post(svc::idle);
        }
    }

    private static String startFailureCode(RuntimeException e) {
        if (isStartNotAllowed(e)) return "not_allowed";
        if (e instanceof SecurityException) return "unavailable";
        return "failed";
    }

    private static boolean isStartNotAllowed(RuntimeException e) {
        if (Build.VERSION.SDK_INT >= 31 && e instanceof ForegroundServiceStartNotAllowedException) {
            return true;
        }
        String name = e.getClass().getSimpleName();
        return "ForegroundServiceStartNotAllowedException".equals(name)
                || "BackgroundServiceStartNotAllowedException".equals(name);
    }

    private static String detailOf(Throwable t) {
        String msg = t.getMessage();
        return msg == null ? t.getClass().getSimpleName() : msg;
    }

    @Override
    public void onCreate() {
        super.onCreate();
        synchronized (DataSyncService.class) {
            service = this;
        }
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        cancelPendingTimeoutStop();
        Session current;
        Snapshot snap;
        synchronized (DataSyncService.class) {
            lastStart = startId;
            current = session;
            snap = current == null ? new Snapshot("", "", 0, 0, true, false) : current.snapshot();
        }
        // Always call startForeground once startForegroundService launched us, even if Go
        // already stopped the session in the meantime, to satisfy Android's FGS contract.
        try {
            Notification initial = buildNotification(snap);
            if (Build.VERSION.SDK_INT >= 29) {
                startForeground(NOTIFICATION_ID, initial, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC);
            } else {
                startForeground(NOTIFICATION_ID, initial);
            }
        } catch (RuntimeException e) {
            if (current != null) failStart(current, startFailureCode(e), detailOf(e));
            idle();
            return START_NOT_STICKY;
        }

        boolean reportStarted = false;
        boolean shouldIdle = false;
        boolean changed = false;
        boolean wantWake = false;
        synchronized (DataSyncService.class) {
            if (current != null && session == current && !current.timedOut) {
                current.started = true;
                reportStarted = true;
                Snapshot latest = current.snapshot();
                wantWake = latest.wakeLock;
                changed = !latest.sameNotification(snap);
            } else {
                shouldIdle = (session == null);
            }
        }
        if (shouldIdle) {
            idle();
            return START_NOT_STICKY;
        }
        if (reportStarted) {
            syncWakeLock(wantWake);
            if (changed) refresh();
            sendReport(current.owner, json(new Json().put("type", "started").put("id", current.id)));
        }
        return START_NOT_STICKY;
    }

    /** API 34 single-arg timeout fallback. */
    @Override
    public void onTimeout(int startId) {
        handleTimeout(startId);
    }

    /** API 35+ typed foreground service timeout (e.g. 6-hour dataSync quota). */
    @Override
    public void onTimeout(int startId, int fgsType) {
        handleTimeout(startId);
    }

    private void handleTimeout(int startId) {
        Session timed = null;
        boolean shouldIdle = false;
        synchronized (DataSyncService.class) {
            lastStart = startId;
            if (session == null) {
                shouldIdle = true;
            } else if (!session.timedOut) {
                session.timedOut = true;
                timed = session;
            }
        }
        if (shouldIdle) {
            idle();
            return;
        }
        if (timed == null) return;
        // Notify Go so it can flush .part / checkpoint state and send "stop".
        sendReport(timed.owner, json(new Json().put("type", "timeout").put("id", timed.id)));
        cancelPendingTimeoutStop();
        final Session target = timed;
        pendingTimeoutStop = () -> {
            pendingTimeoutStop = null;
            boolean stillMine;
            synchronized (DataSyncService.class) {
                stillMine = (session == target);
                if (stillMine) session = null;
            }
            if (stillMine) {
                sendReport(target.owner, endedJson(target.id, "timeout", ""));
                target.owner.dataSyncEnded();
            }
            idle();
        };
        main.postDelayed(pendingTimeoutStop, timeoutGraceMs);
    }

    @Override
    public void onDestroy() {
        cancelPendingTimeoutStop();
        releaseWakeLock();
        Session orphaned;
        synchronized (DataSyncService.class) {
            if (service == this) service = null;
            orphaned = session;
            session = null;
        }
        if (orphaned != null) {
            String code = orphaned.timedOut ? "timeout" : "stopped";
            sendReport(orphaned.owner, endedJson(orphaned.id, code, ""));
            orphaned.owner.dataSyncEnded();
        }
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    /** Updates the notification and wake lock from the latest session state on the main thread. */
    private void refresh() {
        Snapshot snap;
        synchronized (DataSyncService.class) {
            if (session == null || service != this) return;
            snap = session.snapshot();
        }
        syncWakeLock(snap.wakeLock);
        try {
            NotificationManager manager = getSystemService(NotificationManager.class);
            if (manager != null) manager.notify(NOTIFICATION_ID, buildNotification(snap));
        } catch (RuntimeException ignored) {
            // The system may have stopped the service concurrently.
        }
    }

    /** Stops the service on the main thread unless another session is active. */
    private void idle() {
        cancelPendingTimeoutStop();
        int start;
        synchronized (DataSyncService.class) {
            if (session != null) return;
            start = lastStart;
        }
        releaseWakeLock();
        stopForeground(STOP_FOREGROUND_REMOVE);
        stopSelf(start);
    }

    private void cancelPendingTimeoutStop() {
        if (pendingTimeoutStop != null) {
            main.removeCallbacks(pendingTimeoutStop);
            pendingTimeoutStop = null;
        }
    }

    private void syncWakeLock(boolean want) {
        if (want) {
            if (wakeLock != null && wakeLock.isHeld()) return;
            if (checkSelfPermission(WAKE_LOCK_PERMISSION) != PackageManager.PERMISSION_GRANTED) {
                Log.w(TAG, "dataSync: WakeLock requested without android.permission.WAKE_LOCK");
                return;
            }
            try {
                PowerManager pm = getSystemService(PowerManager.class);
                if (pm == null) return;
                PowerManager.WakeLock lock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, getPackageName() + ":nsgo-datasync");
                lock.setReferenceCounted(false);
                lock.acquire(WAKE_LOCK_TIMEOUT_MS);
                wakeLock = lock;
            } catch (RuntimeException e) {
                Log.w(TAG, "dataSync: cannot acquire PARTIAL_WAKE_LOCK", e);
            }
        } else {
            releaseWakeLock();
        }
    }

    private void releaseWakeLock() {
        if (wakeLock != null) {
            try {
                if (wakeLock.isHeld()) wakeLock.release();
            } catch (RuntimeException ignored) {
            }
            wakeLock = null;
        }
    }

    private Notification buildNotification(Snapshot snap) {
        NotificationManager manager = getSystemService(NotificationManager.class);
        if (manager != null && manager.getNotificationChannel(CHANNEL) == null) {
            manager.createNotificationChannel(new NotificationChannel(CHANNEL, "Data sync", NotificationManager.IMPORTANCE_LOW));
        }
        int icon = getApplicationInfo().icon;
        if (icon == 0) icon = android.R.drawable.stat_sys_download;
        CharSequence title = snap.title.isEmpty()
                ? getApplicationInfo().loadLabel(getPackageManager())
                : snap.title;
        Notification.Builder b = new Notification.Builder(this, CHANNEL)
                .setSmallIcon(icon)
                .setContentTitle(title)
                .setOnlyAlertOnce(true)
                .setOngoing(true);
        if (!snap.body.isEmpty()) b.setContentText(snap.body);
        if (snap.indeterminate || snap.total < 0) {
            b.setCategory(Notification.CATEGORY_PROGRESS);
            b.setProgress(0, 0, true);
        } else if (snap.total > 0) {
            b.setCategory(Notification.CATEGORY_PROGRESS);
            long clamped = Math.max(0L, Math.min(snap.current, snap.total));
            int progress = (int) (1000.0 * clamped / snap.total);
            b.setProgress(1000, progress, false);
        } else {
            b.setCategory(Notification.CATEGORY_SERVICE);
            b.setProgress(0, 0, false);
        }
        Intent launch = getPackageManager().getLaunchIntentForPackage(getPackageName());
        if (launch != null) {
            launch.addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP);
            b.setContentIntent(PendingIntent.getActivity(this, 0, launch,
                    PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));
        }
        if (Build.VERSION.SDK_INT >= 31) {
            b.setForegroundServiceBehavior(Notification.FOREGROUND_SERVICE_IMMEDIATE);
        }
        return b.build();
    }

    private static void sendReport(NsgoEngine owner, byte[] data) {
        reports.execute(() -> owner.write(Protocol.DATA_SYNC_REPORT, data));
    }

    private static byte[] endedJson(long id, String code, String detail) {
        return json(new Json().put("type", "ended").put("id", id).put("error", code).put("detail", detail));
    }

    private static final class Session {
        final NsgoEngine owner;
        final long id;
        String title, body;
        long current, total;
        boolean indeterminate;
        boolean wakeLock;
        boolean started;
        boolean timedOut;

        Session(NsgoEngine owner, long id, String title, String body, long current, long total,
                boolean indeterminate, boolean wakeLock) {
            this.owner = owner;
            this.id = id;
            this.title = title == null ? "" : title;
            this.body = body == null ? "" : body;
            this.current = current;
            this.total = total;
            this.indeterminate = indeterminate;
            this.wakeLock = wakeLock;
        }

        Snapshot snapshot() {
            return new Snapshot(title, body, current, total, indeterminate, wakeLock);
        }
    }

    private static final class Snapshot {
        final String title, body;
        final long current, total;
        final boolean indeterminate, wakeLock;

        Snapshot(String title, String body, long current, long total, boolean indeterminate, boolean wakeLock) {
            this.title = title;
            this.body = body;
            this.current = current;
            this.total = total;
            this.indeterminate = indeterminate;
            this.wakeLock = wakeLock;
        }

        boolean sameNotification(Snapshot other) {
            return title.equals(other.title) && body.equals(other.body)
                    && current == other.current && total == other.total
                    && indeterminate == other.indeterminate;
        }
    }

    private static final class Json {
        final JSONObject o = new JSONObject();

        Json put(String key, Object value) {
            try {
                o.put(key, value);
            } catch (JSONException e) {
                throw new IllegalStateException(e);
            }
            return this;
        }
    }

    private static byte[] json(Json j) {
        return j.o.toString().getBytes(StandardCharsets.UTF_8);
    }
}
