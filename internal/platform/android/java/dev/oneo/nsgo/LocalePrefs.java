package dev.oneo.nsgo;

import java.io.DataOutputStream;
import java.io.IOException;
import java.util.Calendar;
import java.util.Locale;

/**
 * The user's regional preferences for Environment.LocaleSettings: first day of
 * the week (ISO 8601, 1 Monday to 7 Sunday, 0 unset), hour cycle ("h11", "h12",
 * "h23" or "h24"), measurement system ("metric", "us" or "uk"), temperature
 * unit ("celsius" or "fahrenheit", "" unset) and format region (ISO 3166).
 * Kept free of Android imports so JVM fixtures can exercise it directly.
 */
final class LocalePrefs {
    final int firstDayOfWeek;
    final String hourCycle;
    final String measurementSystem;
    final String temperatureUnit;
    final String region;

    LocalePrefs(int firstDayOfWeek, String hourCycle, String measurementSystem, String temperatureUnit, String region) {
        this.firstDayOfWeek = firstDayOfWeek;
        this.hourCycle = hourCycle;
        this.measurementSystem = measurementSystem;
        this.temperatureUnit = temperatureUnit;
        this.region = region;
    }

    static LocalePrefs read(Locale formatLocale, boolean is24Hour, String icuMeasurementSystem) {
        return read(formatLocale, null, is24Hour, icuMeasurementSystem);
    }

    static LocalePrefs read(Locale formatLocale, Locale displayLocale, boolean is24Hour, String icuMeasurementSystem) {
        String region = regionOf(formatLocale);
        int first = firstDayOfWeek(formatLocale, displayLocale, region);
        String cycle = hourCycle(formatLocale, displayLocale, is24Hour);
        String measure = measurementSystem(formatLocale, displayLocale, icuMeasurementSystem, region);
        String temp = temperatureUnit(formatLocale, displayLocale);
        return new LocalePrefs(first, cycle, measure, temp, region);
    }

    /** Writes the settings after OSVersion in NsgoView.environmentPayload (readEnvironment in protocol.go). */
    void write(DataOutputStream out) throws IOException {
        out.writeByte(firstDayOfWeek);
        Protocol.writeString(out, hourCycle);
        Protocol.writeString(out, measurementSystem);
        Protocol.writeString(out, temperatureUnit);
        Protocol.writeString(out, region);
    }

    private static int firstDayOfWeek(Locale formatLocale, Locale displayLocale, String region) {
        switch (extension(formatLocale, displayLocale, "fw")) {
            case "mon": return 1;
            case "tue": return 2;
            case "wed": return 3;
            case "thu": return 4;
            case "fri": return 5;
            case "sat": return 6;
            case "sun": return 7;
            default: break;
        }
        if (formatLocale != null && !region.isEmpty()) {
            // Calendar numbers Sunday 1 through Saturday 7; ISO 8601 numbers Monday 1 through Sunday 7.
            int first = Calendar.getInstance(formatLocale).getFirstDayOfWeek();
            if (first >= Calendar.SUNDAY && first <= Calendar.SATURDAY) {
                return ((first + 5) % 7) + 1;
            }
        }
        return 0;
    }

    private static String hourCycle(Locale formatLocale, Locale displayLocale, boolean is24Hour) {
        String hc = extension(formatLocale, displayLocale, "hc");
        if (is24Hour) {
            return "h24".equals(hc) ? "h24" : "h23";
        }
        return "h11".equals(hc) ? "h11" : "h12";
    }

    private static String measurementSystem(Locale formatLocale, Locale displayLocale, String icuMeasurementSystem, String region) {
        switch (extension(formatLocale, displayLocale, "ms")) {
            case "metric": return "metric";
            case "ussystem": return "us";
            case "uksystem": return "uk";
            default: break;
        }
        if ("metric".equals(icuMeasurementSystem) || "us".equals(icuMeasurementSystem) || "uk".equals(icuMeasurementSystem)) {
            return icuMeasurementSystem;
        }
        if (region.isEmpty()) {
            return "";
        }
        switch (region) {
            case "US":
            case "LR":
            case "MM":
            case "PR":
            case "AS":
            case "GU":
            case "MP":
            case "VI":
            case "UM":
                return "us";
            case "GB":
                return "uk";
            default:
                return "metric";
        }
    }

    private static String temperatureUnit(Locale formatLocale, Locale displayLocale) {
        switch (extension(formatLocale, displayLocale, "mu")) {
            case "celsius": return "celsius";
            case "fahrenhe":
            case "fahrenheit":
                return "fahrenheit";
            default:
                return "";
        }
    }

    private static String regionOf(Locale locale) {
        if (locale == null) return "";
        String country = locale.getCountry();
        if (country == null || country.length() != 2) return "";
        char a = country.charAt(0), b = country.charAt(1);
        if (!isAsciiLetter(a) || !isAsciiLetter(b)) return "";
        return country.toUpperCase(Locale.ROOT);
    }

    private static boolean isAsciiLetter(char c) {
        return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z');
    }

    private static String extension(Locale primary, Locale fallback, String key) {
        String value = primary == null ? null : primary.getUnicodeLocaleType(key);
        if ((value == null || value.isEmpty()) && fallback != null) {
            value = fallback.getUnicodeLocaleType(key);
        }
        return value == null ? "" : value.toLowerCase(Locale.ROOT);
    }
}
