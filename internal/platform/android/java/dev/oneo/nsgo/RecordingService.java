package dev.oneo.nsgo;

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
import android.media.AudioFormat;
import android.media.AudioManager;
import android.media.AudioRecord;
import android.media.AudioRecordingConfiguration;
import android.media.MediaRecorder;
import android.os.Build;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.util.Log;
import java.io.File;
import java.io.IOException;
import java.io.RandomAccessFile;
import java.nio.charset.StandardCharsets;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.IdentityHashMap;
import java.util.List;
import org.json.JSONException;
import org.json.JSONObject;

/**
 * Records package microphone's WAV files (microphone/recorder.go) in a foreground service of the microphone type, so a recording goes on while the app's activity is hidden or the screen is off. Only an app whose manifest declares the service records: nsgo.yaml's android.recording adds it with RECORD_AUDIO, FOREGROUND_SERVICE and FOREGROUND_SERVICE_MICROPHONE, and an -embed host declares them itself.
 *
 * An app records one file at a time, which belongs to the engine whose Go process started it: an NsgoView detaching leaves it running, the process's end finishes its file, which the next process to connect hears of, and another start, from any engine, is refused as busy. The service starts only while an activity of the app is visible and RECORD_AUDIO is granted, as Android lets only a foreground app start using the microphone, and never restarts by itself: a killed process's recording is over, and its file plays up to its last header.
 *
 * The reader writes each AudioRecord read to the file itself, holding nothing past one read, and rewrites the header's sizes about once a second, so what it wrote before a kill is in the system's page cache rather than the process's. It never waits for Go: a sender thread writes its reports to the engine's pipe, and a level Go has not taken yet is replaced by the next. It asks for no audio focus, so the app's playback and others' go on.
 */
public final class RecordingService extends Service {
    private static final String TAG = "nsgo";
    private static final String CHANNEL = "nsgo.recording";
    private static final int NOTIFICATION = 0x6e7372;
    static final String PERMISSION = "android.permission.RECORD_AUDIO";
    /** The request code of RECORD_AUDIO's prompt, which asks for POST_NOTIFICATIONS too where the service's notification needs it. */
    static final int PERMISSION_REQUEST = 0x6e7b;
    /** The most PCM a WAV holds: RIFF's sizes are 32 bits and count 36 bytes of header too. */
    private static final long MAX_DATA = 0xffffffffL - 36;
    /** The free space a recording leaves its storage, ending with "space" before it is gone. */
    private static final long RESERVE = 8 << 20;
    private static final String PART = ".part";

    // The app's one recording, from its start to the end of its file, the service running it, and the engine whose process connected last, which hears the terminal record of a recording whose process ended at once when its file finishes after that process connected; guarded by RecordingService.class, as is the journal (RecordingJournal) that keeps such records, and those of repaired files, until the app takes them.
    private static Recorder current;
    private static RecordingService service;
    private static NsgoEngine heir;
    /** Keep owners until their senders have delivered every terminal report. */
    private static final IdentityHashMap<NsgoEngine, Integer> reporting = new IdentityHashMap<>();
    // The service's last start, which it stops with so that a start made since keeps it.
    private int lastStart;

    /** Says the app's manifest declares the service, which recording needs. */
    static boolean declared(Context context) {
        try {
            context.getPackageManager().getServiceInfo(new ComponentName(context, RecordingService.class), 0);
            return true;
        } catch (PackageManager.NameNotFoundException e) {
            return false;
        }
    }

    static boolean granted(Context context) {
        return context.checkSelfPermission(PERMISSION) == PackageManager.PERMISSION_GRANTED;
    }

    /** What the app's Go process hears as it connects and whenever RECORD_AUDIO may have changed: whether the app records and may. */
    static byte[] state(Context context) {
        return json(new Json().put("type", "state").put("supported", declared(context)).put("allowed", granted(context)));
    }

