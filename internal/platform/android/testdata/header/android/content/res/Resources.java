package android.content.res;

import android.util.TypedValue;

public class Resources {
    public static Resources getSystem() { return new Resources(); }
    public int getIdentifier(String name, String defType, String defPackage) { return 0; }
    public String getString(int id) { return ""; }

    public static class Theme {
        public boolean resolveAttribute(int resid, TypedValue outValue, boolean resolveRefs) { return false; }
    }
}
