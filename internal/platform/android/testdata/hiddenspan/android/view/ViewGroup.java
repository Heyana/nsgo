package android.view;

import android.content.Context;

public class ViewGroup extends View {
    public boolean clipChildren = true;
    public boolean clipToPadding = true;

    public ViewGroup(Context context) {
        super(context);
    }

    public void setClipChildren(boolean clipChildren) {
        this.clipChildren = clipChildren;
    }

    public void setClipToPadding(boolean clipToPadding) {
        this.clipToPadding = clipToPadding;
    }
}
