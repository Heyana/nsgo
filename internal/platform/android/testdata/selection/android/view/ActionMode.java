package android.view;

import android.graphics.Rect;

public abstract class ActionMode {
    public static final int TYPE_FLOATING = 1;

    public abstract void finish();

    public abstract void invalidate();

    public abstract void invalidateContentRect();

    public abstract static class Callback2 {
        public abstract boolean onCreateActionMode(ActionMode mode, Menu menu);

        public abstract boolean onPrepareActionMode(ActionMode mode, Menu menu);

        public abstract boolean onActionItemClicked(ActionMode mode, MenuItem item);

        public abstract void onDestroyActionMode(ActionMode mode);

        public abstract void onGetContentRect(ActionMode mode, View view, Rect out);
    }
}
