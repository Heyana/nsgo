package dev.oneo.nsgo;

public final class ScrollEdgeCheck {
    private static void check(boolean value, String why) {
        if (!value) throw new AssertionError(why);
    }

    private static void near(float got, float want, String why) {
        if (Math.abs(got - want) > 1e-4f) {
            throw new AssertionError(why + ": got " + got + ", want " + want);
        }
    }

    public static void main(String[] args) {
        // Constants match internal/driver/scroll.go.
        near(ScrollEdgeMath.SOFT_BLUR, 10f, "SOFT_BLUR");
        near(ScrollEdgeMath.SOFT_TAIL, 24f, "SOFT_TAIL");
        near(ScrollEdgeMath.HARD_BLUR, 6.4f, "HARD_BLUR");
        near(ScrollEdgeMath.EDGE_TINT, 0.8f, "EDGE_TINT");
        near(ScrollEdgeMath.UNBLURRED_SOFT_TINT, 0.85f, "UNBLURRED_SOFT_TINT");
        near(ScrollEdgeMath.UNBLURRED_HARD_TINT, 0.95f, "UNBLURRED_HARD_TINT");
        near(ScrollEdgeMath.HAIRLINE_WIDTH, 0.5f, "HAIRLINE_WIDTH");

        // Band heights.
        near(ScrollEdgeMath.band(ScrollEdgeMath.NONE, 60f), 0f, "band none");
        near(ScrollEdgeMath.band(ScrollEdgeMath.SOFT, 0f), 0f, "band soft zero");
        near(ScrollEdgeMath.band(ScrollEdgeMath.SOFT, -10f), 0f, "band soft negative");
        near(ScrollEdgeMath.band(ScrollEdgeMath.SOFT, Float.NaN), 0f, "band soft NaN");
        near(ScrollEdgeMath.band(ScrollEdgeMath.SOFT, 64f), 88f, "band soft adds tail");
        near(ScrollEdgeMath.band(ScrollEdgeMath.HARD, 64f), 64f, "band hard exact");

        // Progressive blur levels match driver.SoftEdgeLevels().
        ScrollEdgeMath.Level[] levels = ScrollEdgeMath.softLevels();
        check(levels.length == 4, "softLevels count = " + levels.length);
        float[][] wantLevels = {
            {1.25f, 0.875f, 1.0f},
            {2.5f, 0.75f, 0.875f},
            {5.0f, 0.5f, 0.75f},
            {10.0f, 0.0f, 0.5f},
        };
        for (int i = 0; i < wantLevels.length; i++) {
            near(levels[i].total, wantLevels[i][0], "level[" + i + "].total");
            near(levels[i].full, wantLevels[i][1], "level[" + i + "].full");
            near(levels[i].gone, wantLevels[i][2], "level[" + i + "].gone");
        }

        // Band rects for all four sides and clamping.
        int[] r = new int[4];
        ScrollEdgeMath.bandRect(ScrollEdgeMath.SIDE_TOP, 1000, 2000, 300, r);
        check(r[0] == 0 && r[1] == 0 && r[2] == 1000 && r[3] == 300, "top bandRect");
        ScrollEdgeMath.bandRect(ScrollEdgeMath.SIDE_RIGHT, 1000, 2000, 150, r);
        check(r[0] == 850 && r[1] == 0 && r[2] == 1000 && r[3] == 2000, "right bandRect");
        ScrollEdgeMath.bandRect(ScrollEdgeMath.SIDE_BOTTOM, 1000, 2000, 250, r);
        check(r[0] == 0 && r[1] == 1750 && r[2] == 1000 && r[3] == 2000, "bottom bandRect");
        ScrollEdgeMath.bandRect(ScrollEdgeMath.SIDE_LEFT, 1000, 2000, 120, r);
        check(r[0] == 0 && r[1] == 0 && r[2] == 120 && r[3] == 2000, "left bandRect");
        // Oversized band clamps to view bounds.
        ScrollEdgeMath.bandRect(ScrollEdgeMath.SIDE_TOP, 300, 100, 500, r);
        check(r[0] == 0 && r[1] == 0 && r[2] == 300 && r[3] == 100, "top bandRect clamped");
        ScrollEdgeMath.bandRect(ScrollEdgeMath.SIDE_BOTTOM, 300, 100, 500, r);
        check(r[0] == 0 && r[1] == 0 && r[2] == 300 && r[3] == 100, "bottom bandRect clamped");

        // Capture rects keep outer edge flush and pad inward by margin.
        ScrollEdgeMath.captureRect(ScrollEdgeMath.SIDE_TOP, 1000, 2000, 300, 60, r);
        check(r[0] == 0 && r[1] == 0 && r[2] == 1000 && r[3] == 360, "top captureRect");
        ScrollEdgeMath.captureRect(ScrollEdgeMath.SIDE_BOTTOM, 1000, 2000, 300, 60, r);
        check(r[0] == 0 && r[1] == 1640 && r[2] == 1000 && r[3] == 2000, "bottom captureRect");
        ScrollEdgeMath.captureRect(ScrollEdgeMath.SIDE_LEFT, 1000, 2000, 100, 40, r);
        check(r[0] == 0 && r[1] == 0 && r[2] == 140 && r[3] == 2000, "left captureRect");
        ScrollEdgeMath.captureRect(ScrollEdgeMath.SIDE_RIGHT, 1000, 2000, 100, 40, r);
        check(r[0] == 860 && r[1] == 0 && r[2] == 1000 && r[3] == 2000, "right captureRect");

        // Hairline rects sit just outside the band's inner edge.
        float[] f = new float[4];
        ScrollEdgeMath.hairlineRect(ScrollEdgeMath.SIDE_TOP, 0, 0, 1000, 200, 1.5f, f);
        near(f[0], 0f, "top hairline l");
        near(f[1], 200f, "top hairline t");
        near(f[2], 1000f, "top hairline r");
        near(f[3], 201.5f, "top hairline b");
        ScrollEdgeMath.hairlineRect(ScrollEdgeMath.SIDE_BOTTOM, 0, 1800, 1000, 2000, 1.5f, f);
        near(f[1], 1798.5f, "bottom hairline t");
        near(f[3], 1800f, "bottom hairline b");

        // Level mask lines in local capture space.
        ScrollEdgeMath.levelMaskLine(ScrollEdgeMath.SIDE_TOP, 1000, 360, 300, 0.5f, 0.75f, f);
        near(f[0], 0f, "top mask x0");
        near(f[1], 150f, "top mask y0");
        near(f[2], 0f, "top mask x1");
        near(f[3], 225f, "top mask y1");
        ScrollEdgeMath.levelMaskLine(ScrollEdgeMath.SIDE_BOTTOM, 1000, 360, 300, 0.0f, 0.5f, f);
        near(f[1], 360f, "bottom mask y0 at bottom edge");
        near(f[3], 210f, "bottom mask y1 halfway through band");

        // Smoothstep soft tint stops match driver.ScrollEdge.SoftTint().
        int[] colors = new int[ScrollEdgeMath.SOFT_TINT_STEPS + 1];
        float[] pos = new float[ScrollEdgeMath.SOFT_TINT_STEPS + 1];
        // extent=72, band=96 -> from = (72 - 24) / 96 = 0.5
        ScrollEdgeMath.softTintStops(72f, 96f, 0xffffffff, ScrollEdgeMath.EDGE_TINT, colors, pos);
        near(pos[0], 0.5f, "softTint pos[0]");
        near(pos[4], 0.75f, "softTint pos[4]");
        near(pos[8], 1.0f, "softTint pos[8]");
        check(((colors[0] >>> 24) & 0xff) == Math.round(255 * 0.8f), "softTint alpha[0]");
        check(((colors[4] >>> 24) & 0xff) == Math.round(255 * 0.4f), "softTint alpha[4] midpoint");
        check(((colors[8] >>> 24) & 0xff) == 0, "softTint alpha[8] transparent");
        check((colors[0] & 0x00ffffff) == 0x00ffffff, "softTint preserves RGB");

        // Pre-API-31 unblurred soft tint reaches 85%.
        ScrollEdgeMath.softTintStops(10f, 34f, 0xff102030, ScrollEdgeMath.UNBLURRED_SOFT_TINT, colors, pos);
        near(pos[0], 0f, "short extent starts at 0");
        check(((colors[0] >>> 24) & 0xff) == Math.round(255 * 0.85f), "unblurred softTint alpha[0]");
        check((colors[0] & 0x00ffffff) == 0x00102030, "unblurred softTint preserves RGB");

        // Blur margin scales with radius and is at least 1.
        check(ScrollEdgeMath.blurMarginPx(0f) == 1, "margin min 1");
        check(ScrollEdgeMath.blurMarginPx(10f) == 25, "margin for 10px");
    }
}
