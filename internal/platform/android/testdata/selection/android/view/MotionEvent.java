package android.view;

public abstract class MotionEvent {
    public static final int ACTION_DOWN = 0, ACTION_UP = 1, ACTION_MOVE = 2, ACTION_CANCEL = 3;

    public abstract float getRawX();

    public abstract float getRawY();

    public abstract int getActionMasked();
}
