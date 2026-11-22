package android.view;

import android.content.Context;
import java.util.ArrayList;
import java.util.List;

public class ViewGroup extends View {
    private final List<View> children = new ArrayList<>();

    public ViewGroup() { super(); }
    public ViewGroup(Context c) { super(c); }

    public void addChild(View v) { children.add(v); }
    public int getChildCount() { return children.size(); }
    public View getChildAt(int i) { return children.get(i); }
}
