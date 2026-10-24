package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.DashPathEffect;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.RectF;
import android.text.Layout;
import android.text.Spanned;
import android.text.StaticLayout;
import android.view.MotionEvent;
import android.widget.FrameLayout;

/**
 * A text node: it draws the StaticLayout the engine laid out, where the
 * engine placed it. A group: the views of its attachments are its
 * children, drawn over the text where the engine placed them, and take
 * their own touches.
 */
final class TextNode extends FrameLayout {
    private StaticLayout layout;
    private int x, y;
    /** The lines a clipped text shows, 0 for all. */
    private int clipLines;
    /** Where the last touch went down, NaN once a press has used it. */
    private float downX = Float.NaN, downY;
    private final Paint backgroundPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final RectF backgroundRect = new RectF();
    private final Paint linePaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Path wave = new Path();

    TextNode(Context context) {
        super(context);
        setWillNotDraw(false);
        setClipChildren(false);
    }

    /**
     * Draws layout at (x, y) from now on, only its first clipLines lines
     * when that is more than 0, and drops the layout it replaces.
     */
    void setLayout(StaticLayout layout, int x, int y, int clipLines) {
        this.layout = layout;
        this.x = x;
        this.y = y;
        this.clipLines = clipLines;
        invalidate();
    }

    @Override
    public boolean onTouchEvent(MotionEvent e) {
        if (e.getActionMasked() == MotionEvent.ACTION_DOWN) {
            downX = e.getX();
            downY = e.getY();
        }
        return super.onTouchEvent(e);
    }

    /**
     * The UTF-8 byte offset of the character the press went down on, or -1
     * when it missed the text or came without a touch, as from TalkBack.
     */
    int pressedIndex() {
        float px = downX - x, py = downY - y;
        downX = Float.NaN;
        if (layout == null || Float.isNaN(px) || py < 0 || py >= layout.getHeight()) return -1;
        int line = layout.getLineForVertical((int) py);
        if (px < layout.getLineLeft(line) || px >= layout.getLineRight(line)) return -1;
        // The offset is the nearest caret position; step back to the
        // character under the point.
        int offset = layout.getOffsetForHorizontal(line, px);
        if (layout.getParagraphDirection(line) == Layout.DIR_LEFT_TO_RIGHT
                && offset > layout.getLineStart(line) && layout.getPrimaryHorizontal(offset) > px) {
            offset--;
        }
        CharSequence text = layout.getText();
        if (offset < 0 || offset >= text.length()) return -1;
        if (offset > 0 && Character.isLowSurrogate(text.charAt(offset))) offset--;
        return Text.utf8Length(text, offset);
    }

    @Override
    protected void onDraw(Canvas canvas) {
        if (layout == null) return;
        canvas.save();
        canvas.translate(x, y);
        if (clipLines > 0 && layout.getLineCount() > clipLines) {
            canvas.clipRect(0, 0, layout.getWidth(), layout.getLineTop(clipLines));
        }
        drawBackgrounds(canvas);
        layout.draw(canvas);
        drawDecorations(canvas);
        canvas.restore();
    }

    /**
     * Draws the decorations a paint cannot, in their styles and colors: a
     * line on each line a span reaches, under or through its characters.
     */
    private void drawDecorations(Canvas canvas) {
        CharSequence text = layout.getText();
        if (!(text instanceof Spanned)) return;
        Spanned spanned = (Spanned) text;
        int lines = layout.getLineCount();
        for (Text.Decoration d : spanned.getSpans(0, text.length(), Text.Decoration.class)) {
            int start = spanned.getSpanStart(d), end = spanned.getSpanEnd(d);
            linePaint.setColor(d.color);
            for (int line = layout.getLineForOffset(start); line < lines; line++) {
                int lineStart = layout.getLineStart(line), lineEnd = layout.getLineEnd(line);
                if (lineStart >= end) break;
                int a = Math.max(start, lineStart), b = Math.min(end, lineEnd);
                if (layout.getEllipsisCount(line) > 0) b = Math.min(b, lineStart + layout.getEllipsisStart(line));
                if (a >= b) continue;
                float x1 = layout.getPrimaryHorizontal(a);
                float x2 = b < lineEnd ? layout.getPrimaryHorizontal(b) : layout.getLineRight(line);
                if (layout.getParagraphDirection(line) < 0 && b >= lineEnd) x2 = layout.getLineLeft(line);
                float baseline = layout.getLineBaseline(line);
                if (d.underline) drawLine(canvas, d, Math.min(x1, x2), Math.max(x1, x2), baseline + d.under);
                if (d.strike) drawLine(canvas, d, Math.min(x1, x2), Math.max(x1, x2), baseline + d.through);
            }
        }
    }

