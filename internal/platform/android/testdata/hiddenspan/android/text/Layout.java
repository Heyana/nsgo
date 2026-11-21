package android.text;

import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.Path;
import android.text.style.ReplacementSpan;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

public class Layout {
    public static final int DIR_LEFT_TO_RIGHT = 1;
    public static final int DIR_RIGHT_TO_LEFT = -1;
    public static final int JUSTIFICATION_MODE_INTER_WORD = 1;

    public enum Alignment { ALIGN_NORMAL, ALIGN_OPPOSITE, ALIGN_CENTER }

    protected final CharSequence text;
    protected final TextPaint paint;
    protected final int width;
    protected final Alignment align;
    protected final int dir;
    protected final int[] lineStarts;
    protected final int[] lineEnds;
    protected final int[] ellipsisStarts;
    protected final int[] ellipsisCounts;
    protected final int lineHeight = 20;
    protected final int baselineOffset = 15;

    protected Layout(CharSequence text, TextPaint paint, int width, Alignment align, int dir, int maxLines, TextUtils.TruncateAt ellipsize) {
        this.text = text;
        this.paint = paint;
        this.width = width;
        this.align = align;
        this.dir = dir;
        List<Integer> starts = new ArrayList<>();
        List<Integer> ends = new ArrayList<>();
        int len = text.length();
        if (len == 0) {
            starts.add(0);
            ends.add(0);
        } else {
            int pos = 0;
            while (pos < len) {
                int nl = -1;
                for (int i = pos; i < len; i++) {
                    if (text.charAt(i) == '\n') { nl = i; break; }
                }
                int end = nl >= 0 ? nl + 1 : len;
                starts.add(pos);
                ends.add(end);
                pos = end;
            }
        }
        int count = starts.size();
        if (maxLines > 0 && count > maxLines) count = maxLines;
        this.lineStarts = new int[count];
        this.lineEnds = new int[count];
        this.ellipsisStarts = new int[count];
        this.ellipsisCounts = new int[count];
        for (int i = 0; i < count; i++) {
            lineStarts[i] = starts.get(i);
            lineEnds[i] = ends.get(i);
        }
        if (ellipsize == TextUtils.TruncateAt.END && count > 0) {
            int last = count - 1;
            float w = measureSlice(lineStarts[last], lineEnds[last]);
            if (w > width && lineEnds[last] - lineStarts[last] > 2) {
                int keep = Math.max(1, (int) (width / 10) - 1);
                if (keep < lineEnds[last] - lineStarts[last]) {
                    ellipsisStarts[last] = keep;
                    ellipsisCounts[last] = (lineEnds[last] - lineStarts[last]) - keep;
                }
            }
        }
    }

    public static float getDesiredWidth(CharSequence source, TextPaint paint) {
        return source.length() * 10f;
    }

    protected float measureSlice(int from, int to) {
        if (!(text instanceof Spanned)) return paint.measureText(text, from, to);
        Spanned sp = (Spanned) text;
        ReplacementSpan[] reps = sp.getSpans(from, to, ReplacementSpan.class);
        Arrays.sort(reps, (a, b) -> sp.getSpanStart(a) - sp.getSpanStart(b));
        float total = 0;
        int at = from;
        for (ReplacementSpan r : reps) {
            int rs = Math.max(from, sp.getSpanStart(r)), re = Math.min(to, sp.getSpanEnd(r));
            if (re <= at) continue;
            if (rs > at) total += paint.measureText(text, at, rs);
            total += r.getSize(paint, text, rs, re, null);
            at = re;
        }
        if (at < to) total += paint.measureText(text, at, to);
        return total;
    }

