package android.content;

import android.util.TypedValue;

public class Context {
    public static class Theme {
        public boolean resolveAttribute(int resid, TypedValue outValue, boolean resolveRefs) {
            return false;
        }
    }

    public Theme getTheme() {
        return new Theme();
    }

    public int getColor(int id) {
        return 0;
    }
}
