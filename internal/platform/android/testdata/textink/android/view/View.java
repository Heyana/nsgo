package android.view;

import android.content.Context;
import android.graphics.Canvas;

public class View {
    private final Context context;
    public View parent;
    public int width, height;
    public boolean willNotDraw = true;
    public int invalidations;

    public View(Context context) {
        this.context = context;
    }

    public Context getContext() { return context; }
    public int getWidth() { return width; }
    public int getHeight() { return height; }
    public Object getParent() { return parent; }
    public void setWillNotDraw(boolean willNotDraw) { this.willNotDraw = willNotDraw; }
    public void invalidate() { invalidations++; }

    public void size(int w, int h) {
        int oldW = width, oldH = height;
        width = w;
        height = h;
        onSizeChanged(w, h, oldW, oldH);
    }

    protected void onSizeChanged(int w, int h, int oldw, int oldh) {}
    protected void onDraw(Canvas canvas) {}
    public boolean onTouchEvent(MotionEvent e) { return false; }
}