    /** Hands engine, whose Go process just started, the terminal records its recordings directory's journal keeps: those of recordings whose Go process ended and of files a start repaired, which earlier processes heard but did not take. Those of recordings still finishing their file go to it as they finish. */
    static void connected(NsgoEngine engine) {
        File dir = engine.recordings();
        List<byte[]> records = new ArrayList<>();
        synchronized (RecordingService.class) {
            if (dir != null) {
                try {
                    records = RecordingJournal.pending(dir);
                } catch (IOException e) {
                    Log.w(TAG, "recording: the earlier recordings cannot be read");
                }
            }
            heir = engine;
        }
        for (byte[] record : records) engine.write(Protocol.RECORDING_REPORT, record);
    }

    /** Keeps an ended process's terminal record, of the file at path, until the app takes it, and hands it to the engine that connected since; the next one to connect hears it otherwise. */
    private static void orphan(Recorder r, String path, byte[] record) {
        NsgoEngine to = null;
        synchronized (RecordingService.class) {
            keep(path, record);
            // Decided with connected's reading of the journal, so that no record waits for a process that connected already. The engine's lock never takes this one.
            if (heir != null && heir != r.owner && heir.isAlive()) to = heir;
        }
        // Written outside the lock, as a pipe Go does not read may block.
        if (to != null) to.write(Protocol.RECORDING_REPORT, record);
    }

    /** Keeps the terminal record of the file at path in its directory's journal; the caller holds RecordingService.class. */
    private static void keep(String path, byte[] record) {
        try {
            RecordingJournal.put(new File(path).getParentFile(), path, record);
        } catch (IOException e) {
            Log.w(TAG, "recording: an earlier recording cannot be kept for the next process");
        }
    }

    /** Lets go of the terminal record of the file at path, which the app's Earlier took. */
    static void taken(String path) {
        if (path.isEmpty()) return;
        synchronized (RecordingService.class) {
            try {
                RecordingJournal.remove(new File(path).getParentFile(), path);
            } catch (IOException e) {
                Log.w(TAG, "recording: a taken earlier recording cannot be let go of");
            }
        }
    }

    /** A report ending recording id before it started, or the answer to an authorize. */
    static byte[] refusal(String type, long id, String code, String detail) {
        return json(new Json().put("type", type).put("id", id).put("error", code).put("detail", detail));
    }

    /**
     * Starts owner's recording, which command describes, on the UI thread while an activity of the app is visible and RECORD_AUDIO is granted: it repairs the files of recordings a killed process left, reporting each, then starts the service, which opens the microphone. Each refusal ends the recording.
     */
    static void start(Context context, NsgoEngine owner, JSONObject command) {
        long id = command.optLong("id");
        File dir = owner.recordings();
        List<byte[]> reports = new ArrayList<>();
        Recorder r = null;
        synchronized (RecordingService.class) {
            if (current != null) {
                reports.add(refusal("ended", id, "busy", ""));
            } else if (dir == null || !(dir.isDirectory() || dir.mkdirs())) {
                reports.add(refusal("ended", id, "failed", "cannot make the recordings directory"));
            } else {
                reports.addAll(repair(dir));
                String name = command.optString("name");
                File target = new File(dir, name), part = new File(dir, "." + name + PART);
                if (target.exists() || part.exists()) {
                    reports.add(json(new Json().put("type", "ended").put("id", id).put("error", "exists")
                            .put("path", target.getAbsolutePath())));
                } else if (dir.getUsableSpace() < RESERVE) {
                    reports.add(refusal("ended", id, "space", ""));
                } else {
                    r = new Recorder(owner, id, target, part, command.optInt("rate"), command.optInt("channels", 1),
                            command.optInt("level"), command.optBoolean("pause"));
                    current = r;
                }
            }
        }
        for (byte[] report : reports) owner.write(Protocol.RECORDING_REPORT, report);
        if (r == null) return;
        try {
            context.startForegroundService(new Intent(context, RecordingService.class));
        } catch (RuntimeException e) {
            // The system's refusal, such as ForegroundServiceStartNotAllowedException.
            r.end("unavailable", String.valueOf(e.getMessage()));
        }
    }

