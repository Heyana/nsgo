package android.view;

import android.content.Context;

public class View {
    private final Context context;
    private int width, height;

    public View(Context context) { this.context = context; }
    public Context getContext() { return context; }
    public int getWidth() { return width; }
    public int getHeight() { return height; }
    public void invalidate() {}
    public boolean onTouchEvent(MotionEvent e) { return false; }

    protected void onSizeChanged(int w, int h, int oldw, int oldh) {
        this.width = w;
        this.height = h;
    }
}
