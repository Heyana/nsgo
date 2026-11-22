package dev.oneo.nsgo;

public final class HeaderCheck {
    private static void check(boolean value, String why) {
        if (!value) throw new AssertionError(why);
    }

    private static void near(float got, float want, String why) {
        if (Math.abs(got - want) > 1e-4f) {
            throw new AssertionError(why + ": got " + got + ", want " + want);
        }
    }

    public static void main(String[] args) {
        near(HeaderLayout.BAR, 64f, "BAR");
        near(HeaderLayout.LARGE_BAR, 152f, "LARGE_BAR");
        near(HeaderLayout.COLLAPSE, 88f, "COLLAPSE");
        near(HeaderLayout.TITLE, 22f, "TITLE");
        near(HeaderLayout.LARGE_TITLE, 28f, "LARGE_TITLE");

        // A larger font can grow the band without changing its collapse contract.
        near(HeaderLayout.expandedBar(true, 64, 120), 184, "large font expanded bar");
        near(HeaderLayout.collapse(true, 150, 120), 120, "large font collapse");
        near(HeaderLayout.fraction(true, 60, 120), .5f, "large font fraction");
        // Trailing controls track the headline center, then stay on the toolbar.
        check(HeaderLayout.actionTopPx(176, 24, 64, 34, 28, 44) == 109, "expanded action beside title");
        check(HeaderLayout.actionTopPx(88, 24, 64, 34, 28, 44) == 34, "collapsed action on toolbar");
        check(HeaderLayout.actionTopPx(110, 24, 64, 34, 28, 44) == 43, "action follows headline before pinning");

        float density = 3f;
        int statusPx = Math.round(24f * density); // 72 px
        int barPx = Math.round(HeaderLayout.BAR * density); // 192 px

        // Regular bar: never collapses, toolbar stays below status bar, title stays opaque.
        near(HeaderLayout.collapse(false, 40f), 0f, "regular collapse");
        near(HeaderLayout.fraction(false, 0f), 1f, "regular fraction");
        near(HeaderLayout.expandedBar(false, 56f), 64f, "regular expandedBar floor");
        near(HeaderLayout.expandedBar(false, 72f), 72f, "regular expandedBar tall");
        int regularFull = Math.round((24f + 64f) * density);
        check(HeaderLayout.barTopPx(regularFull, 64f, density) == statusPx, "regular barTopPx");
        check(HeaderLayout.containerHeightPx(regularFull, 0f, density) == regularFull, "regular containerHeightPx");
        check(HeaderLayout.largeBandHeightPx(regularFull, statusPx, barPx) == 0, "regular largeBandHeightPx");
        near(HeaderLayout.toolbarTitleAlpha(false, 1f), 1f, "regular toolbarTitleAlpha");

        // Large bar: 152 dp expanded (64 dp pinned toolbar + 88 dp large band).
        float expanded = HeaderLayout.expandedBar(true, 56f);
        near(expanded, 152f, "large expandedBar");
        int largeFull = Math.round((24f + 152f) * density); // 528 px
        int barTop = HeaderLayout.barTopPx(largeFull, expanded, density);
        check(barTop == statusPx, "large barTopPx pinned below status bar");

        // At top and pulled past top (pull to refresh): fully expanded.
        for (float scrolled : new float[]{-30f, 0f, Float.NaN}) {
            float c = HeaderLayout.collapse(true, scrolled);
            near(c, 0f, "expanded collapse " + scrolled);
            float f = HeaderLayout.fraction(true, c);
            near(f, 0f, "expanded fraction");
            int h = HeaderLayout.containerHeightPx(largeFull, c, density);
            check(h == largeFull, "expanded containerHeightPx");
            check(HeaderLayout.largeBandHeightPx(h, barTop, barPx) == Math.round(88f * density), "expanded bandHeight");
            near(HeaderLayout.largeTitleAlpha(f), 1f, "expanded largeTitleAlpha");
            near(HeaderLayout.toolbarTitleAlpha(true, f), 0f, "expanded toolbarTitleAlpha");
        }

        // First phase (fraction 0.3): large title fading out, toolbar title still hidden, toolbar still pinned.
        float c30 = HeaderLayout.collapse(true, 88f * 0.3f);
        float f30 = HeaderLayout.fraction(true, c30);
        near(f30, 0.3f, "f30");
        int h30 = HeaderLayout.containerHeightPx(largeFull, c30, density);
        check(HeaderLayout.barTopPx(largeFull, expanded, density) == statusPx, "barTopPx stays pinned mid-scroll");
        check(HeaderLayout.largeBandHeightPx(h30, barTop, barPx) == h30 - (statusPx + barPx), "bandHeight at 0.3");
        near(HeaderLayout.largeTitleAlpha(f30), 0.5f, "largeTitleAlpha at 0.3");
        near(HeaderLayout.toolbarTitleAlpha(true, f30), 0f, "toolbarTitleAlpha at 0.3");

        // Crossfade handoff (fraction 0.6): neither title overlaps the other.
        near(HeaderLayout.largeTitleAlpha(HeaderLayout.CROSSFADE), 0f, "largeTitleAlpha at handoff");
        near(HeaderLayout.toolbarTitleAlpha(true, HeaderLayout.CROSSFADE), 0f, "toolbarTitleAlpha at handoff");

        // Second phase (fraction 0.8): large title gone, toolbar title fading in.
        float c80 = HeaderLayout.collapse(true, 88f * 0.8f);
        float f80 = HeaderLayout.fraction(true, c80);
        near(HeaderLayout.largeTitleAlpha(f80), 0f, "largeTitleAlpha at 0.8");
        near(HeaderLayout.toolbarTitleAlpha(true, f80), 0.5f, "toolbarTitleAlpha at 0.8");

        // Fully collapsed (at and past 88 dp): height shrinks to status + 64 dp, toolbar title opaque.
        for (float scrolled : new float[]{88f, 320f}) {
            float c = HeaderLayout.collapse(true, scrolled);
            near(c, 88f, "collapsed " + scrolled);
            float f = HeaderLayout.fraction(true, c);
            near(f, 1f, "collapsed fraction");
            int h = HeaderLayout.containerHeightPx(largeFull, c, density);
            check(h == regularFull, "collapsed containerHeightPx matches regular bar");
            check(HeaderLayout.largeBandHeightPx(h, barTop, barPx) == 0, "collapsed bandHeight is 0");
            near(HeaderLayout.largeTitleAlpha(f), 0f, "collapsed largeTitleAlpha");
            near(HeaderLayout.toolbarTitleAlpha(true, f), 1f, "collapsed toolbarTitleAlpha");
        }

        // Tall custom header bar (76 dp): keeps 88 dp large collapse travel and pinned status bar offset.
        float tallExpanded = HeaderLayout.expandedBar(true, 76f);
        near(tallExpanded, 164f, "tall expandedBar");
        int tallFull = Math.round((24f + 164f) * density);
        int tallBarTop = HeaderLayout.barTopPx(tallFull, tallExpanded, density);
        int tallBarPx = Math.round(76f * density);
        check(tallBarTop == statusPx, "tall barTopPx");
        check(HeaderLayout.largeBandHeightPx(tallFull, tallBarTop, tallBarPx) == Math.round(88f * density), "tall expanded band");
        int tallCollapsedH = HeaderLayout.containerHeightPx(tallFull, 88f, density);
        check(HeaderLayout.largeBandHeightPx(tallCollapsedH, tallBarTop, tallBarPx) == 0, "tall collapsed band");

        // Alpha modulation preserves RGB and multiplies existing alpha.
        check(HeaderLayout.withAlpha(0xff112233, 0f) == 0x00112233, "withAlpha 0");
        check(HeaderLayout.withAlpha(0xff112233, 0.5f) == 0x80112233, "withAlpha 0.5");
        check(HeaderLayout.withAlpha(0xff112233, 1f) == 0xff112233, "withAlpha 1");
        check(HeaderLayout.withAlpha(0x80112233, 0.5f) == 0x40112233, "withAlpha translucent");

        // Header background resolution: custom background wins; else transparent=0, opaque=theme.
        check(HeaderLayout.backgroundColor(0, true, 0xffffffff) == 0, "transparent default");
        check(HeaderLayout.backgroundColor(0, false, 0xffffffff) == 0xffffffff, "opaque default light");
        check(HeaderLayout.backgroundColor(0, false, 0xff000000) == 0xff000000, "opaque default dark");
        check(HeaderLayout.backgroundColor(0xff112233, true, 0xffffffff) == 0xff112233, "transparent respects custom opaque bg");
        check(HeaderLayout.backgroundColor(0x80112233, true, 0xffffffff) == 0x80112233, "transparent respects custom translucent bg");
        check(HeaderLayout.backgroundColor(0xff112233, false, 0xffffffff) == 0xff112233, "opaque respects custom bg");
    }
}