    /** A recording and its pending terminal reports keep their owner running when its activity goes away. */
    static synchronized boolean active(NsgoEngine owner) {
        return current != null && current.owner == owner || reporting.containsKey(owner);
    }

    /** Stops owner's recording id; nothing if it ended already. */
    static void stop(NsgoEngine owner, long id) {
        Recorder r;
        synchronized (RecordingService.class) {
            r = current;
        }
        if (r != null && r.owner == owner && r.id == id) r.end("", "");
    }

    /** Finishes owner's recording as its Go process ended: the next process to connect hears of it. */
    static void ownerEnded(NsgoEngine owner) {
        Recorder r;
        synchronized (RecordingService.class) {
            if (heir == owner) heir = null;
            r = current;
            if (r == null || r.owner != owner) return;
            // Under the lock its report reads it by, so the report goes whole to this process or to the next.
            r.orphaned = true;
        }
        r.end("abandoned", "");
    }

    /** Lets the app record again once r's file is done, and stops the service it ran in. */
    private static void released(Recorder r) {
        RecordingService s;
        synchronized (RecordingService.class) {
            if (current == r) current = null;
            s = service;
        }
        if (s != null) new Handler(Looper.getMainLooper()).post(s::idle);
    }

    @Override
    public void onCreate() {
        super.onCreate();
        synchronized (RecordingService.class) {
            service = this;
        }
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        Recorder r;
        synchronized (RecordingService.class) {
            lastStart = startId;
            r = current;
        }
        try {
            if (Build.VERSION.SDK_INT >= 30) {
                startForeground(NOTIFICATION, notification(), ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE);
            } else {
                startForeground(NOTIFICATION, notification());
            }
        } catch (RuntimeException e) {
            // The system refuses a microphone service the app may not run now, such as one started too late to be in the foreground.
            if (r != null) r.end("unavailable", String.valueOf(e.getMessage()));
            idle();
            return START_NOT_STICKY;
        }
        // A start the service hears after its recording ended has none to begin.
        if (r == null || !r.begin(this)) idle();
        // A recording never resumes by itself after the system killed the process.
        return START_NOT_STICKY;
    }

    private Notification notification() {
        NotificationManager manager = getSystemService(NotificationManager.class);
        if (manager.getNotificationChannel(CHANNEL) == null) {
            manager.createNotificationChannel(new NotificationChannel(CHANNEL, "Recording", NotificationManager.IMPORTANCE_LOW));
        }
        int icon = getApplicationInfo().icon;
        if (icon == 0) icon = android.R.drawable.ic_btn_speak_now;
        Notification.Builder b = new Notification.Builder(this, CHANNEL)
                .setSmallIcon(icon)
                .setContentTitle(getApplicationInfo().loadLabel(getPackageManager()))
                .setContentText("Recording")
                .setCategory(Notification.CATEGORY_SERVICE)
                .setOngoing(true);
        Intent launch = getPackageManager().getLaunchIntentForPackage(getPackageName());
        if (launch != null) {
            b.setContentIntent(PendingIntent.getActivity(this, 0, launch, PendingIntent.FLAG_IMMUTABLE));
        }
        if (Build.VERSION.SDK_INT >= 31) b.setForegroundServiceBehavior(Notification.FOREGROUND_SERVICE_IMMEDIATE);
        return b.build();
    }

    /** Stops the service unless a recording runs in it. */
    private void idle() {
        int start;
        synchronized (RecordingService.class) {
            if (current != null) return;
            start = lastStart;
        }
        stopForeground(STOP_FOREGROUND_REMOVE);
        stopSelf(start);
    }

    @Override
    public void onDestroy() {
        Recorder r;
        synchronized (RecordingService.class) {
            if (service == this) service = null;
            r = current;
        }
        if (r != null) r.end("failed", "the recording service stopped");
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }


