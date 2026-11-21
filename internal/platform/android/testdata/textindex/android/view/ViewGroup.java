package android.view;

import android.content.Context;

public abstract class ViewGroup extends View implements ViewParent {
    public ViewGroup(Context context) {
        super(context);
    }

    public void setClipChildren(boolean clipChildren) {}
    public void setClipToPadding(boolean clipToPadding) {}
}
