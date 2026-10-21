package dev.oneo.nsgo;

import android.graphics.Typeface;
import android.text.Layout;
import android.text.StaticLayout;
import android.text.TextPaint;

/** Text measured on the reader thread and drawn on the UI thread, both with StaticLayout. */
final class Text {
    private Text() {}

    static TextPaint paint(float sizePx, int weight, int argb) {
        TextPaint paint = new TextPaint(TextPaint.ANTI_ALIAS_FLAG);
        paint.setTextSize(sizePx);
        paint.setTypeface(Typeface.create(Typeface.DEFAULT, weight, false));
        paint.setColor(argb);
        return paint;
    }

    /** Lays text out at maxWidthPx, or on one line per paragraph when it is negative. */
    static StaticLayout layout(String text, TextPaint paint, float maxWidthPx) {
        int width = maxWidthPx < 0
                ? (int) Math.ceil(Layout.getDesiredWidth(text, paint))
                : (int) Math.ceil(maxWidthPx);
        return StaticLayout.Builder.obtain(text, 0, text.length(), paint, Math.max(width, 1))
                .setIncludePad(false)
                .build();
    }

    /** The size of text in pixels, the widest line and the full height. */
    static float[] measure(String text, float sizePx, int weight, float maxWidthPx) {
        StaticLayout layout = layout(text, paint(sizePx, weight, 0), maxWidthPx);
        float width = 0;
        for (int i = 0; i < layout.getLineCount(); i++) {
            width = Math.max(width, layout.getLineWidth(i));
        }
        if (text.isEmpty()) width = 0;
        return new float[] {(float) Math.ceil(width), layout.getHeight()};
    }

    static int argb(int rgba) {
        return (rgba >>> 8) | (rgba << 24);
    }
}
