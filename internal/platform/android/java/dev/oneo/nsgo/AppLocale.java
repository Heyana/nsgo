package dev.oneo.nsgo;

import android.app.LocaleManager;
import android.content.Context;
import android.content.SharedPreferences;
import android.content.res.Configuration;
import android.os.Build;
import android.os.LocaleList;
import java.util.Locale;

/**
 * Applies a standalone app's forced language to Android 13+'s per-app locale
 * ({@link LocaleManager#setApplicationLocales}) so system text selection menus,
 * dialogs and platform widgets follow {@code Scope.SetLocale}.
 *
 * <p>Tracks the language NSGo itself last wrote in private preferences so that
 * clearing a forced language restores the system locale without overwriting a
 * per-app language the user chose in Android Settings, and so the reported
 * environment locale keeps the user's real system language while NSGo's pin is
 * active.
 */
final class AppLocale {
    static final String PREFS = "dev.oneo.nsgo.locale";
    static final String KEY_PINNED = "pinned";

    private AppLocale() {}

    /** Normalizes a forced BCP 47 tag, returning "" for empty or pseudo locales. */
    static String normalize(String lang) {
        if (lang == null) return "";
        String s = lang.trim().replace('_', '-');
        if (s.isEmpty() || "en-XA".equalsIgnoreCase(s) || "ar-XB".equalsIgnoreCase(s)) {
            return "";
        }
        return s;
    }

    /**
     * Pins {@code lang} through {@link LocaleManager} on API 33+, or clears
     * NSGo's own pin when {@code lang} is empty or a pseudo locale.
     */
    static void pin(Context context, String lang) {
        if (Build.VERSION.SDK_INT < 33 || context == null) return;
        LocaleManager lm = context.getSystemService(LocaleManager.class);
        if (lm == null) return;
        SharedPreferences prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        String pinned = prefs.getString(KEY_PINNED, "");
        if (pinned == null) pinned = "";
        LocaleList current = lm.getApplicationLocales();
        String norm = normalize(lang);
        LocaleList wanted = norm.isEmpty() ? null : LocaleList.forLanguageTags(norm);
        if (wanted != null && !wanted.isEmpty()) {
            String tag = wanted.toLanguageTags();
            if (!wanted.equals(current)) {
                lm.setApplicationLocales(wanted);
            }
            if (!tag.equals(pinned)) {
                prefs.edit().putString(KEY_PINNED, tag).apply();
            }
        } else if (!pinned.isEmpty()) {
            LocaleList pinnedList = LocaleList.forLanguageTags(pinned);
            if (current != null && !current.isEmpty() && current.equals(pinnedList)) {
                lm.setApplicationLocales(LocaleList.getEmptyLocaleList());
            }
            prefs.edit().remove(KEY_PINNED).apply();
        }
    }

    /**
     * Reports whether {@link LocaleManager#getApplicationLocales()} currently
     * holds the language NSGo itself pinned (rather than a user choice in
     * Android Settings).
     */
    static boolean isOwnPin(Context context) {
        if (Build.VERSION.SDK_INT < 33 || context == null) return false;
        SharedPreferences prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        String pinned = prefs.getString(KEY_PINNED, "");
        if (pinned == null || pinned.isEmpty()) return false;
        LocaleManager lm = context.getSystemService(LocaleManager.class);
        if (lm == null) return false;
        LocaleList current = lm.getApplicationLocales();
        if (current == null || current.isEmpty()) return false;
        return current.equals(LocaleList.forLanguageTags(pinned));
    }

    /**
     * Returns the user's locale to report in the environment: when a standalone
     * app's {@link LocaleManager} currently holds NSGo's own pin, returns the
     * system locale so clearing {@code SetLocale} restores it at once;
     * otherwise returns the configuration's first locale.
     */
    static Locale userLocale(Context context, Configuration config, boolean standalone) {
        if (standalone && isOwnPin(context)) {
            LocaleManager lm = context.getSystemService(LocaleManager.class);
            if (lm != null) {
                LocaleList sys = lm.getSystemLocales();
                if (sys != null && !sys.isEmpty()) {
                    return sys.get(0);
                }
            }
        }
        return config.getLocales().get(0);
    }
}
