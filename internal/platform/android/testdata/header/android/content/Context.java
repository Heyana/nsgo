package android.content;

import android.content.res.Resources;

public class Context {
    public Resources.Theme getTheme() { return new Resources.Theme(); }
    public android.content.res.ColorStateList getColorStateList(int id) { return new android.content.res.ColorStateList(); }
}
