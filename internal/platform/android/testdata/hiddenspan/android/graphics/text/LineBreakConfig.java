package android.graphics.text;

public final class LineBreakConfig {
    public static final int LINE_BREAK_WORD_STYLE_PHRASE = 1;

    public static final class Builder {
        public Builder setLineBreakWordStyle(int style) { return this; }
        public LineBreakConfig build() { return new LineBreakConfig(); }
    }
}
