package dev.oneo.nsgo;

import android.app.AlarmManager;
import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.appwidget.AppWidgetProviderInfo;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ActivityInfo;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.res.Configuration;
import android.graphics.Bitmap;
import android.graphics.RectF;
import android.net.Uri;
import android.os.Bundle;
import android.os.FileObserver;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import android.util.Log;
import android.util.TypedValue;
import android.view.Gravity;
import android.widget.RemoteViews;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.File;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/** Package widget's persisted timelines as launcher RemoteViews, with no Go process needed to display or advance them. */
public final class AppWidgets extends AppWidgetProvider {
    private static final String TAG = "nsgo";
    private static final String KIND = "dev.oneo.nsgo.widget.kind";
    private static final String FAMILIES = "dev.oneo.nsgo.widget.families";
    private static final String ADVANCE = "dev.oneo.nsgo.widget.ADVANCE";
    private static final ExecutorService worker = Executors.newSingleThreadExecutor();
    private static final Map<String, FileObserver> observers = new LinkedHashMap<>();
    private static final Handler main = new Handler(Looper.getMainLooper());
    private static final Map<ComponentName, Runnable> advances = new LinkedHashMap<>();

    /** Installs bounded process-wide file watches before Go starts, including a pre-Run Update. Contexts retained here are applications alone. */
    static void prepare(Context context, ProcessBuilder builder) {
        Context app = context.getApplicationContext();
        File root = new File(app.getFilesDir(), "NSGoWidgets");
        Map<String, ComponentName> kinds = providers(app);
        if (kinds.isEmpty()) return;
        if (!root.isDirectory() && !root.mkdirs()) return;
        ArrayList<String> ready = new ArrayList<>();
        synchronized (observers) {
            for (Map.Entry<String, ComponentName> item : kinds.entrySet()) {
                String kind = item.getKey();
                File dir = new File(root, kind);
                if (!dir.isDirectory() && !dir.mkdirs()) continue;
                ready.add(kind);
                if (observers.containsKey(kind)) continue;
                ComponentName component = item.getValue();
                FileObserver observer = new FileObserver(dir.getAbsolutePath(), FileObserver.MOVED_TO | FileObserver.CLOSE_WRITE) {
                    @Override public void onEvent(int event, String path) {
                        if ("timeline.json".equals(path)) worker.execute(() -> refresh(app, component));
                    }
                };
                observers.put(kind, observer);
                observer.startWatching();
            }
        }
        builder.environment().put("NSGO_WIDGETS_DIR", root.getAbsolutePath());
        builder.environment().put("NSGO_WIDGET_KINDS", String.join(",", ready));
    }

    private static Map<String, ComponentName> providers(Context context) {
        LinkedHashMap<String, ComponentName> out = new LinkedHashMap<>();
        try {
            PackageInfo info = context.getPackageManager().getPackageInfo(context.getPackageName(), PackageManager.GET_RECEIVERS | PackageManager.GET_META_DATA);
            if (info.receivers != null) for (ActivityInfo receiver : info.receivers) {
                String kind = receiver.metaData == null ? null : receiver.metaData.getString(KIND);
                if (receiver.name.startsWith(NsgoComponents.WIDGET_PREFIX) && validKind(kind)) out.put(kind, new ComponentName(context.getPackageName(), receiver.name));
            }
        } catch (PackageManager.NameNotFoundException impossible) { Log.w(TAG, "widgets: missing own package", impossible); }
        return out;
    }

    /** TestAgent's opt-in launcher acceptance uses the system's ordinary pin UI. */
    static boolean pin(Context context, String kind) {
        ComponentName component = providers(context).get(kind);
        if (component == null) throw new IllegalArgumentException("unknown widget kind " + kind);
        AppWidgetManager manager = AppWidgetManager.getInstance(context);
        return manager.isRequestPinAppWidgetSupported() && manager.requestPinAppWidget(component, null, null);
    }

    static JSONObject probe(Context context, String kind) throws Exception {
        ComponentName component = providers(context).get(kind);
        if (component == null) throw new IllegalArgumentException("unknown widget kind " + kind);
        JSONArray ids = new JSONArray();
        for (int id : AppWidgetManager.getInstance(context).getAppWidgetIds(component)) ids.put(id);
        JSONObject timeline = readTimeline(new File(new File(new File(context.getFilesDir(), "NSGoWidgets"), kind), "timeline.json"));
        return new JSONObject().put("component", component.flattenToString()).put("ids", ids).put("current", current(timeline, System.currentTimeMillis()));
    }

    private static boolean validKind(String kind) { return kind != null && kind.matches("[A-Za-z0-9][A-Za-z0-9._-]{0,63}"); }

