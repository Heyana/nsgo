package android.view;

import android.graphics.Rect;

public abstract class ActionMode {
    public static final int TYPE_FLOATING = 1;

    public interface Callback {
        boolean onCreateActionMode(ActionMode mode, Menu menu);
        boolean onPrepareActionMode(ActionMode mode, Menu menu);
        boolean onActionItemClicked(ActionMode mode, MenuItem item);
        void onDestroyActionMode(ActionMode mode);
    }

    public abstract static class Callback2 implements Callback {
        public void onGetContentRect(ActionMode mode, View view, Rect outRect) {}
    }

    public void invalidate() {}
    public void invalidateContentRect() {}
    public void finish() {}
}
