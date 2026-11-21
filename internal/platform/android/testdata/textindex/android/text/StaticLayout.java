package android.text;

/**
 * Multi-line LTR/RTL/BiDi layout stub with 10 pixels per UTF-16 code unit.
 * Mirrors Android's Layout behavior, including getPrimaryHorizontal(lineEnd)
 * resolving via getLineForOffset to the next line on wrapped lines.
 */
public final class StaticLayout extends Layout {
    public static final class Run {
        public final int start, end;
        public final float left;
        public final boolean rtl;

        public Run(int start, int end, float left, boolean rtl) {
            this.start = start;
            this.end = end;
            this.left = left;
            this.rtl = rtl;
        }

        float right() { return left + (end - start) * 10f; }
    }

    public static final class Line {
        public final int start, end, top, bottom;
        public final float left, right;
        public final boolean rtl;
        public final Run[] runs;

        public Line(int start, int end, float left, int top, int bottom, boolean rtl, Run... runs) {
            this.start = start;
            this.end = end;
            this.left = left;
            this.top = top;
            this.bottom = bottom;
            this.rtl = rtl;
            this.runs = runs;
            float maxRight = left;
            for (Run r : runs) maxRight = Math.max(maxRight, r.right());
            this.right = maxRight;
        }
    }

    private final String text;
    private final int width, height;
    private final Line[] lines;

    public StaticLayout(String text, int width, int height, Line... lines) {
        this.text = text;
        this.width = width;
        this.height = height;
        this.lines = lines;
    }

    @Override public CharSequence getText() { return text; }
    @Override public int getWidth() { return width; }
    @Override public int getHeight() { return height; }
    @Override public int getLineCount() { return lines.length; }
    @Override public int getLineStart(int line) { return lines[line].start; }
    @Override public int getLineEnd(int line) { return lines[line].end; }
    @Override public int getLineVisibleEnd(int line) { return lines[line].end; }
    @Override public int getLineTop(int line) { return lines[line].top; }
    @Override public int getLineBaseline(int line) { return lines[line].bottom; }
    @Override public float getLineLeft(int line) { return lines[line].left; }
    @Override public float getLineRight(int line) { return lines[line].right; }
    @Override public int getParagraphDirection(int line) {
        return lines[line].rtl ? DIR_RIGHT_TO_LEFT : DIR_LEFT_TO_RIGHT;
    }

    @Override
    public int getLineForVertical(int y) {
        for (int i = 0; i < lines.length; i++) {
            if (y < lines[i].bottom) return i;
        }
        return Math.max(0, lines.length - 1);
    }

    @Override
    public int getLineForOffset(int offset) {
        for (int i = 0; i < lines.length; i++) {
            if (offset < lines[i].end) return i;
        }
        return Math.max(0, lines.length - 1);
    }

    @Override
    public boolean isRtlCharAt(int offset) {
        int line = getLineForOffset(offset);
        for (Run r : lines[line].runs) {
            if (offset >= r.start && offset < r.end) return r.rtl;
        }
        return lines[line].rtl;
    }

    @Override
    public float getPrimaryHorizontal(int offset) {
        int line = getLineForOffset(offset);
        Line l = lines[line];
        for (Run r : l.runs) {
            if (offset >= r.start && offset < r.end) {
                if (offset == r.start && r.rtl != l.rtl) {
                    // BiDi boundary: primary caret follows the paragraph direction.
                    return l.rtl ? r.right() : r.left;
                }
                return r.rtl ? r.right() - (offset - r.start) * 10f : r.left + (offset - r.start) * 10f;
            }
        }
        return l.rtl ? l.left : l.right;
    }

    @Override
    public int getOffsetForHorizontal(int line, float horiz) {
        Line l = lines[line];
        if (l.runs.length == 0) return l.start;
        Run chosen = l.runs[0];
        float bestDist = Float.POSITIVE_INFINITY;
        for (Run r : l.runs) {
            float d = horiz < r.left ? r.left - horiz : horiz > r.right() ? horiz - r.right() : 0f;
            if (d < bestDist) {
                bestDist = d;
                chosen = r;
            }
        }
        int best = chosen.start;
        float minDelta = Float.POSITIVE_INFINITY;
        // Carets stop at code point boundaries (skipping low surrogates), as Android's Layout does.
        for (int off = chosen.start; off <= chosen.end; ) {
            float x = chosen.rtl ? chosen.right() - (off - chosen.start) * 10f : chosen.left + (off - chosen.start) * 10f;
            float delta = Math.abs(horiz - x);
            if (delta < minDelta) {
                minDelta = delta;
                best = off;
            }
            if (off == chosen.end) break;
            off += Character.charCount(Character.codePointAt(text, off));
        }
        return best;
    }
}
