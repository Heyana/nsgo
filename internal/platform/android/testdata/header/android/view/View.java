package android.view;

import android.content.Context;

public class View {
    public static final int VISIBLE = 0;
    public static final int INVISIBLE = 4;
    public static final int GONE = 8;
    public static final int LAYOUT_DIRECTION_RTL = 1;
    public static final int TEXT_ALIGNMENT_VIEW_START = 5;
    public static final int IMPORTANT_FOR_ACCESSIBILITY_AUTO = 0;
    public static final int IMPORTANT_FOR_ACCESSIBILITY_YES = 1;
    public static final int IMPORTANT_FOR_ACCESSIBILITY_NO = 2;
    public static final int IMPORTANT_FOR_ACCESSIBILITY_NO_HIDE_DESCENDANTS = 4;

    public interface OnClickListener {
        void onClick(View v);
    }

    public static class MeasureSpec {
        public static final int UNSPECIFIED = 0;
        public static final int EXACTLY = 1 << 30;
        private static final int MODE_MASK = 0x3 << 30;
        public static int makeMeasureSpec(int size, int mode) { return (size & ~MODE_MASK) | (mode & MODE_MASK); }
        public static int getSize(int spec) { return spec & ~MODE_MASK; }
        public static int getMode(int spec) { return spec & MODE_MASK; }
    }

    private final Context context;
    ViewGroup parent;
    ViewGroup.LayoutParams layoutParams;
    private int visibility = VISIBLE;
    private int paddingLeft, paddingTop, paddingRight, paddingBottom;
    protected int measuredWidth, measuredHeight;

    public View(Context context) { this.context = context; }
    public Context getContext() { return context; }
    public ViewGroup getParent() { return parent; }
    public ViewGroup.LayoutParams getLayoutParams() { return layoutParams; }
    public void setLayoutParams(ViewGroup.LayoutParams p) { this.layoutParams = p; }
    public void setOutlineProvider(Object o) {}
    public void setTranslationZ(float z) {}
    public void setTranslationY(float y) {}
    public void setVisibility(int v) { this.visibility = v; }
    public int getVisibility() { return visibility; }
    public void setBackgroundColor(int c) {}
    public void setAlpha(float a) {}
    public void setImportantForAccessibility(int mode) {}
    public void setTextAlignment(int align) {}
    public void setPaddingRelative(int s, int t, int e, int b) { setPadding(s, t, e, b); }
    public void setPadding(int l, int t, int r, int b) {
        paddingLeft = l; paddingTop = t; paddingRight = r; paddingBottom = b;
    }
    public int getPaddingLeft() { return paddingLeft; }
    public int getPaddingTop() { return paddingTop; }
    public int getPaddingRight() { return paddingRight; }
    public int getPaddingBottom() { return paddingBottom; }
    public void measure(int widthSpec, int heightSpec) {
        onMeasure(widthSpec, heightSpec);
    }
    protected void onMeasure(int widthSpec, int heightSpec) {
        measuredWidth = MeasureSpec.getSize(widthSpec);
        measuredHeight = MeasureSpec.getSize(heightSpec);
    }
    public int getMeasuredWidth() { return measuredWidth; }
    public int getMeasuredHeight() { return measuredHeight; }
}
