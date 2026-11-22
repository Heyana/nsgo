package android.webkit;

import android.content.Context;

public class WebSettings {
    public static final int FORCE_DARK_OFF = 0;
    public static final int FORCE_DARK_AUTO = 1;
    public static final int FORCE_DARK_ON = 2;
    public static final int LOAD_DEFAULT = -1;
    public static final int LOAD_NO_CACHE = 2;

    private boolean jsEnabled = true;

    public void setDomStorageEnabled(boolean flag) {}

    public void setForceDark(int forceDark) {}

    public void setCacheMode(int mode) {}

    public void setSaveFormData(boolean save) {}

    public void setJavaScriptEnabled(boolean flag) {
        jsEnabled = flag;
    }

    public boolean getJavaScriptEnabled() {
        return jsEnabled;
    }

    public void setBuiltInZoomControls(boolean enabled) {}

    public void setDisplayZoomControls(boolean enabled) {}

    public void setUserAgentString(String ua) {}

    public void setAllowFileAccess(boolean allow) {}

    public static String getDefaultUserAgent(Context context) {
        return "NSGoTestWebView/1.0";
    }
}
