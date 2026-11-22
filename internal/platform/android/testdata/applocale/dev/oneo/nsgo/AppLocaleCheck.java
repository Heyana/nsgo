package dev.oneo.nsgo;

import android.app.LocaleManager;
import android.content.Context;
import android.content.res.Configuration;
import android.os.Build;
import android.os.LocaleList;
import java.util.Locale;

public final class AppLocaleCheck {
    private static void check(boolean cond, String msg) {
        if (!cond) throw new AssertionError(msg);
    }

    public static void main(String[] args) {
        // Normalization & pseudo locales.
        check("".equals(AppLocale.normalize(null)), "null normalizes to empty");
        check("".equals(AppLocale.normalize("  ")), "blank normalizes to empty");
        check("zh-Hans".equals(AppLocale.normalize("zh_Hans")), "underscore normalized");
        check("".equals(AppLocale.normalize("en-XA")), "en-XA ignored");
        check("".equals(AppLocale.normalize("ar-XB")), "ar-XB ignored");
        check("".equals(AppLocale.normalize("AR_xb")), "case-insensitive pseudo ignored");

        // API < 33 is a no-op.
        Build.VERSION.SDK_INT = 32;
        LocaleManager lm32 = new LocaleManager();
        Context ctx32 = new Context(lm32);
        AppLocale.pin(ctx32, "de");
        check(lm32.setCalls == 0, "API 32 must not call LocaleManager");
        check(!AppLocale.isOwnPin(ctx32), "API 32 must not report own pin");

        // API 33+: pin, idempotency, system locale reporting, and reset.
        Build.VERSION.SDK_INT = 33;
        LocaleManager lm = new LocaleManager();
        lm.setSystemLocalesForTest(LocaleList.forLanguageTags("en-US"));
        Context ctx = new Context(lm);
        Configuration cfg = new Configuration(LocaleList.forLanguageTags("en-US"));

        // Fresh unforced launch does nothing.
        AppLocale.pin(ctx, "");
        check(lm.setCalls == 0, "unforced launch must not call setApplicationLocales");
        check("en-US".equals(AppLocale.userLocale(ctx, cfg, true).toLanguageTag()), "initial userLocale");

        // Pin zh-Hans.
        AppLocale.pin(ctx, "zh_Hans");
        check(lm.setCalls == 1, "first pin calls setApplicationLocales once");
        check("zh-Hans".equals(lm.getApplicationLocales().toLanguageTags()), "app locales set to zh-Hans");
        check(AppLocale.isOwnPin(ctx), "own pin detected");

        // Simulate Android updating configuration to the pinned app locale.
        cfg.setLocales(LocaleList.forLanguageTags("zh-Hans"));
        check("en-US".equals(AppLocale.userLocale(ctx, cfg, true).toLanguageTag()),
                "standalone userLocale keeps system locale while pinned");
        check("zh-Hans".equals(AppLocale.userLocale(ctx, cfg, false).toLanguageTag()),
                "embedded userLocale uses configuration locale");

        // Pinning the same tag again is idempotent (no redundant config churn).
        AppLocale.pin(ctx, "zh-Hans");
        check(lm.setCalls == 1, "redundant pin must not call setApplicationLocales again");

        // Switching to another language updates LocaleManager.
        AppLocale.pin(ctx, "de");
        check(lm.setCalls == 2, "switching language updates LocaleManager");
        check("de".equals(lm.getApplicationLocales().toLanguageTags()), "app locales switched to de");

        // Pseudo locale clears prior NSGo pin instead of pinning en-XA.
        AppLocale.pin(ctx, "en-XA");
        check(lm.setCalls == 3, "pseudo locale clears prior NSGo pin");
        check(lm.getApplicationLocales().isEmpty(), "pseudo locale leaves app locales empty");
        check(!AppLocale.isOwnPin(ctx), "pin removed after pseudo locale");

        // Pin ja, then clear with "".
        AppLocale.pin(ctx, "ja");
        check(lm.setCalls == 4, "pin ja");
        AppLocale.pin(ctx, "");
        check(lm.setCalls == 5, "clear ja");
        check(lm.getApplicationLocales().isEmpty(), "cleared app locales empty");
        check(!AppLocale.isOwnPin(ctx), "own pin cleared");

        // Clearing again is a no-op.
        AppLocale.pin(ctx, "");
        check(lm.setCalls == 5, "second clear is no-op");

        // User-chosen per-app language in Android Settings is preserved when unforced.
        LocaleManager userLm = new LocaleManager();
        userLm.setSystemLocalesForTest(LocaleList.forLanguageTags("en-US"));
        userLm.setApplicationLocalesByUserForTest(LocaleList.forLanguageTags("fr"));
        Context userCtx = new Context(userLm);
        Configuration userCfg = new Configuration(LocaleList.forLanguageTags("fr"));
        AppLocale.pin(userCtx, "");
        check(userLm.setCalls == 0, "unforced app must preserve user's Android Settings locale");
        check("fr".equals(userLm.getApplicationLocales().toLanguageTags()), "user's fr preserved");
        check("fr".equals(AppLocale.userLocale(userCtx, userCfg, true).toLanguageTag()),
                "user's Android Settings locale reported as userLocale");

        // If user overrides in Android Settings after NSGo pinned de, clearing leaves user's choice alone.
        AppLocale.pin(userCtx, "de");
        check("de".equals(userLm.getApplicationLocales().toLanguageTags()), "NSGo pinned de");
        int callsBefore = userLm.setCalls;
        userLm.setApplicationLocalesByUserForTest(LocaleList.forLanguageTags("es"));
        userCfg.setLocales(LocaleList.forLanguageTags("es"));
        check(!AppLocale.isOwnPin(userCtx), "user override is no longer NSGo's own pin");
        check("es".equals(AppLocale.userLocale(userCtx, userCfg, true).toLanguageTag()),
                "user override reported as userLocale");
        AppLocale.pin(userCtx, "");
        check(userLm.setCalls == callsBefore, "clearing after user override must not wipe user's choice");
        check("es".equals(userLm.getApplicationLocales().toLanguageTags()), "user's es preserved");
    }
}
