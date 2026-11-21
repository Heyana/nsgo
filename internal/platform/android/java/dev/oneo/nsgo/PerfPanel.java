package dev.oneo.nsgo;

import android.graphics.Canvas;
import android.graphics.ColorFilter;
import android.graphics.Paint;
import android.graphics.PixelFormat;
import android.graphics.Rect;
import android.graphics.Typeface;
import android.graphics.drawable.Drawable;

import java.io.DataInputStream;
import java.io.IOException;

/** The performance readings and trace in the primary view's noninteractive overlay. */
final class PerfPanel extends Drawable {
    record Bar(float x, float y, float width, float height, int color) {}
    record Reading(String[] lines, float width, float margin, float pad, float graph, float font,
                   float radius, int background, int text, Bar[] bars) {}

    private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private Reading reading;
    private float density;

    PerfPanel() {
        paint.setTypeface(Typeface.MONOSPACE);
    }

    static Reading read(DataInputStream in) throws IOException {
        String[] lines = Protocol.readString(in).split("\\n");
        float width = in.readFloat(), margin = in.readFloat(), pad = in.readFloat();
        float graph = in.readFloat(), font = in.readFloat(), radius = in.readFloat();
        int background = Text.argb(in.readInt()), text = Text.argb(in.readInt());
        int count = in.readInt();
        if (count < 0 || count > in.available() / 20) throw new IOException("invalid performance trace size");
        Bar[] bars = new Bar[count];
        for (int i = 0; i < count; i++) {
            bars[i] = new Bar(in.readFloat(), in.readFloat(), in.readFloat(), in.readFloat(), Text.argb(in.readInt()));
        }
        return new Reading(lines, width, margin, pad, graph, font, radius, background, text, bars);
    }

    void update(Reading next, int viewWidth, int safeTop, int safeLeft, int safeRight, float density) {
        reading = next;
        this.density = density;
        paint.setTextSize(next.font());
        int margin = Math.round(next.margin() * density);
        int width = Math.min(Math.round(next.width() * density), viewWidth - safeLeft - safeRight - 2 * margin);
        int height = Math.round((3 * next.pad() + next.graph() + next.lines().length * paint.getFontSpacing()) * density);
        int right = viewWidth - safeRight - margin, top = safeTop + margin;
        setBounds(right - Math.max(0, width), top, right, top + height);
        invalidateSelf();
    }

    @Override
    public void draw(Canvas canvas) {
        Reading data = reading;
        Rect bounds = getBounds();
        if (data == null || bounds.isEmpty()) return;
        canvas.save();
        canvas.clipRect(bounds);
        canvas.translate(bounds.left, bounds.top);
        canvas.scale(density, density);
        float width = bounds.width() / density, height = bounds.height() / density;
        paint.setColor(data.background());
        canvas.drawRoundRect(0, 0, width, height, data.radius(), data.radius(), paint);
        float pad = data.pad(), traceWidth = Math.max(0, width - 2 * pad);
        for (Bar bar : data.bars()) {
            float left = pad + bar.x() * traceWidth, top = pad + bar.y();
            paint.setColor(bar.color());
            canvas.drawRect(left, top, left + bar.width() * traceWidth, top + bar.height(), paint);
        }
        paint.setColor(data.text());
        float baseline = 2 * pad + data.graph() - paint.ascent();
        for (String line : data.lines()) {
            canvas.drawText(line, pad, baseline, paint);
            baseline += paint.getFontSpacing();
        }
        canvas.restore();
    }

    @Override public void setAlpha(int alpha) {}
    @Override public void setColorFilter(ColorFilter filter) {}
    @Override public int getOpacity() { return PixelFormat.TRANSLUCENT; }
}
