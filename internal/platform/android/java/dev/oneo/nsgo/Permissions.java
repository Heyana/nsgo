package dev.oneo.nsgo;

import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.List;

/**
 * Checks and asks for the system's permissions on Android (core.PermissionStatus,
 * RequestPermission, OpenPermissionSettings): RECORD_AUDIO, CAMERA, READ_CALENDAR
 * and WRITE_CALENDAR, BLUETOOTH_CONNECT (API 31+, BLUETOOTH before), and
 * READ_MEDIA_AUDIO (API 33+, READ_EXTERNAL_STORAGE before). Whether a permission
 * has been asked before is kept in SharedPreferences, as Notifications.java keeps
 * POST_NOTIFICATIONS, so a refused permission reads as denied rather than prompt.
 * Kept free of Android imports so JVM fixtures can exercise every path directly.
 */
final class Permissions {
    /** Request code passed to Activity.requestPermissions and matched in NsgoView.permissionResult. */
    static final int PERMISSION_REQUEST = 0x6e7d;
    static final String PREFS = "nsgo.permissions";

    static final String RECORD_AUDIO = "android.permission.RECORD_AUDIO";
    static final String CAMERA = "android.permission.CAMERA";
    static final String READ_CALENDAR = "android.permission.READ_CALENDAR";
    static final String WRITE_CALENDAR = "android.permission.WRITE_CALENDAR";
    static final String BLUETOOTH_CONNECT = "android.permission.BLUETOOTH_CONNECT";
    static final String BLUETOOTH = "android.permission.BLUETOOTH";
    static final String READ_MEDIA_AUDIO = "android.permission.READ_MEDIA_AUDIO";
    static final String READ_EXTERNAL_STORAGE = "android.permission.READ_EXTERNAL_STORAGE";

    interface Host {
        int sdkInt();
        boolean granted(String permission);
        boolean declared(String permission);
        boolean asked(String key);
        void markAsked(String key);
        /** Starts Activity.requestPermissions; returns false when no activity is available or it fails. */
        boolean request(String[] permissions);
        /** Opens Settings.ACTION_APPLICATION_DETAILS_SETTINGS for the app; returns whether it opened. */
        boolean openSettings();
        /** Logs a warning when a request lacks manifest permission(s). */
        void warn(String message);
    }

    @FunctionalInterface
    interface Answer {
        void accept(long count, int status, boolean opened);
    }

    private static final class Pending {
        final long count;
        final String name;
        final Answer answer;

        Pending(long count, String name, Answer answer) {
            this.count = count;
            this.name = name;
            this.answer = answer;
        }
    }

    private final Host host;
    private final ArrayDeque<Pending> queue = new ArrayDeque<>();
    private boolean awaitingResult;

    Permissions(Host host) {
        this.host = host;
    }

    /** Android manifest permissions for core.Permission key name at API level sdkInt, or null when unsupported. */
    static String[] androidPermissions(String name, int sdkInt) {
        if (name == null) return null;
        switch (name) {
            case "microphone":
                return new String[] { RECORD_AUDIO };
            case "camera":
                return new String[] { CAMERA };
            case "calendars":
                return new String[] { READ_CALENDAR, WRITE_CALENDAR };
            case "bluetooth":
                return new String[] { sdkInt >= 31 ? BLUETOOTH_CONNECT : BLUETOOTH };
            case "media-library":
                return new String[] { sdkInt >= 33 ? READ_MEDIA_AUDIO : READ_EXTERNAL_STORAGE };
            default:
                return null;
        }
    }

    /** Whether name is one of the sensor permissions whose check/ask goes through Sensors.java but whose settings open here. */
    static boolean isSensor(String name) {
        return "location".equals(name) || "background-location".equals(name) || "motion".equals(name) || "activity-recognition".equals(name);
    }

    /** Required permissions for name: the mapped permissions declared in the manifest (empty when none declared). */
    private List<String> required(String name) {
        String[] mapped = androidPermissions(name, host.sdkInt());
        if (mapped == null) return null;
        List<String> declared = new ArrayList<>(mapped.length);
        for (String p : mapped) {
            if (host.declared(p)) declared.add(p);
        }
        return declared;
    }

