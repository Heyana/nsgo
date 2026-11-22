package dev.oneo.nsgo;

/**
 * Pure layout and collapse math for Android's Material 3 top app bar, kept free
 * of Android framework types so host JVM tests can verify every transition.
 */
final class HeaderLayout {
    private HeaderLayout() {}

    /** Material 3's top app bar height, in points. */
    static final float BAR = 64;
    /** Material 3's large top app bar height before it collapses, in points. */
    static final float LARGE_BAR = 152;
    /** Vertical travel from expanded large bar to regular bar, in points. */
    static final float COLLAPSE = LARGE_BAR - BAR;
    /** Material 3's title sizes in the regular bar and large band, in sp. */
    static final float TITLE = 22, LARGE_TITLE = 28;
    /** Material 3's large title horizontal and bottom padding, in points. */
    static final float LARGE_PADDING_HORIZONTAL = 16, LARGE_PADDING_BOTTOM = 28;
    /** Collapse fraction at which the large title finishes fading out and the toolbar title begins fading in. */
    static final float CROSSFADE = 0.6f;

    /** Clamps how far a large title collapses for a scroll offset of scrolled points below the header. */
    static float collapse(boolean large, float scrolled) {
        if (!large || Float.isNaN(scrolled) || scrolled <= 0) return 0;
        return Math.min(scrolled, COLLAPSE);
    }

    /** Normalized collapse progress in [0, 1]; a regular bar is always fully collapsed (1). */
    static float fraction(boolean large, float collapsed) {
        if (!large) return 1;
        return clamp01(collapsed / COLLAPSE);
    }

    /** The bar's expanded height reported to the core, in points. */
    static float expandedBar(boolean large, float measuredBar) {
        float bar = Math.max(BAR, measuredBar);
        return large ? bar + COLLAPSE : bar;
    }

    /** Total visible header container height in pixels, shrinking from fullPx as the large title collapses. */
    static int containerHeightPx(int fullPx, float collapsed, float density) {
        return Math.max(0, fullPx - Math.round(collapsed * density));
    }

    /** Top margin of the pinned toolbar in pixels (the status bar inset above the expanded bar). */
    static int barTopPx(int fullPx, float expandedBar, float density) {
        return Math.max(0, fullPx - Math.round(expandedBar * density));
    }

    /** Visible height of the large-title band below the pinned toolbar, in pixels. */
    static int largeBandHeightPx(int containerHeightPx, int barTopPx, int barHeightPx) {
        return Math.max(0, containerHeightPx - (barTopPx + barHeightPx));
    }

    /** Opacity of the large title in the lower band as fraction moves from 0 (expanded) to 1 (collapsed). */
    static float largeTitleAlpha(float fraction) {
        return clamp01(1f - clamp01(fraction) / CROSSFADE);
    }

    /** Opacity of the pinned toolbar's title: always 1 for a regular bar, fading in after CROSSFADE for a large bar. */
    static float toolbarTitleAlpha(boolean large, float fraction) {
        if (!large) return 1;
        return clamp01((clamp01(fraction) - CROSSFADE) / (1f - CROSSFADE));
    }

    /**
     * Resolves the header container's ARGB background color: an explicit
     * app-supplied color wins, else transparent (0) when HeaderTransparent is
     * set, else the platform theme's background color.
     */
    static int backgroundColor(int customArgb, boolean transparent, int themeArgb) {
        if (customArgb != 0) return customArgb;
        return transparent ? 0 : themeArgb;
    }

    /** Scales the alpha channel of an ARGB color by alpha in [0, 1]. */
    static int withAlpha(int argb, float alpha) {
        int a = Math.round(((argb >>> 24) & 0xff) * clamp01(alpha));
        return (a << 24) | (argb & 0x00ffffff);
    }

    private static float clamp01(float v) {
        if (Float.isNaN(v) || v <= 0) return 0;
        return Math.min(v, 1);
    }
}
