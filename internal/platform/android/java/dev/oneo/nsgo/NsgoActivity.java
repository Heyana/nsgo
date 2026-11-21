package dev.oneo.nsgo;

import android.app.Activity;
import android.content.Intent;
import android.content.MutableContextWrapper;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageManager;
import android.content.pm.ResolveInfo;
import android.content.res.Configuration;
import android.graphics.Color;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.view.ContextThemeWrapper;
import android.view.Surface;
import android.view.ViewGroup;
import android.view.Window;
import android.view.WindowManager;

/**
 * The launcher activity of a standalone NSGo app: one NsgoView filling the
 * window edge to edge, behind the system bars it reports as the safe area.
 * Its manifest entry, as in AndroidManifest.xml, declares the
 * configuration changes it handles, or the system restarts the app on each.
 */
public class NsgoActivity extends Activity {
    /**
     * The view of an activity that ended while the app's overlays over other apps or its background location streams kept its process running, as when the user removes the app's task, which the next activity shows again: the engine serves the one view that first attached to it, so the view moves to the new activity, its context wrapping that one. Null otherwise.
     */
    private static NsgoView kept;

    private NsgoView view;
    private Integer previousOrientation;
    /** The theme the window shows, which a kept view's context takes while no activity holds it. */
    private int theme = android.R.style.Theme_DeviceDefault_NoActionBar;

