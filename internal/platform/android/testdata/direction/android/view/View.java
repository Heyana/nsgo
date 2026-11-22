package android.view;

import android.content.Context;
import android.content.res.Resources;
import android.graphics.Canvas;
import android.view.accessibility.AccessibilityNodeInfo;

public class View {
    public static final int LAYOUT_DIRECTION_LTR = 0;
    public static final int LAYOUT_DIRECTION_RTL = 1;

    private final Context context;
    private int layoutDirection = LAYOUT_DIRECTION_LTR;
    private int width = 52, height = 32;
    private boolean enabled = true;
    public int invalidations;

    public View(Context context) { this.context = context; }
    public Context getContext() { return context; }
    public Resources getResources() { return context.getResources(); }
    public int getWidth() { return width; }
    public int getHeight() { return height; }
    public void setClickable(boolean clickable) {}
    public void setFocusable(boolean focusable) {}
    public void setEnabled(boolean enabled) { this.enabled = enabled; }
    public boolean isEnabled() { return enabled; }
    public void setBackground(Object bg) {}
    public void requestLayout() {}
    public void invalidate() { invalidations++; }
    public int getLayoutDirection() { return layoutDirection; }
    public void setLayoutDirection(int dir) {
        layoutDirection = dir;
        onRtlPropertiesChanged(dir);
    }
    public void onRtlPropertiesChanged(int layoutDirection) {}
    protected void onMeasure(int widthSpec, int heightSpec) {}
    protected void setMeasuredDimension(int w, int h) { width = w; height = h; }
    public static int resolveSize(int size, int measureSpec) { return size; }
    protected void onDraw(Canvas canvas) {}
    public void drawForTest(Canvas canvas) { onDraw(canvas); }
    public void onInitializeAccessibilityNodeInfo(AccessibilityNodeInfo info) {}
}
