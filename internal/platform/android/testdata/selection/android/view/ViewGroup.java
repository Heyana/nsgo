package android.view;

import android.content.Context;
import java.util.ArrayList;
import java.util.List;

public class ViewGroup extends View {
    private final List<View> children = new ArrayList<>();

    public ViewGroup(Context context) {
        super(context, 0, 0);
    }

    public int getChildCount() { return children.size(); }

    public View getChildAt(int index) { return children.get(index); }

    public void addView(View view) {
        children.add(view);
        view.parent = this;
    }

    public void removeView(View view) {
        children.remove(view);
        view.parent = null;
    }

    public ActionMode startActionMode(ActionMode.Callback2 callback, int type) {
        throw new UnsupportedOperationException("no floating toolbar on the JVM");
    }
}
