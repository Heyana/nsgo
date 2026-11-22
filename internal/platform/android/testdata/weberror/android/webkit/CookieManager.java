package android.webkit;

public class CookieManager {
    private static final CookieManager INSTANCE = new CookieManager();

    public static CookieManager getInstance() {
        return INSTANCE;
    }

    public String getCookie(String url) {
        return "";
    }
}
