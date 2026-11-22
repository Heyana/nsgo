package android.widget;

import android.view.View;
import android.view.ViewGroup;

public class ListView extends ViewGroup {
    public int getFirstVisiblePosition() { return 0; }
    public long getItemIdAtPosition(int pos) { return pos; }
    public boolean performItemClick(View v, int pos, long id) { return true; }
}
