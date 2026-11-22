package android.media;
import java.util.List;
public final class AudioManager {
    public abstract static class AudioRecordingCallback { public abstract void onRecordingConfigChanged(List<AudioRecordingConfiguration> configs); }
}
