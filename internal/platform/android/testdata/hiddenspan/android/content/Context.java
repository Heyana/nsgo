package android.content;

import android.util.TypedValue;

public class Context {
    public static final class Theme {
        public boolean resolveAttribute(int id, TypedValue v, boolean resolve) { return false; }
    }
    public Theme getTheme() { return new Theme(); }
    public int getColor(int id) { return 0; }
}
