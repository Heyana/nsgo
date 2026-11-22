package dev.oneo.nsgo;

import android.app.Activity;
import android.app.AlarmManager;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Build;
import android.service.notification.StatusBarNotification;
import java.util.Calendar;
import java.util.Map;
import org.json.JSONException;
import org.json.JSONObject;

/**
 * System notifications through NotificationManager, as core's
 * ShowNotification: a channel for those with the system's sound and one
 * for silent ones, POST_NOTIFICATIONS asked from API 33 where the app's
 * manifest asks for it, and a press that brings the app's activity back
 * with the notification's ID. Android has no badge count of its own:
 * launchers mark an app with notifications showing, and the count goes
 * with those shown after it is set.
 *
 * A notification scheduled for later is kept in the app's preferences and shown by an AlarmManager alarm, which NotificationReceiver hears without starting the app's Go: inexact by default, as setAndAllowWhileIdle may defer it within Doze's windows, and exact where the app's manifest asks for SCHEDULE_EXACT_ALARM or USE_EXACT_ALARM and the system lets it. A repeated one sets its next alarm as it shows. Android forgets alarms on a reboot and a force stop, so the receiver sets them again after a boot and the runtime as the app starts.
 */
final class Notifications {
    private Notifications() {}

    static final String PERMISSION = "android.permission.POST_NOTIFICATIONS";
    static final int PERMISSION_REQUEST = 0x6e79;
    /** The extra of the intent a press starts the activity with: the notification's ID. */
    static final String EXTRA = "dev.oneo.nsgo.notification";
    private static final String CHANNEL = "nsgo", SILENT = "nsgo.silent";
    private static final String PREFS = "nsgo.notifications", ASKED = "asked";
    /** The preferences the scheduled notifications are kept in, by ID, and the action of their alarms' broadcasts. */
    private static final String PENDING = "nsgo.notifications.pending";
    static final String ALARM = "dev.oneo.nsgo.NOTIFICATION_ALARM";
    /** How a scheduled notification repeats, as the driver's NotificationRepeat. */
    private static final int ONCE = 0, DAILY = 1, WEEKLY = 2;

    /** The count ITEM_BADGE set, which the notifications shown after it carry. */
    static int badge;

    /** Whether the app may show notifications, as core.NotificationPermission. */
    static int permission(Context context) {
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        if (manager == null) return Protocol.NOTIFICATIONS_UNAVAILABLE;
        if (Build.VERSION.SDK_INT >= 33
                && context.checkSelfPermission(PERMISSION) != PackageManager.PERMISSION_GRANTED) {
            return asked(context) ? Protocol.NOTIFICATIONS_DENIED : Protocol.NOTIFICATIONS_NOT_ASKED;
        }
        return manager.areNotificationsEnabled() ? Protocol.NOTIFICATIONS_GRANTED : Protocol.NOTIFICATIONS_DENIED;
    }

    /**
     * Whether a request for the permission needs the system's prompt: from
     * API 33, not asked yet, with the permission in the app's manifest.
     * Without it in the manifest the system refuses at once.
     */
    static boolean needsPrompt(Context context) {
        if (permission(context) != Protocol.NOTIFICATIONS_NOT_ASKED) return false;
        if (!declares(context)) {
            markAsked(context);
            return false;
        }
        return true;
    }

    static void markAsked(Context context) {
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().putBoolean(ASKED, true).apply();
    }

    private static boolean asked(Context context) {
        SharedPreferences prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        return prefs.getBoolean(ASKED, false);
    }

    private static boolean declares(Context context) {
        return declares(context, PERMISSION);
    }

    /** Whether the app's manifest asks for permission. */
    private static boolean declares(Context context, String permission) {
        try {
            String[] asked = context.getPackageManager()
                    .getPackageInfo(context.getPackageName(), PackageManager.GET_PERMISSIONS).requestedPermissions;
            return asked != null && java.util.Arrays.asList(asked).contains(permission);
        } catch (PackageManager.NameNotFoundException e) {
            return false;
        }
    }