    /**
     * Repairs the files in dir that killed processes left under their temporary names: each header takes the sizes of what its file holds, and the file its final name unless that is taken. Returns their terminal records, which dir's journal keeps until the app takes them; the caller holds RecordingService.class.
     */
    private static List<byte[]> repair(File dir) {
        List<byte[]> records = new ArrayList<>();
        File[] files = dir.listFiles();
        if (files == null) return records;
        for (File part : files) {
            String name = part.getName();
            if (!name.startsWith(".") || !name.endsWith(PART) || name.length() <= 1 + PART.length()) continue;
            try (RandomAccessFile f = new RandomAccessFile(part, "rw")) {
                byte[] h = new byte[44];
                if (f.read(h) != 44) {
                    // Made but never given its header: no audio to keep.
                    f.close();
                    part.delete();
                    continue;
                }
                int channels = Math.max(1, (h[22] & 0xff) | (h[23] & 0xff) << 8);
                int rate = (h[24] & 0xff) | (h[25] & 0xff) << 8 | (h[26] & 0xff) << 16 | (h[27] & 0xff) << 24;
                int frame = channels * 2;
                long data = Math.min(f.length() - 44, MAX_DATA) / frame * frame;
                f.setLength(44 + data);
                f.seek(0);
                f.write(header(rate, channels, data));
                f.getFD().sync();
                File target = new File(dir, name.substring(1, name.length() - PART.length()));
                File at = !target.exists() && part.renameTo(target) ? target : part;
                byte[] record = json(new Json().put("type", "earlier").put("path", at.getAbsolutePath()).put("rate", rate)
                        .put("channels", channels).put("frames", data / frame).put("error", "partial"));
                // Kept until the app takes it, as the process that hears it may end first.
                keep(at.getAbsolutePath(), record);
                records.add(record);
            } catch (IOException e) {
                Log.w(TAG, "recording: a partial file cannot be repaired");
            }
        }
        return records;
    }

    /** A WAV header for data bytes of 16-bit PCM. */
    private static byte[] header(int rate, int channels, long data) {
        byte[] h = new byte[44];
        ascii(h, 0, "RIFF");
        le(h, 4, 36 + data, 4);
        ascii(h, 8, "WAVEfmt ");
        le(h, 16, 16, 4);
        le(h, 20, 1, 2);
        le(h, 22, channels, 2);
        le(h, 24, rate, 4);
        le(h, 28, (long) rate * channels * 2, 4);
        le(h, 32, channels * 2, 2);
        le(h, 34, 16, 2);
        ascii(h, 36, "data");
        le(h, 40, data, 4);
        return h;
    }

    private static void ascii(byte[] b, int at, String s) {
        for (int i = 0; i < s.length(); i++) b[at + i] = (byte) s.charAt(i);
    }

    private static void le(byte[] b, int at, long v, int n) {
        for (int i = 0; i < n; i++) b[at + i] = (byte) (v >>> (8 * i));
    }

    /** A recording from its start to the end of its file. */
    private static final class Recorder {
        final NsgoEngine owner;
        final long id;
        private final File target, part;
        private final int rate, channels, levelMillis;
        private final boolean pause;
        private final Sender sender;
        // Guarded by this: whether it is opening the microphone, whether it ends and why, and the microphone while it reads.
        private boolean opening, ending;
        private String code = "", detail = "";
        private AudioRecord audio;
        /** Set once the owner's process ended, after which its terminal record goes to the next process; guarded by RecordingService.class. */
        private boolean orphaned;
        /** Whether the system silences the capture, from the recording callback. */
        private volatile boolean silenced;

        Recorder(NsgoEngine owner, long id, File target, File part, int rate, int channels, int levelMillis, boolean pause) {
            this.owner = owner;
            this.id = id;
            this.target = target;
            this.part = part;
            this.rate = rate;
            this.channels = channels == 2 ? 2 : 1;
            this.levelMillis = levelMillis;
            this.pause = pause;
            sender = new Sender(owner);
        }

