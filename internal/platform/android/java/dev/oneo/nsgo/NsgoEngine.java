package dev.oneo.nsgo;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.net.LocalSocket;
import android.util.Log;

import java.io.BufferedInputStream;
import java.io.BufferedReader;
import java.io.BufferedOutputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.util.Arrays;
import java.util.TimeZone;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.locks.ReentrantReadWriteLock;

/**
 * Owns one NSGo process and its pipe. A host may keep this object in an Application or foreground service, so that the app runs on while the NsgoView showing it is detached from its Activity.
 *
 * The engine answers text requests for each surface. Until it can resync a tree in another view, it keeps its first NsgoView, detached or not, to receive view messages and host overlays. The process starts once. A host makes a new engine after the process ends or is destroyed.
 */
public final class NsgoEngine {
    private static final String TAG = "nsgo";
    /** Running engines by lifetime token, so embedded apps never receive one another's actions. */
    private static final ConcurrentHashMap<String, NsgoEngine> liveActions = new ConcurrentHashMap<>();
    private final String liveActionToken = UUID.randomUUID().toString();

    /** Receives view messages from the Go process on the engine reader thread. */
    interface Listener {
        void onMessage(int kind, byte[] payload);

        /** The process ended, by itself or through destroy; called once, after the last message. */
        void onProcessEnded(IOException error);

        void onLog(String line);

        /** A recording's sender has sent its reports. */
        default void onRecordingEnded() {}

        /** A dataSync foreground service session ended. */
        default void onDataSyncEnded() {}
    }

    private final Context application;
    final TextCache text;
    private Process process;
    private LocalSocket socket;
    /** The development socket's input; a started process's comes from the process. */
    private InputStream input;
    private DataOutputStream out;
    private volatile boolean bulkReady;
    private final Object bulkTransfer = new Object();
    private final ReentrantReadWriteLock bulkPriority = new ReentrantReadWriteLock(true);
    private Listener listener;
    private boolean readerStarted;
    private boolean closed;
    /** Tells the process of the device's time zone changes while it runs. */
    private BroadcastReceiver zones;
    /** The app's data directory, which its process was started with. */
    private File data;
    /** Identifies this test even if its TestMain never starts the Go host. */
    private String testRun;

    /** Creates an engine without retaining an Activity or another UI context. */
    public NsgoEngine(Context context) {
        if (context == null) throw new NullPointerException("context");
        application = context.getApplicationContext();
        text = new TextCache(application.getResources().getDisplayMetrics().density);
    }

    /**
     * Starts the process; an engine starts one only. NsgoView supplies the immutable launch config, to which this adds the device's time zone as TZ, as Go reads it from nowhere else; the zone follows in TIME_ZONE, then again whenever it changes.
     */
    synchronized void start(ProcessBuilder builder) throws IOException {
        if (!canStart()) throw new IllegalStateException(closed ? "engine is destroyed" : "engine has started its process");
        builder.environment().put("TZ", TimeZone.getDefault().getID());
        testRun = builder.environment().get("NSGO_TEST_RUN");
        process = builder.start();
        out = new DataOutputStream(new BufferedOutputStream(process.getOutputStream()));
        String dir = builder.environment().get("NSGO_DATA_DIR");
        data = dir == null ? application.getFilesDir() : new File(dir);
        watchTimeZone();
        liveActions.put(liveActionToken, this);
    }

    /** Starts serving the host process over its adb-forwarded socket. */
    synchronized void start(LocalSocket peer) throws IOException {
        if (!canStart()) throw new IllegalStateException("engine already started");
        socket = peer;
        input = socket.getInputStream();
        out = new DataOutputStream(new BufferedOutputStream(socket.getOutputStream()));
        data = application.getFilesDir();
        watchTimeZone();
    }

    /**
     * Sends the zone now and as ACTION_TIMEZONE_CHANGED reports it, until the process ends: a zone Go cannot load from the system's tzdata still gets its current offset, and a change between the launch and the receiver's registration is not lost.
     */
    private void watchTimeZone() {
        zones = new BroadcastReceiver() {
            @Override
            public void onReceive(Context context, Intent intent) {
                // The intent's ID is the new zone; the process's default may not have followed it yet.
                String id = intent.getStringExtra(Intent.EXTRA_TIMEZONE);
                timeZone(id != null ? TimeZone.getTimeZone(id) : TimeZone.getDefault());
            }
        };
        // A system broadcast, which the receiver hears without being exported.
        application.registerReceiver(zones, new IntentFilter(Intent.ACTION_TIMEZONE_CHANGED));
        timeZone(TimeZone.getDefault());
    }

