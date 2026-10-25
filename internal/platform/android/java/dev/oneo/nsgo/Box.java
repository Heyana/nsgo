package dev.oneo.nsgo;

import android.graphics.BlurMaskFilter;
import android.graphics.Canvas;
import android.graphics.DashPathEffect;
import android.graphics.LinearGradient;
import android.graphics.Matrix;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.RadialGradient;
import android.graphics.Rect;
import android.graphics.Shader;
import android.graphics.SweepGradient;
import android.graphics.drawable.GradientDrawable;

/**
 * A view's background, drawn as CSS draws a box: its outer shadows, its
 * fill and gradients, its inset shadows, its border and its outline, in
 * its corners. GradientDrawable keeps the corners and the fill only to give
 * the view its outline, for clipping and elevation shadows.
 */
final class Box extends GradientDrawable {
    private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Path path = new Path();
    private final Path ring = new Path();
    private final Path side = new Path();
    private Look look = new Look();

    /** Draws look: its background, corners, borders, shadows, gradients and outline, in pixels. */
    void decorate(Look look) {
        this.look = look;
        setColor(look.background);
        float[] c = corners();
        // Equal corners give a round rect outline, which clips before API
        // 33 as the path outline of per-corner radii does not.
        if (c[0] == c[1] && c[1] == c[2] && c[2] == c[3]) {
            setCornerRadius(c[0]);
        } else {
            setCornerRadii(new float[] {c[0], c[0], c[1], c[1], c[2], c[2], c[3], c[3]});
        }
        invalidateSelf();
    }

    /** Each corner's radius: top left, top right, bottom right, bottom left. */
    private float[] corners() {
        if (look.corners != null) return look.corners;
        float r = look.radius;
        return new float[] {r, r, r, r};
    }

    @Override
    public void draw(Canvas canvas) {
        Rect b = getBounds();
        float w = b.width(), h = b.height();
        if (w <= 0 || h <= 0) return;
        float[] radii = Shape.fit(corners(), w, h);
        boolean continuous = look.continuous;
        int save = canvas.save();
        canvas.translate(b.left, b.top);
        box(path, 0, 0, w, h, radii, continuous);
        // The first shadow is on top.
        Look.Shadow[] shadows = look.shadows;
        for (int i = shadows.length - 1; i >= 0; i--) {
            Look.Shadow s = shadows[i];
            if (s.inset || (s.color >>> 24) == 0) continue;
            int at = canvas.save();
            canvas.clipOutPath(path);
            box(ring, s.dx - s.spread, s.dy - s.spread, w + 2 * s.spread, h + 2 * s.spread,
                    Shape.inset(radii, -s.spread), continuous);
            blurred(s);
            canvas.drawPath(ring, paint);
            paint.setMaskFilter(null);
            canvas.restoreToCount(at);
        }
        paint.setStyle(Paint.Style.FILL);
        if ((look.background >>> 24) != 0) {
            paint.setColor(look.background);
            canvas.drawPath(path, paint);
        }
        for (int i = look.gradients.length - 1; i >= 0; i--) {
            paint.setShader(look.gradients[i].shader(w, h));
            paint.setColor(0xff000000);
            canvas.drawPath(path, paint);
            paint.setShader(null);
        }
        float[] widths = widths();
        insets(canvas, w, h, radii, widths);
        border(canvas, w, h, radii, widths);
        Look.Outline o = look.outline;
        if (o != null && o.width > 0 && (o.color >>> 24) != 0) {
            float out = o.offset + o.width / 2;
            stroke(canvas, -out, -out, w + 2 * out, h + 2 * out, Shape.inset(radii, -out), o.width, o.color, o.style);
        }
        canvas.restoreToCount(save);
    }

    /** Sets path to a box. */
    private static void box(Path path, float x, float y, float w, float h, float[] radii, boolean continuous) {
        path.reset();
        Shape.roundedRect(path, x, y, w, h, radii, continuous);
    }

    /** Readies the paint for a shadow: its color, blurred by CSS's blur radius, twice its deviation. */
    private void blurred(Look.Shadow s) {
        paint.setStyle(Paint.Style.FILL);
        paint.setColor(s.color);
        float sigma = s.blur / 2;
        // Skia's radius for a deviation.
        float radius = (sigma - 0.5f) / 0.57735f;
        paint.setMaskFilter(radius > 0 ? new BlurMaskFilter(radius, BlurMaskFilter.Blur.NORMAL) : null);
    }

