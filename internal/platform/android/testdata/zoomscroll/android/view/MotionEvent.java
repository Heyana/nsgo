package android.view;

public final class MotionEvent {
    public static final int ACTION_DOWN = 0;
    public static final int ACTION_UP = 1;
    public static final int ACTION_MOVE = 2;
    public static final int ACTION_CANCEL = 3;
    public static final int ACTION_POINTER_DOWN = 5;
    public static final int ACTION_POINTER_UP = 6;

    private final int action, actionIndex;
    private final float[] xs, ys;
    private final long eventTime;

    public MotionEvent(int action, float x, float y, long eventTime) {
        this(action, 0, new float[]{x}, new float[]{y}, eventTime);
    }

    /** An event of several fingers; actionIndex is the one a pointer action names. */
    public MotionEvent(int action, int actionIndex, float[] xs, float[] ys, long eventTime) {
        this.action = action;
        this.actionIndex = actionIndex;
        this.xs = xs;
        this.ys = ys;
        this.eventTime = eventTime;
    }

    public int getActionMasked() {
        return action;
    }

    public int getActionIndex() {
        return actionIndex;
    }

    public float getX() {
        return xs[0];
    }

    public float getY() {
        return ys[0];
    }

    public float getX(int i) {
        return xs[i];
    }

    public float getY(int i) {
        return ys[i];
    }

    public float getRawX() {
        return xs[0];
    }

    public float getRawY() {
        return ys[0];
    }

    public int getPointerCount() {
        return xs.length;
    }

    public long getEventTime() {
        return eventTime;
    }
}
