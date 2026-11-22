package android.content.res;

import android.util.DisplayMetrics;
import android.util.TypedValue;

public class Resources {
    public DisplayMetrics getDisplayMetrics() { return new DisplayMetrics(); }
    public Configuration getConfiguration() { return new Configuration(); }
    public static class Theme {
        public boolean resolveAttribute(int resid, TypedValue outValue, boolean resolveRefs) { return false; }
    }
}
