package android.content.res;

import android.os.LocaleList;

public class Configuration {
    private LocaleList locales;

    public Configuration(LocaleList locales) {
        this.locales = locales;
    }

    public LocaleList getLocales() {
        return locales;
    }

    public void setLocales(LocaleList locales) {
        this.locales = locales;
    }
}
