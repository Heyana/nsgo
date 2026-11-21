package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.DashPathEffect;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.PorterDuff;
import android.graphics.PorterDuffXfermode;
import android.graphics.RectF;
import android.text.Layout;
import android.text.Spanned;
import android.text.StaticLayout;
import android.view.MotionEvent;
import android.view.View;
import android.widget.FrameLayout;

/**
 * A text node: it draws the StaticLayout the engine laid out, where the
 * engine placed it. A group: the views of its attachments are its
 * children, drawn over the text where the engine placed them, and take
 * their own touches.
 */
final class TextNode extends FrameLayout {
    private static final PorterDuffXfermode TINT_MODE = new PorterDuffXfermode(PorterDuff.Mode.SRC_IN);

    private StaticLayout layout;
    private int x, y;
    /** The lines a clipped text shows, 0 for all. */
    private int clipLines;
    /** The text shadow behind glyphs and drawn decorations, or null for none. */
    private Text.Shadow shadow;
    /** Whether the current layout's ink reaches past this view's bounds. */
    private boolean overflows;
    /** Where the last touch went down, NaN once a press has used it. */
    private float downX = Float.NaN, downY;
    private final Paint backgroundPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final RectF backgroundRect = new RectF();
    private final Paint linePaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Path wave = new Path();
    private final RectF ink = new RectF();
    private final Paint tintPaint = new Paint();
    /** Whether the text selects, as driver.Selectable, and its highlight's color, 0 for the theme's. */
    int selectable;
    private int selectionColor;
    /** The UTF-16 range a selection highlights, and its outline. */
    private int selStart, selEnd;
    private final Path selection = new Path();
    private final Paint selectionPaint = new Paint(Paint.ANTI_ALIAS_FLAG);

    TextNode(Context context) {
        super(context);
        setWillNotDraw(false);
        setClipChildren(false);
        setClipToPadding(false);
    }

    /**
     * Draws layout at (x, y) from now on, only its first clipLines lines
     * when that is more than 0, and drops the layout it replaces.
     */
    void setLayout(StaticLayout layout, int x, int y, int clipLines) {
        setLayout(layout, x, y, clipLines, null);
    }

    /**
     * Draws layout at (x, y) with shadow (or null for none), only its first
     * clipLines lines when that is more than 0, and drops the layout it replaces.
     */
    void setLayout(StaticLayout layout, int x, int y, int clipLines, Text.Shadow shadow) {
        boolean hadOverflow = overflows;
        this.layout = layout;
        this.x = x;
        this.y = y;
        this.clipLines = clipLines;
        this.shadow = shadow != null && (shadow.color >>> 24) != 0 ? shadow : null;
        overflows = overflowsBox();
        invalidate();
        if ((hadOverflow || overflows) && getParent() instanceof View) {
            ((View) getParent()).invalidate();
        }
    }

    @Override
    protected void onSizeChanged(int w, int h, int oldw, int oldh) {
        super.onSizeChanged(w, h, oldw, oldh);
        overflows = overflowsBox();
    }

    StaticLayout layout() {
        return layout;
    }

    /** Where the layout is drawn in the view, in pixels. */
    int textX() {
        return x;
    }

    int textY() {
        return y;
    }

    void selectable(int selectable, int color) {
        this.selectable = selectable;
        selectionColor = color;
    }

    /** Highlights the UTF-16 range from start to end; none when they are equal. */
    void select(int start, int end) {
        if (start == selStart && end == selEnd) return;
        selStart = start;
        selEnd = end;
        invalidate();
    }

