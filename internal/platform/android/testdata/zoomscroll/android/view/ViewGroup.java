package android.view;

import android.content.Context;
import java.util.ArrayList;
import java.util.List;

public class ViewGroup extends View implements ViewParent {
    public static class LayoutParams {
        public static final int MATCH_PARENT = -1;

        public LayoutParams(int width, int height) {}
    }

    private final List<View> children = new ArrayList<>();
    private View firstTouchTarget;
    private boolean disallowIntercept;

    public ViewGroup(Context context) {
        super(context);
    }

    public void addView(View child) {
        child.parent = this;
        children.add(child);
    }

    public void addView(View child, Object params) {
        addView(child);
    }

    public void removeView(View child) {
        children.remove(child);
        if (child.parent == this) child.parent = null;
    }

    public void setClipToPadding(boolean clip) {}

    public void setClipChildren(boolean clip) {}

    @Override
    public void requestDisallowInterceptTouchEvent(boolean disallowIntercept) {
        this.disallowIntercept = disallowIntercept;
    }

    public boolean onInterceptTouchEvent(MotionEvent e) {
        return false;
    }

    @Override
    public boolean dispatchTouchEvent(MotionEvent e) {
        int action = e.getActionMasked();
        if (action == MotionEvent.ACTION_DOWN) {
            firstTouchTarget = null;
            disallowIntercept = false;
        }
        boolean intercepted;
        if (action == MotionEvent.ACTION_DOWN || firstTouchTarget != null) {
            intercepted = !disallowIntercept && onInterceptTouchEvent(e);
        } else {
            intercepted = true;
        }
        if (action == MotionEvent.ACTION_DOWN && !intercepted) {
            for (int i = children.size() - 1; i >= 0; i--) {
                View child = children.get(i);
                if (child.dispatchTouchEvent(e)) {
                    firstTouchTarget = child;
                    return true;
                }
            }
        }
        if (firstTouchTarget == null) {
            return super.dispatchTouchEvent(e);
        }
        if (intercepted) {
            MotionEvent cancel = new MotionEvent(MotionEvent.ACTION_CANCEL, e.getX(), e.getY(), e.getEventTime());
            firstTouchTarget.dispatchTouchEvent(cancel);
            firstTouchTarget = null;
            return true;
        }
        boolean handled = firstTouchTarget.dispatchTouchEvent(e);
        if (action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL) {
            firstTouchTarget = null;
        }
        return handled;
    }
}
