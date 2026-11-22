package android.view;

public class MotionEvent {
    public static final int ACTION_DOWN = 0, ACTION_UP = 1, ACTION_MOVE = 2;

    public final int action;
    public final float x, y;
    public final int meta;

    private MotionEvent(int action, float x, float y, int meta) {
        this.action = action;
        this.x = x;
        this.y = y;
        this.meta = meta;
    }

    public static MotionEvent obtain(long down, long time, int action, float x, float y, int meta) {
        return new MotionEvent(action, x, y, meta);
    }

    public void setSource(int source) {}
    public void recycle() {}
}
