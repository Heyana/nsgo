package dev.oneo.nsgo;

/** The axis-independent scroll calculations from internal/driver/scroll.go. */
final class ScrollPhysics {
    private ScrollPhysics() {}

    /** Releases slower than this (points per millisecond) settle at the nearest snap point. */
    private static final float SLOW = 0.3f;

    /** The snap points of a view snapping along a viewport and content of those lengths. */
    private static float[] snapPoints(float interval, int alignment, float[] offsets, boolean paging, float viewport, float content) {
        float limit = Math.max(content - viewport, 0);
        if (interval > 0) {
            float shift = alignment == 1 ? (viewport - interval) / 2 : alignment == 2 ? viewport - interval : 0;
            int n = Math.max((int) Math.ceil((limit + shift) / interval), 0);
            float[] out = new float[n + 1];
            for (int i = 0; i <= n; i++) out[i] = Math.min(Math.max(i * interval - shift, 0), limit);
            return out;
        }
        if (offsets.length > 0) {
            float[] out = new float[offsets.length];
            for (int i = 0; i < out.length; i++) out[i] = Math.min(Math.max(offsets[i], 0), limit);
            return out;
        }
        if (paging) {
            float page = Math.max(viewport, 1);
            int n = (int) Math.ceil(limit / page);
            float[] out = new float[n + 1];
            for (int i = 0; i <= n; i++) out[i] = Math.min(i * page, limit);
            return out;
        }
        return new float[0];
    }

    /**
     * driver.ScrollBehavior.SnapTarget: the snap point a fling released at
     * offset, which would glide to projected at velocity (points per
     * millisecond), ends at; NaN when the view does not snap.
     */
    static float snapTarget(float interval, int alignment, float[] offsets, boolean paging, boolean oneStep,
            float offset, float projected, float velocity, float viewport, float content) {
        float[] points = snapPoints(interval, alignment, offsets, paging, viewport, content);
        if (points.length == 0) return Float.NaN;
        float limit = Math.max(content - viewport, 0);
        boolean step = oneStep || paging && interval <= 0 && offsets.length == 0;
        float from = step ? offset : projected;
        if (velocity > SLOW) {
            for (float p : points) if (step ? p > from + 0.5f : p >= from - 0.5f) return p;
            return limit;
        }
        if (velocity < -SLOW) {
            for (int i = points.length - 1; i >= 0; i--) {
                float p = points[i];
                if (step ? p < from - 0.5f : p <= from + 0.5f) return p;
            }
            return 0;
        }
        float best = points[0];
        for (float p : points) if (Math.abs(p - from) < Math.abs(best - from)) best = p;
        return best;
    }

    /** Where a glide at velocity from offset comes to rest. */
    static float projected(float offset, float velocity, float rate) {
        return offset - velocity / (float) Math.log(rate);
    }

    /** driver.StickyCell.StickyShift, with an optional animated inset as in driver.Animator.StickyShift. */
    static float stickyShift(float start, float end, float offset, double animatedInset, float inset) {
        if (!Double.isNaN(animatedInset)) offset += (float) animatedInset - inset;
        float travel = end - start;
        return Math.min(Math.max(offset - start, Math.min(travel, 0)), Math.max(travel, 0));
    }
}
