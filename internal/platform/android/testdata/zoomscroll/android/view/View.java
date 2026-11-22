package android.view;

import android.content.Context;
import android.content.res.Resources;
import android.graphics.Canvas;
import java.util.ArrayList;
import java.util.List;

public class View {
    public static final int OVER_SCROLL_IF_CONTENT_SCROLLS = 1;
    public static final int OVER_SCROLL_NEVER = 2;
    public static final int SCROLL_AXIS_HORIZONTAL = 1;
    public static final int SCROLL_AXIS_VERTICAL = 2;
    public static final int VISIBLE = 0;
    public static final int GONE = 8;

    public interface OnLayoutChangeListener {
        void onLayoutChange(View v, int left, int top, int right, int bottom, int oldLeft, int oldTop, int oldRight, int oldBottom);
    }

    private final Resources resources = new Resources();
    private final List<OnLayoutChangeListener> layoutListeners = new ArrayList<>();
    private int visibility = VISIBLE;

    private final Context context;
    private int width, height;
    private int scrollX, scrollY;
    private int padL, padT, padR, padB;
    private float translationX, translationY, scaleX = 1, scaleY = 1, pivotX, pivotY;
    ViewParent parent;

    public View(Context context) {
        this.context = context;
    }

    public Context getContext() {
        return context;
    }

    public void setSize(int width, int height) {
        this.width = width;
        this.height = height;
    }

    public int getWidth() {
        return width;
    }

    public int getHeight() {
        return height;
    }

    public int getScrollX() {
        return scrollX;
    }

    public int getScrollY() {
        return scrollY;
    }

    public void scrollTo(int x, int y) {
        int oldX = scrollX, oldY = scrollY;
        if (x != oldX || y != oldY) {
            scrollX = x;
            scrollY = y;
            onScrollChanged(x, y, oldX, oldY);
        }
    }

    protected void onScrollChanged(int l, int t, int oldl, int oldt) {}

    public void setPadding(int l, int t, int r, int b) {
        padL = l;
        padT = t;
        padR = r;
        padB = b;
    }

    public int getPaddingLeft() {
        return padL;
    }

    public int getPaddingTop() {
        return padT;
    }

    public int getPaddingRight() {
        return padR;
    }

    public int getPaddingBottom() {
        return padB;
    }

    public void setTranslationX(float x) {
        translationX = x;
    }

    public void setTranslationY(float y) {
        translationY = y;
    }

    public float getTranslationY() {
        return translationY;
    }

    public void setScaleX(float x) {
        scaleX = x;
    }

    public void setScaleY(float y) {
        scaleY = y;
    }

    public void setPivotX(float x) {
        pivotX = x;
    }

    public void setPivotY(float y) {
        pivotY = y;
    }

    public void setVerticalScrollBarEnabled(boolean enabled) {}

    public void setHorizontalScrollBarEnabled(boolean enabled) {}

    public void setOverScrollMode(int mode) {}

    public void getLocationInWindow(int[] out) {
        out[0] = 0;
        out[1] = 0;
    }

    public boolean post(Runnable r) {
        r.run();
        return true;
    }

    public void postOnAnimation(Runnable r) {}

    public boolean removeCallbacks(Runnable r) {
        return true;
    }

    public void postInvalidateOnAnimation() {}

    public boolean startNestedScroll(int axes) {
        return false;
    }

    public void stopNestedScroll() {}

    public boolean dispatchNestedScroll(int dxConsumed, int dyConsumed, int dxUnconsumed, int dyUnconsumed, int[] offsetInWindow) {
        return false;
    }

    public ViewParent getParent() {
        return parent;
    }

    public ViewPropertyAnimator animate() {
        return new ViewPropertyAnimator();
    }

    public boolean dispatchTouchEvent(MotionEvent e) {
        return onTouchEvent(e);
    }

    public boolean onTouchEvent(MotionEvent e) {
        return false;
    }

    public void computeScroll() {}

    public boolean canScrollHorizontally(int direction) {
        return false;
    }

    public boolean canScrollVertically(int direction) {
        return false;
    }

    public Resources getResources() {
        return resources;
    }

    public void addOnLayoutChangeListener(OnLayoutChangeListener l) {
        layoutListeners.add(l);
    }

    public void removeOnLayoutChangeListener(OnLayoutChangeListener l) {
        layoutListeners.remove(l);
    }

    public void setVisibility(int visibility) {
        this.visibility = visibility;
    }

    public int getVisibility() {
        return visibility;
    }

    public void invalidate() {}

    public void setWillNotDraw(boolean willNotDraw) {}

    public void bringToFront() {}

    protected void onDraw(Canvas canvas) {}
}