        /** Opens the microphone and its file and starts the reader, in the running service; false once the recording ended. */
        boolean begin(Context context) {
            synchronized (this) {
                if (ending || opening || audio != null) return !ending;
                opening = true;
            }
            AudioRecord a;
            try {
                a = open(rate, channels);
            } catch (SecurityException e) {
                return failToOpen(null, null, null, "denied", "");
            } catch (RuntimeException e) {
                return failToOpen(null, null, null, "unavailable", String.valueOf(e.getMessage()));
            }
            RandomAccessFile file = null;
            try {
                if (!part.createNewFile()) {
                    a.release();
                    return failToOpen(null, null, null, "failed", "the recording's file exists");
                }
                file = new RandomAccessFile(part, "rw");
                file.write(header(a.getSampleRate(), a.getChannelCount(), 0));
            } catch (IOException e) {
                if (file == null) part.delete();
                return failToOpen(a, null, file, "failed", String.valueOf(e.getMessage()));
            }
            AudioManager.AudioRecordingCallback watch = null;
            if (Build.VERSION.SDK_INT >= 29) {
                // Android requires the callback before startRecording. A call or a capture of higher priority silences this one rather than stopping it.
                int session = a.getAudioSessionId();
                watch = new AudioManager.AudioRecordingCallback() {
                    @Override
                    public void onRecordingConfigChanged(List<AudioRecordingConfiguration> configs) {
                        for (AudioRecordingConfiguration c : configs) {
                            if (c.getClientAudioSessionId() == session) silence(c.isClientSilenced());
                        }
                    }
                };
                a.registerAudioRecordingCallback(Runnable::run, watch);
            }
            try {
                a.startRecording();
            } catch (IllegalStateException e) {
                // Found not recording below.
            }
            if (a.getRecordingState() != AudioRecord.RECORDSTATE_RECORDING) {
                // Before Android 10 another app's capture keeps this one from starting.
                return failToOpen(a, watch, file, "busy", "");
            }
            boolean stopped;
            synchronized (this) {
                stopped = ending;
                if (!stopped) {
                    opening = false;
                    audio = a;
                }
            }
            // Stopped or abandoned while opening: nothing was recorded.
            if (stopped) return failToOpen(a, watch, file, null, null);
            sender.post(json(new Json().put("type", "started").put("id", id).put("path", target.getAbsolutePath())
                    .put("rate", a.getSampleRate()).put("channels", a.getChannelCount()).put("frames", 0)));
            AudioManager.AudioRecordingCallback watching = watch;
            RandomAccessFile writing = file;
            Thread reader = new Thread(() -> read(a, watching, writing), "nsgo-recording");
            reader.setDaemon(true);
            reader.start();
            return true;
        }

        /** Lets go of what opening took, the empty file it made included, and reports the end: code's, unless the recording ended meanwhile. */
        private boolean failToOpen(AudioRecord a, AudioManager.AudioRecordingCallback watch, RandomAccessFile file,
                String code, String detail) {
            if (a != null) close(a, watch, file);
            // The file holds no more than its header, as the microphone never started.
            if (file != null) part.delete();
            synchronized (this) {
                opening = false;
                if (!ending) {
                    ending = true;
                    this.code = code;
                    this.detail = detail;
                }
            }
            report(0, 0, 0, false);
            return false;
        }

        /** The microphone at rate and channels, or at the device's rate and in mono where it has not those. */
        private static AudioRecord open(int rate, int channels) {
            int[] rates = {rate, 48000, 44100, 16000};
            int[] masks = channels == 2 ? new int[] {AudioFormat.CHANNEL_IN_STEREO, AudioFormat.CHANNEL_IN_MONO}
                    : new int[] {AudioFormat.CHANNEL_IN_MONO};
            String failure = "the microphone opens at no rate";
            for (int mask : masks) {
                int frame = mask == AudioFormat.CHANNEL_IN_STEREO ? 4 : 2;
                for (int r : rates) {
                    if (r <= 0) continue;
                    int min = AudioRecord.getMinBufferSize(r, mask, AudioFormat.ENCODING_PCM_16BIT);
                    if (min <= 0) continue;
                    try {
                        // Half a second of buffer covers a slow disk write.
                        return new AudioRecord.Builder().setAudioSource(MediaRecorder.AudioSource.MIC)
                                .setAudioFormat(new AudioFormat.Builder().setSampleRate(r)
                                        .setEncoding(AudioFormat.ENCODING_PCM_16BIT).setChannelMask(mask).build())
                                .setBufferSizeInBytes(Math.max(min, r / 2 * frame)).build();
                    } catch (UnsupportedOperationException e) {
                        // build() throws rather than return a record it could not initialize, so the next format is tried.
                        failure = String.valueOf(e.getMessage());
                    }
                }
            }
            throw new IllegalStateException(failure);
        }

