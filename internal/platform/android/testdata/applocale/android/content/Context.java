package android.content;

import android.app.LocaleManager;
import java.util.HashMap;
import java.util.Map;

public class Context {
    public static final int MODE_PRIVATE = 0;

    private final LocaleManager localeManager;
    private final Map<String, SharedPreferences> prefs = new HashMap<>();

    public Context(LocaleManager localeManager) {
        this.localeManager = localeManager;
    }

    @SuppressWarnings("unchecked")
    public <T> T getSystemService(Class<T> serviceClass) {
        if (serviceClass == LocaleManager.class) return (T) localeManager;
        return null;
    }

    public SharedPreferences getSharedPreferences(String name, int mode) {
        return prefs.computeIfAbsent(name, k -> new SharedPreferences.InMemory());
    }
}