    @Override public void onReceive(Context context, Intent intent) {
        if (intent == null) return;
        ComponentName component = intent.getComponent();
        if (component == null || !component.getPackageName().equals(context.getPackageName())) return;
        String action = intent.getAction();
        if (!(ADVANCE.equals(action) || AppWidgetManager.ACTION_APPWIDGET_UPDATE.equals(action)
                || AppWidgetManager.ACTION_APPWIDGET_OPTIONS_CHANGED.equals(action)
                || AppWidgetManager.ACTION_APPWIDGET_ENABLED.equals(action) || AppWidgetManager.ACTION_APPWIDGET_DISABLED.equals(action)
                || AppWidgetManager.ACTION_APPWIDGET_DELETED.equals(action) || AppWidgetManager.ACTION_APPWIDGET_RESTORED.equals(action)
                || Intent.ACTION_BOOT_COMPLETED.equals(action) || Intent.ACTION_MY_PACKAGE_REPLACED.equals(action)
                || Intent.ACTION_TIME_CHANGED.equals(action) || Intent.ACTION_TIMEZONE_CHANGED.equals(action) || Intent.ACTION_LOCALE_CHANGED.equals(action))) return;
        Context app = context.getApplicationContext();
        PendingResult result = goAsync();
        worker.execute(() -> { try { refresh(app, component); } finally { result.finish(); } });
    }

    /** Reads a complete atomic timeline, renders all current instances, and keeps one future alarm per kind. */
    private static void refresh(Context context, ComponentName component) {
        try {
            ActivityInfo receiver = context.getPackageManager().getReceiverInfo(component, PackageManager.GET_META_DATA);
            String kind = receiver.metaData == null ? null : receiver.metaData.getString(KIND);
            if (!validKind(kind)) return;
            AppWidgetManager manager = AppWidgetManager.getInstance(context);
            int[] ids = manager.getAppWidgetIds(component);
            if (ids.length == 0) { schedule(context, component, 0); return; }
            File dir = new File(new File(context.getFilesDir(), "NSGoWidgets"), kind);
            JSONObject timeline = readTimeline(new File(dir, "timeline.json"));
            long now = System.currentTimeMillis();
            JSONObject entry = current(timeline, now);
            long next = nextDate(timeline, now);
            for (int id : ids) {
                AppWidgetProviderInfo info = manager.getAppWidgetInfo(id);
                if (info == null || !info.provider.equals(component)) continue;
                Bundle options = manager.getAppWidgetOptions(id);
                boolean portrait = context.getResources().getConfiguration().orientation != Configuration.ORIENTATION_LANDSCAPE;
                int width = options.getInt(portrait ? AppWidgetManager.OPTION_APPWIDGET_MIN_WIDTH : AppWidgetManager.OPTION_APPWIDGET_MAX_WIDTH, 158);
                int height = options.getInt(portrait ? AppWidgetManager.OPTION_APPWIDGET_MAX_HEIGHT : AppWidgetManager.OPTION_APPWIDGET_MIN_HEIGHT, 158);
                width = Math.max(1, Math.min(width, 2048)); height = Math.max(1, Math.min(height, 2048));
                String families = receiver.metaData.getString(FAMILIES, "small,medium,large");
                String family = family(families, width, height);
                JSONObject view = entry == null ? null : entry.optJSONObject("view");
                JSONObject overrides = entry == null ? null : entry.optJSONObject("families");
                if (overrides != null && overrides.optJSONObject(family) != null) view = overrides.optJSONObject(family);
                if (view == null) view = new JSONObject().put("t", "column").put("gap", 4).put("children", new JSONArray()
                        .put(new JSONObject().put("t", "text").put("text", receiver.loadLabel(context.getPackageManager())))
                        .put(new JSONObject().put("t", "text").put("text", context.getText(receiver.metaData.getInt("dev.oneo.nsgo.widget.description")))));
                WidgetRenderer.Result rendered = WidgetRenderer.render(context, dir, view, entry == null ? null : entry.optJSONObject("bg"), width, height, now);
                RemoteViews remote = remote(context, id, entry, rendered, width, height);
                manager.updateAppWidget(id, remote);
                if (rendered.nextRefresh > now && (next == 0 || rendered.nextRefresh < next)) next = rendered.nextRefresh;
            }
            schedule(context, component, next);
        } catch (Exception error) { Log.w(TAG, "widgets: refresh " + component, error); }
    }

    static JSONObject readTimeline(File file) {
        try {
            if (file.length() > 8 * 1024 * 1024) return null;
            JSONObject timeline = new JSONObject(new String(Files.readAllBytes(file.toPath()), StandardCharsets.UTF_8));
            return timeline.optInt("v", 0) == 1 ? timeline : null;
        } catch (Exception missing) { return null; }
    }

    static JSONObject current(JSONObject timeline, long now) {
        JSONArray entries = timeline == null ? null : timeline.optJSONArray("entries");
        if (entries == null || entries.length() == 0) return null;
        JSONObject selected = entries.optJSONObject(0);
        double latest = -Double.MAX_VALUE;
        for (int i = 0; i < entries.length(); i++) {
            JSONObject entry = entries.optJSONObject(i);
            if (entry == null) continue;
            double date = entry.optDouble("date", Double.MAX_VALUE);
            if (date * 1000 <= now && date >= latest) { selected = entry; latest = date; }
        }
        return selected;
    }

