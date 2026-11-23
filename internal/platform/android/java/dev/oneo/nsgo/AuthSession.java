package dev.oneo.nsgo;

import android.app.Activity;
import android.app.Application;
import android.content.ActivityNotFoundException;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.content.pm.ResolveInfo;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import java.io.DataInputStream;
import java.io.IOException;
import java.net.URI;
import java.net.URISyntaxException;
import java.util.Locale;

/**
 * A system-browser authentication session, prepared before Go builds its authorization URL. Only a callback claimed by the requesting Activity can end it. An ordinary link is left to the app's open-link handlers.
 */
final class AuthSession {
    private static final String EXTRA_SESSION = "android.support.customtabs.extra.SESSION";
    private static final String EXTRA_EPHEMERAL = "androidx.browser.customtabs.extra.ENABLE_EPHEMERAL_BROWSING";
    /** Android has one foreground browser interaction per process. Main thread only. */
    private static Watch current;

    private AuthSession() {}

    /** The same decoder serves normal batches and the JVM wire regression. */
    static void read(NsgoView view, DataInputStream in) throws IOException {
        long id = in.readInt() & 0xffffffffL;
        int command = in.readUnsignedByte();
        String scheme = Protocol.readString(in), host = Protocol.readString(in), path = Protocol.readString(in);
        boolean ephemeral = Protocol.flag(in.readUnsignedByte(), 0);
        String url = Protocol.readString(in);
        handle(view, id, command, scheme, host, path, ephemeral, url);
    }

    static void handle(NsgoView view, long id, int command, String scheme, String host,
            String path, boolean ephemeral, String url) {
        if (command == Protocol.AUTH_START) {
            begin(view, id, scheme, host, path, ephemeral);
            return;
        }
        Watch watch = current;
        if (watch == null || watch.view != view || watch.id != id) return;
        if (command == Protocol.AUTH_CANCEL) {
            boolean returnToApp = watch.opened && watch.left;
            watch.stop();
            if (returnToApp) watch.returnToApp();
        } else if (command == Protocol.AUTH_OPEN && !watch.opened) {
            watch.open(url);
        }
    }

    private static void begin(NsgoView view, long id, String scheme, String host, String path, boolean ephemeral) {
        Activity activity = view.activity();
        if (activity == null || activity.isFinishing() || activity.isDestroyed()) {
            ended(view, id, "unavailable", "authentication needs a live Activity");
            return;
        }
        if (scheme.isEmpty() || scheme.equalsIgnoreCase("http") || scheme.equalsIgnoreCase("https") || !host.isEmpty()) {
            ended(view, id, "unavailable", "Android authentication needs an app-owned callback scheme");
            return;
        }
        URI redirect;
        try {
            // Path is decoded application text, as net/url.URL.Path is in the Go adapters; literal percent signs must not become escapes.
            redirect = new URI(scheme.toLowerCase(Locale.ROOT), null, path, null, null).normalize();
            if (redirect.isOpaque() || redirect.getRawAuthority() != null ||
                    redirect.getRawQuery() != null || redirect.getRawFragment() != null ||
                    redirect.getRawPath() == null || !redirect.getRawPath().startsWith("/")) {
                ended(view, id, "unavailable", "Android authentication needs an app-owned callback scheme");
                return;
            }
            redirect = new URI(redirect.toASCIIString());
        } catch (URISyntaxException e) {
            ended(view, id, "failed", "invalid authentication callback");
            return;
        }
        Intent probe = new Intent(Intent.ACTION_VIEW, Uri.parse(redirect.toString()))
                .addCategory(Intent.CATEGORY_BROWSABLE).setPackage(activity.getPackageName());
        boolean claimed = false;
        for (ResolveInfo match : activity.getPackageManager().queryIntentActivities(probe, PackageManager.MATCH_DEFAULT_ONLY)) {
            if (match.activityInfo == null) continue;
            String target = match.activityInfo.targetActivity != null ? match.activityInfo.targetActivity : match.activityInfo.name;
            if (activity.getClass().getName().equals(target)) {
                claimed = true;
                break;
            }
        }
        if (!claimed) {
            ended(view, id, "unavailable", "declare the callback scheme in url_schemes and forward its links to NsgoView");
            return;
        }
        if (current != null) current.end("cancelled", "");
        Watch watch = new Watch(view, activity, id, redirect, ephemeral);
        current = watch;
        view.authSessionEvent(id, Protocol.AUTH_REDIRECT, redirect.toString(), "");
    }

