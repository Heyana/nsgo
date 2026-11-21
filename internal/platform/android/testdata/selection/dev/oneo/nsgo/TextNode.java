package dev.oneo.nsgo;

import android.content.Context;
import android.text.Layout;
import android.view.View;

/** A laid-out text box that records the highlight Selection applies. */
final class TextNode extends View {
    private final Layout text;
    int selectable = Protocol.SELECTABLE_ON;
    int from, to;

    TextNode(Context context, String text, int x, int y, int width, int height) {
        super(context, x, y);
        this.text = new Layout(text, width, height);
    }

    Layout layout() { return text; }

    int textX() { return 0; }

    int textY() { return 0; }

    void select(int from, int to) {
        this.from = from;
        this.to = to;
    }
}
