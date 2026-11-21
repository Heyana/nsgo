package dev.oneo.nsgo;

import android.app.Activity;
import android.app.AppOpsManager;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Insets;
import android.graphics.PixelFormat;
import android.graphics.Rect;
import android.hardware.display.DisplayManager;
import android.hardware.input.InputManager;
import android.net.Uri;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.provider.Settings;
import android.util.DisplayMetrics;
import android.util.Log;
import android.util.SparseArray;
import android.view.ContextThemeWrapper;
import android.view.Display;
import android.view.Gravity;
import android.view.View;
import android.view.WindowInsets;
import android.view.WindowManager;
import android.view.WindowMetrics;

import java.io.ByteArrayOutputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.util.ArrayList;
import java.util.Arrays;

/**
 * The overlays over other apps the app opened (driver.SurfaceOverlay, mobile.OpenOverlay), for the first view, on the UI thread. Each is a TYPE_APPLICATION_OVERLAY window of WindowManager's showing an NsgoView of its own, which the first view hands its surface's batches and images on to and whose events name its surface. The window is added first and OverlayService, the foreground service that keeps the process while an overlay shows, starts only once a window is visible, as Android 15 lets an app holding SYSTEM_ALERT_WINDOW start one from the background only then; it stops with the last overlay. An overlay the system refuses, loses its service or whose grant the user revokes ends with its reason (SURFACE_ENDED), the last heard by watching the app's SYSTEM_ALERT_WINDOW op while an overlay is open, as the system hides the windows of a revoked app without telling them and the app need not come back to the front; one the app closes, or the process ending, ends without one.
 */
final class Overlays {
    private static final String TAG = "nsgo";
    static final String PERMISSION = "android.permission.SYSTEM_ALERT_WINDOW";

    /** An overlay showing: its view, its window's parameters and its modes. */
    private static final class Overlay {
        final int id;
        final NsgoView view;
        final WindowManager windows;
        final WindowManager.LayoutParams params;
        final boolean focusable;
        /** Whether the app hid it, so a hiding of the system's tells a revoked grant. */
        boolean hidden;
        /** The size in pixels the app asked for, which the window takes again where the usable area grows back. */
        int width, height;

        Overlay(int id, NsgoView view, WindowManager windows, WindowManager.LayoutParams params, boolean focusable) {
            this.id = id;
            this.view = view;
            this.windows = windows;
            this.params = params;
            this.focusable = focusable;
        }
    }

    private final NsgoView first;
    private final SparseArray<Overlay> open = new SparseArray<>();
    /** The permission requests waiting for the app to come back from the setting: each the view asking and its count. */
    private final ArrayList<Object[]> asks = new ArrayList<>();
    /** Whether this started OverlayService, which it stops with the last overlay. */
    private boolean serviceStarted;
    /** How long after the op changed the grant is read a second time, as Settings.canDrawOverlays can still answer the old grant as the change is reported. */
    private static final long RECHECK_LATER_MS = 500;
    private final Handler main = new Handler(Looper.getMainLooper());
    private final Runnable recheck = this::recheck;
    /** What hears the app's SYSTEM_ALERT_WINDOW op change while an overlay is open, null while none is or the system refused the watch. */
    private AppOpsManager.OnOpChangedListener watcher;

    Overlays(NsgoView first) {
        this.first = first;
    }

    /** The view of overlay id, or null once it closed. */
    NsgoView view(int id) {
        Overlay o = open.get(id);
        return o == null ? null : o.view;
    }

    /** Updates the overlay window's own preferred display rate. */
    void preferFrameRate(int id, float rate) {
        Overlay o = open.get(id);
        if (o == null || o.params.preferredRefreshRate == rate) return;
        o.params.preferredRefreshRate = rate;
        try {
            o.windows.updateViewLayout(o.view, o.params);
        } catch (IllegalArgumentException | SecurityException ignored) {
            // The window closed or the system revoked its overlay permission.
        }
    }

    /** Whether an overlay shows. */
    boolean any() {
        return open.size() > 0;
    }

    void redrawCanvases() {
        for (int i = 0; i < open.size(); i++) open.valueAt(i).view.redrawCanvases();
    }

