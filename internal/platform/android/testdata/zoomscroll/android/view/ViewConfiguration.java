package android.view;

import android.content.Context;

public class ViewConfiguration {
    public static ViewConfiguration get(Context context) {
        return new ViewConfiguration();
    }

    public int getScaledTouchSlop() {
        return 8;
    }

    public static float getScrollFriction() {
        return 0.015f;
    }
}