    @Override
    protected void onCreate(Bundle state) {
        CrashReports.install(this);
        // The manifest's theme is dark; web views take prefers-color-scheme
        // from the theme, so it follows the system's mode at launch.
        int night = getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK;
        if (night != Configuration.UI_MODE_NIGHT_YES) {
            theme = android.R.style.Theme_DeviceDefault_Light_NoActionBar;
            setTheme(theme);
        }
        super.onCreate(state);
        Window window = getWindow();
        WindowManager.LayoutParams lp = window.getAttributes();
        lp.layoutInDisplayCutoutMode = WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES;
        window.setAttributes(lp);
        // Edge to edge, this reports the keyboard in the insets rather
        // than resizing the view.
        window.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);
        window.setStatusBarColor(Color.TRANSPARENT);
        window.setNavigationBarColor(Color.TRANSPARENT);
        if (Build.VERSION.SDK_INT >= 30) window.setDecorFitsSystemWindows(false);
        // The view lays itself out behind the system bars and styles them.
        Intent intent = getIntent();
        boolean share = isShare(intent.getAction());
        NsgoView again = kept;
        kept = null;
        if (again != null && again.keptOffScreen()) {
            ((MutableContextWrapper) again.getContext()).setBaseContext(this);
            view = again;
            view.showAgain();
        } else {
            view = new NsgoView(new MutableContextWrapper(this));
            if (!share) {
                view.setFlags(intent.getExtras());
                view.setTest(intent.getStringExtra(TestAgent.EXTRA));
            }
        }
        setContentView(view);
        if (!share) openNotification(intent);
        // A share delivers content, so it reaches a restored activity too; a link only opens a fresh one.
        if (state == null || share) openIntent(intent);
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        if (!isShare(intent.getAction())) openNotification(intent);
        openIntent(intent);
    }

    private void openNotification(Intent intent) {
        view.notificationPressed(intent.getStringExtra(Notifications.EXTRA));
        view.liveActionPressed(intent);
    }

    private static boolean isShare(String action) {
        return Intent.ACTION_SEND.equals(action) || Intent.ACTION_SEND_MULTIPLE.equals(action);
    }

    /** Delivers an incoming link or share that the launcher declares. */
    private void openIntent(Intent intent) {
        String action = intent.getAction();
        if (Intent.ACTION_VIEW.equals(action)) {
            Uri uri = intent.getData();
            if (uri != null && uri.getScheme() != null &&
                    accepts(new Intent(action, uri).addCategory(Intent.CATEGORY_BROWSABLE))) {
                view.openLink(uri.toString());
            }
        } else if (isShare(action)) {
            String mime = Intent.normalizeMimeType(intent.getType());
            if (mime != null && accepts(new Intent(action).setType(mime))) {
                IncomingShare.receive(getApplicationContext(), view, intent, mime);
            }
        }
    }

    /** Resolves an implicit intent in this package because an explicit intent can bypass its filters. */
    private boolean accepts(Intent intent) {
        intent.setPackage(getPackageName());
        for (ResolveInfo match : getPackageManager().queryIntentActivities(intent, PackageManager.MATCH_DEFAULT_ONLY)) {
            if (getComponentName().getClassName().equals(match.activityInfo.name)) return true;
        }
        return false;
    }

    /**
     * Restyles the window in the appearance the app shows: the theme the
     * views it makes from now on read, and the window's background.
     */
    void followAppearance(boolean dark) {
        theme = dark ? android.R.style.Theme_DeviceDefault_NoActionBar
                : android.R.style.Theme_DeviceDefault_Light_NoActionBar;
        setTheme(theme);
        getTheme().applyStyle(theme, true);
        android.content.res.TypedArray a = getTheme().obtainStyledAttributes(new int[] {android.R.attr.windowBackground});
        getWindow().setBackgroundDrawable(a.getDrawable(0));
        a.recycle();
    }

    /** Applies the app's orientation request until it releases its last scope. */
    void requestOrientation(int mask) {
        int next;
        if (mask == 0) {
            if (previousOrientation == null) return;
            next = previousOrientation;
            previousOrientation = null;
        } else {
            if (previousOrientation == null) previousOrientation = getRequestedOrientation();
            next = orientationMode(mask);
        }
        if (getRequestedOrientation() != next) setRequestedOrientation(next);
    }

    private int orientationMode(int mask) {
        final int portrait = Protocol.ORIENTATION_PORTRAIT;
        final int upsideDown = Protocol.ORIENTATION_PORTRAIT_UPSIDE_DOWN;
        final int left = Protocol.ORIENTATION_LANDSCAPE_LEFT;
        final int right = Protocol.ORIENTATION_LANDSCAPE_RIGHT;
        final int bothPortrait = portrait | upsideDown;
        final int all = bothPortrait | Protocol.ORIENTATION_LANDSCAPE;
        switch (mask) {
            case bothPortrait: return ActivityInfo.SCREEN_ORIENTATION_SENSOR_PORTRAIT;
            case Protocol.ORIENTATION_LANDSCAPE: return ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE;
            case all: return ActivityInfo.SCREEN_ORIENTATION_FULL_USER;
        }
        // Android cannot express every mixed set. Keep the current orientation
        // if it is allowed, else lock to an allowed one in the same axis when possible.
        boolean upright = getResources().getConfiguration().orientation == Configuration.ORIENTATION_PORTRAIT;
        int rotation = getWindowManager().getDefaultDisplay().getRotation();
        boolean reverse = rotation == Surface.ROTATION_180 || rotation == Surface.ROTATION_270;
        int current = upright ? (reverse ? upsideDown : portrait) : (reverse ? right : left);
        int sameAxis = mask & (upright ? bothPortrait : Protocol.ORIENTATION_LANDSCAPE);
        int chosen = (current & mask) != 0 ? current : Integer.lowestOneBit(sameAxis != 0 ? sameAxis : mask);
        switch (chosen) {
            case portrait: return ActivityInfo.SCREEN_ORIENTATION_PORTRAIT;
            case upsideDown: return ActivityInfo.SCREEN_ORIENTATION_REVERSE_PORTRAIT;
            case left: return ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE;
            case right: return ActivityInfo.SCREEN_ORIENTATION_REVERSE_LANDSCAPE;
            default: return ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED;
        }
    }

    @Override
    public void onBackPressed() {
        // Back arrives here before API 33, or without predictive back.
        if (!view.back()) super.onBackPressed();
    }

    @Override
    protected void onActivityResult(int request, int result, Intent data) {
        super.onActivityResult(request, result, data);
        view.activityResult(request, result, data);
    }

    @Override
    public void onRequestPermissionsResult(int request, String[] permissions, int[] results) {
        super.onRequestPermissionsResult(request, permissions, results);
        view.permissionResult(request, permissions, results);
    }

    /** Ends the app with the activity, unless its overlays over other apps or its background location streams keep it running, off screen, for the next activity. */
    @Override
    protected void onDestroy() {
        if (view.keepOffScreen()) {
            ((ViewGroup) view.getParent()).removeView(view);
            ((MutableContextWrapper) view.getContext()).setBaseContext(new ContextThemeWrapper(getApplicationContext(), theme));
            kept = view;
        } else {
            view.stop();
        }
        super.onDestroy();
    }
}
