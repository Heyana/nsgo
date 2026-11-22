package android.os;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Locale;

public final class LocaleList {
    private static final LocaleList EMPTY = new LocaleList(new Locale[0]);
    private final Locale[] locales;

    public LocaleList(Locale... list) {
        this.locales = list == null ? new Locale[0] : list.clone();
    }

    public static LocaleList getEmptyLocaleList() {
        return EMPTY;
    }

    public static LocaleList forLanguageTags(String list) {
        if (list == null || list.trim().isEmpty()) return EMPTY;
        String[] parts = list.split(",");
        List<Locale> out = new ArrayList<>();
        for (String p : parts) {
            String t = p.trim();
            if (t.isEmpty()) continue;
            Locale loc = Locale.forLanguageTag(t);
            if (!loc.getLanguage().isEmpty()) out.add(loc);
        }
        return out.isEmpty() ? EMPTY : new LocaleList(out.toArray(new Locale[0]));
    }

    public boolean isEmpty() {
        return locales.length == 0;
    }

    public Locale get(int index) {
        return locales[index];
    }

    public String toLanguageTags() {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < locales.length; i++) {
            if (i > 0) sb.append(',');
            sb.append(locales[i].toLanguageTag());
        }
        return sb.toString();
    }

    @Override
    public boolean equals(Object o) {
        return o instanceof LocaleList && Arrays.equals(locales, ((LocaleList) o).locales);
    }

    @Override
    public int hashCode() {
        return Arrays.hashCode(locales);
    }
}
