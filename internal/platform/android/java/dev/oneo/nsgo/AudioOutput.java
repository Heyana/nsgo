package dev.oneo.nsgo;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.media.AudioAttributes;
import android.media.AudioDeviceInfo;
import android.media.AudioFocusRequest;
import android.media.AudioFormat;
import android.media.AudioManager;
import android.media.AudioRouting;
import android.media.AudioTimestamp;
import android.media.AudioTrack;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.os.Process;
import android.util.Log;

import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.LinkedBlockingQueue;

/**
 * Pull-based Float32 PCM output streams for the audio package (audiooutput.go).
 * Each stream owns an Android AudioTrack in MODE_STREAM with a dedicated writer
 * thread paced by AudioTrack.write, hardware position/timestamp reporting via
 * getTimestamp and getPlaybackHeadPosition, and interruption reporting on audio
 * focus loss, ACTION_AUDIO_BECOMING_NOISY, routing changes and device errors.
 */
final class AudioOutput {
    private static final String TAG = "nsgo";
    private static final long NANOS_PER_SECOND = 1_000_000_000L;
    private static final long STALE_TIMESTAMP_NANOS = -500_000_000L;

    interface Writer {
        void write(int kind, byte[] payload);
    }

    private static final class Chunk {
        static final Chunk STOP = new Chunk(0, 0, null);
        final int generation;
        final int frames;
        final byte[] bytes;

        Chunk(int generation, int frames, byte[] bytes) {
            this.generation = generation;
            this.frames = frames;
            this.bytes = bytes;
        }
    }

    private static final class Timing {
        final long played;
        final long offsetNanos;
        final long latencyNanos;

        Timing(long played, long offsetNanos, long latencyNanos) {
            this.played = played;
            this.offsetNanos = offsetNanos;
            this.latencyNanos = latencyNanos;
        }
    }

    private final Context context;
    private final Writer out;
    private final Handler mainHandler = new Handler(Looper.getMainLooper());
    private final ConcurrentHashMap<Integer, Stream> streams = new ConcurrentHashMap<>();
    private final ExecutorService control = Executors.newSingleThreadExecutor(r -> {
        Thread t = new Thread(r, "nsgo-pcm-ctl");
        t.setDaemon(true);
        return t;
    });
    private volatile boolean closed;

    AudioOutput(Context context, Writer out) {
        this.context = context.getApplicationContext();
        this.out = out;
    }

    /** Reads one Protocol.EXT_AUDIO_OUTPUT payload on the pipe reader thread without blocking. */
    void message(DataInputStream in) throws IOException {
        if (closed) return;
        int op = in.readUnsignedByte();
        switch (op) {
            case Protocol.PCM_OPEN: {
                int id = in.readInt();
                int req = in.readInt();
                int sampleRate = in.readInt();
                int channels = in.readUnsignedByte();
                long latencyNanos = in.readLong();
                control.execute(() -> open(id, req, sampleRate, channels, latencyNanos));
                break;
            }
            case Protocol.PCM_START: {
                int id = in.readInt();
                int req = in.readInt();
                int gen = in.readInt();
                control.execute(() -> start(id, req, gen));
                break;
            }
            case Protocol.PCM_PAUSE: {
                int id = in.readInt();
                int req = in.readInt();
                int gen = in.readInt();
                boolean keepFocus = (in.readUnsignedByte() & 1) != 0;
                control.execute(() -> pause(id, req, gen, keepFocus));
                break;
            }
            case Protocol.PCM_CLOSE: {
                int id = in.readInt();
                int req = in.readInt();
                control.execute(() -> close(id, req));
                break;
            }
            case Protocol.PCM_DATA: {
                int id = in.readInt();
                int gen = in.readInt();
                int frames = in.readInt();
                int len = in.readInt();
                if (len < 0 || len > Protocol.MAX_MESSAGE_SIZE) return;
                byte[] bytes = new byte[len];
                in.readFully(bytes);
                Stream s = streams.get(id);
                if (s != null) s.offerData(gen, frames, bytes);
                break;
            }
            default:
                Log.w(TAG, "unknown PCM output op: " + op);
        }
    }