    private void timeZone(TimeZone zone) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            Protocol.writeString(d, zone.getID());
            d.writeInt(zone.getOffset(System.currentTimeMillis()) / 1000);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        write(Protocol.TIME_ZONE, bytes.toByteArray());
    }

    /** Stops telling the process of the zone's changes, as it ends; it is safe to call twice. */
    private synchronized void unwatchTimeZone() {
        if (zones == null) return;
        application.unregisterReceiver(zones);
        zones = null;
    }

    /** Where the app's recordings go: under its data directory, the host's storage root. */
    synchronized File recordings() {
        return data == null ? null : new File(data, "recordings");
    }

    /** Tells the view to check whether the ended recording was its last keeper. */
    void recordingEnded() {
        Listener current;
        synchronized (this) {
            current = listener;
        }
        if (current != null) current.onRecordingEnded();
    }

    /** Tells the view to check whether an ended dataSync session was its last keeper. */
    void dataSyncEnded() {
        Listener current;
        synchronized (this) {
            current = listener;
        }
        if (current != null) current.onDataSyncEnded();
    }

    /** Says whether the engine has neither started a process nor been destroyed. */
    synchronized boolean canStart() {
        return !closed && process == null && socket == null;
    }

    /**
     * Binds the view the engine serves and starts the pipe readers once the process runs. The view stays bound until the process ends because the runtime cannot resync a tree in a different view.
     */
    synchronized void attach(Listener next) {
        if (closed) throw new IllegalStateException("engine is destroyed");
        if (listener != null && listener != next) {
            throw new IllegalStateException("engine serves another NsgoView: until the surface protocol lands, an engine serves the one view that first attached to it");
        }
        listener = next;
        if (readerStarted || (process == null && socket == null)) return;
        readerStarted = true;
        Process running = process;
        InputStream source = running != null ? running.getInputStream() : input;
        Thread reader = new Thread(() -> read(source), "nsgo-reader");
        reader.setDaemon(true);
        reader.start();
        if (running != null) {
            Thread logger = new Thread(() -> log(running, next), "nsgo-log");
            logger.setDaemon(true);
            logger.start();
        }
    }

    /** Says whether the process is running and the engine has not been closed. */
    public synchronized boolean isAlive() {
        return !closed && (process != null ? process.isAlive() : socket != null && out != null);
    }

    /** Writes a message, letting text replies pass between bulk reply fragments. */
    void write(int kind, byte[] payload) {
        if (kind == Protocol.TEXT_REPLY) {
            bulkPriority.writeLock().lock();
            try {
                writeFrame(kind, payload, 0, payload.length, false);
            } finally {
                bulkPriority.writeLock().unlock();
            }
        } else if (bulkReady && payload.length > Protocol.BULK_CHUNK &&
                (kind == Protocol.FOLDER_REPLY || kind == Protocol.FETCHED)) {
            synchronized (bulkTransfer) {
                for (int offset = 0; offset < payload.length; ) {
                    int length = Math.min(Protocol.BULK_CHUNK, payload.length - offset);
                    bulkPriority.readLock().lock();
                    try {
                        if (!writeFrame(kind, payload, offset, length, true)) return;
                    } finally {
                        bulkPriority.readLock().unlock();
                    }
                    offset += length;
                }
            }
        } else {
            writeFrame(kind, payload, 0, payload.length, false);
        }
    }

    private synchronized boolean writeFrame(int kind, byte[] payload, int offset, int length, boolean fragment) {
        if (out == null || closed) return false;
        try {
            out.writeByte(fragment ? Protocol.BULK_REPLY : kind);
            out.writeInt(length + (fragment ? Protocol.BULK_HEADER : 0));
            if (fragment) {
                out.writeByte(kind);
                out.writeInt(payload.length);
                out.writeInt(offset);
            }
            out.write(payload, offset, length);
            out.flush();
            return true;
        } catch (IOException e) {
            Log.w(TAG, "app process unreachable", e);
            return false;
        }
    }

    /** Delivers a Live Update action without manufacturing an Activity. */
    static boolean deliverLiveAction(String token, byte[] press) {
        NsgoEngine engine = token == null ? null : liveActions.get(token);
        return press != null && engine != null && engine.writeLiveAction(press);
    }

    /** Identifies this engine's notification actions for this process lifetime. */
    String liveActionToken() { return liveActionToken; }

    private synchronized boolean writeLiveAction(byte[] press) {
        if (closed || process == null || !process.isAlive() || out == null) return false;
        write(Protocol.LIVE_REPORT, press);
        return true;
    }

    /**
     * Stops the process and releases the pipe; it is safe to call twice. The bound view hears of it through onProcessEnded once the reader has passed it the last message, or at once if no reader runs.
     */
    public void destroy() {
        Listener idle;
        // Its recording's file is finished, for the next process to hear of.
        RecordingService.ownerEnded(this);
        BackgroundDownloadService.ownerEnded(this);
        DataSyncService.ownerEnded(this);
        synchronized (this) {
            if (closed) return;
            closed = true;
            liveActions.remove(liveActionToken, this);
            if (socket != null) AndroidHostDev.closeSocket(socket);
            text.close();
            if (out != null) {
                try {
                    out.close();
                } catch (IOException ignored) {
                    // The process is being destroyed.
                }
                out = null;
            }
            if (process != null) process.destroy();
            unwatchTimeZone();
            // Without a reader, nothing else tells the view.
            idle = readerStarted ? null : listener;
            if (!readerStarted) listener = null;
        }
        if (idle != null) idle.onProcessEnded(null);
    }

    private void read(InputStream source) {
        IOException failure = null;
        try (DataInputStream in = new DataInputStream(new BufferedInputStream(source))) {
            Protocol.BulkReader bulk = new Protocol.BulkReader();
            for (Protocol.Message message; (message = Protocol.readMessage(in, bulk)) != null; ) {
                // An empty EXT_BULK is Go's hello: the acknowledgement tells it this runtime reads bulk fragments.
                if (message.kind == Protocol.EXTENDED && message.payload.length == 1 && (message.payload[0] & 0xff) == Protocol.EXT_BULK) {
                    write(Protocol.BULK_REPLY, new byte[0]);
                    bulkReady = true;
                    continue;
                }
                if (message.kind == Protocol.EXTENDED && (message.payload[0] & 0xff) == Protocol.EXT_DOWNLOAD) {
                    BackgroundDownloadService.message(application, this, Arrays.copyOfRange(message.payload, 1, message.payload.length));
                    continue;
                }
                if (message.kind == Protocol.EXTENDED && (message.payload[0] & 0xff) == Protocol.EXT_DATA_SYNC) {
                    DataSyncService.message(application, this, Arrays.copyOfRange(message.payload, 1, message.payload.length));
                    continue;
                }
                Listener current;
                synchronized (this) {
                    current = listener;
                }
                if (message.kind == Protocol.TEXT_REQUEST) {
                    write(Protocol.TEXT_REPLY, text.request(message.payload));
                } else if (message.kind == Protocol.RELEASE) {
                    try {
                        text.release(message.payload);
                    } catch (IOException e) {
                        Log.w(TAG, "invalid text release from app", e);
                    }
                } else if (current != null) {
                    current.onMessage(message.kind, message.payload);
                }
            }
            bulk.end();
        } catch (IOException e) {
            failure = e;
        }
        // A corrupt pipe cannot be resynchronized; do not leave its writer running.
        if (failure != null) destroy();
        text.close();
        RecordingService.ownerEnded(this);
        BackgroundDownloadService.ownerEnded(this);
        DataSyncService.ownerEnded(this);
        Listener current;
        synchronized (this) {
            if (socket != null) AndroidHostDev.closeSocket(socket);
            current = listener;
            listener = null;
            out = null;
            liveActions.remove(liveActionToken, this);
            unwatchTimeZone();
        }
        if (current != null) {
            if (failure != null && testRun != null && !testRun.isEmpty()) {
                current.onLog("nsgo-test-error " + testRun + " " + failure.getMessage());
            }
            current.onProcessEnded(failure);
        }
    }

    private void log(Process running, Listener recipient) {
        boolean marked = false;
        try (BufferedReader err = new BufferedReader(new InputStreamReader(running.getErrorStream()))) {
            for (String line; (line = err.readLine()) != null; ) {
                if (testRun != null && (line.startsWith("nsgo-test-exit " + testRun + " ") || line.startsWith("nsgo-test-error " + testRun + " "))) {
                    marked = true;
                }
                // stdout may reach EOF before stderr's buffered tail, so this process's recipient keeps hearing its log until it drains, the test host's final result and exit marker included.
                recipient.onLog(line);
            }
        } catch (IOException ignored) {
            // The process ended.
        }
        // Logcat stays open after the child exits. Report an unmarked exit only after draining stderr, so its panic and stack precede it.
        String reason;
        try {
            reason = "the test process ended without its exit code (status " + running.waitFor() + ")";
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            reason = "interrupted while waiting for the test process's exit code";
        }
        synchronized (this) {
            if (closed || marked || testRun == null || testRun.isEmpty()) return;
        }
        recipient.onLog("nsgo-test-error " + testRun + " " + reason);
    }
}
