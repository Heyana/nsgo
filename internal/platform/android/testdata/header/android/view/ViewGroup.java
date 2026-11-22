package android.view;

import android.content.Context;
import java.util.ArrayList;
import java.util.List;

public class ViewGroup extends View {
    public static class LayoutParams {
        public static final int MATCH_PARENT = -1;
        public static final int WRAP_CONTENT = -2;
        public int width;
        public int height;
        public LayoutParams(int w, int h) { this.width = w; this.height = h; }
    }

    public static class MarginLayoutParams extends LayoutParams {
        public int leftMargin, topMargin, rightMargin, bottomMargin;
        private int marginEnd;
        public MarginLayoutParams(int w, int h) { super(w, h); }
        public void setMarginEnd(int end) { this.marginEnd = end; this.rightMargin = end; }
        public int getMarginEnd() { return marginEnd; }
    }

    protected final List<View> children = new ArrayList<>();

    public ViewGroup(Context context) { super(context); }
    public int getChildCount() { return children.size(); }
    public View getChildAt(int i) { return children.get(i); }
    public void addView(View v, LayoutParams p) {
        v.setLayoutParams(p);
        v.parent = this;
        children.add(v);
    }
    public void removeView(View v) {
        if (children.remove(v)) v.parent = null;
    }
    public void removeViewAt(int i) {
        View v = children.remove(i);
        if (v != null) v.parent = null;
    }
    public void setClipChildren(boolean clip) {}
}
