package android.text;

public final class StaticLayout extends Layout {
    private StaticLayout(CharSequence text, TextPaint paint, int width, Alignment align, int dir, int maxLines, TextUtils.TruncateAt ellipsize) {
        super(text, paint, width, align, dir, maxLines, ellipsize);
    }

    public static final class Builder {
        private final CharSequence text;
        private final TextPaint paint;
        private final int width;
        private Alignment align = Alignment.ALIGN_NORMAL;
        private int dir = DIR_LEFT_TO_RIGHT;
        private int maxLines = Integer.MAX_VALUE;
        private TextUtils.TruncateAt ellipsize;

        private Builder(CharSequence text, TextPaint paint, int width) {
            this.text = text;
            this.paint = paint;
            this.width = width;
        }

        public static Builder obtain(CharSequence source, int start, int end, TextPaint paint, int width) {
            return new Builder(start == 0 && end == source.length() ? source : source.subSequence(start, end), paint, width);
        }

        public Builder setAlignment(Alignment align) { this.align = align; return this; }
        public Builder setTextDirection(TextDirectionHeuristic d) {
            this.dir = d == TextDirectionHeuristics.RTL ? DIR_RIGHT_TO_LEFT : DIR_LEFT_TO_RIGHT;
            return this;
        }
        public Builder setIncludePad(boolean includePad) { return this; }
        public Builder setJustificationMode(int mode) { return this; }
        public Builder setMaxLines(int maxLines) { this.maxLines = maxLines; return this; }
        public Builder setEllipsize(TextUtils.TruncateAt ellipsize) { this.ellipsize = ellipsize; return this; }
        public Builder setLineBreakConfig(android.graphics.text.LineBreakConfig config) { return this; }

        public StaticLayout build() {
            return new StaticLayout(text, paint, width, align, dir, maxLines, ellipsize);
        }
    }
}
