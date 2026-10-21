package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Canvas;
import android.text.StaticLayout;
import android.text.TextPaint;
import android.widget.FrameLayout;

/** A node that draws its text with the same StaticLayout the engine measured with. */
final class TextNode extends FrameLayout {
    private String text = "";
    private TextPaint paint;
    private int padLeft, padTop, padRight;
    private StaticLayout layout;

    TextNode(Context context) {
        super(context);
        setWillNotDraw(false);
    }

    void setText(String text, TextPaint paint, int padLeft, int padTop, int padRight) {
        this.text = text;
        this.paint = paint;
        this.padLeft = padLeft;
        this.padTop = padTop;
        this.padRight = padRight;
        layout = null;
        setContentDescription(text);
        invalidate();
    }

    @Override
    protected void onDraw(Canvas canvas) {
        if (paint == null) return;
        // Wrap at the width the engine laid the node out at.
        int width = getWidth() - padLeft - padRight;
        if (layout == null || layout.getWidth() != width) {
            layout = Text.layout(text, paint, width);
        }
        canvas.save();
        canvas.translate(padLeft, padTop);
        layout.draw(canvas);
        canvas.restore();
    }
}