    /** Stops and releases every open stream when the Go process ends. */
    void closeAll() {
        if (closed) return;
        closed = true;
        control.execute(() -> {
            List<Stream> all = new ArrayList<>(streams.values());
            streams.clear();
            for (Stream s : all) {
                s.destroy();
            }
            control.shutdown();
        });
    }

    private void open(int id, int req, int sampleRate, int channels, long latencyNanos) {
        if (closed) return;
        try {
            Stream s = new Stream(id, sampleRate, channels, latencyNanos);
            s.buildTrack();
            streams.put(id, s);
            sendReply(id, req, "", s.bufferFrames, s.chunkFrames, 0L, 0L, s.nominalLatencyNanos());
        } catch (Throwable t) {
            String msg = t.getMessage();
            sendReply(id, req, msg == null || msg.isEmpty() ? t.getClass().getSimpleName() : msg, 0, 0, 0L, 0L, 0L);
        }
    }

    private void start(int id, int req, int gen) {
        if (closed) return;
        Stream s = streams.get(id);
        if (s == null) {
            sendReply(id, req, "PCM output is closed", 0, 0, 0L, 0L, 0L);
            return;
        }
        try {
            s.start(gen);
            sendReply(id, req, "", s.bufferFrames, s.chunkFrames, 0L, 0L, s.nominalLatencyNanos());
            s.primeCredits(gen);
        } catch (Throwable t) {
            s.cleanupAfterFailedStart();
            String msg = t.getMessage();
            sendReply(id, req, msg == null || msg.isEmpty() ? t.getClass().getSimpleName() : msg, 0, 0, 0L, 0L, 0L);
        }
    }

    private void pause(int id, int req, int gen, boolean keepFocus) {
        if (closed) return;
        Stream s = streams.get(id);
        if (s == null) {
            sendReply(id, req, "", 0, 0, 0L, 0L, 0L);
            return;
        }
        Timing t = s.pause(gen, keepFocus);
        sendReply(id, req, "", s.bufferFrames, s.chunkFrames, t.played, t.offsetNanos, t.latencyNanos);
    }

    private void close(int id, int req) {
        Stream s = streams.remove(id);
        if (s != null) s.destroy();
        if (!closed) {
            sendReply(id, req, "", 0, 0, 0L, 0L, 0L);
        }
    }

    private void sendReply(int id, int req, String err, int bufferFrames, int chunkFrames,
            long played, long offsetNanos, long latencyNanos) {
        if (closed) return;
        ByteArrayOutputStream bytes = new ByteArrayOutputStream(64);
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeByte(Protocol.PCM_REPLY);
            d.writeInt(id);
            d.writeInt(req);
            Protocol.writeString(d, err == null ? "" : err);
            d.writeInt(bufferFrames);
            d.writeInt(chunkFrames);
            d.writeLong(played);
            d.writeLong(offsetNanos);
            d.writeLong(latencyNanos);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        out.write(Protocol.AUDIO_OUTPUT_REPORT, bytes.toByteArray());
    }

    private void sendPull(int id, int gen, int requestedFrames, Timing timing) {
        if (closed) return;
        ByteArrayOutputStream bytes = new ByteArrayOutputStream(40);
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeByte(Protocol.PCM_PULL);
            d.writeInt(id);
            d.writeInt(gen);
            d.writeInt(requestedFrames);
            d.writeLong(timing.played);
            d.writeLong(timing.offsetNanos);
            d.writeLong(timing.latencyNanos);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        out.write(Protocol.AUDIO_OUTPUT_REPORT, bytes.toByteArray());
    }

    private void sendInterrupted(int id, int gen, Timing timing, String reason) {
        if (closed) return;
        ByteArrayOutputStream bytes = new ByteArrayOutputStream(64);
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeByte(Protocol.PCM_INTERRUPTED);
            d.writeInt(id);
            d.writeInt(gen);
            d.writeLong(timing.played);
            d.writeLong(timing.offsetNanos);
            Protocol.writeString(d, reason == null ? "" : reason);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        out.write(Protocol.AUDIO_OUTPUT_REPORT, bytes.toByteArray());
    }

    private final class Stream {
        final int id;
        final int sampleRate;
        final int channels;
        final long latencyHintNanos;
        final AudioAttributes attrs;

