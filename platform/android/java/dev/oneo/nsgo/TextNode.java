package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Canvas;
import android.text.StaticLayout;
import android.view.View;

/** A text node: it draws the StaticLayout the engine laid out, where the engine placed it. */
final class TextNode extends View {
    private StaticLayout layout;
    private int x, y;

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
    protected void onDraw(Canvas canvas) {
        if (layout == null) return;
        canvas.save();
        canvas.translate(x, y);
        layout.draw(canvas);
        canvas.restore();
    }
}
