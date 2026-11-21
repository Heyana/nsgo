package android.graphics;

public final class Typeface {
    public static final int NORMAL = 0;
    public static final Typeface DEFAULT = new Typeface("default", 400, false);
    public static final Typeface SERIF = new Typeface("serif", 400, false);
    public static final Typeface MONOSPACE = new Typeface("monospace", 400, false);
    public final String family;
    public final int weight;
    public final boolean italic;

    private Typeface(String family, int weight, boolean italic) {
        this.family = family;
        this.weight = weight;
        this.italic = italic;
    }

    public static Typeface create(String family, int style) {
        return new Typeface(family, 400, false);
    }

    public static Typeface create(Typeface base, int weight, boolean italic) {
        return new Typeface(base == null ? "default" : base.family, weight, italic);
    }

    public static final class CustomFallbackBuilder {
        public CustomFallbackBuilder(android.graphics.fonts.FontFamily family) {}
        public static int getMaxCustomFallbackCount() { return 64; }
        public CustomFallbackBuilder addCustomFallback(android.graphics.fonts.FontFamily family) { return this; }
        public CustomFallbackBuilder setStyle(android.graphics.fonts.FontStyle style) { return this; }
        public CustomFallbackBuilder setSystemFallback(String family) { return this; }
        public Typeface build() { return DEFAULT; }
    }
}