    /** Shows a notification now, replacing one of its ID, shown or scheduled; whether the system took it. */
    static boolean showNow(Context context, Activity activity, String id, String title, String body, boolean silent) {
        unschedule(context, id);
        return show(context, activity, id, title, body, silent);
    }

    /** Shows a notification, replacing one of its ID; whether the system took it. */
    static boolean show(Context context, Activity activity, String id, String title, String body, boolean silent) {
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        if (manager == null || permission(context) != Protocol.NOTIFICATIONS_GRANTED) return false;
        String channel = silent ? SILENT : CHANNEL;
        if (manager.getNotificationChannel(channel) == null) {
            NotificationChannel c = new NotificationChannel(channel, silent ? "Silent notifications" : "Notifications",
                    silent ? NotificationManager.IMPORTANCE_LOW : NotificationManager.IMPORTANCE_DEFAULT);
            manager.createNotificationChannel(c);
        }
        int icon = context.getApplicationInfo().icon;
        Notification.Builder b = new Notification.Builder(context, channel)
                .setSmallIcon(icon != 0 ? icon : android.R.drawable.ic_dialog_info)
                .setContentTitle(title)
                .setAutoCancel(true)
                .setContentIntent(pressIntent(context, activity, id));
        if (!body.isEmpty()) b.setContentText(body);
        if (badge > 0) b.setNumber(badge);
        try {
            manager.notify(id, 0, b.build());
        } catch (RuntimeException e) {
            return false;
        }
        return true;
    }

    /**
     * Removes the notification called id, if it still shows, keeps it from showing if it is scheduled, and with an empty id every one show posted or scheduled, those of earlier runs included. It leaves the app's other notifications, such as Live Updates' and foreground services', which cancelAll would end.
     */
    static void close(Context context, String id) {
        if (id.isEmpty()) {
            for (String scheduled : pending(context).getAll().keySet()) unschedule(context, scheduled);
        } else {
            unschedule(context, id);
        }
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        if (manager == null) return;
        if (!id.isEmpty()) {
            manager.cancel(id, 0);
            return;
        }
        for (StatusBarNotification shown : manager.getActiveNotifications()) {
            String channel = shown.getNotification().getChannelId();
            if (CHANNEL.equals(channel) || SILENT.equals(channel)) manager.cancel(shown.getTag(), shown.getId());
        }
    }

    /**
     * Schedules a notification to show at at, in Unix milliseconds, and again each day or week after as repeat says, replacing a scheduled one of its ID; whether the system took it, which needs the permission now.
     */
    static boolean schedule(Context context, String id, String title, String body, boolean silent, long at, int repeat) {
        if (permission(context) != Protocol.NOTIFICATIONS_GRANTED) return false;
        JSONObject n = new JSONObject();
        try {
            n.put("title", title).put("body", body).put("silent", silent).put("at", at).put("repeat", repeat);
        } catch (JSONException e) {
            return false;
        }
        pending(context).edit().putString(id, n.toString()).commit();
        return arm(context, id, at);
    }

    /** Takes the notification called id off the schedule, if it is on it. */
    private static void unschedule(Context context, String id) {
        SharedPreferences prefs = pending(context);
        if (!prefs.contains(id)) return;
        prefs.edit().remove(id).commit();
        AlarmManager alarms = context.getSystemService(AlarmManager.class);
        PendingIntent alarm = PendingIntent.getBroadcast(context, 0, alarmIntent(context, id),
                PendingIntent.FLAG_NO_CREATE | PendingIntent.FLAG_IMMUTABLE);
        if (alarms != null && alarm != null) {
            alarms.cancel(alarm);
            alarm.cancel();
        }
    }

    /** Shows the scheduled notification called id, whose alarm went off, and sets its next alarm if it repeats. */
    static void fire(Context context, String id) {
        JSONObject n = scheduled(context, id);
        if (n == null) return;
        show(context, null, id, n.optString("title"), n.optString("body"), n.optBoolean("silent"));
        next(context, id, n, System.currentTimeMillis());
    }

