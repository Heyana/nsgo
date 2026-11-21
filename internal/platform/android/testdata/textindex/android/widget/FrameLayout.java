package android.widget;

import android.content.Context;
import android.graphics.Canvas;
import android.view.MotionEvent;

public class FrameLayout {
    private final Context context;

    public FrameLayout(Context context) {
        this.context = context;
    }

    public Context getContext() { return context; }
    public void setWillNotDraw(boolean willNotDraw) {}
    public void setClipChildren(boolean clipChildren) {}
    public void invalidate() {}
    public boolean onTouchEvent(MotionEvent e) { return true; }
    protected void onDraw(Canvas canvas) {}
}
