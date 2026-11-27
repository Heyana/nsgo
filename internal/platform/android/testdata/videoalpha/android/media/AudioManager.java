package android.media;

public class AudioManager {
    public static final String ACTION_AUDIO_BECOMING_NOISY = "android.media.AUDIO_BECOMING_NOISY";
    public static final int AUDIOFOCUS_GAIN = 1;
    public static final int AUDIOFOCUS_GAIN_TRANSIENT = 2;
    public static final int AUDIOFOCUS_LOSS = -1;
    public static final int AUDIOFOCUS_REQUEST_GRANTED = 1;

    public interface OnAudioFocusChangeListener {
        void onAudioFocusChange(int focusChange);
    }

    public int requestAudioFocus(AudioFocusRequest req) { return AUDIOFOCUS_REQUEST_GRANTED; }
    public int abandonAudioFocusRequest(AudioFocusRequest req) { return AUDIOFOCUS_REQUEST_GRANTED; }
}