    public CharSequence getText() { return text; }
    public TextPaint getPaint() { return paint; }
    public int getWidth() { return width; }
    public int getHeight() { return lineStarts.length * lineHeight; }
    public int getLineCount() { return lineStarts.length; }
    public int getLineStart(int line) { return lineStarts[line]; }
    public int getLineEnd(int line) { return lineEnds[line]; }
    public int getLineVisibleEnd(int line) {
        int e = lineEnds[line];
        while (e > lineStarts[line] && (text.charAt(e - 1) == '\n' || text.charAt(e - 1) == ' ')) e--;
        return e;
    }
    public int getLineTop(int line) { return line * lineHeight; }
    public int getLineBottom(int line) { return (line + 1) * lineHeight; }
    public int getLineBaseline(int line) { return line * lineHeight + baselineOffset; }
    public int getLineAscent(int line) { return -baselineOffset; }
    public int getLineDescent(int line) { return lineHeight - baselineOffset; }
    public float getLineWidth(int line) { return measureSlice(lineStarts[line], getLineVisibleEnd(line)); }
    public float getLineLeft(int line) {
        if (align == Alignment.ALIGN_CENTER) return (width - getLineWidth(line)) / 2f;
        if (align == Alignment.ALIGN_OPPOSITE) return width - getLineWidth(line);
        return 0;
    }
    public float getLineRight(int line) { return getLineLeft(line) + getLineWidth(line); }
    public int getParagraphDirection(int line) { return dir; }
    public int getEllipsisStart(int line) { return ellipsisStarts[line]; }
    public int getEllipsisCount(int line) { return ellipsisCounts[line]; }
    public boolean isRtlCharAt(int offset) { return dir < 0; }
    public int getLineForOffset(int offset) {
        for (int i = 0; i < lineStarts.length; i++) {
            if (offset < lineEnds[i] || i == lineStarts.length - 1) return i;
        }
        return 0;
    }
    public int getLineForVertical(int y) {
        return Math.max(0, Math.min(lineStarts.length - 1, y / lineHeight));
    }
    public float getPrimaryHorizontal(int offset) {
        int line = getLineForOffset(offset);
        return getLineLeft(line) + measureSlice(lineStarts[line], Math.min(offset, lineEnds[line]));
    }
    public float getSecondaryHorizontal(int offset) { return getPrimaryHorizontal(offset); }
    public int getOffsetForHorizontal(int line, float x) {
        float rel = Math.max(0, x - getLineLeft(line));
        return Math.min(lineEnds[line], lineStarts[line] + Math.round(rel / 10f));
    }
    public void getSelectionPath(int start, int end, Path dest) {}

    public void draw(Canvas canvas) {
        for (int i = 0; i < lineStarts.length; i++) {
            float x = getLineLeft(i);
            int top = getLineTop(i);
            int baseline = getLineBaseline(i);
            int bottom = getLineBottom(i);
            int from = lineStarts[i], to = lineEnds[i];
            while (to > from && (text.charAt(to - 1) == '\n' || text.charAt(to - 1) == '\r')) to--;
            if (!(text instanceof Spanned)) {
                if (to > from) canvas.recordGlyphs(text.subSequence(from, to).toString(), x, baseline, paint.hasShadow());
                continue;
            }
            Spanned sp = (Spanned) text;
            ReplacementSpan[] reps = sp.getSpans(from, to, ReplacementSpan.class);
            Arrays.sort(reps, (a, b) -> sp.getSpanStart(a) - sp.getSpanStart(b));
            int at = from;
            for (ReplacementSpan r : reps) {
                int rs = Math.max(from, sp.getSpanStart(r)), re = Math.min(to, sp.getSpanEnd(r));
                if (re <= at) continue;
                if (rs > at) {
                    String s = text.subSequence(at, rs).toString();
                    canvas.recordGlyphs(s, x, baseline, paint.hasShadow());
                    x += paint.measureText(text, at, rs);
                }
                int w = r.getSize(paint, text, rs, re, null);
                r.draw(canvas, text, rs, re, x, top, baseline, bottom, paint);
                x += w;
                at = re;
            }
            if (at < to) {
                String s = text.subSequence(at, to).toString();
                canvas.recordGlyphs(s, x, baseline, paint.hasShadow());
            }
        }
    }
}
