package android.media;

public class AudioFocusRequest {
    public static class Builder {
        public Builder(int focusGain) {}
        public Builder setAudioAttributes(AudioAttributes attrs) { return this; }
        public Builder setWillPauseWhenDucked(boolean pause) { return this; }
        public Builder setOnAudioFocusChangeListener(AudioManager.OnAudioFocusChangeListener l) { return this; }
        public AudioFocusRequest build() { return new AudioFocusRequest(); }
    }
}
