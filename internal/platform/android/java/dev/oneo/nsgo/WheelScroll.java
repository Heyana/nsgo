package dev.oneo.nsgo;

/**
 * Tracks wheel turns and touchpad scrolls for View.OnWheel on Android.
 *
 * Mouse wheel turns report GestureChanged directly at three lines of 33 points
 * (99 points) a notch. Touchpad scrolls (SOURCE_TOUCHPAD) have no native
 * begin/end action or momentum events on Android, so this tracker frames a
 * gesture as GESTURE_BEGAN -> GESTURE_CHANGED -> GESTURE_ENDED and computes
 * the release velocity (points per second over the last WINDOW_MS) when the
 * fingers lift so the view glides on via Decay, as on iOS and Linux.
 */
final class WheelScroll {
    /** Milliseconds without touchpad scroll events before the fingers are taken as lifted. */
    static final long IDLE_MS = 80;
    /** Sliding window over which touchpad release velocity is measured, in milliseconds. */
    static final long WINDOW_MS = 100;
    /** Content distance moved by one discrete wheel notch, in points (3 lines of 33pt). */
    static final float NOTCH = 99f;
    /** Minimum speed (points per second) to glide after lifting; slower lifts settle at rest. */
    static final float MIN_VELOCITY = 10f;

    private static final int SAMPLES = 16;

    interface Sink {
        void emit(int nodeId, int phase, float x, float y, float pageX, float pageY,
                float dx, float dy, float vx, float vy, boolean touchpad, int mods);
    }

    private final Sink sink;
    private int activeNode;
    private float lastX, lastY, lastPageX, lastPageY;
    private int lastMods;
    private float totalX, totalY;
    private final long[] times = new long[SAMPLES];
    private final float[] xs = new float[SAMPLES];
    private final float[] ys = new float[SAMPLES];
    private int count, head;

    WheelScroll(Sink sink) {
        this.sink = sink;
    }

    /** Returns the node currently latched to a touchpad gesture, or 0 if idle. */
    int activeNode() {
        return activeNode;
    }

    /** Processes one ACTION_SCROLL event over nodeId at timeMs. */
    void scroll(int nodeId, boolean touchpad, float hscroll, float vscroll,
            float x, float y, float pageX, float pageY, int mods, long timeMs) {
        if (!touchpad) {
            if (activeNode != 0) end(false);
            // Positive axes show what is above or right, as a wheel turned away or tilted right does.
            float dx = -hscroll * NOTCH, dy = vscroll * NOTCH;
            sink.emit(nodeId, Protocol.GESTURE_CHANGED, x, y, pageX, pageY, dx, dy, 0, 0, false, mods);
            return;
        }
        float dx = -hscroll, dy = vscroll;
        if (activeNode != 0 && activeNode != nodeId) {
            end(true);
        }
        boolean began = activeNode == 0;
        if (began) {
            activeNode = nodeId;
            totalX = 0;
            totalY = 0;
            count = 0;
            head = 0;
        }
        totalX += dx;
        totalY += dy;
        addSample(timeMs, totalX, totalY);
        lastX = x;
        lastY = y;
        lastPageX = pageX;
        lastPageY = pageY;
        lastMods = mods;
        sink.emit(nodeId, began ? Protocol.GESTURE_BEGAN : Protocol.GESTURE_CHANGED,
                x, y, pageX, pageY, dx, dy, 0, 0, true, mods);
    }

    /** Ends an active touchpad gesture after the idle timeout, reporting its release velocity. */
    boolean end() {
        return end(true);
    }

    private boolean end(boolean glide) {
        if (activeNode == 0) return false;
        int node = activeNode;
        float vx = 0, vy = 0;
        if (glide) {
            float[] v = speed();
            vx = v[0];
            vy = v[1];
        }
        activeNode = 0;
        count = 0;
        head = 0;
        sink.emit(node, Protocol.GESTURE_ENDED, lastX, lastY, lastPageX, lastPageY,
                0, 0, vx, vy, true, lastMods);
        return true;
    }

    /** Drops any active touchpad gesture for nodeId (or all if nodeId == 0) without emitting. */
    void cancel(int nodeId) {
        if (activeNode != 0 && (nodeId == 0 || activeNode == nodeId)) {
            activeNode = 0;
            count = 0;
            head = 0;
        }
    }

    private void addSample(long timeMs, float x, float y) {
        times[head] = timeMs;
        xs[head] = x;
        ys[head] = y;
        head = (head + 1) % SAMPLES;
        if (count < SAMPLES) count++;
    }

    /** Release velocity in points per second over the last WINDOW_MS, or (0, 0) when paused/slow. */
    private float[] speed() {
        if (count < 2) return new float[] {0, 0};
        int newestIdx = (head + SAMPLES - 1) % SAMPLES;
        long newestTime = times[newestIdx];
        int oldestIdx = newestIdx;
        for (int i = 1; i < count; i++) {
            int idx = (head + SAMPLES - 1 - i) % SAMPLES;
            if (newestTime - times[idx] > WINDOW_MS) break;
            oldestIdx = idx;
        }
        long dt = newestTime - times[oldestIdx];
        if (dt < 1) return new float[] {0, 0};
        float vx = (xs[newestIdx] - xs[oldestIdx]) * 1000f / dt;
        float vy = (ys[newestIdx] - ys[oldestIdx]) * 1000f / dt;
        if (Math.hypot(vx, vy) < MIN_VELOCITY) return new float[] {0, 0};
        return new float[] {vx, vy};
    }
}