    static long nextDate(JSONObject timeline, long now) {
        JSONArray entries = timeline == null ? null : timeline.optJSONArray("entries");
        long next = 0;
        if (entries != null) for (int i = 0; i < entries.length(); i++) {
            JSONObject entry = entries.optJSONObject(i);
            if (entry == null) continue;
            double date = entry.optDouble("date", 0) * 1000;
            if (Double.isFinite(date) && date > now && date < Long.MAX_VALUE && (next == 0 || date < next)) next = (long) Math.ceil(date);
        }
        return next;
    }

    static String family(String allowed, int width, int height) {
        String candidate = width >= 500 ? "extra_large" : height >= 240 ? "large" : width >= 240 ? "medium" : "small";
        String[] families = allowed.split(",");
        for (String family : families) if (family.equals(candidate)) return candidate;
        return families.length == 0 || families[0].isEmpty() ? "small" : families[0];
    }

    private static void schedule(Context context, ComponentName component, long when) {
        Intent intent = new Intent(ADVANCE).setComponent(component);
        PendingIntent pending = PendingIntent.getBroadcast(context, 0, intent, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        AlarmManager alarm = (AlarmManager) context.getSystemService(Context.ALARM_SERVICE);
        if (alarm == null) return;
        alarm.cancel(pending);
        synchronized (advances) {
            Runnable previous = advances.remove(component);
            if (previous != null) main.removeCallbacks(previous);
            if (when > 0) {
                Runnable advance = () -> worker.execute(() -> refresh(context.getApplicationContext(), component));
                advances.put(component, advance);
                main.postDelayed(advance, Math.max(1000, when - System.currentTimeMillis()));
            }
        }
        // Inexact alarms need no exact-alarm permission. The OS may defer a
        // background timeline transition for its power budget, as WidgetKit does.
        if (when > 0) alarm.set(AlarmManager.RTC, Math.max(when, System.currentTimeMillis() + 1000), pending);
    }

    private static PendingIntent open(Context context, int widgetID, String url, int index) {
        Intent launch = new Intent(context, NsgoActivity.class).setAction(Intent.ACTION_VIEW)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TOP | Intent.FLAG_ACTIVITY_SINGLE_TOP);
        if (url != null && !url.isEmpty()) launch.setData(Uri.parse(url));
        // Intent identity includes data; request codes distinguish equal links in
        // different widgets. No mutable extras are supplied by the launcher.
        return PendingIntent.getActivity(context, widgetID * 257 + index, launch, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }

    private static RemoteViews remote(Context context, int id, JSONObject entry, WidgetRenderer.Result rendered, int width, int height) {
        int layout = context.getResources().getIdentifier("nsgo_widget", "layout", context.getPackageName());
        RemoteViews remote = new RemoteViews(context.getPackageName(), layout);
        remote.removeAllViews(android.R.id.empty);
        remote.setImageViewBitmap(android.R.id.icon, rendered.bitmap);
        remote.setContentDescription(android.R.id.icon, rendered.description);
        String url = entry == null ? "" : entry.optString("url", "");
        remote.setOnClickPendingIntent(android.R.id.content, open(context, id, url, 0));
        float density = context.getResources().getDisplayMetrics().density;
        int index = 1;
        for (WidgetRenderer.Link link : rendered.links) {
            RemoteViews overlay = new RemoteViews(context.getPackageName(), layout);
            RectF r = link.bounds;
            overlay.setViewPadding(android.R.id.content, Math.round(r.left * density), Math.round(r.top * density),
                    Math.max(0, Math.round((width - r.right) * density)), Math.max(0, Math.round((height - r.bottom) * density)));
            overlay.setContentDescription(android.R.id.icon, link.label);
            overlay.setOnClickPendingIntent(android.R.id.icon, open(context, id, link.url, index++));
            remote.addView(android.R.id.empty, overlay);
        }
        for (WidgetRenderer.Timer timer : rendered.timers) {
            int timerLayout = context.getResources().getIdentifier("nsgo_widget_timer", "layout", context.getPackageName());
            RemoteViews overlay = new RemoteViews(context.getPackageName(), timerLayout);
            RectF r = timer.bounds;
            overlay.setViewPadding(android.R.id.content, Math.round(r.left * density), Math.round(r.top * density),
                    Math.max(0, Math.round((width - r.right) * density)), 0);
            overlay.setTextViewTextSize(android.R.id.text1, TypedValue.COMPLEX_UNIT_PX, timer.size * density);
            overlay.setTextColor(android.R.id.text1, timer.color);
            overlay.setInt(android.R.id.text1, "setGravity", timer.gravity | Gravity.CENTER_VERTICAL);
            long base = SystemClock.elapsedRealtime() + timer.date - System.currentTimeMillis();
            overlay.setChronometer(android.R.id.text1, base, null, true);
            overlay.setChronometerCountDown(android.R.id.text1, timer.date > System.currentTimeMillis());
            remote.addView(android.R.id.empty, overlay);
        }
        return remote;
    }
}
