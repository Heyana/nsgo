package android.widget;

import android.content.Context;
import android.view.ViewGroup;

public class FrameLayout extends ViewGroup {
    public static class LayoutParams {
        public int topMargin;

        public LayoutParams(int width, int height, int gravity) {}
    }

    public FrameLayout(Context context) {
        super(context);
    }

    protected void onLayout(boolean changed, int l, int t, int r, int b) {}
}