    @Override
    public boolean onTouchEvent(MotionEvent e) {
        int action = e.getActionMasked();
        if (action == MotionEvent.ACTION_DOWN) {
            downX = e.getX();
            downY = e.getY();
        } else if (action == MotionEvent.ACTION_CANCEL) {
            downX = Float.NaN;
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
        return indexAt(layout, clipLines, px, py);
    }

    /**
     * The UTF-8 byte offset of the character at (px, py) in layout, or -1
     * when the point misses the visible lines or no touch was recorded.
     */
    static int indexAt(Layout layout, int clipLines, float px, float py) {
        if (layout == null || Float.isNaN(px) || Float.isNaN(py) || py < 0 || py >= layout.getHeight()) return -1;
        int lines = layout.getLineCount();
        if (lines <= 0) return -1;
        int line = layout.getLineForVertical((int) py);
        if (line < 0 || line >= lines) return -1;
        if (clipLines > 0 && line >= clipLines) return -1;
        if (px < layout.getLineLeft(line) || px >= layout.getLineRight(line)) return -1;
        int lineStart = layout.getLineStart(line);
        int lineEnd = breakStart(layout, line);
        if (lineEnd <= lineStart) return -1;
        CharSequence text = layout.getText();
        // The offset is the nearest caret position; step back to the
        // character under the point.
        int offset = layout.getOffsetForHorizontal(line, px);
        if (offset >= lineEnd) {
            offset = lineEnd - 1;
        } else if (offset > lineStart) {
            int prev = offset - 1;
            if (prev > lineStart && Character.isLowSurrogate(text.charAt(prev))) prev--;
            boolean rtl = layout.isRtlCharAt(prev);
            float caret = layout.getPrimaryHorizontal(offset);
            if (rtl ? caret < px : caret > px) offset = prev;
        }
        if (offset < lineStart || offset >= lineEnd) return -1;
        if (offset > lineStart && Character.isLowSurrogate(text.charAt(offset))) offset--;
        return utf8Length(text, offset);
    }

    /**
     * Where a line's break begins: its end, or before the character that
     * ends it ("\r\n" as one). Carets and character hit tests stay before it.
     */
    static int breakStart(Layout l, int line) {
        int start = l.getLineStart(line), end = l.getLineEnd(line);
        if (end <= start) return end;
        CharSequence text = l.getText();
        char c = text.charAt(end - 1);
        if (c != '\n' && c != '\r' && c != '\u0085' && c != '\u2028' && c != '\u2029') return end;
        return c == '\n' && end - 1 > start && text.charAt(end - 2) == '\r' ? end - 2 : end - 1;
    }

    /** The UTF-8 length of the first end chars of s. */
    static int utf8Length(CharSequence s, int end) {
        int n = 0;
        for (int i = 0; i < end; i++) {
            char c = s.charAt(i);
            if (c < 0x80) {
                n += 1;
            } else if (c < 0x800) {
                n += 2;
            } else if (Character.isHighSurrogate(c) && i + 1 < end && Character.isLowSurrogate(s.charAt(i + 1))) {
                n += 4;
                i++;
            } else {
                n += 3;
            }
        }
        return n;
    }

    @Override
    protected void onDraw(Canvas canvas) {
        if (layout == null) return;
        // A bound color paints over the whole text, in a layer grown to hold
        // decorations and shadows that reach past the view's box.
        int tinted = -1;
        if (tint != 0) {
            inkBounds(ink);
            tinted = canvas.saveLayer(ink.left, ink.top, ink.right, ink.bottom, null);
        }
        canvas.save();
        canvas.translate(x, y);
        if (clipLines > 0 && layout.getLineCount() > clipLines) {
            canvas.clipRect(0, 0, layout.getWidth(), layout.getLineTop(clipLines));
        }
        drawBackgrounds(canvas);
        if (selEnd > selStart) {
            int color = selectionColor;
            if (color == 0) {
                android.util.TypedValue v = new android.util.TypedValue();
                color = !getContext().getTheme().resolveAttribute(android.R.attr.textColorHighlight, v, true) ? 0x6633b5e5
                        : v.resourceId != 0 ? getContext().getColor(v.resourceId) : v.data;
            }
            selectionPaint.setColor(color);
            layout.getSelectionPath(selStart, Math.min(selEnd, layout.getText().length()), selection);
            canvas.drawPath(selection, selectionPaint);
        }
        layout.draw(canvas);
        drawDecorations(canvas);
        canvas.restore();
        if (tinted >= 0) {
            tintPaint.setColor(tint);
            tintPaint.setXfermode(TINT_MODE);
            canvas.drawRect(ink, tintPaint);
            canvas.restoreToCount(tinted);
        }
    }

    /** The ARGB color an animated value paints the whole text in; 0 for none. */
    private int tint;

    void setTint(int argb) {
        if (argb == tint) return;
        tint = argb;
        invalidate();
        if (overflows && getParent() instanceof View) {
            ((View) getParent()).invalidate();
        }
    }

    /** Reports whether the laid-out ink extends past this view's bounds. */
    private boolean overflowsBox() {
        if (layout == null) return false;
        inkBounds(ink);
        return ink.left < 0 || ink.top < 0 || ink.right > getWidth() || ink.bottom > getHeight();
    }

    /**
     * Writes into out the view-local pixel bounds covering this view and any
     * text backgrounds, decoration lines and shadow reaching past its box.
     */
    void inkBounds(RectF out) {
        float left = 0, top = 0, right = getWidth(), bottom = getHeight();
        if (layout == null) {
            out.set(left, top, right, bottom);
            return;
        }
        boolean clipped = clipLines > 0 && layout.getLineCount() > clipLines;
        float textW = layout.getWidth();
        float textH = clipped ? layout.getLineTop(clipLines) : layout.getHeight();
        float drawnLeft = x, drawnTop = y, drawnRight = x + textW, drawnBottom = y + textH;
        float shadowLeft = drawnLeft, shadowTop = drawnTop, shadowRight = drawnRight, shadowBottom = drawnBottom;
        CharSequence text = layout.getText();
        if (text instanceof Spanned) {
            Spanned spanned = (Spanned) text;
            int lines = layout.getLineCount();
            for (Text.Background bg : spanned.getSpans(0, text.length(), Text.Background.class)) {
                int start = spanned.getSpanStart(bg), end = spanned.getSpanEnd(bg);
                for (int line = layout.getLineForOffset(start); line < lines; line++) {
                    int lineStart = layout.getLineStart(line), lineEnd = layout.getLineVisibleEnd(line);
                    if (lineStart >= end) break;
                    int a = Math.max(start, lineStart), b = Math.min(end, lineEnd);
                    if (a >= b) continue;
                    float x1 = layout.getPrimaryHorizontal(a);
                    float x2 = b < layout.getLineEnd(line) ? layout.getPrimaryHorizontal(b) : layout.getLineRight(line);
                    if (layout.getParagraphDirection(line) < 0 && b >= lineEnd) x2 = layout.getLineLeft(line);
                    float baseline = layout.getLineBaseline(line);
                    drawnLeft = Math.min(drawnLeft, x + Math.min(x1, x2));
                    drawnTop = Math.min(drawnTop, y + baseline + bg.top);
                    drawnRight = Math.max(drawnRight, x + Math.max(x1, x2));
                    drawnBottom = Math.max(drawnBottom, y + baseline + bg.bottom);
                }
            }
            for (Text.Decoration d : spanned.getSpans(0, text.length(), Text.Decoration.class)) {
                int start = spanned.getSpanStart(d), end = spanned.getSpanEnd(d);
                float vpad = decorationVerticalPad(d);
                float hpad = d.style == Protocol.DECORATION_WAVY ? d.thickness / 2 + 1 : 1;
                for (int line = layout.getLineForOffset(start); line < lines; line++) {
                    int lineStart = layout.getLineStart(line), lineEnd = layout.getLineEnd(line);
                    if (lineStart >= end) break;
                    int a = Math.max(start, lineStart), b = Math.min(end, lineEnd);
                    if (layout.getEllipsisCount(line) > 0) b = Math.min(b, lineStart + layout.getEllipsisStart(line));
                    if (a >= b) continue;
                    float x1 = layout.getPrimaryHorizontal(a);
                    float x2 = b < lineEnd ? layout.getPrimaryHorizontal(b) : layout.getLineRight(line);
                    if (layout.getParagraphDirection(line) < 0 && b >= lineEnd) x2 = layout.getLineLeft(line);
                    float dx0 = x + Math.min(x1, x2) - hpad, dx1 = x + Math.max(x1, x2) + hpad;
                    float baseline = layout.getLineBaseline(line);
                    if (d.underline) {
                        float dy = y + baseline + d.under;
                        drawnLeft = Math.min(drawnLeft, dx0);
                        drawnTop = Math.min(drawnTop, dy - vpad);
                        drawnRight = Math.max(drawnRight, dx1);
                        drawnBottom = Math.max(drawnBottom, dy + vpad);
                        shadowLeft = Math.min(shadowLeft, dx0);
                        shadowTop = Math.min(shadowTop, dy - vpad);
                        shadowRight = Math.max(shadowRight, dx1);
                        shadowBottom = Math.max(shadowBottom, dy + vpad);
                    }
                    if (d.strike) {
                        float dy = y + baseline + d.through;
                        drawnLeft = Math.min(drawnLeft, dx0);
                        drawnTop = Math.min(drawnTop, dy - vpad);
                        drawnRight = Math.max(drawnRight, dx1);
                        drawnBottom = Math.max(drawnBottom, dy + vpad);
                        shadowLeft = Math.min(shadowLeft, dx0);
                        shadowTop = Math.min(shadowTop, dy - vpad);
                        shadowRight = Math.max(shadowRight, dx1);
                        shadowBottom = Math.max(shadowBottom, dy + vpad);
                    }
                }
            }
        }
        if (shadow != null) {
            float blur = 2 * Math.max(shadow.radius, 0) + 1;
            drawnLeft = Math.min(drawnLeft, shadowLeft + shadow.dx - blur);
            drawnTop = Math.min(drawnTop, shadowTop + shadow.dy - blur);
            drawnRight = Math.max(drawnRight, shadowRight + shadow.dx + blur);
            drawnBottom = Math.max(drawnBottom, shadowBottom + shadow.dy + blur);
        }
        if (clipped) {
            drawnLeft = Math.max(drawnLeft, x);
            drawnTop = Math.max(drawnTop, y);
            drawnRight = Math.min(drawnRight, x + textW);
            drawnBottom = Math.min(drawnBottom, y + textH);
        }
        if (drawnRight > drawnLeft && drawnBottom > drawnTop) {
            left = Math.min(left, drawnLeft);
            top = Math.min(top, drawnTop);
            right = Math.max(right, drawnRight);
            bottom = Math.max(bottom, drawnBottom);
        }
        out.set((float) Math.floor(left), (float) Math.floor(top), (float) Math.ceil(right), (float) Math.ceil(bottom));
    }

    /** Half the vertical extent of decoration d around its center, with AA margin. */
    private static float decorationVerticalPad(Text.Decoration d) {
        float t = d.thickness;
        switch (d.style) {
            case Protocol.DECORATION_DOUBLE:
                return 1.5f * t + 1;
            case Protocol.DECORATION_WAVY:
                return Math.max(t, 1) + t / 2 + 1;
            default:
                return t / 2 + 1;
        }
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
        if (shadow != null) {
            linePaint.setShadowLayer(Math.max(0.01f, shadow.radius), shadow.dx, shadow.dy, shadow.color);
        } else {
            linePaint.clearShadowLayer();
        }
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
     * Fills the spans' backgrounds: a box on each line a span reaches, around its visible characters (trailing spaces left out), within its font's ascent and descent, rounded at most to half its painted height.
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
                backgroundRect.set(Math.min(x1, x2), baseline + bg.top, Math.max(x1, x2), baseline + bg.bottom);
                canvas.drawRoundRect(backgroundRect, bg.radius, bg.radius, backgroundPaint);
            }
        }
    }
}
