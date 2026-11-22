package dev.oneo.nsgo;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.os.Build;
import android.os.IBinder;
import android.system.ErrnoException;
import android.system.Os;
import android.system.StructStat;
import android.util.AtomicFile;
import android.util.Log;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.StandardCopyOption;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import org.json.JSONException;
import org.json.JSONObject;

/** Owns background file transfers independently of the Activity and the Go engine. */
public final class BackgroundDownloadService extends Service {
    private static final String TAG = "NsgoBackgroundDownload";
    private static final String CHANNEL = "nsgo.downloads";
    private static final int NOTIFICATION_ID = 0x4e534744;
    private static final int PARALLEL = 2;

    // One thread owns commands, transitions and journal writes. Workers only
    // stream data. A stalled Go pipe can hold neither of them.
    private static final ExecutorService commands = Executors.newSingleThreadExecutor();
    private static final ExecutorService workers = Executors.newFixedThreadPool(PARALLEL);
    private static final Reports reports = new Reports();
    private static final Map<String, Task> tasks = new LinkedHashMap<>();
    private static final Set<NsgoEngine> listeners = new LinkedHashSet<>();
    private static File journal;
    private static BackgroundDownloadService service;
    private static volatile boolean used;
    private static boolean starting;
    private static int running;

    static void message(Context context, NsgoEngine owner, byte[] bytes) {
        used = true;
        commands.execute(() -> {
            try {
                JSONObject request = new JSONObject(new String(bytes, StandardCharsets.UTF_8));
                handle(context, owner, request);
            } catch (JSONException e) {
                Log.w(TAG, "invalid download command");
            }
        });
    }

    static void ownerEnded(NsgoEngine owner) {
        if (!used) return;
        commands.execute(() -> {
            listeners.remove(owner);
            reports.remove(owner);
        });
    }

    private static void handle(Context context, NsgoEngine owner, JSONObject request) {
        listeners.add(owner);
        if (journal == null) restore(new File(context.getFilesDir(), "nsgo-downloads"));
        String op = request.optString("Op");
        if (op.equals("list")) {
            for (Task task : tasks.values()) reports.send(owner, task);
            return;
        }
        String id = request.optString("ID");
        if (!id.matches("[0-9A-Za-z]{1,64}")) return;
        if (op.equals("start")) {
            start(context, owner, id, request);
            return;
        }
        Task task = tasks.get(id);
        if (task == null) return;
        switch (op) {
            case "pause":
                if ("queued".equals(task.state)) {
                    update(task, "paused", "");
                    pump();
                } else if ("running".equals(task.state) && task.stop == null) {
                    stop(task, "paused", "");
                }
                break;
            case "resume":
                if ("paused".equals(task.state)) {
                    update(task, "queued", "");
                    wake(context);
                }
                break;
            case "cancel":
                if ("running".equals(task.state)) {
                    stop(task, "canceled", "");
                } else if (!task.terminal()) {
                    end(task, "canceled", "");
                    pump();
                }
                break;
            case "forget":
                if (task.terminal()) {
                    record(task).delete();
                    tasks.remove(id);
                }
                break;
        }
    }

    private static void start(Context context, NsgoEngine owner, String id, JSONObject request) {
        if (tasks.containsKey(id)) return;
        String path = request.optString("Path");
        Task task = null;
        try {
            File destination = new File(path).getCanonicalFile();
            if (!new File(path).isAbsolute() || destination.getParentFile() == null ||
                    !destination.getParentFile().isDirectory() || destination.isDirectory()) {
                throw new IOException("the destination needs an existing parent directory");
            }
            task = new Task(id, destination);
            if (task.partial.exists()) throw new IOException("the partial path is already in use");
            for (Task other : tasks.values()) {
                if (!other.terminal() && destination.equals(other.destination)) {
                    throw new IOException("a download already uses that destination");
                }
            }
            Map<String, String> headers = new LinkedHashMap<>();
            JSONObject options = request.optJSONObject("Headers");
            if (options != null) options.keys().forEachRemaining(key -> headers.put(key, options.optString(key)));
            task.transfer = new BackgroundTransfer(task.partial, request.optString("URL"), headers,
                    request.optString("CertificatePEM"));
            save(task);
            tasks.put(id, task);
            publish(task);
            wake(context);
        } catch (IOException | RuntimeException e) {
            if (task == null) task = new Task(id, new File(path));
            task.state = "failed";
            task.error = "cannot prepare the download destination or journal";
            reports.send(owner, task);
        }
    }

    private static void wake(Context context) {
        if (service != null) {
            pump();
        } else if (!starting) {
            starting = true;
            try {
                if (context.startForegroundService(new Intent(context, BackgroundDownloadService.class)) == null) {
                    starting = false;
                    interrupt("the app declares no background download service");
                }
            } catch (RuntimeException e) {
                starting = false;
                // Starting a foreground service requires a visible app on current Android versions.
                interrupt("cannot start the download service while the app is in the background or the service is undeclared");
            }
        }
    }