    /** Claims only this Activity's active callback; query and fragment reach Go unchanged. */
    static boolean redirect(NsgoView receiver, String link) {
        Watch watch = current;
        if (watch == null || !watch.opened || watch.activity != receiver.activity() || !matches(watch.redirect, link)) return false;
        watch.stop();
        watch.view.authSessionEvent(watch.id, Protocol.AUTH_ENDED, "", link);
        return true;
    }

    /** Compares callback destinations without interpreting the provider's code or state. */
    static boolean matches(URI redirect, String link) {
        if (link == null) return false;
        try {
            int end = link.length();
            int query = link.indexOf('?'), fragment = link.indexOf('#');
            if (query >= 0) end = Math.min(end, query);
            if (fragment >= 0) end = Math.min(end, fragment);
            URI target = new URI(new URI(link.substring(0, end)).normalize().toASCIIString());
            return redirect.equals(target);
        } catch (URISyntaxException | IllegalArgumentException e) {
            return false;
        }
    }

    static void dispose(NsgoView view) {
        if (current != null && current.view == view) current.stop();
    }

    private static void ended(NsgoView view, long id, String error, String message) {
        view.authSessionEvent(id, Protocol.AUTH_ENDED, error, message);
    }

    private static final class Watch implements Application.ActivityLifecycleCallbacks {
        final NsgoView view;
        final Activity activity;
        final long id;
        final URI redirect;
        final boolean ephemeral;
        final Handler main = new Handler(Looper.getMainLooper());
        boolean opened, left, stopped;

        Watch(NsgoView view, Activity activity, long id, URI redirect, boolean ephemeral) {
            this.view = view;
            this.activity = activity;
            this.id = id;
            this.redirect = redirect;
            this.ephemeral = ephemeral;
            activity.getApplication().registerActivityLifecycleCallbacks(this);
        }

        void open(String url) {
            Uri page = Uri.parse(url);
            if (!("https".equalsIgnoreCase(page.getScheme()) || "http".equalsIgnoreCase(page.getScheme())) || page.getHost() == null) {
                end("failed", "the authentication page is not a web URL");
                return;
            }
            Intent tab = new Intent(Intent.ACTION_VIEW, page).addCategory(Intent.CATEGORY_BROWSABLE);
            Bundle extras = new Bundle();
            extras.putBinder(EXTRA_SESSION, null);
            tab.putExtras(extras);
            if (ephemeral) tab.putExtra(EXTRA_EPHEMERAL, true);
            opened = true;
            try {
                activity.startActivity(tab);
            } catch (ActivityNotFoundException e) {
                // A browser that has no Custom Tabs support opens ACTION_VIEW normally. Also try without extras if a handler refuses them.
                try {
                    activity.startActivity(new Intent(Intent.ACTION_VIEW, page).addCategory(Intent.CATEGORY_BROWSABLE));
                } catch (ActivityNotFoundException | SecurityException fallback) {
                    end("unavailable", "no browser can open the authentication page");
                }
            } catch (SecurityException e) {
                end("failed", "the browser refused the authentication page");
            }
        }

        void stop() {
            if (stopped) return;
            stopped = true;
            activity.getApplication().unregisterActivityLifecycleCallbacks(this);
            if (current == this) current = null;
        }

        void end(String error, String message) {
            if (stopped) return;
            stop();
            ended(view, id, error, message);
        }

        void returnToApp() {
            if (activity.isFinishing() || activity.isDestroyed()) return;
            Intent back = new Intent(activity, activity.getClass())
                    .addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP | Intent.FLAG_ACTIVITY_SINGLE_TOP);
            try {
                activity.startActivity(back);
            } catch (ActivityNotFoundException | SecurityException e) {
                // A separately-tasked browser stays until the user closes it.
            }
        }

        @Override public void onActivityPaused(Activity a) {
            if (a == activity && opened) left = true;
        }

        @Override public void onActivityResumed(Activity a) {
            if (a != activity || !left || stopped) return;
            // onNewIntent precedes resume. Defer once more so a callback wins over this cancellation, including a late posted resume.
            main.post(() -> {
                if (current == this && !stopped) end("cancelled", "");
            });
        }

        @Override public void onActivityDestroyed(Activity a) {
            if (a == activity) end("cancelled", "");
        }

        @Override public void onActivityCreated(Activity a, Bundle state) {}
        @Override public void onActivityStarted(Activity a) {}
        @Override public void onActivityStopped(Activity a) {}
        @Override public void onActivitySaveInstanceState(Activity a, Bundle state) {}
    }
}