    private boolean allGranted(String[] permissions) {
        for (String p : permissions) {
            if (!host.granted(p)) return false;
        }
        return true;
    }

    private boolean allGranted(List<String> permissions) {
        for (String p : permissions) {
            if (!host.granted(p)) return false;
        }
        return true;
    }

    /** Current Protocol.PERMISSION_* status for core.Permission key name without prompting. */
    int status(String name) {
        String[] mapped = androidPermissions(name, host.sdkInt());
        if (mapped == null) return Protocol.PERMISSION_UNAVAILABLE;
        List<String> req = required(name);
        if (req.isEmpty()) {
            return allGranted(mapped) ? Protocol.PERMISSION_GRANTED : Protocol.PERMISSION_UNAVAILABLE;
        }
        if (allGranted(req)) return Protocol.PERMISSION_GRANTED;
        return host.asked(name) ? Protocol.PERMISSION_DENIED : Protocol.PERMISSION_NOT_ASKED;
    }

    /** Handles an ItemPermission command for core.Permission key name. */
    void handle(long count, int command, String name, Answer answer) {
        switch (command) {
            case Protocol.PERMISSION_STOP_GUIDE:
                return;
            case Protocol.PERMISSION_OPEN_SETTINGS: {
                boolean canOpen = isSensor(name) || status(name) != Protocol.PERMISSION_UNAVAILABLE;
                answer.accept(count, 0, canOpen && host.openSettings());
                return;
            }
            case Protocol.PERMISSION_CHECK:
            case Protocol.PERMISSION_GUIDE:
                answer.accept(count, status(name), false);
                return;
            case Protocol.PERMISSION_ASK:
                ask(count, name, answer);
                return;
            default:
                answer.accept(count, Protocol.PERMISSION_UNAVAILABLE, false);
        }
    }

    private void ask(long count, String name, Answer answer) {
        int st = status(name);
        if (st != Protocol.PERMISSION_NOT_ASKED) {
            if (st == Protocol.PERMISSION_UNAVAILABLE) {
                String[] mapped = androidPermissions(name, host.sdkInt());
                if (mapped != null) {
                    host.warn("nsgo: requesting " + name + " needs " + String.join(" or ", mapped)
                            + " in the app's manifest (nsgo.yaml's android.permissions)");
                }
            }
            answer.accept(count, st, false);
            return;
        }
        queue.add(new Pending(count, name, answer));
        if (!awaitingResult) {
            pump();
        }
    }

    /** Called from NsgoView.permissionResult when request == PERMISSION_REQUEST. */
    void permissionResult(String[] permissions, int[] results) {
        awaitingResult = false;
        Pending active = queue.poll();
        if (active != null) {
            // Android passes empty arrays when the prompt is interrupted before the user answers; leave asked unset then.
            if (permissions != null && results != null && permissions.length > 0 && results.length > 0) {
                host.markAsked(active.name);
            }
            active.answer.accept(active.count, status(active.name), false);
        }
        pump();
    }

    private void pump() {
        while (!awaitingResult && !queue.isEmpty()) {
            Pending next = queue.peek();
            int st = status(next.name);
            if (st != Protocol.PERMISSION_NOT_ASKED) {
                queue.poll();
                next.answer.accept(next.count, st, false);
                continue;
            }
            List<String> req = required(next.name);
            List<String> missing = new ArrayList<>(req.size());
            for (String p : req) {
                if (!host.granted(p)) missing.add(p);
            }
            if (missing.isEmpty()) {
                queue.poll();
                next.answer.accept(next.count, Protocol.PERMISSION_GRANTED, false);
                continue;
            }
            awaitingResult = true;
            if (!host.request(missing.toArray(new String[0]))) {
                awaitingResult = false;
                queue.poll();
                next.answer.accept(next.count, status(next.name), false);
            }
        }
    }

    void clear() {
        queue.clear();
        awaitingResult = false;
    }
}
