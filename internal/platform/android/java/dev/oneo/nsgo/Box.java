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
import android.graphics.drawable.Drawable;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.LayerDrawable;
import android.view.View;

/**
 * A view's background, drawn as CSS draws a box: its outer shadows, its
 * fill and gradients, its inset shadows, its border and its outline, in
 * its corners. GradientDrawable keeps the corners and the fill only to give
 * the view its outline for elevation shadows. Overflow clips only the body of the view: begin/end draw its outer decoration around that clip.
 */
final class Box extends GradientDrawable {
    private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Path path = new Path();
    private final Path ring = new Path();
    private final Path side = new Path();
    private final Look look = new Look();
    private final float[] cornerBuf = new float[4];
    private final float[] widthBuf = new float[4];
    private final int[] colorBuf = new int[4];
    private final Path clipPath = new Path();
    private final Path childClip = new Path();
    private boolean decorated;
    private boolean hasOuter, hasInset, hasBorder, hasBody, hasOutline;
    private Look appliedLook;
    private boolean hasApplied;
    private float appliedAlpha;
    private float appliedElevation;
    private boolean appliedShadowed;
    private static final int OUTER = 1, BODY = 2, OUTLINE = 4;
    /** Whether the view clips its content, independently of its outer decoration. */
    boolean clip;

    boolean sameLook(Look next) {
        return decorated && look.sameBox(next);
    }

    Look applied() {
        return hasApplied ? appliedLook : null;
    }

    float appliedAlpha() {
        return appliedAlpha;
    }

    float appliedElevation() {
        return appliedElevation;
    }

    boolean appliedShadowed() {
        return appliedShadowed;
    }

    void recordApplied(Look l, float alpha, float elevation, boolean shadowed) {
        if (appliedLook == null) appliedLook = new Look();
        appliedLook.copyFrom(l);
        hasApplied = true;
        appliedAlpha = alpha;
        appliedElevation = elevation;
        appliedShadowed = shadowed;
    }

    void resetApplied() {
        hasApplied = false;
    }

    /** Finds the box even when an under-content ripple shares the background. */
    private static Box background(View view) {
        Drawable background = view.getBackground();
        if (background instanceof Box) return (Box) background;
        if (background instanceof LayerDrawable) {
            LayerDrawable layers = (LayerDrawable) background;
            if (layers.getNumberOfLayers() > 0 && layers.getDrawable(0) instanceof Box)
                return (Box) layers.getDrawable(0);
        }
        return null;
    }

    /** The logical clip, also used by hit testing once the render node stays unclipped. */
    static boolean clips(View view) {
        Box box = background(view);
        return box != null && box.clip || view.getClipToOutline();
    }

    /** Draws the outer shadows and clips the following View.draw to the body's corners. */
    static int begin(Canvas canvas, View view) {
        Box box = background(view);
        if (box == null || !box.clip) return -1;
        // View.draw normally updates these bounds, after begin has run.
        int w = view.getWidth(), h = view.getHeight();
        box.setBounds(0, 0, w, h);
        if (box.hasOuter) box.draw(canvas, OUTER);
        int save = canvas.save();
        if (!box.look.rounded()) {
            canvas.clipRect(0, 0, w, h);
        } else {
            canvas.clipPath(box.shape(box.getBounds(), box.clipPath));
        }
        return save;
    }

    /**
     * Clips the children drawn next to the padding box, inside the border, as CSS clips overflow; begin has clipped the
     * body, border included, to the box's outer corners. Returns the count to restore, or -1 when there is no border.
     */
    static int children(Canvas canvas, View view) {
        Box box = background(view);
        if (box == null || !box.clip || !box.hasBorder) return -1;
        float[] widths = box.widths();
        if (widths[0] <= 0 && widths[1] <= 0 && widths[2] <= 0 && widths[3] <= 0) return -1;
        float w = view.getWidth(), h = view.getHeight();
        float x = widths[3], y = widths[0], iw = Math.max(w - widths[1] - x, 0), ih = Math.max(h - widths[2] - y, 0);
        int save = canvas.save();
        if (!box.look.rounded()) {
            canvas.clipRect(x, y, x + iw, y + ih);
        } else {
            box.childClip.reset();
            Shape.ellipticRect(box.childClip, x, y, iw, ih,
                    innerRadii(Shape.fit(box.corners(), w, h), widths), box.look.continuous);
            canvas.clipPath(box.childClip);
        }
        return save;
    }

    /** Restores the ancestor's clip and draws the outline over the clipped body. */
    static void end(Canvas canvas, View view, int save) {
        if (save < 0) return;
        canvas.restoreToCount(save);
        Box box = background(view);
        if (box != null && box.hasOutline) box.draw(canvas, OUTLINE);
    }