    /** Each side's border width: top, right, bottom, left. */
    private float[] widths() {
        if (look.sides) return look.borders;
        float b = look.border;
        return new float[] {b, b, b, b};
    }

    /** Draws the inset shadows inside the padding box. */
    private void insets(Canvas canvas, float w, float h, float[] radii, float[] widths) {
        boolean any = false;
        for (Look.Shadow s : look.shadows) any |= s.inset;
        if (!any) return;
        float x = widths[3], y = widths[0], iw = w - widths[1] - x, ih = h - widths[2] - y;
        float[][] inner = innerRadii(radii, widths);
        side.reset();
        Shape.ellipticRect(side, x, y, iw, ih, inner, look.continuous);
        int at = canvas.save();
        canvas.clipPath(side);
        for (int i = look.shadows.length - 1; i >= 0; i--) {
            Look.Shadow s = look.shadows[i];
            if (!s.inset || (s.color >>> 24) == 0) continue;
            // A ring around the padding box shrunk by the spread casts the
            // shadow inwards.
            float far = s.blur * 2 + Math.abs(s.dx) + Math.abs(s.dy) + Math.abs(s.spread) + 1;
            ring.reset();
            ring.setFillType(Path.FillType.EVEN_ODD);
            ring.addRect(x - far + s.dx, y - far + s.dy, x + iw + far + s.dx, y + ih + far + s.dy, Path.Direction.CW);
            float[][] hole = new float[4][2];
            for (int k = 0; k < 4; k++) {
                hole[k][0] = inner[k][0] > 0 ? Math.max(inner[k][0] - s.spread, 0) : 0;
                hole[k][1] = inner[k][1] > 0 ? Math.max(inner[k][1] - s.spread, 0) : 0;
            }
            Shape.ellipticRect(ring, x + s.spread + s.dx, y + s.spread + s.dy, Math.max(iw - 2 * s.spread, 0),
                    Math.max(ih - 2 * s.spread, 0), hole, look.continuous);
            blurred(s);
            canvas.drawPath(ring, paint);
            paint.setMaskFilter(null);
        }
        canvas.restoreToCount(at);
    }

    /** The radii of the padding box: each corner less the widths of the sides it joins. */
    private static float[][] innerRadii(float[] r, float[] widths) {
        float t = widths[0], ri = widths[1], b = widths[2], l = widths[3];
        return new float[][] {
            {Math.max(r[0] - l, 0), Math.max(r[0] - t, 0)}, {Math.max(r[1] - ri, 0), Math.max(r[1] - t, 0)},
            {Math.max(r[2] - ri, 0), Math.max(r[2] - b, 0)}, {Math.max(r[3] - l, 0), Math.max(r[3] - b, 0)},
        };
    }

    /**
     * Draws the border: a stroke along its middle when it is dashed or the
     * same all round, else the ring between the box and the padding box,
     * each side filling its part, as CSS joins sides that differ.
     */
    private void border(Canvas canvas, float w, float h, float[] radii, float[] widths) {
        int[] colors = look.sides ? look.borderColors
                : new int[] {look.borderColor, look.borderColor, look.borderColor, look.borderColor};
        float widest = Math.max(Math.max(widths[0], widths[1]), Math.max(widths[2], widths[3]));
        if (widest <= 0) return;
        boolean same = widths[0] == widths[1] && widths[1] == widths[2] && widths[2] == widths[3]
                && colors[0] == colors[1] && colors[1] == colors[2] && colors[2] == colors[3];
        if (look.borderStyle != Protocol.BORDER_SOLID || same) {
            float wd = Math.max(1, widest);
            stroke(canvas, wd / 2, wd / 2, w - wd, h - wd, Shape.inset(radii, wd / 2), wd, colors[0], look.borderStyle);
            return;
        }
        float top = Math.min(widths[0], h), right = Math.min(widths[1], w);
        float bottom = Math.min(widths[2], h), left = Math.min(widths[3], w);
        ring.reset();
        ring.setFillType(Path.FillType.EVEN_ODD);
        Shape.roundedRect(ring, 0, 0, w, h, radii, look.continuous);
        if (w - left - right > 0 && h - top - bottom > 0) {
            Shape.ellipticRect(ring, left, top, w - left - right, h - top - bottom, innerRadii(radii, widths), look.continuous);
        }
        // Each side fills its part of the ring: the fan from its outer
        // corners through the inner ones to the center.
        float cx = w / 2, cy = h / 2;
        int save = canvas.save();
        canvas.clipPath(ring);
        paint.setStyle(Paint.Style.FILL);
        fill(canvas, widths[0], colors[0], 0, 0, w, 0, w - right, top, left, top, cx, cy);
        fill(canvas, widths[1], colors[1], w, 0, w, h, w - right, h - bottom, w - right, top, cx, cy);
        fill(canvas, widths[2], colors[2], w, h, 0, h, left, h - bottom, w - right, h - bottom, cx, cy);
        fill(canvas, widths[3], colors[3], 0, h, 0, 0, left, top, left, h - bottom, cx, cy);
        canvas.restoreToCount(save);
    }

