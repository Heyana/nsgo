package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Canvas;
import android.text.Layout;
import android.text.StaticLayout;
import android.view.MotionEvent;
import android.view.View;

/** A text node: it draws the StaticLayout the engine laid out, where the engine placed it. */
final class TextNode extends View {
    private StaticLayout layout;
    private int x, y;
    /** Where the last touch went down, NaN once a press has used it. */
    private float downX = Float.NaN, downY;

    TextNode(Context context) {
        super(context);
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
        layout.draw(canvas);
        canvas.restore();
    }
}