        AudioTrack track;
        int bufferFrames;
        int chunkFrames;

        volatile boolean running;
        volatile int generation;
        final LinkedBlockingQueue<Chunk> queue = new LinkedBlockingQueue<>();
        Thread writer;

        AudioFocusRequest focusRequest;
        boolean hasFocus;
        BroadcastReceiver noisyReceiver;
        boolean hearsNoisy;
        AudioRouting.OnRoutingChangedListener routingListener;
        volatile int routedDeviceId = -1;

        // Owned by writer while running, and by control thread once writer stops.
        long writtenInGen;
        long lastRawHead;
        long headWrap;
        long lastPlayedInGen;
        long lastRawTsFrames;
        long tsBase;
        boolean tsBaseDecided;
        Timing lastTiming = new Timing(0L, 0L, 0L);

        Stream(int id, int sampleRate, int channels, long latencyHintNanos) {
            this.id = id;
            this.sampleRate = sampleRate;
            this.channels = channels;
            this.latencyHintNanos = Math.max(0L, latencyHintNanos);
            this.attrs = new AudioAttributes.Builder()
                    .setUsage(AudioAttributes.USAGE_MEDIA)
                    .setContentType(AudioAttributes.CONTENT_TYPE_MUSIC)
                    .build();
        }

        long nominalLatencyNanos() {
            return (bufferFrames * NANOS_PER_SECOND) / sampleRate;
        }

        void buildTrack() {
            int channelMask = channels == 1
                    ? AudioFormat.CHANNEL_OUT_MONO
                    : AudioFormat.CHANNEL_OUT_STEREO;
            int frameBytes = channels * 4;
            int minBytes = AudioTrack.getMinBufferSize(sampleRate, channelMask, AudioFormat.ENCODING_PCM_FLOAT);
            if (minBytes <= 0) {
                throw new IllegalStateException("unsupported PCM format (" + sampleRate + " Hz, " + channels + " ch): " + minBytes);
            }
            long hintFrames = latencyHintNanos > 0
                    ? Math.max(1L, (sampleRate * latencyHintNanos) / NANOS_PER_SECOND)
                    : 0L;
            long requestedBytes = hintFrames > 0 ? hintFrames * frameBytes : minBytes;
            int bufferBytes = (int) Math.max(minBytes, Math.min(requestedBytes, Integer.MAX_VALUE - frameBytes));
            if (bufferBytes % frameBytes != 0) {
                bufferBytes += frameBytes - (bufferBytes % frameBytes);
            }
            AudioFormat format = new AudioFormat.Builder()
                    .setSampleRate(sampleRate)
                    .setEncoding(AudioFormat.ENCODING_PCM_FLOAT)
                    .setChannelMask(channelMask)
                    .build();

            AudioTrack created = null;
            if (latencyHintNanos > 0) {
                try {
                    created = new AudioTrack.Builder()
                            .setAudioAttributes(attrs)
                            .setAudioFormat(format)
                            .setBufferSizeInBytes(bufferBytes)
                            .setTransferMode(AudioTrack.MODE_STREAM)
                            .setPerformanceMode(AudioTrack.PERFORMANCE_MODE_LOW_LATENCY)
                            .build();
                    if (created.getState() != AudioTrack.STATE_INITIALIZED) {
                        created.release();
                        created = null;
                    }
                } catch (UnsupportedOperationException | IllegalArgumentException e) {
                    created = null;
                }
            }
            if (created == null) {
                created = new AudioTrack.Builder()
                        .setAudioAttributes(attrs)
                        .setAudioFormat(format)
                        .setBufferSizeInBytes(bufferBytes)
                        .setTransferMode(AudioTrack.MODE_STREAM)
                        .build();
            }
            if (created.getState() != AudioTrack.STATE_INITIALIZED) {
                created.release();
                throw new IllegalStateException("AudioTrack failed to initialize");
            }
            if (track != null) {
                track.release();
            }
            track = created;
            bufferFrames = Math.max(1, created.getBufferSizeInFrames());
            chunkFrames = chooseChunkFrames(bufferFrames, hintFrames, sampleRate);
            lastRawTsFrames = 0L;
            lastTiming = new Timing(0L, 0L, nominalLatencyNanos());
        }