    /** Strokes a box in color, width wide, dashed or dotted as style says. */
    private void stroke(Canvas canvas, float x, float y, float w, float h, float[] radii, float width, int color, int style) {
        if ((color >>> 24) == 0 || width <= 0) return;
        side.reset();
        Shape.roundedRect(side, x, y, w, h, radii, look.continuous);
        paint.setStyle(Paint.Style.STROKE);
        paint.setStrokeWidth(width);
        paint.setColor(color);
        paint.setStrokeCap(Paint.Cap.BUTT);
        if (style == Protocol.BORDER_DASHED) {
            paint.setPathEffect(new DashPathEffect(new float[] {width * 3, width * 3}, 0));
        } else if (style == Protocol.BORDER_DOTTED) {
            // Zero-length dashes with round caps draw dots.
            paint.setPathEffect(new DashPathEffect(new float[] {0, width * 2}, 0));
            paint.setStrokeCap(Paint.Cap.ROUND);
        }
        canvas.drawPath(side, paint);
        paint.setPathEffect(null);
        paint.setStyle(Paint.Style.FILL);
    }

    /** Fills a side's fan: outer corners a and b, then b's inner corner, the center and a's inner corner. */
    private void fill(Canvas canvas, float width, int color, float ax, float ay, float bx, float by,
            float bix, float biy, float aix, float aiy, float cx, float cy) {
        if (width <= 0 || (color >>> 24) == 0) return;
        side.reset();
        side.moveTo(ax, ay);
        side.lineTo(bx, by);
        side.lineTo(bix, biy);
        side.lineTo(cx, cy);
        side.lineTo(aix, aiy);
        side.close();
        paint.setColor(color);
        canvas.drawPath(side, paint);
    }

    /** A gradient resolved against its box in fractions, as driver.GradientPaint. */
    static final class Gradient {
        int kind;
        float x0, y0, x1, y1, cx, cy, rx, ry, angle;
        float[] offsets;
        int[] colors;

        Shader shader(float w, float h) {
            switch (kind) {
                case Protocol.GRADIENT_RADIAL: {
                    float r = Math.max(rx * w, 0.001f);
                    RadialGradient g = new RadialGradient(cx * w, cy * h, r, colors, offsets, Shader.TileMode.CLAMP);
                    Matrix m = new Matrix();
                    m.setScale(1, Math.max(ry * h, 0.001f) / r, cx * w, cy * h);
                    g.setLocalMatrix(m);
                    return g;
                }
                case Protocol.GRADIENT_CONIC: {
                    SweepGradient g = new SweepGradient(cx * w, cy * h, colors, offsets);
                    // A sweep starts at three o'clock, CSS's at twelve.
                    Matrix m = new Matrix();
                    m.setRotate(angle - 90, cx * w, cy * h);
                    g.setLocalMatrix(m);
                    return g;
                }
                default:
                    return new LinearGradient(x0 * w, y0 * h, x1 * w, y1 * h, colors, offsets, Shader.TileMode.CLAMP);
            }
        }
    }
}