    /** Sends each overlay's environment again, as the first view's settings changed. */
    void environment() {
        for (int i = 0; i < open.size(); i++) open.valueAt(i).view.environment();
    }

    /** Whether the app's manifest asks for SYSTEM_ALERT_WINDOW, without which the setting has no switch for it. */
    private boolean declared() {
        Context c = first.getContext();
        try {
            String[] asked = c.getPackageManager().getPackageInfo(c.getPackageName(), PackageManager.GET_PERMISSIONS)
                    .requestedPermissions;
            return asked != null && Arrays.asList(asked).contains(PERMISSION);
        } catch (PackageManager.NameNotFoundException e) {
            return false;
        }
    }

    /** The permission, an OVERLAY_* of Protocol's. */
    private int permission() {
        if (!declared()) return Protocol.OVERLAY_UNAVAILABLE;
        return Settings.canDrawOverlays(first.getContext()) ? Protocol.OVERLAY_GRANTED : Protocol.OVERLAY_DENIED;
    }

    /**
     * Answers view's ITEM_OVERLAY_PERMISSION count: at once, or, asked from the first view while denied, once the app comes back from the setting the request opens.
     */
    void permission(NsgoView view, long count, boolean ask) {
        int now = permission();
        if (ask && now == Protocol.OVERLAY_DENIED && view == first && openSetting()) {
            asks.add(new Object[] {view, count});
            return;
        }
        view.answerOverlayPermission(count, now);
    }

    /** Opens the app's page of ACTION_MANAGE_OVERLAY_PERMISSION; whether it opened. */
    private boolean openSetting() {
        Activity activity = first.activity();
        Context context = activity != null ? activity : first.getContext();
        Intent intent = new Intent(Settings.ACTION_MANAGE_OVERLAY_PERMISSION, Uri.parse("package:" + context.getPackageName()));
        if (activity == null) intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        try {
            context.startActivity(intent);
            return true;
        } catch (RuntimeException e) {
            Log.w(TAG, "the overlay permission's setting does not open", e);
            return false;
        }
    }

    /** The app came back to the front: answers the requests the setting held, and ends the overlays whose grant the user revoked meanwhile. */
    void returned() {
        int now = permission();
        for (Object[] a : asks) ((NsgoView) a[0]).answerOverlayPermission((Long) a[1], now);
        asks.clear();
        if (now != Protocol.OVERLAY_GRANTED) endAll(Protocol.OVERLAY_END_REVOKED);
    }

