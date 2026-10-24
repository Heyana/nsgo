package dev.oneo.nsgo;

import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.Rect;
import android.graphics.RectF;
import android.graphics.drawable.GradientDrawable;

/**
 * A view's background: GradientDrawable's fill, corners and uniform border,
 * which also give the view its outline for clipping and shadows, and a
 * border of its own width and color on each side when they differ.
 */
final class Box extends GradientDrawable {
    private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final Path ring = new Path();
    private final Path side = new Path();
    private final RectF outer = new RectF();
    private final RectF inner = new RectF();
    private final float[] outerRadii = new float[8];
    private final float[] innerRadii = new float[8];
    /** Each side's width in pixels and ARGB color: top, right, bottom, left; null for none. */
    private float[] widths;
    private int[] colors;
    private float radius;

    Box() {
        paint.setStyle(Paint.Style.FILL);
    }

    /** Draws each side's border, or none for null widths. */
    void setSides(float[] widths, int[] colors, float radius) {
        this.widths = widths;
        this.colors = colors;
        this.radius = radius;
        invalidateSelf();
    }

    @Override
    public void draw(Canvas canvas) {
        super.draw(canvas);
        if (widths == null) return;
        Rect b = getBounds();
        float w = b.width(), h = b.height();
        if (w <= 0 || h <= 0) return;
        float top = Math.min(widths[0], h), right = Math.min(widths[1], w);
        float bottom = Math.min(widths[2], h), left = Math.min(widths[3], w);
        // GradientDrawable clamps its corners so, too.
        float r = Math.max(0, Math.min(radius, Math.min(w, h) / 2));

        // The ring between the outer rounded box and the inner one, whose
        // corners shrink by the borders beside them.
        outer.set(0, 0, w, h);
        inner.set(left, top, w - right, h - bottom);
        corners(outerRadii, r, r, r, r, r, r, r, r);
        corners(innerRadii,
                Math.max(0, r - left), Math.max(0, r - top),
                Math.max(0, r - right), Math.max(0, r - top),
                Math.max(0, r - right), Math.max(0, r - bottom),
                Math.max(0, r - left), Math.max(0, r - bottom));
        ring.reset();
        ring.setFillType(Path.FillType.EVEN_ODD);
        ring.addRoundRect(outer, outerRadii, Path.Direction.CW);
        if (inner.width() > 0 && inner.height() > 0) ring.addRoundRect(inner, innerRadii, Path.Direction.CW);

        // Each side fills its part of the ring: the fan from its outer
        // corners through the inner ones to the center, mitered at the
        // corners without radius.
        float cx = w / 2, cy = h / 2;
        int save = canvas.save();
        canvas.translate(b.left, b.top);
        canvas.clipPath(ring);
        fill(canvas, widths[0], colors[0], 0, 0, w, 0, w - right, top, left, top, cx, cy);
        fill(canvas, widths[1], colors[1], w, 0, w, h, w - right, h - bottom, w - right, top, cx, cy);
        fill(canvas, widths[2], colors[2], w, h, 0, h, left, h - bottom, w - right, h - bottom, cx, cy);
        fill(canvas, widths[3], colors[3], 0, h, 0, 0, left, top, left, h - bottom, cx, cy);
        canvas.restoreToCount(save);
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

    /** Sets the x and y radii of the top-left, top-right, bottom-right and bottom-left corners. */
    private static void corners(float[] radii, float tlx, float tly, float trx, float try_, float brx, float bry,
            float blx, float bly) {
        radii[0] = tlx;
        radii[1] = tly;
        radii[2] = trx;
        radii[3] = try_;
        radii[4] = brx;
        radii[5] = bry;
        radii[6] = blx;
        radii[7] = bly;
    }
}