    /** Draws look: its background, corners, borders, shadows, gradients and outline, in pixels. */
    void decorate(Look next) {
        look.copyFrom(next);
        decorated = true;
        boolean outer = false, inset = false;
        for (Look.Shadow s : look.shadows) {
            if ((s.color >>> 24) == 0) continue;
            if (s.inset) inset = true;
            else outer = true;
        }
        hasOuter = outer;
        hasInset = inset;
        boolean bordered;
        if (look.sides) {
            bordered = (look.borders[0] > 0 && (look.borderColors[0] >>> 24) != 0)
                    || (look.borders[1] > 0 && (look.borderColors[1] >>> 24) != 0)
                    || (look.borders[2] > 0 && (look.borderColors[2] >>> 24) != 0)
                    || (look.borders[3] > 0 && (look.borderColors[3] >>> 24) != 0);
        } else {
            bordered = look.border > 0 && (look.borderColor >>> 24) != 0;
        }
        hasBorder = bordered;
        hasBody = (look.background >>> 24) != 0 || look.gradients.length > 0 || hasInset || hasBorder;
        hasOutline = look.outline != null && look.outline.width > 0 && (look.outline.color >>> 24) != 0;
        setColor(look.background);
        float[] c = corners();
        // Equal corners give a round rect outline for native elevation shadows; other corners use a path outline where the platform supports it.
        if (c[0] == c[1] && c[1] == c[2] && c[2] == c[3]) {
            setCornerRadius(c[0]);
        } else {
            setCornerRadii(new float[] {c[0], c[0], c[1], c[1], c[2], c[2], c[3], c[3]});
        }
        invalidateSelf();
    }

    /** Writes the box's rounded shape in bounds b to path, in pixels, and returns it. */
    Path shape(Rect b, Path path) {
        path.reset();
        box(path, b.left, b.top, b.width(), b.height(), Shape.fit(corners(), b.width(), b.height()), look.continuous);
        return path;
    }

    /** Each corner's radius: top left, top right, bottom right, bottom left. */
    private float[] corners() {
        if (look.corners != null) return look.corners;
        float r = look.radius;
        cornerBuf[0] = r;
        cornerBuf[1] = r;
        cornerBuf[2] = r;
        cornerBuf[3] = r;
        return cornerBuf;
    }

    @Override
    public void draw(Canvas canvas) {
        draw(canvas, clip ? BODY : OUTER | BODY | OUTLINE);
    }