        private void silence(boolean now) {
            if (now == silenced) return;
            silenced = now;
            if (pause) {
                sender.post(json(new Json().put("type", "interrupted").put("id", id).put("silenced", now)));
            } else if (now) {
                end("interrupted", "");
            }
        }

        /** Ends the recording with code, empty for a stop: a reading one once its reader leaves AudioRecord, an opening one once it opened, one that never opened at once. */
        void end(String code, String detail) {
            synchronized (this) {
                if (ending) return;
                ending = true;
                this.code = code;
                this.detail = detail == null ? "" : detail;
                if (audio != null) {
                    audio.stop();
                    return;
                }
                if (opening) return;
            }
            report(0, 0, 0, false);
        }

        private synchronized boolean ending() {
            return ending;
        }

        private void read(AudioRecord a, AudioManager.AudioRecordingCallback watch, RandomAccessFile file) {
            int rate = a.getSampleRate(), channels = a.getChannelCount(), frame = channels * 2;
            File dir = target.getParentFile();
            // Reads of about 20 ms, levels over levelMillis and the header about once a second.
            byte[] buf = new byte[Math.max(256, rate / 50) * frame];
            long data = 0, levelFrames = levelMillis > 0 ? Math.max(1, (long) rate * levelMillis / 1000) : 0;
            long heard = 0, checked = 0;
            int peak = 0;
            double squares = 0;
            while (!ending()) {
                int n = a.read(buf, 0, buf.length, AudioRecord.READ_BLOCKING);
                if (n < 0) {
                    end("failed", "AudioRecord.read: " + n);
                    break;
                }
                n -= n % frame;
                if (n == 0 || (pause && silenced)) continue;
                if (data + n > MAX_DATA) {
                    end("full", "");
                    break;
                }
                try {
                    file.write(buf, 0, n);
                } catch (IOException e) {
                    end(dir.getUsableSpace() < RESERVE ? "space" : "failed", String.valueOf(e.getMessage()));
                    break;
                }
                data += n;
                if (levelFrames > 0) {
                    for (int i = 0; i + 1 < n; i += 2) {
                        int s = (short) ((buf[i] & 0xff) | (buf[i + 1] << 8));
                        peak = Math.max(peak, Math.abs(s));
                        squares += (double) s * s;
                    }
                    heard += n / frame;
                    if (heard >= levelFrames) {
                        sender.level(json(new Json().put("type", "level").put("id", id).put("frames", data / frame)
                                .put("peak", Math.min(1, peak / 32768.0))
                                .put("rms", Math.sqrt(squares / (heard * channels)) / 32768.0)));
                        heard = 0;
                        peak = 0;
                        squares = 0;
                    }
                }
                if (data - checked >= (long) rate * frame) {
                    checked = data;
                    try {
                        file.seek(0);
                        file.write(header(rate, channels, data));
                        file.seek(44 + data);
                    } catch (IOException e) {
                        end("failed", String.valueOf(e.getMessage()));
                        break;
                    }
                    if (dir.getUsableSpace() < RESERVE) {
                        end("space", "");
                        break;
                    }
                    // Without levels, which carry it, the duration so far goes with each header.
                    if (levelFrames == 0) {
                        sender.level(json(new Json().put("type", "written").put("id", id).put("frames", data / frame)));
                    }
                }
            }
            String failure = "";
            try {
                // A failed write may have written only part of its PCM block. Keep only the complete blocks counted in data.
                file.setLength(44 + data);
                file.seek(0);
                file.write(header(rate, channels, data));
                file.getFD().sync();
            } catch (IOException e) {
                failure = String.valueOf(e.getMessage());
            }
            close(a, watch, file);
            synchronized (this) {
                audio = null;
                if (!failure.isEmpty() && code.isEmpty()) {
                    code = "failed";
                    detail = failure;
                }
            }
            report(rate, channels, data / frame, part.renameTo(target));
        }

