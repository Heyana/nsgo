package android.app;

import android.os.LocaleList;

public class LocaleManager {
    private LocaleList appLocales = LocaleList.getEmptyLocaleList();
    private LocaleList systemLocales = LocaleList.forLanguageTags("en-US");
    public int setCalls;

    public LocaleList getApplicationLocales() {
        return appLocales;
    }

    public void setApplicationLocales(LocaleList locales) {
        setCalls++;
        appLocales = locales == null ? LocaleList.getEmptyLocaleList() : locales;
    }

    public LocaleList getSystemLocales() {
        return systemLocales;
    }

    public void setSystemLocalesForTest(LocaleList locales) {
        systemLocales = locales;
    }

    public void setApplicationLocalesByUserForTest(LocaleList locales) {
        appLocales = locales == null ? LocaleList.getEmptyLocaleList() : locales;
    }
}
