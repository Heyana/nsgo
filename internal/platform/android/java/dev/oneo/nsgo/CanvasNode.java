package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.LinearGradient;
import android.graphics.Matrix;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.PorterDuff;
import android.graphics.RadialGradient;
import android.graphics.RectF;
import android.graphics.Shader;
import android.graphics.Typeface;
import android.os.Build;
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
        /** The gradients filling and stroking in place of the colors, or null. */
        Shader fillShader, strokeShader;

        State copy() {
            State s = new State();
            s.fill = fill;
            s.stroke = stroke;
            s.lineWidth = lineWidth;
            s.alpha = alpha;
            s.cap = cap;
            s.join = join;
            s.fillShader = fillShader;
            s.strokeShader = strokeShader;
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
                        path.setFillType(b.get() != 0 ? Path.FillType.EVEN_ODD : Path.FillType.WINDING);
                        fill(state.fill);
                        canvas.drawPath(path, paint);
                        break;
                    case Protocol.DRAW_STROKE:
                        paint.reset();
                        paint.setAntiAlias(true);
                        paint.setStyle(Paint.Style.STROKE);
                        paint.setColor(fade(state.strokeShader != null ? Color.BLACK : state.stroke));
                        paint.setShader(state.strokeShader);
                        paint.setStrokeWidth(state.lineWidth);
                        paint.setStrokeCap(state.cap);
                        paint.setStrokeJoin(state.join);
                        canvas.drawPath(path, paint);
                        break;
                    case Protocol.DRAW_CLIP:
                        path.setFillType(b.get() != 0 ? Path.FillType.EVEN_ODD : Path.FillType.WINDING);
                        canvas.clipPath(path);
                        break;
                    case Protocol.DRAW_FILL_COLOR:
                        state.fill = Text.argb(b.getInt());
                        state.fillShader = null;
                        break;
                    case Protocol.DRAW_STROKE_COLOR:
                        state.stroke = Text.argb(b.getInt());
                        state.strokeShader = null;
                        break;
                    case Protocol.DRAW_FILL_GRADIENT: {
                        // An invalid gradient keeps the color before it.
                        Shader g = gradient(b);
                        if (g != null) state.fillShader = g;
                        break;
                    }
                    case Protocol.DRAW_STROKE_GRADIENT: {
                        Shader g = gradient(b);
                        if (g != null) state.strokeShader = g;
                        break;
                    }
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

        /** Sets the paint to fill with color, or with the fill gradient over it. */
        private void fill(int color) {
            paint.reset();
            paint.setAntiAlias(true);
            paint.setStyle(Paint.Style.FILL);
            // A shader takes only the paint's alpha.
            paint.setColor(fade(state.fillShader != null ? Color.BLACK : color));
            paint.setShader(state.fillShader);
        }

        /**
         * Reads a core.Gradient as android.go's writeGradient writes it and
         * returns its shader, or null for one Android cannot paint. Before
         * API 31 a radial gradient's focal circle is its center.
         */
        private static Shader gradient(ByteBuffer b) {
            boolean radial = b.get() != 0;
            float x1 = b.getFloat(), y1 = b.getFloat(), r1 = b.getFloat();
            float x2 = b.getFloat(), y2 = b.getFloat(), r2 = b.getFloat();
            float[] m = new float[6];
            for (int i = 0; i < 6; i++) m[i] = b.getFloat();
            int spread = b.get();
            int n = b.getInt();
            float[] offsets = new float[Math.max(n, 2)];
            int[] colors = new int[Math.max(n, 2)];
            for (int i = 0; i < n; i++) {
                offsets[i] = b.getFloat();
                colors[i] = Text.argb(b.getInt());
            }
            if (n == 0) return null;
            if (n == 1) {
                offsets[1] = 1;
                colors[1] = colors[0];
            }
            Shader.TileMode tile = spread == Protocol.SPREAD_REFLECT ? Shader.TileMode.MIRROR
                    : spread == Protocol.SPREAD_REPEAT ? Shader.TileMode.REPEAT : Shader.TileMode.CLAMP;
            Shader shader;
            try {
                if (!radial) {
                    shader = new LinearGradient(x1, y1, x2, y2, colors, offsets, tile);
                } else if (Build.VERSION.SDK_INT >= 31) {
                    long[] packed = new long[colors.length];
                    for (int i = 0; i < colors.length; i++) packed[i] = Color.pack(colors[i]);
                    shader = new RadialGradient(x1, y1, Math.max(r1, 0), x2, y2, r2, packed, offsets, tile);
                } else {
                    if (r2 <= 0) return null;
                    // The focal circle's radius moves the stops outward.
                    float inner = Math.max(0, Math.min(r1 / r2, 1));
                    float[] stops = new float[offsets.length];
                    for (int i = 0; i < stops.length; i++) stops[i] = inner + offsets[i] * (1 - inner);
                    shader = new RadialGradient(x2, y2, r2, colors, stops, tile);
                }
            } catch (IllegalArgumentException e) {
                return null;
            }
            boolean identity = true;
            for (float v : m) identity &= v == 0;
            if (!identity) {
                Matrix local = new Matrix();
                local.setValues(new float[] {m[0], m[2], m[4], m[1], m[3], m[5], 0, 0, 1});
                shader.setLocalMatrix(local);
            }
            return shader;
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
