package dev.oneo.nsgo;

/**
 * Pure geometry, blur-level, and smoothstep tint math for scroll edge effects,
 * ported from internal/driver/scroll.go and kept free of Android framework
 * types so host JVM tests can verify every formula.
 */
final class ScrollEdgeMath {
    private ScrollEdgeMath() {}

    /** Drawn edge effect modes encoded in ItemScrollConfig. */
    static final int NONE = 0, SOFT = 1, HARD = 2;
    /** Box sides in driver.Side order. */
    static final int SIDE_TOP = 0, SIDE_RIGHT = 1, SIDE_BOTTOM = 2, SIDE_LEFT = 3;

    /** Shared edge constants from internal/driver/scroll.go, lengths in points. */
    static final float SOFT_BLUR = 10f;
    static final float SOFT_TAIL = 24f;
    static final float HARD_BLUR = 6.4f;
    static final float EDGE_TINT = 0.8f;
    static final float UNBLURRED_SOFT_TINT = 0.85f;
    static final float UNBLURRED_HARD_TINT = 0.95f;
    static final float HAIRLINE_WIDTH = 0.5f;
    static final int SOFT_TINT_STEPS = 8;

    private static final float FINEST_LEVEL = 1.5f;

    /** One blur level of a soft scroll edge, matching driver.BlurLevel. */
    static final class Level {
        final float total, full, gone;

        Level(float total, float full, float gone) {
            this.total = total;
            this.full = full;
            this.gone = gone;
        }
    }

    private static final Level[] LEVELS = buildSoftLevels();

    private static Level[] buildSoftLevels() {
        int count = 1;
        for (float h = SOFT_BLUR; h > FINEST_LEVEL; h /= 2f) count++;
        float[] his = new float[count];
        his[0] = SOFT_BLUR;
        for (int i = 1; i < count; i++) his[i] = his[i - 1] / 2f;
        Level[] out = new Level[count];
        float lo = 0f;
        for (int i = count - 1, k = 0; i >= 0; i--, k++) {
            float hi = his[i];
            out[k] = new Level(hi, 1f - hi / SOFT_BLUR, 1f - lo / SOFT_BLUR);
            lo = hi;
        }
        return out;
    }

    /** Returns the progressive blur levels from finest to coarsest (driver.SoftEdgeLevels). */
    static Level[] softLevels() {
        return LEVELS;
    }

    /** Returns how far from its side the drawn effect reaches, in points (driver.ScrollEdge.Band). */
    static float band(int effect, float extent) {
        if (Float.isNaN(extent) || extent <= 0f) return 0f;
        if (effect == SOFT) return extent + SOFT_TAIL;
        if (effect == HARD) return extent;
        return 0f;
    }

    /** Writes [left, top, right, bottom] of the side's band in view pixels (driver.BandRect). */
    static void bandRect(int side, int w, int h, int bandPx, int[] out) {
        if (w <= 0 || h <= 0 || bandPx <= 0) {
            out[0] = out[1] = out[2] = out[3] = 0;
            return;
        }
        switch (side) {
            case SIDE_RIGHT: {
                int b = Math.min(bandPx, w);
                out[0] = w - b;
                out[1] = 0;
                out[2] = w;
                out[3] = h;
                return;
            }
            case SIDE_BOTTOM: {
                int b = Math.min(bandPx, h);
                out[0] = 0;
                out[1] = h - b;
                out[2] = w;
                out[3] = h;
                return;
            }
            case SIDE_LEFT: {
                int b = Math.min(bandPx, w);
                out[0] = 0;
                out[1] = 0;
                out[2] = b;
                out[3] = h;
                return;
            }
            default: {
                int b = Math.min(bandPx, h);
                out[0] = 0;
                out[1] = 0;
                out[2] = w;
                out[3] = b;
            }
        }
    }

    /**
     * Writes [left, top, right, bottom] of the region recorded for a blur pass:
     * flush with the outer edge (for CLAMP edge treatment) and padded inward by
     * marginPx so the blur kernel near the band's inner edge samples real
     * content rather than clipping at the band boundary.
     */
    static void captureRect(int side, int w, int h, int bandPx, int marginPx, int[] out) {
        if (w <= 0 || h <= 0 || bandPx <= 0) {
            out[0] = out[1] = out[2] = out[3] = 0;
            return;
        }
        int span = bandPx + Math.max(0, marginPx);
        switch (side) {
            case SIDE_RIGHT:
                out[0] = Math.max(0, w - span);
                out[1] = 0;
                out[2] = w;
                out[3] = h;
                return;
            case SIDE_BOTTOM:
                out[0] = 0;
                out[1] = Math.max(0, h - span);
                out[2] = w;
                out[3] = h;
                return;
            case SIDE_LEFT:
                out[0] = 0;
                out[1] = 0;
                out[2] = Math.min(span, w);
                out[3] = h;
                return;
            default:
                out[0] = 0;
                out[1] = 0;
                out[2] = w;
                out[3] = Math.min(span, h);
        }
    }

