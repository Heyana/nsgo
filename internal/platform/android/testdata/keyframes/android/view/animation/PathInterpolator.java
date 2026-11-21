package android.view.animation;

import dev.oneo.nsgo.KeyframesCheck;

/** Supplies the runtime's JVM-safe cubic solver in place of Android's path sampler. */
public final class PathInterpolator {
    private final float x1, y1, x2, y2;

    public PathInterpolator(float x1, float y1, float x2, float y2) {
        this.x1 = x1;
        this.y1 = y1;
        this.x2 = x2;
        this.y2 = y2;
    }

    public float getInterpolation(float x) {
        return KeyframesCheck.interpolate(x1, y1, x2, y2, x);
    }
}
