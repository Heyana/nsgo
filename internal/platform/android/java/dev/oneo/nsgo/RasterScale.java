package dev.oneo.nsgo;

/**
 * Stepped raster scale for views scaled up by a style transform or a zooming
 * scroll view, matching driver.RasterStep / apple.RasterStep: 1 up to a view's
 * own size, then in steps of a half up to 4, so texts and canvases redraw
 * sharply only as a zoom crosses a step and stay within a bounded backing
 * bitmap budget.
 */
final class RasterScale {
    static final float MAX_STEP = 4f;
    static final long MAX_PIXELS = 4096L * 4096L;
    static final int MAX_SIDE = 16384;

    private RasterScale() {}

    /**
     * How many times its pixels a view scaled by scale draws its content at:
     * 1 up to its own size, then in steps of 0.5 up to 4.
     */
    static float step(float scale) {
        if (!(scale > 1f)) return 1f;
        return Math.min((float) (Math.ceil(scale * 2.0) / 2.0), MAX_STEP);
    }

    /** Combines two raster steps, ignoring non-upscaling factors, capped at MAX_STEP. */
    static float combine(float a, float b) {
        float sa = a > 1f ? a : 1f;
        float sb = b > 1f ? b : 1f;
        return Math.min(sa * sb, MAX_STEP);
    }

    /**
     * Effective scale in [1, step] whose backing bitmap for a basePx-sized
     * view fits within MAX_PIXELS and MAX_SIDE. Returns 1 when no upscaling
     * fits within the budget.
     */
    static float boundedScale(int widthPx, int heightPx, float step) {
        if (!(step > 1f) || widthPx <= 0 || heightPx <= 0) return 1f;
        double w = widthPx, h = heightPx;
        double s = Math.min(step, MAX_STEP);
        double areaLimit = Math.sqrt((double) MAX_PIXELS / (w * h));
        double sideLimit = (double) MAX_SIDE / Math.max(w, h);
        s = Math.min(s, Math.min(areaLimit, sideLimit));
        // Guard against ceil rounding pushing width*height just past the cap.
        while (s > 1.0 && ((long) Math.ceil(w * s) * (long) Math.ceil(h * s) > MAX_PIXELS
                || Math.ceil(Math.max(w, h) * s) > MAX_SIDE)) {
            s -= 0.01;
        }
        return s > 1.0 ? (float) s : 1f;
    }

    /** Backing bitmap pixel dimension for basePx at scale. */
    static int scaledPixels(int basePx, float scale) {
        if (basePx <= 0) return 0;
        return Math.max(1, (int) Math.ceil(basePx * (double) scale));
    }
}
