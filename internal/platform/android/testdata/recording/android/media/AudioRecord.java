package android.media;
import java.util.*;
import java.util.concurrent.Executor;
/** A controlled PCM source. No microphone or Android SDK is used by the JVM tests. */
public final class AudioRecord {
    public static final int RECORDSTATE_RECORDING = 3;
    public static volatile AudioRecord latest;
    public static volatile boolean denied, busy, stereo;
    /** Sizes a buffer for any rate, as an audio server may for one AudioFormat cannot describe. */
    public static volatile boolean anyRate;
    /** Runs before each read, to pace the reader against what the test has heard. */
    public static volatile Runnable beforeRead;
    public static volatile int supportedRate = 48000, starve, failAfter = Integer.MAX_VALUE;
    private final int rate, channels;
    private volatile boolean running;
    public volatile boolean released;
    private int reads;
    private Executor executor;
    private AudioManager.AudioRecordingCallback callback;
    public static void reset() { denied = busy = stereo = anyRate = false; beforeRead = null; supportedRate = 48000; starve = 0; failAfter = Integer.MAX_VALUE; latest = null; }
    private AudioRecord(AudioFormat format) { rate = format.rate; channels = format.mask == AudioFormat.CHANNEL_IN_STEREO ? 2 : 1; latest = this; }
    public static int getMinBufferSize(int rate, int mask, int format) { return anyRate ? 256 : rate != supportedRate || mask == AudioFormat.CHANNEL_IN_STEREO && !stereo ? -2 : 256; }
    public int getSampleRate() { return rate; }
    public int getChannelCount() { return channels; }
    public int getAudioSessionId() { return 1; }
    public void registerAudioRecordingCallback(Executor e, AudioManager.AudioRecordingCallback c) { executor = e; callback = c; }
    public void unregisterAudioRecordingCallback(AudioManager.AudioRecordingCallback c) { callback = null; }
    public void startRecording() { running = !busy; }
    public int getRecordingState() { return running ? RECORDSTATE_RECORDING : 1; }
    public static final int READ_BLOCKING = 0;
    public int read(byte[] bytes, int offset, int length, int mode) {
        Runnable pace = beforeRead;
        if (pace != null) pace.run();
        try { Thread.sleep(2); } catch (InterruptedException e) { Thread.currentThread().interrupt(); return -3; }
        if (!running) return -3;
        if (reads++ < starve) return 0;
        if (reads >= failAfter) return -6;
        Arrays.fill(bytes, offset, offset + length, (byte) 1);
        return length;
    }
    public void silenced(boolean value) {
        AudioManager.AudioRecordingCallback c = callback;
        if (c != null) executor.execute(() -> c.onRecordingConfigChanged(List.of(new AudioRecordingConfiguration(value))));
    }
    public void stop() { running = false; }
    public void release() { running = false; released = true; }
    public static final class Builder {
        private AudioFormat format;
        public Builder setAudioSource(int source) { return this; }
        public Builder setAudioFormat(AudioFormat f) { format = f; return this; }
        public Builder setBufferSizeInBytes(int size) { return this; }
        public AudioRecord build() { if (denied) throw new SecurityException(); return new AudioRecord(format); }
    }
}
