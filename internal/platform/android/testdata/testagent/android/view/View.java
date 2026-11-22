package android.view;

import android.content.Context;
import android.graphics.Canvas;
import android.os.Handler;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputConnection;

public class View {
    public static final int VISIBLE = 0;

    private final Context context;

    public View() { this.context = null; }
    public View(Context c) { this.context = c; }

    public Context getContext() { return context; }
    public int getVisibility() { return VISIBLE; }
    public void getLocationInWindow(int[] at) { at[0] = 0; at[1] = 0; }
    public int getWidth() { return 100; }
    public int getHeight() { return 100; }
    public int getLeft() { return 0; }
    public int getTop() { return 0; }
    public float getTranslationX() { return 0; }
    public float getTranslationY() { return 0; }
    public int getScrollX() { return 0; }
    public int getScrollY() { return 0; }
    public float getAlpha() { return 1; }
    public float getScaleX() { return 1; }
    public boolean canScrollHorizontally(int d) { return false; }
    public boolean canScrollVertically(int d) { return false; }
    public boolean post(Runnable r) { Handler.QUEUE.addLast(r); return true; }
    public void postOnAnimation(Runnable r) { Handler.QUEUE.addLast(r); }
    public InputConnection onCreateInputConnection(EditorInfo info) { return null; }
    public boolean dispatchKeyEvent(KeyEvent e) { return false; }
    public void draw(Canvas c) {}
}