    /** Draws one decoration line from x1 to x2 centered on y, in d's style. */
    private void drawLine(Canvas canvas, Text.Decoration d, float x1, float x2, float y) {
        float t = d.thickness;
        linePaint.setPathEffect(null);
        switch (d.style) {
            case Protocol.DECORATION_DOUBLE:
                linePaint.setStyle(Paint.Style.FILL);
                canvas.drawRect(x1, y - 1.5f * t, x2, y - 0.5f * t, linePaint);
                canvas.drawRect(x1, y + 0.5f * t, x2, y + 1.5f * t, linePaint);
                break;
            case Protocol.DECORATION_DOTTED:
            case Protocol.DECORATION_DASHED: {
                boolean dotted = d.style == Protocol.DECORATION_DOTTED;
                linePaint.setStyle(Paint.Style.STROKE);
                linePaint.setStrokeWidth(t);
                linePaint.setPathEffect(new DashPathEffect(dotted ? new float[] {t, t} : new float[] {3 * t, 2 * t}, 0));
                canvas.drawLine(x1, y, x2, y, linePaint);
                break;
            }
            case Protocol.DECORATION_WAVY: {
                float amp = Math.max(t, 1), step = 2 * amp;
                wave.rewind();
                wave.moveTo(x1, y);
                boolean up = true;
                for (float x = x1; x < x2; x += step) {
                    wave.quadTo(x + step / 2, y + (up ? -2 : 2) * amp, Math.min(x + step, x2), y);
                    up = !up;
                }
                linePaint.setStyle(Paint.Style.STROKE);
                linePaint.setStrokeWidth(t);
                canvas.drawPath(wave, linePaint);
                break;
            }
            default:
                linePaint.setStyle(Paint.Style.FILL);
                canvas.drawRect(x1, y - t / 2, x2, y + t / 2, linePaint);
        }
    }

    /**
     * Fills the spans' backgrounds: a box on each line a span reaches,
     * around its visible characters (trailing spaces left out), from its
     * font's ascent to its descent, rounded at most to half its height.
     */
    private void drawBackgrounds(Canvas canvas) {
        CharSequence text = layout.getText();
        if (!(text instanceof Spanned)) return;
        Spanned spanned = (Spanned) text;
        Text.Background[] backgrounds = spanned.getSpans(0, text.length(), Text.Background.class);
        int lines = layout.getLineCount();
        for (Text.Background bg : backgrounds) {
            int start = spanned.getSpanStart(bg), end = spanned.getSpanEnd(bg);
            backgroundPaint.setColor(bg.color);
            for (int line = layout.getLineForOffset(start); line < lines; line++) {
                int lineStart = layout.getLineStart(line), lineEnd = layout.getLineVisibleEnd(line);
                if (lineStart >= end) break;
                int a = Math.max(start, lineStart), b = Math.min(end, lineEnd);
                if (a >= b) continue;
                float x1 = layout.getPrimaryHorizontal(a);
                float x2 = b < layout.getLineEnd(line) ? layout.getPrimaryHorizontal(b) : layout.getLineRight(line);
                if (layout.getParagraphDirection(line) < 0 && b >= lineEnd) x2 = layout.getLineLeft(line);
                float baseline = layout.getLineBaseline(line);
                backgroundRect.set(Math.min(x1, x2), baseline - bg.ascent, Math.max(x1, x2), baseline + bg.descent);
                float r = Math.min(bg.radius, backgroundRect.height() / 2);
                canvas.drawRoundRect(backgroundRect, r, r, backgroundPaint);
            }
        }
    }
}