        private static void close(AudioRecord a, AudioManager.AudioRecordingCallback watch, RandomAccessFile file) {
            if (watch != null) a.unregisterAudioRecordingCallback(watch);
            a.release();
            if (file == null) return;
            try {
                file.close();
            } catch (IOException ignored) {
                // The header was written and synced, or its failure is reported.
            }
        }

        /** Reports the end, with the file of frames at rate and channels when one was written, to the owner or, its process gone, to the next one; then lets the app record again. */
        private void report(int rate, int channels, long frames, boolean renamed) {
            String c, d;
            synchronized (this) {
                c = code;
                d = detail;
            }
            // Read once, as ownerEnded may set it at any time.
            boolean gone;
            synchronized (RecordingService.class) {
                gone = orphaned;
            }
            Json end = new Json().put("type", gone ? "earlier" : "ended").put("id", id).put("error", c).put("detail", d);
            String path = (renamed ? target : part).getAbsolutePath();
            if (rate > 0) {
                end.put("path", path).put("rate", rate).put("channels", channels).put("frames", frames);
            }
            // Nothing reports what nobody recorded.
            if (gone && rate > 0) orphan(this, path, json(end));
            released(this);
            sender.finish(gone ? null : json(end));
        }
    }

    /** Writes one recording's reports to its owner's pipe on a thread of its own, keeping only the latest level Go has not taken. */
    private static final class Sender implements Runnable {
        private final NsgoEngine owner;
        private final ArrayDeque<byte[]> reports = new ArrayDeque<>();
        private byte[] level;
        private boolean finished;

        Sender(NsgoEngine owner) {
            this.owner = owner;
            synchronized (RecordingService.class) {
                reporting.put(owner, reporting.getOrDefault(owner, 0) + 1);
            }
            Thread t = new Thread(this, "nsgo-recording-send");
            t.setDaemon(true);
            t.start();
        }

        synchronized void post(byte[] report) {
            if (finished) return;
            reports.add(report);
            notifyAll();
        }

        /** Posts a level, or the frames written of a recording without levels; only the latest waits. */
        synchronized void level(byte[] report) {
            if (finished) return;
            level = report;
            notifyAll();
        }

        /** Ends the reports with last, if any: no level goes after it. */
        synchronized void finish(byte[] last) {
            if (finished) return;
            finished = true;
            level = null;
            if (last != null) reports.add(last);
            notifyAll();
        }

        @Override
        public void run() {
            try {
                sendReports();
            } finally {
                synchronized (RecordingService.class) {
                    int remaining = reporting.get(owner) - 1;
                    if (remaining == 0) reporting.remove(owner);
                    else reporting.put(owner, remaining);
                }
                owner.recordingEnded();
            }
        }

        private void sendReports() {
            while (true) {
                byte[] next;
                synchronized (this) {
                    while (reports.isEmpty() && level == null && !finished) {
                        try {
                            wait();
                        } catch (InterruptedException e) {
                            return;
                        }
                    }
                    if (!reports.isEmpty()) {
                        next = reports.poll();
                    } else if (level != null) {
                        next = level;
                        level = null;
                    } else {
                        return;
                    }
                }
                owner.write(Protocol.RECORDING_REPORT, next);
            }
        }
    }

    /** A JSONObject whose puts cannot fail, as every key here is a literal and every value finite. */
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
