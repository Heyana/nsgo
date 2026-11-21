package android.view;

import android.content.Context;

public class View {
    public static final int VISIBLE = 0;
    private final Context context;
    private final int x, y;
    ViewGroup parent;

    protected View(Context context, int x, int y) {
        this.context = context;
        this.x = x;
        this.y = y;
    }

    public Context getContext() { return context; }

    public int getVisibility() { return VISIBLE; }

    public boolean isAttachedToWindow() { return true; }

    public ViewGroup getParent() { return parent; }

    public int getLeft() { return x; }

    public int getRight() { return x; }

    public int getTop() { return y; }

    public int getBottom() { return y; }

    public void getLocationInWindow(int[] at) {
        at[0] = x;
        at[1] = y;
    }

    public void getLocationOnScreen(int[] at) { getLocationInWindow(at); }

    public void setLayoutParams(Object params) {}

    public void bringToFront() {}

    public void setOnTouchListener(OnTouchListener listener) {}

    public interface OnTouchListener {
        boolean onTouch(View view, MotionEvent event);
    }
}
