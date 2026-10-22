package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.PorterDuff;
import android.graphics.RectF;
import android.graphics.Typeface;
import android.view.View;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.util.ArrayDeque;

/**
 * A canvas node: it plays the latest drawing the engine recorded, as
 * android.go's writeDrawing writes it, in points over its background. The
 * ops play into a layer of their own, so Clear leaves the background.
 * Paths stay in the coordinates they were built in and take the transform
 * current when they are filled, stroked or clipped, which differs from the
 * web's canvas only when the transform changes in between.
 */
final class CanvasNode extends View {
    private byte[] ops;

    CanvasNode(Context context) {
        super(context);
    }

    void setOps(byte[] ops) {
        this.ops = ops;
        invalidate();
    }

    @Override
    protected void onDraw(Canvas canvas) {
        if (ops == null || ops.length == 0) return;
        int layer = canvas.saveLayer(0, 0, getWidth(), getHeight(), null);
        float density = getResources().getDisplayMetrics().density;
        canvas.scale(density, density);
        new Player(canvas).play(ByteBuffer.wrap(ops));
        canvas.restoreToCount(layer);
    }

    /** The settings Save keeps and Restore brings back, besides the canvas's transform and clip. */
    private static final class State {
        int fill = Color.BLACK, stroke = Color.BLACK;
        float lineWidth = 1, alpha = 1;
        Paint.Cap cap = Paint.Cap.BUTT;
        Paint.Join join = Paint.Join.MITER;

        State copy() {
            State s = new State();
            s.fill = fill;
            s.stroke = stroke;
            s.lineWidth = lineWidth;
            s.alpha = alpha;
            s.cap = cap;
            s.join = join;
            return s;
        }
    }

    private static final class Player {
        private final Canvas canvas;
        private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
        private final ArrayDeque<State> saved = new ArrayDeque<>();
        private final RectF oval = new RectF();
        private Path path = new Path();
        private State state = new State();

        Player(Canvas canvas) {
            this.canvas = canvas;
        }

