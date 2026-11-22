package android.view;

import android.content.Context;
import android.content.res.Configuration;
import android.content.res.Resources;

public class View {
    public interface OnTouchListener {
        boolean onTouch(View v, MotionEvent event);
    }

    private final Context context;

    public View(Context context) {
        this.context = context;
    }

    public Context getContext() {
        return context;
    }

    public ViewParent getParent() {
        return null;
    }

    public Resources getResources() {
        return new Resources();
    }

    public void dispatchConfigurationChanged(Configuration newConfig) {}

    public void setVerticalScrollBarEnabled(boolean enabled) {}

    public void setHorizontalScrollBarEnabled(boolean enabled) {}

    public void setOnTouchListener(OnTouchListener l) {}
}
