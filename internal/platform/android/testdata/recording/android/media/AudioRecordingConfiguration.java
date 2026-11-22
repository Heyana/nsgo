package android.media;
public final class AudioRecordingConfiguration {
    private final boolean silenced;
    public AudioRecordingConfiguration(boolean s) { silenced = s; }
    public int getClientAudioSessionId() { return 1; }
    public boolean isClientSilenced() { return silenced; }
}