        void play(ByteBuffer b) {
            while (b.hasRemaining()) {
                switch (b.get()) {
                    case Protocol.DRAW_SAVE:
                        canvas.save();
                        saved.push(state.copy());
                        break;
                    case Protocol.DRAW_RESTORE:
                        if (saved.isEmpty()) break;
                        canvas.restore();
                        state = saved.pop();
                        break;
                    case Protocol.DRAW_TRANSLATE:
                        canvas.translate(b.getFloat(), b.getFloat());
                        break;
                    case Protocol.DRAW_SCALE:
                        canvas.scale(b.getFloat(), b.getFloat());
                        break;
                    case Protocol.DRAW_ROTATE:
                        canvas.rotate((float) Math.toDegrees(b.getFloat()));
                        break;
                    case Protocol.DRAW_BEGIN_PATH:
                        path = new Path();
                        break;
                    case Protocol.DRAW_MOVE_TO:
                        path.moveTo(b.getFloat(), b.getFloat());
                        break;
                    case Protocol.DRAW_LINE_TO:
                        path.lineTo(b.getFloat(), b.getFloat());
                        break;
                    case Protocol.DRAW_QUAD_TO:
                        path.quadTo(b.getFloat(), b.getFloat(), b.getFloat(), b.getFloat());
                        break;
                    case Protocol.DRAW_CUBIC_TO:
                        path.cubicTo(b.getFloat(), b.getFloat(), b.getFloat(), b.getFloat(),
                                b.getFloat(), b.getFloat());
                        break;
                    case Protocol.DRAW_ARC:
                        arc(b.getFloat(), b.getFloat(), b.getFloat(), b.getFloat(), b.getFloat(), b.get() != 0);
                        break;
                    case Protocol.DRAW_RECT: {
                        float x = b.getFloat(), y = b.getFloat(), w = b.getFloat(), h = b.getFloat();
                        path.addRect(x, y, x + w, y + h, Path.Direction.CW);
                        break;
                    }
                    case Protocol.DRAW_ROUND_RECT: {
                        float x = b.getFloat(), y = b.getFloat(), w = b.getFloat(), h = b.getFloat(), r = b.getFloat();
                        path.addRoundRect(x, y, x + w, y + h, r, r, Path.Direction.CW);
                        break;
                    }
                    case Protocol.DRAW_ELLIPSE: {
                        float x = b.getFloat(), y = b.getFloat(), rx = b.getFloat(), ry = b.getFloat();
                        path.addOval(x - rx, y - ry, x + rx, y + ry, Path.Direction.CW);
                        break;
                    }
                    case Protocol.DRAW_CLOSE_PATH:
                        path.close();
                        break;
                    case Protocol.DRAW_FILL:
                        fill(state.fill);
                        canvas.drawPath(path, paint);
                        break;
                    case Protocol.DRAW_STROKE:
                        paint.reset();
                        paint.setAntiAlias(true);
                        paint.setStyle(Paint.Style.STROKE);
                        paint.setColor(fade(state.stroke));
                        paint.setStrokeWidth(state.lineWidth);
                        paint.setStrokeCap(state.cap);
                        paint.setStrokeJoin(state.join);
                        canvas.drawPath(path, paint);
                        break;
                    case Protocol.DRAW_CLIP:
                        canvas.clipPath(path);
                        break;
                    case Protocol.DRAW_FILL_COLOR:
                        state.fill = Text.argb(b.getInt());
                        break;
                    case Protocol.DRAW_STROKE_COLOR:
                        state.stroke = Text.argb(b.getInt());
                        break;
                    case Protocol.DRAW_LINE_WIDTH:
                        state.lineWidth = b.getFloat();
                        break;
                    case Protocol.DRAW_LINE_CAP: {
                        int cap = b.get();
                        state.cap = cap == Protocol.CAP_ROUND ? Paint.Cap.ROUND
                                : cap == Protocol.CAP_SQUARE ? Paint.Cap.SQUARE : Paint.Cap.BUTT;
                        break;
                    }
                    case Protocol.DRAW_LINE_JOIN: {
                        int join = b.get();
                        state.join = join == Protocol.JOIN_ROUND ? Paint.Join.ROUND
                                : join == Protocol.JOIN_BEVEL ? Paint.Join.BEVEL : Paint.Join.MITER;
                        break;
                    }
                    case Protocol.DRAW_ALPHA:
                        state.alpha = Math.max(0, Math.min(1, b.getFloat()));
                        break;
                    case Protocol.DRAW_FILL_TEXT: {
                        String text = string(b);
                        float x = b.getFloat(), y = b.getFloat();
                        String family = string(b);
                        float size = b.getFloat();
                        int weight = b.getShort() & 0xffff;
                        boolean italic = b.get() != 0;
                        fill(state.fill);
                        Typeface base = family.isEmpty() ? Typeface.DEFAULT : Typeface.create(family, Typeface.NORMAL);
                        paint.setTypeface(Typeface.create(base, Math.max(1, Math.min(weight, 1000)), italic));
                        paint.setTextSize(size);
                        canvas.drawText(text, x, y, paint);
                        break;
                    }
                    case Protocol.DRAW_CLEAR: {
                        float x = b.getFloat(), y = b.getFloat(), w = b.getFloat(), h = b.getFloat();
                        canvas.save();
                        canvas.clipRect(x, y, x + w, y + h);
                        canvas.drawColor(Color.TRANSPARENT, PorterDuff.Mode.CLEAR);
                        canvas.restore();
                        break;
                    }
                    default:
                        return; // an op this runtime does not know: its fields are unknown too
                }
            }
        }

        private void fill(int color) {
            paint.reset();
            paint.setAntiAlias(true);
            paint.setStyle(Paint.Style.FILL);
            paint.setColor(fade(color));
        }

        /** The color with the global alpha applied. */
        private int fade(int color) {
            int alpha = Math.round(Color.alpha(color) * state.alpha);
            return (color & 0x00ffffff) | (alpha << 24);
        }

        /**
         * Adds an arc as the web's canvas does: a line to its start, then
         * clockwise from a0 to a1 unless ccw, a full circle at most.
         */
        private void arc(float x, float y, float r, float a0, float a1, boolean ccw) {
            double tau = 2 * Math.PI;
            double d = ccw ? a0 - a1 : a1 - a0;
            double sweep = d >= tau ? tau : ((d % tau) + tau) % tau;
            if (ccw) sweep = -sweep;
            oval.set(x - r, y - r, x + r, y + r);
            float start = (float) Math.toDegrees(a0), degrees = (float) Math.toDegrees(sweep);
            // Path.arcTo takes sweeps below a full turn; draw a circle in halves.
            if (Math.abs(degrees) >= 360) {
                path.arcTo(oval, start, degrees / 2, false);
                path.arcTo(oval, start + degrees / 2, degrees / 2, false);
            } else {
                path.arcTo(oval, start, degrees, false);
            }
        }

        private static String string(ByteBuffer b) {
            byte[] bytes = new byte[b.getInt()];
            b.get(bytes);
            return new String(bytes, StandardCharsets.UTF_8);
        }
    }
}
