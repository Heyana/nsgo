package android.widget;

import android.content.Context;
import android.graphics.drawable.Drawable;
import android.view.View;
import android.view.ViewGroup;

/** Mirrors AOSP Toolbar's child-order widthUsed measurement for custom views. */
public class Toolbar extends ViewGroup {
    public static class LayoutParams extends ViewGroup.MarginLayoutParams {
        public int gravity;
        public LayoutParams(int w, int h, int g) { super(w, h); this.gravity = g; }
    }

    private Drawable navIcon;
    private int navButtonWidth;

    public Toolbar(Context context) { super(context); }
    public void setNavButtonWidth(int px) { this.navButtonWidth = px; }
    public void setNavigationOnClickListener(View.OnClickListener l) {}
    public void setNavigationIcon(Drawable icon) { this.navIcon = icon; }
    public void setNavigationContentDescription(CharSequence s) {}
    public void setTitle(CharSequence title) {}
    public void setTitleTextColor(int color) {}
    public int getContentInsetStart() { return 0; }
    public int getContentInsetEnd() { return 0; }
    public void setContentInsetsRelative(int start, int end) {}

    @Override
    protected void onMeasure(int widthSpec, int heightSpec) {
        int totalWidth = MeasureSpec.getSize(widthSpec);
        int widthUsed = getPaddingLeft() + getPaddingRight() + (navIcon != null ? navButtonWidth : 0);
        int maxChildHeight = 0;
        for (int i = 0; i < getChildCount(); i++) {
            View child = getChildAt(i);
            MarginLayoutParams lp = (MarginLayoutParams) child.getLayoutParams();
            int hMargins = lp.leftMargin + lp.rightMargin;
            int childW = lp.width == LayoutParams.MATCH_PARENT
                    ? Math.max(0, totalWidth - widthUsed - hMargins)
                    : Math.max(0, lp.width);
            int childH = Math.max(0, lp.height);
            child.measure(
                    MeasureSpec.makeMeasureSpec(childW, MeasureSpec.EXACTLY),
                    MeasureSpec.makeMeasureSpec(childH, MeasureSpec.EXACTLY));
            widthUsed += child.getMeasuredWidth() + hMargins;
            if (child.getMeasuredHeight() > maxChildHeight) {
                maxChildHeight = child.getMeasuredHeight();
            }
        }
        measuredWidth = totalWidth;
        measuredHeight = getPaddingTop() + getPaddingBottom() + maxChildHeight;
    }
}