        private int chooseChunkFrames(int actualBufferFrames, long hintFrames, int rate) {
            if (actualBufferFrames < 128) {
                return Math.max(1, actualBufferFrames / 2);
            }
            int maxChunk = Math.max(64, Math.min(2048, rate / 25));
            int halfBuffer = Math.max(64, actualBufferFrames / 2);
            int target = hintFrames > 0 ? (int) Math.min(hintFrames, Integer.MAX_VALUE) : Math.min(1024, halfBuffer);
            return Math.max(64, Math.min(target, Math.min(maxChunk, halfBuffer)));
        }

        void offerData(int gen, int frames, byte[] bytes) {
            if (!running || gen != generation) return;
            queue.offer(new Chunk(gen, frames, bytes));
        }

        void start(int gen) {
            if (running) {
                stopWriterAndFlush(true);
            }
            if (track == null || track.getState() != AudioTrack.STATE_INITIALIZED) {
                buildTrack();
            }
            queue.clear();
            writtenInGen = 0L;
            lastRawHead = 0L;
            headWrap = 0L;
            lastPlayedInGen = 0L;
            tsBase = 0L;
            tsBaseDecided = false;
            lastTiming = new Timing(0L, 0L, nominalLatencyNanos());
            routedDeviceId = -1;

            generation = gen;
            requestFocus(gen);
            registerListeners(gen);

            track.play();
            if (track.getPlayState() != AudioTrack.PLAYSTATE_PLAYING) {
                throw new IllegalStateException("AudioTrack did not enter playing state");
            }
            AudioDeviceInfo routed = track.getRoutedDevice();
            if (routed != null) {
                routedDeviceId = routed.getId();
            }
            running = true;
            Thread t = new Thread(() -> writeLoop(gen), "nsgo-pcm-" + id);
            t.setDaemon(true);
            writer = t;
            t.start();
        }

        void primeCredits(int gen) {
            if (!running || generation != gen) return;
            int credits = Math.max(2, Math.min(4, bufferFrames / Math.max(1, chunkFrames)));
            Timing initial = new Timing(0L, 0L, nominalLatencyNanos());
            for (int i = 0; i < credits; i++) {
                sendPull(id, gen, chunkFrames, initial);
            }
        }

        void cleanupAfterFailedStart() {
            running = false;
            unregisterListeners();
            abandonFocus();
            if (track != null && track.getState() == AudioTrack.STATE_INITIALIZED) {
                try {
                    track.pause();
                    track.flush();
                } catch (IllegalStateException ignored) {
                }
            }
        }

        Timing pause(int gen, boolean keepFocus) {
            if (!running) {
                if (!keepFocus) {
                    unregisterListeners();
                    abandonFocus();
                }
                return lastTiming;
            }
            return stopWriterAndFlush(keepFocus);
        }

        private Timing stopWriterAndFlush(boolean keepFocus) {
            running = false;
            queue.clear();
            queue.offer(Chunk.STOP);
            AudioTrack activeTrack = track;
            if (activeTrack != null && activeTrack.getState() == AudioTrack.STATE_INITIALIZED) {
                try {
                    activeTrack.pause();
                } catch (IllegalStateException ignored) {
                }
            }
            Thread t = writer;
            writer = null;
            if (t != null && t != Thread.currentThread()) {
                t.interrupt();
                try {
                    t.join(1000);
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                }
            }
            Timing finalTiming = lastTiming;
            if (activeTrack != null && activeTrack.getState() == AudioTrack.STATE_INITIALIZED) {
                finalTiming = readTiming(activeTrack);
                lastTiming = finalTiming;
                try {
                    activeTrack.flush();
                } catch (IllegalStateException ignored) {
                }
            }
            queue.clear();
            if (!keepFocus) {
                unregisterListeners();
                abandonFocus();
            }
            return finalTiming;
        }

        void scheduleInterrupt(int gen, String reason, boolean deadTrack) {
            if (closed) return;
            control.execute(() -> handleInterrupt(gen, reason, deadTrack));
        }

