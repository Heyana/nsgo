package android.content;

import android.content.res.Resources;

public class Context {
    public int checkSelfPermission(String permission) {
        return 0;
    }

    public Resources.Theme getTheme() {
        return new Resources.Theme();
    }
}
