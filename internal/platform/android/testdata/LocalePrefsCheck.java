package dev.oneo.nsgo;

import java.io.DataOutputStream;
import java.util.Locale;

/** Exercises LocalePrefs on the JVM without Android, and encodes wire fixtures for Go's readEnvironment. */
public final class LocalePrefsCheck {
    public static void main(String[] args) throws Exception {
        if (args.length >= 4 && "encode".equals(args[0])) {
            Locale format = args[1].isEmpty() ? null : Locale.forLanguageTag(args[1]);
            boolean is24 = Boolean.parseBoolean(args[2]);
            String icu = args[3];
            LocalePrefs prefs = LocalePrefs.read(format, is24, icu);
            DataOutputStream out = new DataOutputStream(System.out);
            prefs.write(out);
            out.flush();
            return;
        }
        unicodeExtensions();
        allWeekdays();
        hourCycles();
        measurementAndRegionFallbacks();
        displayFallbackAndEdgeCases();
    }

    private static void unicodeExtensions() {
        expect(LocalePrefs.read(Locale.forLanguageTag("en-US-u-fw-mon-mu-celsius-ms-metric"), true, "us"),
                1, "h23", "metric", "celsius", "US", "en-US with mon/celsius/metric and 24h");
        expect(LocalePrefs.read(Locale.forLanguageTag("en-US-u-fw-sun-mu-fahrenhe-ms-ussystem"), false, "metric"),
                7, "h12", "us", "fahrenheit", "US", "en-US with sun/fahrenhe/ussystem and 12h");
        expect(LocalePrefs.read(Locale.forLanguageTag("en-US-u-ms-uksystem"), false, "us"),
                7, "h12", "uk", "", "US", "en-US with uksystem overriding ICU");
        expect(LocalePrefs.read(Locale.forLanguageTag("de-DE-u-fw-sat-mu-fahrenhe"), true, "metric"),
                6, "h23", "metric", "fahrenheit", "DE", "de-DE with sat/fahrenhe");
    }

    private static void allWeekdays() {
        String[] tags = {"mon", "tue", "wed", "thu", "fri", "sat", "sun"};
        for (int i = 0; i < tags.length; i++) {
            LocalePrefs p = LocalePrefs.read(Locale.forLanguageTag("en-u-fw-" + tags[i]), true, "");
            if (p.firstDayOfWeek != i + 1) {
                throw new AssertionError("fw=" + tags[i] + " gave " + p.firstDayOfWeek + ", want " + (i + 1));
            }
        }
    }

    private static void hourCycles() {
        // DateFormat.is24HourFormat governs 12h vs 24h; -u-hc- only refines h11/h24 within it.
        expect(LocalePrefs.read(Locale.forLanguageTag("en-US"), true, ""),
                7, "h23", "us", "", "US", "24h true");
        expect(LocalePrefs.read(Locale.forLanguageTag("en-US"), false, ""),
                7, "h12", "us", "", "US", "24h false");
        expect(LocalePrefs.read(Locale.forLanguageTag("en-US-u-hc-h12"), true, ""),
                7, "h23", "us", "", "US", "24h true overrides -u-hc-h12");
        expect(LocalePrefs.read(Locale.forLanguageTag("de-DE-u-hc-h23"), false, ""),
                1, "h12", "metric", "", "DE", "24h false overrides -u-hc-h23");
        expect(LocalePrefs.read(Locale.forLanguageTag("ja-JP-u-hc-h11"), false, ""),
                7, "h11", "metric", "", "JP", "12h refines to h11");
        expect(LocalePrefs.read(Locale.forLanguageTag("de-DE-u-hc-h24"), true, ""),
                1, "h24", "metric", "", "DE", "24h refines to h24");
    }

    private static void measurementAndRegionFallbacks() {
        expect(LocalePrefs.read(Locale.forLanguageTag("en-US"), false, ""),
                7, "h12", "us", "", "US", "US region fallback");
        expect(LocalePrefs.read(Locale.forLanguageTag("en-GB"), true, ""),
                1, "h23", "uk", "", "GB", "GB region fallback");
        expect(LocalePrefs.read(Locale.forLanguageTag("de-DE"), true, ""),
                1, "h23", "metric", "", "DE", "DE region fallback");
        expect(LocalePrefs.read(Locale.forLanguageTag("ar-EG"), false, ""),
                6, "h12", "metric", "", "EG", "EG Saturday fallback");
        expect(LocalePrefs.read(Locale.forLanguageTag("en-US"), false, "metric"),
                7, "h12", "metric", "", "US", "ICU measurement overrides region fallback");
        expect(LocalePrefs.read(Locale.forLanguageTag("en-DE"), true, "uk"),
                1, "h23", "uk", "", "DE", "ICU uk overrides metric region");
        expect(LocalePrefs.read(Locale.forLanguageTag("en"), true, ""),
                0, "h23", "", "", "", "regionless locale leaves week/measure/region unset");
        expect(LocalePrefs.read(Locale.forLanguageTag("es-419"), true, ""),
                0, "h23", "", "", "", "UN M.49 numeric region is not ISO 3166");
    }

    private static void displayFallbackAndEdgeCases() {
        expect(LocalePrefs.read(Locale.forLanguageTag("en-US"), Locale.forLanguageTag("en-US-u-fw-mon-mu-celsius"), false, ""),
                1, "h12", "us", "celsius", "US", "display locale supplies missing extensions");
        expect(LocalePrefs.read(Locale.forLanguageTag("en-US-u-fw-wed-mu-fahrenhe"), Locale.forLanguageTag("en-US-u-fw-mon-mu-celsius"), false, ""),
                3, "h12", "us", "fahrenheit", "US", "format locale beats display locale");
        expect(LocalePrefs.read(Locale.forLanguageTag("en-US-u-fw-xxx-mu-kelvin-ms-bogus"), false, ""),
                7, "h12", "us", "", "US", "unknown extensions fall back cleanly");
        expect(LocalePrefs.read(null, true, ""),
                0, "h23", "", "", "", "null format locale");
    }

    private static void expect(LocalePrefs got, int fw, String hc, String ms, String mu, String region, String name) {
        if (got.firstDayOfWeek != fw || !got.hourCycle.equals(hc) || !got.measurementSystem.equals(ms)
                || !got.temperatureUnit.equals(mu) || !got.region.equals(region)) {
            throw new AssertionError(name + ": got (" + got.firstDayOfWeek + ", " + got.hourCycle + ", "
                    + got.measurementSystem + ", " + got.temperatureUnit + ", " + got.region + "), want ("
                    + fw + ", " + hc + ", " + ms + ", " + mu + ", " + region + ")");
        }
    }
}