        private void handleInterrupt(int gen, String reason, boolean deadTrack) {
            if (closed || !running || generation != gen) return;
            Timing finalTiming = stopWriterAndFlush(false);
            if (deadTrack && track != null) {
                try {
                    track.release();
                } catch (RuntimeException ignored) {
                }
                track = null;
                lastRawTsFrames = 0L;
            }
            sendInterrupted(id, gen, finalTiming, reason);
        }

        void destroy() {
            if (running) {
                stopWriterAndFlush(false);
            } else {
                unregisterListeners();
                abandonFocus();
            }
            if (track != null) {
                try {
                    track.release();
                } catch (RuntimeException ignored) {
                }
                track = null;
            }
        }

        private void writeLoop(int gen) {
            Process.setThreadPriority(Process.THREAD_PRIORITY_AUDIO);
            float[] scratch = new float[Math.max(1, chunkFrames) * channels];
            while (running && generation == gen) {
                Chunk chunk;
                try {
                    chunk = queue.take();
                } catch (InterruptedException e) {
                    break;
                }
                if (chunk == Chunk.STOP || !running || generation != gen) {
                    break;
                }
                if (chunk.generation != gen || chunk.bytes == null || chunk.bytes.length == 0) {
                    continue;
                }
                int samples = chunk.bytes.length / 4;
                if (samples <= 0) continue;
                if (scratch.length < samples) {
                    scratch = new float[samples];
                }
                ByteBuffer.wrap(chunk.bytes, 0, samples * 4)
                        .order(ByteOrder.LITTLE_ENDIAN)
                        .asFloatBuffer()
                        .get(scratch, 0, samples);

                AudioTrack activeTrack = track;
                if (activeTrack == null) break;
                int offset = 0;
                boolean failed = false;
                while (offset < samples && running && generation == gen) {
                    int n = activeTrack.write(scratch, offset, samples - offset, AudioTrack.WRITE_BLOCKING);
                    if (n < 0) {
                        if (running && generation == gen) {
                            scheduleInterrupt(gen, "audio output device error (" + n + ")",
                                    n == AudioTrack.ERROR_DEAD_OBJECT);
                        }
                        failed = true;
                        break;
                    }
                    if (n == 0) {
                        if (!running || generation != gen) break;
                        continue;
                    }
                    offset += n;
                    writtenInGen += n / channels;
                }
                if (failed || !running || generation != gen) {
                    break;
                }
                Timing timing = readTiming(activeTrack);
                lastTiming = timing;
                sendPull(id, gen, chunkFrames, timing);
            }
        }

        private long readPlaybackHead(AudioTrack activeTrack) {
            long uHead = activeTrack.getPlaybackHeadPosition() & 0xFFFFFFFFL;
            if (uHead < lastRawHead && (lastRawHead - uHead) > 0x80000000L) {
                headWrap += 0x1_0000_0000L;
            }
            lastRawHead = uHead;
            return headWrap + uHead;
        }

        private Timing readTiming(AudioTrack activeTrack) {
            long nowNano = System.nanoTime();
            long head = readPlaybackHead(activeTrack);
            AudioTimestamp ts = new AudioTimestamp();
            long played = head;
            long offsetNanos = 0L;
            long rawLeadNanos = 0L;
            if (activeTrack.getTimestamp(ts) && ts.nanoTime > 0 && ts.framePosition >= 0) {
                long deltaNanos = ts.nanoTime - nowNano;
                if (deltaNanos >= STALE_TIMESTAMP_NANOS) {
                    long rawPos = ts.framePosition;
                    if (!tsBaseDecided) {
                        if (lastRawTsFrames > 0 && rawPos >= lastRawTsFrames && rawPos > writtenInGen) {
                            tsBase = lastRawTsFrames;
                        } else if (rawPos > writtenInGen) {
                            tsBase = Math.max(0L, rawPos - Math.min(head, writtenInGen));
                        } else {
                            tsBase = 0L;
                        }
                        tsBaseDecided = true;
                    }
                    long tsPlayed = rawPos - tsBase;
                    rawLeadNanos = Math.max(0L, deltaNanos);
                    if (deltaNanos > 0) {
                        long futureFrames = (deltaNanos * sampleRate) / NANOS_PER_SECOND;
                        tsPlayed = Math.max(0L, tsPlayed - futureFrames);
                        offsetNanos = 0L;
                    } else {
                        offsetNanos = deltaNanos;
                    }
                    if (tsPlayed > 0) {
                        played = tsPlayed;
                        lastRawTsFrames = rawPos;
                    }
                }
            }
            if (played > writtenInGen) played = writtenInGen;
            if (played < lastPlayedInGen) {
                played = lastPlayedInGen;
                offsetNanos = 0L;
            }
            lastPlayedInGen = played;

            long bufferLatencyNanos = nominalLatencyNanos();
            long queuedFrames = Math.max(0L, writtenInGen - played);
            long queuedNanos = (queuedFrames * NANOS_PER_SECOND) / sampleRate + rawLeadNanos;
            long latency = Math.max(bufferLatencyNanos, queuedNanos);
            return new Timing(played, offsetNanos, latency);
        }