    /** Writes [left, top, right, bottom] of a hard edge's hairline in view pixels (driver.HairlineRect). */
    static void hairlineRect(int side, int bL, int bT, int bR, int bB, float linePx, float[] out) {
        switch (side) {
            case SIDE_RIGHT:
                out[0] = bL - linePx;
                out[1] = bT;
                out[2] = bL;
                out[3] = bB;
                return;
            case SIDE_BOTTOM:
                out[0] = bL;
                out[1] = bT - linePx;
                out[2] = bR;
                out[3] = bT;
                return;
            case SIDE_LEFT:
                out[0] = bR;
                out[1] = bT;
                out[2] = bR + linePx;
                out[3] = bB;
                return;
            default:
                out[0] = bL;
                out[1] = bB;
                out[2] = bR;
                out[3] = bB + linePx;
        }
    }

    /** Writes [x0, y0, x1, y1] of the gradient axis across a band from its outer edge (driver.EdgeLine). */
    static void edgeLine(int side, float l, float t, float r, float b, float[] out) {
        switch (side) {
            case SIDE_RIGHT:
                out[0] = r;
                out[1] = (t + b) * 0.5f;
                out[2] = l;
                out[3] = out[1];
                return;
            case SIDE_BOTTOM:
                out[0] = (l + r) * 0.5f;
                out[1] = b;
                out[2] = out[0];
                out[3] = t;
                return;
            case SIDE_LEFT:
                out[0] = l;
                out[1] = (t + b) * 0.5f;
                out[2] = r;
                out[3] = out[1];
                return;
            default:
                out[0] = (l + r) * 0.5f;
                out[1] = t;
                out[2] = out[0];
                out[3] = b;
        }
    }

    /**
     * Writes [x0, y0, x1, y1] for a blur level's LinearGradient alpha mask in
     * the capture RenderNode's local [0, capW] x [0, capH] coordinates, running
     * from full (opaque) to gone (transparent) across bandPx from the outer side.
     */
    static void levelMaskLine(int side, int capW, int capH, int bandPx, float full, float gone, float[] out) {
        switch (side) {
            case SIDE_RIGHT:
                out[0] = capW - full * bandPx;
                out[1] = 0f;
                out[2] = capW - gone * bandPx;
                out[3] = 0f;
                return;
            case SIDE_BOTTOM:
                out[0] = 0f;
                out[1] = capH - full * bandPx;
                out[2] = 0f;
                out[3] = capH - gone * bandPx;
                return;
            case SIDE_LEFT:
                out[0] = full * bandPx;
                out[1] = 0f;
                out[2] = gone * bandPx;
                out[3] = 0f;
                return;
            default:
                out[0] = 0f;
                out[1] = full * bandPx;
                out[2] = 0f;
                out[3] = gone * bandPx;
        }
    }

    /**
     * Populates SOFT_TINT_STEPS + 1 smoothstep tint gradient stops (driver.ScrollEdge.SoftTint):
     * most opacity up to SOFT_TAIL short of the bars' end, easing to 0 at the band's end.
     */
    static void softTintStops(float extent, float band, int tintArgb, float most, int[] colors, float[] positions) {
        float from = band > 0f ? Math.max(extent - SOFT_TAIL, 0f) / band : 0f;
        if (from > 1f) from = 1f;
        for (int i = 0; i <= SOFT_TINT_STEPS; i++) {
            float t = (float) i / SOFT_TINT_STEPS;
            float fade = most * (1f - t * t * (3f - 2f * t));
            positions[i] = from + (1f - from) * t;
            colors[i] = withAlpha(tintArgb, fade);
        }
    }

    /** Scales the alpha channel of an ARGB color by factor in [0, 1] (driver.Color.Faded). */
    static int withAlpha(int argb, float factor) {
        float clamped = Float.isNaN(factor) || factor <= 0f ? 0f : Math.min(factor, 1f);
        int a = Math.round(((argb >>> 24) & 0xff) * clamped);
        return (a << 24) | (argb & 0x00ffffff);
    }

    /** Inward capture margin in pixels for a blur pass of radiusPx pixels. */
    static int blurMarginPx(float radiusPx) {
        return Math.max(1, (int) Math.ceil(Math.max(0f, radiusPx) * 2.5f));
    }
}
