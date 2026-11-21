package android.view;

import android.content.Context;
import android.graphics.Canvas;

public class View {
    private final Context context;

    public View(Context context) {
        this.context = context;
    }

    public Context getContext() { return context; }
    public final int getWidth() { return 0; }
    public final int getHeight() { return 0; }
    public final ViewParent getParent() { return null; }
    public void setWillNotDraw(boolean willNotDraw) {}
    public void invalidate() {}
    public boolean onTouchEvent(MotionEvent e) { return true; }
    protected void onSizeChanged(int w, int h, int oldw, int oldh) {}
    protected void onDraw(Canvas canvas) {}
}