    /**
     * Opens overlay id of width by height points at x, y, as ITEM_OPEN_SURFACE's flags say: placed (bit 1), hidden, focusable, passive and accessible. Without the permission it ends at once, with no window.
     */
    void open(int id, float width, float height, float x, float y, int flags) {
        if (open.get(id) != null) return;
        int now = permission();
        if (now != Protocol.OVERLAY_GRANTED) {
            ended(id, now == Protocol.OVERLAY_UNAVAILABLE ? Protocol.OVERLAY_END_UNAVAILABLE : Protocol.OVERLAY_END_DENIED);
            return;
        }
        boolean placed = Protocol.flag(flags, 1), hidden = Protocol.flag(flags, 2), passive = Protocol.flag(flags, 4);
        boolean focusable = Protocol.flag(flags, 3) && !passive, accessible = Protocol.flag(flags, 5);
        Context context = windowContext();
        WindowManager windows = context.getSystemService(WindowManager.class);
        NsgoView view = new NsgoView(context, first, id);
        // Touches outside the window go to the app below; only a focusable one takes the keys, and a passive one no touches.
        int f = WindowManager.LayoutParams.FLAG_LAYOUT_IN_SCREEN | WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL;
        if (!focusable) f |= WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE;
        if (passive) f |= WindowManager.LayoutParams.FLAG_NOT_TOUCHABLE;
        WindowManager.LayoutParams params = new WindowManager.LayoutParams(
                WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY, f, PixelFormat.TRANSLUCENT);
        params.gravity = Gravity.TOP | Gravity.LEFT;
        params.setTitle("nsgo-overlay-" + id);
        if (passive && Build.VERSION.SDK_INT >= 31) {
            // From Android 12 a touch passes through another app's overlay only while the windows' combined opacity is at most this, else the system drops it as untrusted.
            InputManager input = context.getSystemService(InputManager.class);
            if (input != null) params.alpha = Math.min(1f, input.getMaximumObscuringOpacityForTouch());
        }
        float density = view.density();
        int w = Math.round(width * density), h = Math.round(height * density);
        place(windows, params, Math.round(x * density), Math.round(y * density), w, h, placed);
        if (!accessible) view.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO_HIDE_DESCENDANTS);
        if (hidden) view.setVisibility(View.GONE);
        Overlay o = new Overlay(id, view, windows, params, focusable);
        o.hidden = hidden;
        o.width = w;
        o.height = h;
        open.put(id, o);
        try {
            windows.addView(view, params);
        } catch (RuntimeException e) {
            Log.w(TAG, "overlay " + id + " cannot be added", e);
            open.remove(id);
            view.releaseOverlay();
            ended(id, Protocol.OVERLAY_END_FAILED);
            return;
        }
        watch();
        // The first report attaches the tree, also for a hidden window that lays nothing out.
        view.environment();
    }

    /**
     * Watches the app's SYSTEM_ALERT_WINDOW op, once an overlay is open: revoking the grant hides the app's windows by policy, which leaves their views' visibility as it was, so neither the windows nor a resumed activity, which the app may never have again, would tell. The system calls the listener on a binder thread; the grant is read on the main thread at once and once more a moment later.
     */
    private void watch() {
        if (watcher != null) return;
        Context app = first.getContext().getApplicationContext();
        AppOpsManager ops = app.getSystemService(AppOpsManager.class);
        if (ops == null) return;
        String pkg = app.getPackageName();
        AppOpsManager.OnOpChangedListener w = (op, packageName) -> {
            if (!pkg.equals(packageName)) return;
            main.removeCallbacks(recheck);
            main.post(recheck);
            main.postDelayed(recheck, RECHECK_LATER_MS);
        };
        try {
            ops.startWatchingMode(AppOpsManager.OPSTR_SYSTEM_ALERT_WINDOW, pkg, w);
        } catch (RuntimeException e) {
            // The overlays still end on the resume check and the window's own visibility.
            Log.w(TAG, "the overlay permission cannot be watched", e);
            return;
        }
        watcher = w;
    }

    /** Stops watching the op, as the last overlay closed. */
    private void unwatch() {
        main.removeCallbacks(recheck);
        if (watcher == null) return;
        AppOpsManager ops = first.getContext().getApplicationContext().getSystemService(AppOpsManager.class);
        if (ops != null) ops.stopWatchingMode(watcher);
        watcher = null;
    }

    /** Reads the grant again as the op changed: a revoked one ends every overlay, removing its window, and the service with the last. */
    private void recheck() {
        if (open.size() > 0 && !Settings.canDrawOverlays(first.getContext().getApplicationContext())) {
            endAll(Protocol.OVERLAY_END_REVOKED);
        }
    }

    /**
     * The context an overlay's view and window take: a window context of the overlay type on the first view's display from API 30, as Android asks of what adds a window without an activity, themed as the app.
     */
    private Context windowContext() {
        Context app = first.getContext().getApplicationContext();
        Context base = app;
        if (Build.VERSION.SDK_INT >= 30) {
            Display display = first.getDisplay();
            if (display == null) {
                display = app.getSystemService(DisplayManager.class).getDisplay(Display.DEFAULT_DISPLAY);
            }
            base = app.createDisplayContext(display)
                    .createWindowContext(WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY, null);
        }
        int theme = app.getApplicationInfo().theme;
        return new ContextThemeWrapper(base, theme != 0 ? theme : android.R.style.Theme_DeviceDefault_NoActionBar);
    }

    /** The display's usable area in pixels: without the system bars and the cutout from API 30, the whole display before. */
    private static Rect usable(WindowManager windows) {
        if (Build.VERSION.SDK_INT >= 30) {
            WindowMetrics m = windows.getCurrentWindowMetrics();
            Insets in = m.getWindowInsets().getInsetsIgnoringVisibility(
                    WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout());
            Rect r = new Rect(m.getBounds());
            r.left += in.left;
            r.top += in.top;
            r.right -= in.right;
            r.bottom -= in.bottom;
            return r;
        }
        DisplayMetrics metrics = new DisplayMetrics();
        windows.getDefaultDisplay().getRealMetrics(metrics);
        return new Rect(0, 0, metrics.widthPixels, metrics.heightPixels);
    }

    /**
     * Sets params to a frame of width by height pixels at x, y kept wholly on the usable area, as driver.WindowFrame.Within keeps it: no larger than it and moved in where it pokes out, centered when not placed.
     */
    private static void place(WindowManager windows, WindowManager.LayoutParams params, int x, int y, int width,
            int height, boolean placed) {
        Rect area = usable(windows);
        width = Math.max(1, Math.min(width, area.width()));
        height = Math.max(1, Math.min(height, area.height()));
        if (placed) {
            x = Math.min(Math.max(x, area.left), area.right - width);
            y = Math.min(Math.max(y, area.top), area.bottom - height);
        } else {
            x = area.left + (area.width() - width) / 2;
            y = area.top + (area.height() - height) / 2;
        }
        params.x = x;
        params.y = y;
        params.width = width;
        params.height = height;
    }

    /**
     * What an overlay's view reports of its surface (SURFACE_STATE): its size and its window's place in points, then env, its environment as ENVIRONMENT's; null once it closed.
     */
    byte[] state(NsgoView view, byte[] env) {
        Overlay o = open.get(view.surface());
        if (o == null || o.view != view) return null;
        float density = view.density();
        int width = view.getWidth() > 0 ? view.getWidth() : o.params.width;
        int height = view.getHeight() > 0 ? view.getHeight() : o.params.height;
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeInt(o.id);
            d.writeByte(Protocol.SURFACE_STATE);
            d.writeFloat(width / density);
            d.writeFloat(height / density);
            d.writeFloat(o.params.x / density);
            d.writeFloat(o.params.y / density);
            d.write(env);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        return bytes.toByteArray();
    }

    /**
     * Applies an overlay's ITEM_WINDOW: a frame in points, moving it when move and resizing it when resize, kept on the usable area; showing, hiding, bringing it up with the focus where it takes it, or closing it for good.
     */
    void command(NsgoView view, int op, float x, float y, float width, float height, boolean move, boolean resize) {
        Overlay o = open.get(view.surface());
        if (o == null || o.view != view) return;
        switch (op) {
            case Protocol.WINDOW_FRAME: {
                float density = view.density();
                WindowManager.LayoutParams p = o.params;
                if (resize) {
                    o.width = Math.round(width * density);
                    o.height = Math.round(height * density);
                }
                place(o.windows, p, move ? Math.round(x * density) : p.x, move ? Math.round(y * density) : p.y, o.width,
                        o.height, true);
                try {
                    o.windows.updateViewLayout(view, p);
                } catch (IllegalArgumentException e) {
                    // The system removed the window meanwhile.
                    return;
                }
                view.environment();
                break;
            }
            case Protocol.WINDOW_SHOW:
            case Protocol.WINDOW_FRONT:
                o.hidden = false;
                view.setVisibility(View.VISIBLE);
                if (op == Protocol.WINDOW_FRONT && o.focusable) view.requestFocus();
                break;
            case Protocol.WINDOW_HIDE:
                o.hidden = true;
                view.setVisibility(View.GONE);
                break;
            case Protocol.WINDOW_DESTROY:
                close(o, 0);
                break;
            default:
                break;
        }
    }

    /**
     * Keeps overlay view's window wholly on the usable area again as the display rotates or changes, or its system bars or cutout do, at the size the app asked for where it fits, before the view reports its frame: the window would otherwise stay where the old area put it, off screen or moved by the system to a place Go does not know.
     */
    void fit(NsgoView view) {
        Overlay o = open.get(view.surface());
        if (o == null || o.view != view) return;
        WindowManager.LayoutParams p = o.params;
        int x = p.x, y = p.y, width = p.width, height = p.height;
        place(o.windows, p, x, y, o.width, o.height, true);
        if (p.x == x && p.y == y && p.width == width && p.height == height) return;
        try {
            o.windows.updateViewLayout(view, p);
        } catch (IllegalArgumentException e) {
            // The system removed the window meanwhile.
        }
    }

    /**
     * Hears an overlay's window's visibility: showing starts the service once the window's first frame is on screen, as the system counts the overlay visible from then; the system hiding one the app did not hide reads the grant again, where a revoked grant has not ended it yet (watch).
     */
    void visibility(NsgoView view, int visibility) {
        Overlay o = open.get(view.surface());
        if (o == null || o.view != view) return;
        if (visibility == View.VISIBLE) {
            if (serviceStarted) return;
            Runnable start = () -> start(view, true);
            if (Build.VERSION.SDK_INT >= 29) {
                view.getViewTreeObserver().registerFrameCommitCallback(start);
                // A frame comes, to commit.
                view.invalidate();
            } else {
                view.post(start);
            }
            return;
        }
        if (!o.hidden && !Settings.canDrawOverlays(first.getContext())) close(o, Protocol.OVERLAY_END_REVOKED);
    }

    /** Starts the service for view's overlay, still open, retrying once a moment later where the system refuses it as its window shows, and ends the overlay if it refuses again. */
    private void start(NsgoView view, boolean retry) {
        Overlay o = open.get(view.surface());
        if (o == null || o.view != view || serviceStarted || startService()) return;
        if (retry) view.postDelayed(() -> start(view, false), 250);
        else close(o, Protocol.OVERLAY_END_FAILED);
    }

    /** Starts OverlayService in the foreground; whether the system took it. */
    private boolean startService() {
        Context app = first.getContext().getApplicationContext();
        OverlayService.owner = this;
        OverlayService.wanted = true;
        try {
            ComponentName started = app.startForegroundService(new Intent(app, OverlayService.class));
            if (started == null) {
                Log.w(TAG, "the app's manifest declares no OverlayService: nsgo build adds it with SYSTEM_ALERT_WINDOW in android.permissions");
                OverlayService.wanted = false;
                return false;
            }
        } catch (RuntimeException e) {
            // ForegroundServiceStartNotAllowedException, a missing permission among them.
            Log.w(TAG, "the overlay's foreground service does not start", e);
            OverlayService.wanted = false;
            return false;
        }
        serviceStarted = true;
        return true;
    }

    /** Stops OverlayService: at once once it is in the foreground, else it stops itself as it gets there, as the system crashes an app whose service started with startForegroundService ends before calling startForeground. */
    private void stopService() {
        if (!serviceStarted) return;
        serviceStarted = false;
        OverlayService.wanted = false;
        OverlayService running = OverlayService.running;
        if (running != null) running.stop();
    }

    /** OverlayService ended without being asked, or could not go into the foreground: every overlay ends, as nothing keeps the process. */
    void serviceEnded(boolean failed) {
        serviceStarted = false;
        OverlayService.wanted = false;
        endAll(failed ? Protocol.OVERLAY_END_FAILED : Protocol.OVERLAY_END_REMOVED);
    }

    private void endAll(int reason) {
        while (open.size() > 0) close(open.valueAt(0), reason);
    }

    /** Closes every overlay without a report, as the process ends, and stops the service. */
    void closeAll() {
        endAll(0);
        unwatch();
        asks.clear();
        stopService();
    }

    /** Removes o's window and lets go of its view; reason, unless 0, ends its tree. The service stops with the last overlay, and a first view kept only for the overlays ends the app's process unless a background location stream keeps it. */
    private void close(Overlay o, int reason) {
        open.remove(o.id);
        try {
            o.windows.removeView(o.view);
        } catch (IllegalArgumentException e) {
            // The system removed it already.
        }
        o.view.releaseOverlay();
        if (reason != 0) ended(o.id, reason);
        if (open.size() > 0) return;
        unwatch();
        stopService();
        first.keeperEnded();
    }

    /** Reports that overlay id ended for reason (SURFACE_ENDED): its tree closes. */
    private void ended(int id, int reason) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeInt(id);
            d.writeByte(Protocol.SURFACE_ENDED);
            d.writeByte(reason);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        first.write(Protocol.SURFACE, bytes.toByteArray());
    }
}
