package android.graphics.fonts;

public final class FontFamily {
    private FontFamily() {}

    public static final class Builder {
        public Builder(Font font) {}
        public Builder addFont(Font font) { return this; }
        public FontFamily build() { return new FontFamily(); }
    }
}