    /**
     * Sets the alarms of every scheduled notification again, as after a reboot or a force stop: one whose time passed meanwhile shows now, and a repeated one goes on from its next time.
     */
    static void rearm(Context context) {
        long now = System.currentTimeMillis();
        for (Map.Entry<String, ?> e : pending(context).getAll().entrySet()) {
            JSONObject n = scheduled(context, e.getKey());
            if (n == null) continue;
            long at = n.optLong("at");
            if (at > now) {
                arm(context, e.getKey(), at);
            } else if (n.optInt("repeat") == ONCE) {
                fire(context, e.getKey());
            } else {
                next(context, e.getKey(), n, now);
            }
        }
    }

    /** Moves a notification that showed to its next time after now, or off the schedule if it shows once. */
    private static void next(Context context, String id, JSONObject n, long now) {
        int repeat = n.optInt("repeat");
        if (repeat == ONCE) {
            pending(context).edit().remove(id).commit();
            return;
        }
        // The calendar keeps the time of day across daylight saving's changes.
        Calendar at = Calendar.getInstance();
        at.setTimeInMillis(n.optLong("at"));
        while (at.getTimeInMillis() <= now) at.add(repeat == WEEKLY ? Calendar.WEEK_OF_YEAR : Calendar.DAY_OF_YEAR, 1);
        try {
            n.put("at", at.getTimeInMillis());
        } catch (JSONException e) {
            return;
        }
        pending(context).edit().putString(id, n.toString()).commit();
        arm(context, id, at.getTimeInMillis());
    }

    /** Sets the alarm of the notification called id for at: exact where the app may, else inexact. */
    private static boolean arm(Context context, String id, long at) {
        AlarmManager alarms = context.getSystemService(AlarmManager.class);
        if (alarms == null) return false;
        PendingIntent alarm = PendingIntent.getBroadcast(context, 0, alarmIntent(context, id),
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        try {
            if (exact(context, alarms)) {
                alarms.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, at, alarm);
                return true;
            }
        } catch (SecurityException e) {
            // The user took the exact alarms' access away meanwhile.
        }
        alarms.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, at, alarm);
        return true;
    }

    /** Whether the app may set exact alarms: it asks for them in its manifest and, from Android 12, the system grants them. */
    private static boolean exact(Context context, AlarmManager alarms) {
        if (!declares(context, "android.permission.SCHEDULE_EXACT_ALARM") && !declares(context, "android.permission.USE_EXACT_ALARM")) {
            return false;
        }
        return Build.VERSION.SDK_INT < 31 || alarms.canScheduleExactAlarms();
    }

    /** The broadcast an alarm sends NotificationReceiver: the notification's ID in its data, so each ID has an alarm of its own. */
    private static Intent alarmIntent(Context context, String id) {
        return new Intent(ALARM, Uri.fromParts("nsgo-notification", id, null), context, NotificationReceiver.class);
    }

    private static SharedPreferences pending(Context context) {
        return context.getSharedPreferences(PENDING, Context.MODE_PRIVATE);
    }

    /** The scheduled notification called id, null if none is. */
    private static JSONObject scheduled(Context context, String id) {
        String kept = pending(context).getString(id, null);
        if (kept == null) return null;
        try {
            return new JSONObject(kept);
        } catch (JSONException e) {
            return null;
        }
    }

    /**
     * The intent a press sends: the app's activity, brought back if it
     * runs, with the notification's ID.
     */
    static PendingIntent pressIntent(Context context, Activity activity, String id) {
        Intent intent = activity != null ? new Intent(context, activity.getClass())
                : context.getPackageManager().getLaunchIntentForPackage(context.getPackageName());
        if (intent == null) intent = new Intent();
        intent.setPackage(context.getPackageName());
        // PendingIntent ignores extras when matching an earlier intent. The
        // complete ID keeps Java hash collisions from redirecting a press.
        intent.setData(Uri.fromParts("nsgo-notification", id, null));
        intent.putExtra(EXTRA, id);
        intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        return PendingIntent.getActivity(context, id.hashCode(), intent,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }
}
