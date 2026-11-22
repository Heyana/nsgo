package android.media;
public final class AudioFormat {
    public static final int CHANNEL_IN_MONO = 16, CHANNEL_IN_STEREO = 12, ENCODING_PCM_16BIT = 2;
    int rate, mask;
    public static final class Builder {
        private final AudioFormat value = new AudioFormat();
        /** Android refuses a rate outside the range a format describes. */
        public Builder setSampleRate(int r) { if (r < 4000 || r > 192000) throw new IllegalArgumentException("rate " + r); value.rate = r; return this; }
        public Builder setChannelMask(int m) { value.mask = m; return this; }
        public Builder setEncoding(int e) { return this; }
        public AudioFormat build() { return value; }
    }
}
