package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Paint;
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
    /** Where the last touch went down, NaN once a press has used it. */
    private float downX = Float.NaN, downY;
    private final Paint backgroundPaint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final RectF backgroundRect = new RectF();

    TextNode(Context context) {
        super(context);
        setWillNotDraw(false);
        setClipChildren(false);
    }

    /** Draws layout at (x, y) from now on and drops the layout it replaces. */
    void setLayout(StaticLayout layout, int x, int y) {
        this.layout = layout;
        this.x = x;
        this.y = y;
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
        drawBackgrounds(canvas);
        layout.draw(canvas);
        canvas.restore();
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