    @Override public IBinder onBind(Intent intent) { return null; }

    @Override public int onStartCommand(Intent intent, int flags, int startId) {
        try {
            Notification initial = notification(null, 0);
            if (Build.VERSION.SDK_INT >= 29) {
                startForeground(NOTIFICATION_ID, initial, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC);
            } else {
                startForeground(NOTIFICATION_ID, initial);
            }
        } catch (RuntimeException e) {
            commands.execute(() -> {
                starting = false;
                interrupt("Android refused to start the download foreground service");
            });
            stopSelf(startId);
            return START_NOT_STICKY;
        }
        commands.execute(() -> {
            starting = false;
            service = this;
            pump();
        });
        return START_NOT_STICKY;
    }

    @Override public void onTimeout(int startId, int type) {
        commands.execute(() -> serviceEnded(this, "Android's foreground data transfer time limit expired"));
        stopSelf(startId);
    }

    @Override public void onDestroy() {
        commands.execute(() -> serviceEnded(this, "Android stopped the download service"));
        super.onDestroy();
    }

    private static void serviceEnded(BackgroundDownloadService ended, String reason) {
        if (service == ended) {
            service = null;
            interrupt(reason);
        }
    }

    private static void interrupt(String reason) {
        for (Task task : tasks.values()) {
            if ("running".equals(task.state) && task.stop == null) {
                stop(task, "interrupted", reason);
            } else if ("queued".equals(task.state)) {
                end(task, "interrupted", reason);
            }
        }
    }

    private static void stop(Task task, String state, String error) {
        task.stop = state;
        task.stopError = error;
        task.transfer.stopped = true;
    }

    private static void pump() {
        if (service == null) return;
        for (Task task : tasks.values()) {
            if (running == PARALLEL) break;
            if (!"queued".equals(task.state)) continue;
            running++;
            task.transfer.stopped = false;
            update(task, "running", "");
            workers.execute(() -> {
                String error = task.transfer.run((loaded, total) -> commands.execute(() -> {
                    task.loaded = loaded;
                    task.total = total;
                    publish(task);
                    refreshNotification();
                }));
                commands.execute(() -> finish(task, error));
            });
        }
        if (running == 0 && !starting) {
            BackgroundDownloadService idle = service;
            service = null;
            idle.stopForeground(STOP_FOREGROUND_REMOVE);
            idle.stopSelf();
        } else {
            refreshNotification();
        }
    }

    private static void finish(Task task, String error) {
        running--;
        String stop = task.stop;
        task.stop = null;
        if ("paused".equals(stop)) {
            task.loaded = task.partial.length();
            update(task, "paused", "");
        } else if (stop != null) {
            end(task, stop, task.stopError);
        } else if (error != null) {
            end(task, "failed", error);
        } else {
            try {
                // If Android kills the process after the rename, restore can
                // recognize the published file from this journal entry.
                task.identity = identity(task.partial);
                if (task.identity == null) throw new IOException("the partial download is missing");
                task.state = "publishing";
                save(task);
                Files.move(task.partial.toPath(), task.destination.toPath(),
                        StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING);
                end(task, "completed", "");
            } catch (IOException | RuntimeException e) {
                end(task, "failed", "cannot publish the downloaded file");
            }
        }
        pump();
    }

    private static void end(Task task, String state, String error) {
        task.identity = null;
        try {
            Files.deleteIfExists(task.partial.toPath());
        } catch (IOException | RuntimeException e) {
            Log.w(TAG, "cannot remove a partial download");
        }
        update(task, state, error);
        task.transfer = null;
    }

    private static void update(Task task, String state, String error) {
        task.state = state;
        task.error = error;
        try {
            save(task);
        } catch (IOException e) {
            Log.w(TAG, "cannot save a download report");
        }
        publish(task);
    }

    private static void publish(Task task) {
        for (NsgoEngine owner : listeners) reports.send(owner, task);
    }

    private static void refreshNotification() {
        if (service == null) return;
        Task first = null;
        int active = 0;
        for (Task task : tasks.values()) {
            if ("queued".equals(task.state) || "running".equals(task.state)) {
                if (first == null) first = task;
                active++;
            }
        }
        if (first != null) {
            try {
                service.getSystemService(NotificationManager.class)
                        .notify(NOTIFICATION_ID, service.notification(first, active));
            } catch (RuntimeException ignored) {
                // The system may have stopped the service between reports.
            }
        }
    }