        private void requestFocus(int gen) {
            AudioManager am = (AudioManager) context.getSystemService(Context.AUDIO_SERVICE);
            if (am == null) return;
            if (hasFocus && focusRequest != null) {
                // Keep existing focus across Flush; update listener generation via current field.
                return;
            }
            focusRequest = new AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN)
                    .setAudioAttributes(attrs)
                    .setWillPauseWhenDucked(true)
                    .setOnAudioFocusChangeListener(change -> {
                        if (change == AudioManager.AUDIOFOCUS_LOSS
                                || change == AudioManager.AUDIOFOCUS_LOSS_TRANSIENT
                                || change == AudioManager.AUDIOFOCUS_LOSS_TRANSIENT_CAN_DUCK) {
                            scheduleInterrupt(generation, "audio focus lost", false);
                        }
                    }, mainHandler)
                    .build();
            hasFocus = am.requestAudioFocus(focusRequest) == AudioManager.AUDIOFOCUS_REQUEST_GRANTED;
        }

        private void abandonFocus() {
            if (!hasFocus || focusRequest == null) {
                hasFocus = false;
                focusRequest = null;
                return;
            }
            AudioManager am = (AudioManager) context.getSystemService(Context.AUDIO_SERVICE);
            if (am != null) {
                am.abandonAudioFocusRequest(focusRequest);
            }
            hasFocus = false;
            focusRequest = null;
        }

        private void registerListeners(int gen) {
            if (!hearsNoisy) {
                noisyReceiver = new BroadcastReceiver() {
                    @Override
                    public void onReceive(Context ctx, Intent intent) {
                        if (AudioManager.ACTION_AUDIO_BECOMING_NOISY.equals(intent.getAction())) {
                            scheduleInterrupt(generation, "audio output route changed", false);
                        }
                    }
                };
                IntentFilter filter = new IntentFilter(AudioManager.ACTION_AUDIO_BECOMING_NOISY);
                if (Build.VERSION.SDK_INT >= 33) {
                    context.registerReceiver(noisyReceiver, filter, Context.RECEIVER_NOT_EXPORTED);
                } else {
                    context.registerReceiver(noisyReceiver, filter);
                }
                hearsNoisy = true;
            }
            if (track != null && routingListener == null) {
                routingListener = router -> {
                    AudioDeviceInfo dev = router.getRoutedDevice();
                    if (dev == null) return;
                    int devId = dev.getId();
                    if (routedDeviceId == -1) {
                        routedDeviceId = devId;
                        return;
                    }
                    if (devId != routedDeviceId) {
                        routedDeviceId = devId;
                        scheduleInterrupt(generation, "audio output route changed", false);
                    }
                };
                track.addOnRoutingChangedListener(routingListener, mainHandler);
            }
        }

        private void unregisterListeners() {
            if (hearsNoisy && noisyReceiver != null) {
                try {
                    context.unregisterReceiver(noisyReceiver);
                } catch (IllegalArgumentException ignored) {
                }
            }
            hearsNoisy = false;
            noisyReceiver = null;
            if (routingListener != null && track != null) {
                try {
                    track.removeOnRoutingChangedListener(routingListener);
                } catch (RuntimeException ignored) {
                }
            }
            routingListener = null;
        }
    }
}