    private void draw(Canvas canvas, int parts) {
        boolean drawOuter = (parts & OUTER) != 0 && hasOuter;
        boolean drawBody = (parts & BODY) != 0 && hasBody;
        boolean drawOutline = (parts & OUTLINE) != 0 && hasOutline;
        if (!drawOuter && !drawBody && !drawOutline) return;
        Rect b = getBounds();
        float w = b.width(), h = b.height();
        if (w <= 0 || h <= 0) return;
        // Fast path for square boxes with only solid fill and/or solid border (most table rows and cells).
        if (!drawOuter && drawBody && !drawOutline && !look.rounded() && look.gradients.length == 0 && !hasInset) {
            if (drawSquareBody(canvas, b.left, b.top, w, h)) return;
        }
        float[] radii = Shape.fit(corners(), w, h);
        boolean continuous = look.continuous;
        int save = canvas.save();
        canvas.translate(b.left, b.top);
        box(path, 0, 0, w, h, radii, continuous);
        // The first shadow is on top.
        Look.Shadow[] shadows = look.shadows;
        if (drawOuter) for (int i = shadows.length - 1; i >= 0; i--) {
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
        if (drawBody) {
            paint.setStyle(Paint.Style.FILL);
            boolean rimmed = look.hasRim();
            if (rimmed) {
                // Stop the fill inset under the border so curved outer edge pixels take only the rimmed border color, matching driver.ViewStyle.Rim (and border()'s 1 px minimum stroke width).
                float inset = Math.max(0.75f, look.rimInset());
                if (w > 2 * inset && h > 2 * inset) {
                    box(path, inset, inset, w - 2 * inset, h - 2 * inset, Shape.inset(radii, inset), continuous);
                    paint.setColor(look.background);
                    canvas.drawPath(path, paint);
                }
            } else if ((look.background >>> 24) != 0) {
                paint.setColor(look.background);
                canvas.drawPath(path, paint);
            }
            for (int i = look.gradients.length - 1; i >= 0; i--) {
                paint.setShader(look.gradients[i].shader(w, h));
                paint.setColor(0xff000000);
                canvas.drawPath(path, paint);
                paint.setShader(null);
            }
            if (hasInset || hasBorder) {
                float[] widths = widths();
                if (hasInset) insets(canvas, w, h, radii, widths);
                if (hasBorder) border(canvas, w, h, radii, widths, rimmed ? look.rimColor() : look.borderColor);
            }
        }
        Look.Outline o = look.outline;
        if (drawOutline && o != null && o.width > 0 && (o.color >>> 24) != 0) {
            float out = o.offset + o.width / 2;
            stroke(canvas, -out, -out, w + 2 * out, h + 2 * out, Shape.inset(radii, -out), o.width, o.color, o.style);
        }
        canvas.restoreToCount(save);
    }

    /**
     * Draws a square box with solid background and optional solid uniform or single-side border directly via drawRect,
     * avoiding Path construction, canvas.save, and Shape.roundedRect allocations. Returns false for complex borders.
     */
    private boolean drawSquareBody(Canvas canvas, float left, float top, float w, float h) {
        boolean dashed = look.borderStyle == Protocol.BORDER_DASHED || look.borderStyle == Protocol.BORDER_DOTTED;
        if (hasBorder) {
            if (dashed) return false;
            if (look.sides) {
                int active = 0;
                for (int i = 0; i < 4; i++) {
                    if (look.borders[i] > 0) active++;
                }
                boolean same = look.borders[0] == look.borders[1] && look.borders[1] == look.borders[2]
                        && look.borders[2] == look.borders[3]
                        && look.borderColors[0] == look.borderColors[1]
                        && look.borderColors[1] == look.borderColors[2]
                        && look.borderColors[2] == look.borderColors[3];
                if (!same && active > 1) return false;
            }
        }
        if ((look.background >>> 24) != 0) {
            paint.setStyle(Paint.Style.FILL);
            paint.setColor(look.background);
            canvas.drawRect(left, top, left + w, top + h, paint);
        }
        if (!hasBorder) return true;
        if (!look.sides) {
            if ((look.borderColor >>> 24) == 0 || look.border <= 0) return true;
            float wd = Math.max(1, look.border);
            float half = wd / 2;
            paint.setStyle(Paint.Style.STROKE);
            paint.setStrokeWidth(wd);
            paint.setColor(look.borderColor);
            paint.setStrokeCap(Paint.Cap.BUTT);
            canvas.drawRect(left + half, top + half, left + w - half, top + h - half, paint);
            paint.setStyle(Paint.Style.FILL);
            return true;
        }
        float[] bw = look.borders;
        int[] bc = look.borderColors;
        if (bw[0] == bw[1] && bw[1] == bw[2] && bw[2] == bw[3]
                && bc[0] == bc[1] && bc[1] == bc[2] && bc[2] == bc[3]) {
            if ((bc[0] >>> 24) == 0 || bw[0] <= 0) return true;
            float wd = Math.max(1, bw[0]);
            float half = wd / 2;
            paint.setStyle(Paint.Style.STROKE);
            paint.setStrokeWidth(wd);
            paint.setColor(bc[0]);
            paint.setStrokeCap(Paint.Cap.BUTT);
            canvas.drawRect(left + half, top + half, left + w - half, top + h - half, paint);
            paint.setStyle(Paint.Style.FILL);
            return true;
        }
        paint.setStyle(Paint.Style.FILL);
        if (bw[0] > 0 && (bc[0] >>> 24) != 0) {
            paint.setColor(bc[0]);
            canvas.drawRect(left, top, left + w, top + Math.min(bw[0], h), paint);
        } else if (bw[1] > 0 && (bc[1] >>> 24) != 0) {
            paint.setColor(bc[1]);
            canvas.drawRect(left + w - Math.min(bw[1], w), top, left + w, top + h, paint);
        } else if (bw[2] > 0 && (bc[2] >>> 24) != 0) {
            paint.setColor(bc[2]);
            canvas.drawRect(left, top + h - Math.min(bw[2], h), left + w, top + h, paint);
        } else if (bw[3] > 0 && (bc[3] >>> 24) != 0) {
            paint.setColor(bc[3]);
            canvas.drawRect(left, top, left + Math.min(bw[3], w), top + h, paint);
        }
        return true;
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
        widthBuf[0] = b;
        widthBuf[1] = b;
        widthBuf[2] = b;
        widthBuf[3] = b;
        return widthBuf;
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
    private void border(Canvas canvas, float w, float h, float[] radii, float[] widths, int borderColor) {
        int[] colors;
        if (look.sides) {
            colors = look.borderColors;
        } else {
            colorBuf[0] = borderColor;
            colorBuf[1] = borderColor;
            colorBuf[2] = borderColor;
            colorBuf[3] = borderColor;
            colors = colorBuf;
        }
        float widest = Math.max(Math.max(widths[0], widths[1]), Math.max(widths[2], widths[3]));
        if (widest <= 0) return;
        boolean same = widths[0] == widths[1] && widths[1] == widths[2] && widths[2] == widths[3]
                && colors[0] == colors[1] && colors[1] == colors[2] && colors[2] == colors[3];
        boolean dashed = look.borderStyle == Protocol.BORDER_DASHED || look.borderStyle == Protocol.BORDER_DOTTED;
        if (dashed || same) {
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
