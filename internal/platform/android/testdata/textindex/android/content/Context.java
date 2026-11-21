package android.content;

import android.util.TypedValue;

public class Context {
    public static final class Theme {
        public boolean resolveAttribute(int resid, TypedValue outValue, boolean resolveRefs) {
            return false;
        }
    }

    private final Theme theme = new Theme();

    public Theme getTheme() {
        return theme;
    }

    public int getColor(int id) {
        return 0;
    }
}