    private Notification notification(Task first, int active) {
        NotificationManager manager = getSystemService(NotificationManager.class);
        manager.createNotificationChannel(new NotificationChannel(CHANNEL, "Downloads", NotificationManager.IMPORTANCE_LOW));
        Notification.Builder builder = new Notification.Builder(this, CHANNEL)
                .setSmallIcon(android.R.drawable.stat_sys_download)
                .setContentTitle("Downloading files")
                .setOnlyAlertOnce(true)
                .setOngoing(true);
        if (first != null) {
            builder.setContentText(first.destination.getName());
            if (active > 1) builder.setNumber(active);
        }
        if (active == 1 && first.total > 0) {
            int progress = (int) (1000.0 * Math.min(first.loaded, first.total) / first.total);
            builder.setProgress(1000, progress, false);
        } else {
            builder.setProgress(0, 0, true);
        }
        Intent launch = getPackageManager().getLaunchIntentForPackage(getPackageName());
        if (launch != null) {
            builder.setContentIntent(PendingIntent.getActivity(this, 0, launch,
                    PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE));
        }
        return builder.build();
    }

    private static AtomicFile record(Task task) {
        return new AtomicFile(new File(journal, task.id + ".json"));
    }

    private static void save(Task task) throws IOException {
        AtomicFile file = record(task);
        FileOutputStream stream = file.startWrite();
        try {
            JSONObject saved = task.snapshot();
            if (task.identity != null) saved.put("Identity", task.identity);
            stream.write(saved.toString().getBytes(StandardCharsets.UTF_8));
            file.finishWrite(stream);
        } catch (IOException | JSONException e) {
            file.failWrite(stream);
            throw new IOException(e);
        }
    }

    private static String identity(File file) {
        try {
            StructStat stat = Os.stat(file.getPath());
            return stat.st_dev + ":" + stat.st_ino;
        } catch (ErrnoException e) {
            return null;
        }
    }

    private static void restore(File directory) {
        journal = directory;
        File[] entries = journal.listFiles((dir, name) -> name.matches("[0-9A-Za-z]{1,64}\\.json"));
        if (entries == null) return;
        for (File path : entries) {
            try {
                byte[] bytes = new AtomicFile(path).readFully();
                JSONObject saved = new JSONObject(new String(bytes, StandardCharsets.UTF_8));
                Task task = new Task(saved.getString("ID"), new File(saved.getString("Path")));
                if (!path.getName().equals(task.id + ".json")) continue;
                task.state = saved.getString("State");
                task.error = saved.optString("Error");
                task.loaded = saved.optLong("Loaded");
                task.total = saved.optLong("Total", -1);
                tasks.put(task.id, task);
                String identity = saved.optString("Identity");
                if ("publishing".equals(task.state) && !task.partial.exists() &&
                        identity.equals(identity(task.destination)) && task.destination.length() == task.loaded) {
                    end(task, "completed", "");
                } else if (!task.terminal()) {
                    task.loaded = task.partial.length();
                    end(task, "interrupted", "Android ended the previous download process");
                }
            } catch (IOException | JSONException | RuntimeException e) {
                Log.w(TAG, "cannot read a download journal entry");
            }
        }
    }

    private static final class Task {
        final String id;
        final File destination, partial;
        String state = "queued", error = "", stop, stopError, identity;
        long loaded, total = -1;
        BackgroundTransfer transfer;

        Task(String id, File destination) {
            this.id = id;
            this.destination = destination;
            partial = new File(destination.getParentFile(), ".nsgo-download-" + id + ".part");
        }

        boolean terminal() {
            return "completed".equals(state) || "failed".equals(state) || "canceled".equals(state) ||
                    "interrupted".equals(state);
        }

        JSONObject snapshot() {
            try {
                return new JSONObject().put("ID", id).put("Path", destination.getPath())
                        .put("State", state).put("Error", error).put("Loaded", loaded).put("Total", total);
            } catch (JSONException e) {
                throw new AssertionError(e);
            }
        }
    }

    /** Keeps only the last snapshot for each task waiting for a process's pipe. */
    private static final class Reports {
        private final ExecutorService writer = Executors.newSingleThreadExecutor();
        private final Map<NsgoEngine, Map<String, byte[]>> pending = new LinkedHashMap<>();
        private boolean writing;

        synchronized void send(NsgoEngine engine, Task task) {
            pending.computeIfAbsent(engine, ignored -> new LinkedHashMap<>())
                    .put(task.id, task.snapshot().toString().getBytes(StandardCharsets.UTF_8));
            if (!writing) {
                writing = true;
                writer.execute(this::write);
            }
        }

        synchronized void remove(NsgoEngine engine) {
            pending.remove(engine);
        }

        private void write() {
            for (;;) {
                NsgoEngine owner;
                ArrayList<byte[]> batch;
                synchronized (this) {
                    if (pending.isEmpty()) {
                        writing = false;
                        return;
                    }
                    owner = pending.keySet().iterator().next();
                    batch = new ArrayList<>(pending.remove(owner).values());
                }
                for (byte[] data : batch) owner.write(Protocol.DOWNLOAD_REPORT, data);
            }
        }
    }
}
