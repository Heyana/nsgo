package android.widget;

import android.content.Context;
import android.view.ViewGroup;

public class FrameLayout extends ViewGroup {
    public static class LayoutParams extends ViewGroup.MarginLayoutParams {
        public int gravity;
        public LayoutParams(int w, int h) { super(w, h); }
        public LayoutParams(int w, int h, int g) { super(w, h); this.gravity = g; }
    }

    public FrameLayout(Context context) { super(context); }
}
