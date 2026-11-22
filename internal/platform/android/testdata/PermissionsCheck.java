package dev.oneo.nsgo;

import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashSet;
import java.util.List;
import java.util.Set;

/**
 * Exercises Permissions.java on the JVM without Android, and checks wire
 * round-trips with Go's writePermission and readEvent.
 */
public final class PermissionsCheck {
    private static final class FakeHost implements Permissions.Host {
        int sdk = 34;
        final Set<String> declared = new HashSet<>();
        final Set<String> granted = new HashSet<>();
        final Set<String> asked = new HashSet<>();
        final List<String[]> requests = new ArrayList<>();
        final List<String> warnings = new ArrayList<>();
        boolean requestSucceeds = true;
        boolean settingsSucceeds = true;
        int settingsOpened;

        @Override public int sdkInt() { return sdk; }
        @Override public boolean granted(String permission) { return granted.contains(permission); }
        @Override public boolean declared(String permission) { return declared.contains(permission); }
        @Override public boolean asked(String key) { return asked.contains(key); }
        @Override public void markAsked(String key) { asked.add(key); }
        @Override public boolean request(String[] permissions) {
            if (!requestSucceeds) return false;
            requests.add(permissions);
            return true;
        }
        @Override public boolean openSettings() {
            if (!settingsSucceeds) return false;
            settingsOpened++;
            return true;
        }
        @Override public void warn(String message) { warnings.add(message); }
    }

    private static final class Recorded {
        final long count;
        final int status;
        final boolean opened;

        Recorded(long count, int status, boolean opened) {
            this.count = count;
            this.status = status;
            this.opened = opened;
        }
    }

    public static void main(String[] args) throws Exception {
        if (args.length >= 1 && "roundtrip".equals(args[0])) {
            roundtrip();
            return;
        }
        apiLevelMapping();
        checkAskAndPersistence();
        interruptedPrompt();
        undeclaredPermission();
        calendarsMultiPermission();
        queueAndCoalescing();
        noActivityFailsCleanly();
        openSettings();
        guideAndStopGuide();
    }

    /** Reads one ItemPermission payload from stdin, runs Permissions, and writes one msgEvent payload to stdout. */
    private static void roundtrip() throws IOException {
        DataInputStream in = new DataInputStream(System.in);
        long count = in.readInt() & 0xffffffffL;
        int command = in.readUnsignedByte();
        String name = Protocol.readString(in);

        FakeHost host = new FakeHost();
        host.sdk = 34;
        host.declared.addAll(Arrays.asList(
                Permissions.RECORD_AUDIO, Permissions.CAMERA,
                Permissions.READ_CALENDAR, Permissions.WRITE_CALENDAR,
                Permissions.BLUETOOTH_CONNECT, Permissions.READ_MEDIA_AUDIO));
        host.granted.add(Permissions.RECORD_AUDIO);
        Permissions perms = new Permissions(host);

        DataOutputStream out = new DataOutputStream(System.out);
        perms.handle(count, command, name, (c, status, opened) -> {
            try {
                // Mirror NsgoView.event's exact wire layout (surface=0).
                out.writeInt(0);
                out.writeByte(Protocol.EVENT_PERMISSION);
                out.writeInt(0);
                out.writeInt((int) c);
                out.writeFloat(0);
                out.writeFloat(0);
                Protocol.writeString(out, "");
                out.writeFloat(0);
                out.writeFloat(0);
                out.writeInt(status);
                out.writeByte(0);
                out.writeFloat(0);
                out.writeFloat(0);
                out.writeFloat(0);
                out.writeByte(0); // no navigation
                out.writeLong(0);
                out.writeLong(0);
                out.writeByte(0);
                Protocol.writeString(out, "");
                out.writeByte(opened ? 1 : 0);
                out.writeFloat(0);
                out.writeFloat(0);
                out.writeFloat(0);
                out.writeFloat(0);
                out.writeByte(0);
                out.flush();
            } catch (IOException e) {
                throw new RuntimeException(e);
            }
        });
    }

    private static void apiLevelMapping() {
        assertArrayEquals(new String[] { Permissions.RECORD_AUDIO }, Permissions.androidPermissions("microphone", 28), "mic 28");
        assertArrayEquals(new String[] { Permissions.CAMERA }, Permissions.androidPermissions("camera", 34), "camera 34");
        assertArrayEquals(new String[] { Permissions.READ_CALENDAR, Permissions.WRITE_CALENDAR },
                Permissions.androidPermissions("calendars", 34), "calendars 34");

        // Bluetooth: BLUETOOTH before API 31, BLUETOOTH_CONNECT from API 31.
        assertArrayEquals(new String[] { Permissions.BLUETOOTH }, Permissions.androidPermissions("bluetooth", 28), "bt 28");
        assertArrayEquals(new String[] { Permissions.BLUETOOTH }, Permissions.androidPermissions("bluetooth", 30), "bt 30");
        assertArrayEquals(new String[] { Permissions.BLUETOOTH_CONNECT }, Permissions.androidPermissions("bluetooth", 31), "bt 31");
        assertArrayEquals(new String[] { Permissions.BLUETOOTH_CONNECT }, Permissions.androidPermissions("bluetooth", 36), "bt 36");

        // Media library: READ_EXTERNAL_STORAGE before API 33, READ_MEDIA_AUDIO from API 33.
        assertArrayEquals(new String[] { Permissions.READ_EXTERNAL_STORAGE }, Permissions.androidPermissions("media-library", 28), "media 28");
        assertArrayEquals(new String[] { Permissions.READ_EXTERNAL_STORAGE }, Permissions.androidPermissions("media-library", 32), "media 32");
        assertArrayEquals(new String[] { Permissions.READ_MEDIA_AUDIO }, Permissions.androidPermissions("media-library", 33), "media 33");
        assertArrayEquals(new String[] { Permissions.READ_MEDIA_AUDIO }, Permissions.androidPermissions("media-library", 36), "media 36");

        for (String unsupported : new String[] {
                "accessibility", "screen-recording", "input-monitoring", "full-disk-access",
                "app-management", "developer-tools", "reminders", "unknown", "", null }) {
            if (Permissions.androidPermissions(unsupported, 34) != null) {
                throw new AssertionError("expected null for unsupported permission " + unsupported);
            }
        }

        // Legacy API 30 Bluetooth (install-time granted when declared) and API 32 media library.
        FakeHost legacy = new FakeHost();
        legacy.sdk = 30;
        legacy.declared.add(Permissions.BLUETOOTH);
        legacy.granted.add(Permissions.BLUETOOTH);
        legacy.declared.add(Permissions.READ_EXTERNAL_STORAGE);
        Permissions perms = new Permissions(legacy);
        assertEquals(Protocol.PERMISSION_GRANTED, perms.status("bluetooth"), "legacy bt granted");
        assertEquals(Protocol.PERMISSION_NOT_ASKED, perms.status("media-library"), "legacy media not asked");
        List<Recorded> out = new ArrayList<>();
        perms.handle(1, Protocol.PERMISSION_ASK, "media-library", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertEquals(1, legacy.requests.size(), "legacy media request count");
        assertArrayEquals(new String[] { Permissions.READ_EXTERNAL_STORAGE }, legacy.requests.get(0), "legacy media prompt");
    }

    private static void checkAskAndPersistence() {
        FakeHost host = new FakeHost();
        host.declared.add(Permissions.CAMERA);
        host.declared.add(Permissions.RECORD_AUDIO);
        Permissions perms = new Permissions(host);
        List<Recorded> out = new ArrayList<>();

        // Initial check: prompt.
        perms.handle(1, Protocol.PERMISSION_CHECK, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, 0, 1, Protocol.PERMISSION_NOT_ASKED, false);

        // Ask and deny: marks asked, answers denied.
        perms.handle(2, Protocol.PERMISSION_ASK, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertEquals(1, host.requests.size(), "camera prompted once");
        assertArrayEquals(new String[] { Permissions.CAMERA }, host.requests.get(0), "camera perm");
        perms.permissionResult(new String[] { Permissions.CAMERA }, new int[] { -1 });
        assertRecorded(out, 1, 2, Protocol.PERMISSION_DENIED, false);
        if (!host.asked.contains("camera")) throw new AssertionError("camera not marked asked");

        // Subsequent check and ask stay denied without prompting again.
        perms.handle(3, Protocol.PERMISSION_CHECK, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, 2, 3, Protocol.PERMISSION_DENIED, false);
        perms.handle(4, Protocol.PERMISSION_ASK, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, 3, 4, Protocol.PERMISSION_DENIED, false);
        assertEquals(1, host.requests.size(), "camera must not prompt again after denial");

        // Granted later in system Settings: status flips to granted despite asked=true.
        host.granted.add(Permissions.CAMERA);
        perms.handle(5, Protocol.PERMISSION_CHECK, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, 4, 5, Protocol.PERMISSION_GRANTED, false);

        // Revoked again in Settings: back to denied (asked persisted).
        host.granted.remove(Permissions.CAMERA);
        perms.handle(6, Protocol.PERMISSION_CHECK, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, 5, 6, Protocol.PERMISSION_DENIED, false);

        // Ask and grant microphone.
        perms.handle(7, Protocol.PERMISSION_ASK, "microphone", (c, s, o) -> out.add(new Recorded(c, s, o)));
        host.granted.add(Permissions.RECORD_AUDIO);
        perms.permissionResult(new String[] { Permissions.RECORD_AUDIO }, new int[] { 0 });
        assertRecorded(out, 6, 7, Protocol.PERMISSION_GRANTED, false);
    }

    private static void interruptedPrompt() {
        FakeHost host = new FakeHost();
        host.declared.add(Permissions.CAMERA);
        Permissions perms = new Permissions(host);
        List<Recorded> out = new ArrayList<>();

        perms.handle(10, Protocol.PERMISSION_ASK, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        // Android delivers empty arrays when a dialog is interrupted (rotation/cancellation).
        perms.permissionResult(new String[0], new int[0]);
        assertRecorded(out, 0, 10, Protocol.PERMISSION_NOT_ASKED, false);
        if (host.asked.contains("camera")) throw new AssertionError("interrupted prompt must not mark asked");

        // Retry still prompts.
        perms.handle(11, Protocol.PERMISSION_ASK, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertEquals(2, host.requests.size(), "retry after interruption prompts again");
        host.granted.add(Permissions.CAMERA);
        perms.permissionResult(new String[] { Permissions.CAMERA }, new int[] { 0 });
        assertRecorded(out, 1, 11, Protocol.PERMISSION_GRANTED, false);
    }

    private static void undeclaredPermission() {
        FakeHost host = new FakeHost();
        Permissions perms = new Permissions(host);
        List<Recorded> out = new ArrayList<>();

        perms.handle(1, Protocol.PERMISSION_CHECK, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, 0, 1, Protocol.PERMISSION_UNAVAILABLE, false);

        perms.handle(2, Protocol.PERMISSION_ASK, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, 1, 2, Protocol.PERMISSION_UNAVAILABLE, false);
        assertEquals(0, host.requests.size(), "undeclared must not prompt");
        if (host.warnings.size() != 1 || !host.warnings.get(0).contains(Permissions.CAMERA)) {
            throw new AssertionError("expected missing manifest warning, got " + host.warnings);
        }

        // Unsupported macOS-only permission answers unavailable without warning.
        perms.handle(3, Protocol.PERMISSION_ASK, "accessibility", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, 2, 3, Protocol.PERMISSION_UNAVAILABLE, false);
        assertEquals(1, host.warnings.size(), "macOS-only permission must not log manifest warning");
    }

    private static void calendarsMultiPermission() {
        // Both READ_CALENDAR and WRITE_CALENDAR declared: both required for granted.
        FakeHost host = new FakeHost();
        host.declared.add(Permissions.READ_CALENDAR);
        host.declared.add(Permissions.WRITE_CALENDAR);
        Permissions perms = new Permissions(host);
        List<Recorded> out = new ArrayList<>();

        perms.handle(1, Protocol.PERMISSION_ASK, "calendars", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertArrayEquals(new String[] { Permissions.READ_CALENDAR, Permissions.WRITE_CALENDAR },
                host.requests.get(0), "ask both calendar permissions");
        // Partial grant: only READ_CALENDAR granted -> denied.
        host.granted.add(Permissions.READ_CALENDAR);
        perms.permissionResult(new String[] { Permissions.READ_CALENDAR, Permissions.WRITE_CALENDAR }, new int[] { 0, -1 });
        assertRecorded(out, 0, 1, Protocol.PERMISSION_DENIED, false);

        // Once WRITE_CALENDAR is also granted -> granted.
        host.granted.add(Permissions.WRITE_CALENDAR);
        perms.handle(2, Protocol.PERMISSION_CHECK, "calendars", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, 1, 2, Protocol.PERMISSION_GRANTED, false);

        // When READ_CALENDAR is already granted and WRITE_CALENDAR is not asked yet, only WRITE_CALENDAR is prompted.
        FakeHost partial = new FakeHost();
        partial.declared.add(Permissions.READ_CALENDAR);
        partial.declared.add(Permissions.WRITE_CALENDAR);
        partial.granted.add(Permissions.READ_CALENDAR);
        Permissions partialPerms = new Permissions(partial);
        partialPerms.handle(3, Protocol.PERMISSION_ASK, "calendars", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertArrayEquals(new String[] { Permissions.WRITE_CALENDAR }, partial.requests.get(0), "ask only missing calendar perm");
        partial.granted.add(Permissions.WRITE_CALENDAR);
        partialPerms.permissionResult(new String[] { Permissions.WRITE_CALENDAR }, new int[] { 0 });
        assertRecorded(out, 2, 3, Protocol.PERMISSION_GRANTED, false);

        // Only READ_CALENDAR declared: granted when READ_CALENDAR is granted.
        FakeHost readOnly = new FakeHost();
        readOnly.declared.add(Permissions.READ_CALENDAR);
        Permissions readOnlyPerms = new Permissions(readOnly);
        readOnlyPerms.handle(4, Protocol.PERMISSION_ASK, "calendars", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertArrayEquals(new String[] { Permissions.READ_CALENDAR }, readOnly.requests.get(0), "read-only calendar perm");
        readOnly.granted.add(Permissions.READ_CALENDAR);
        readOnlyPerms.permissionResult(new String[] { Permissions.READ_CALENDAR }, new int[] { 0 });
        assertRecorded(out, 3, 4, Protocol.PERMISSION_GRANTED, false);
    }

    private static void queueAndCoalescing() {
        FakeHost host = new FakeHost();
        host.declared.add(Permissions.CAMERA);
        host.declared.add(Permissions.RECORD_AUDIO);
        Permissions perms = new Permissions(host);
        List<Recorded> out = new ArrayList<>();

        // Queue camera twice, then microphone.
        perms.handle(10, Protocol.PERMISSION_ASK, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        perms.handle(11, Protocol.PERMISSION_ASK, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        perms.handle(12, Protocol.PERMISSION_ASK, "microphone", (c, s, o) -> out.add(new Recorded(c, s, o)));

        assertEquals(1, host.requests.size(), "only first prompt active");
        assertArrayEquals(new String[] { Permissions.CAMERA }, host.requests.get(0), "first is camera");

        // Resolve camera: both 10 and 11 resolve, and microphone starts immediately.
        host.granted.add(Permissions.CAMERA);
        perms.permissionResult(new String[] { Permissions.CAMERA }, new int[] { 0 });
        assertEquals(2, out.size(), "both camera asks answered");
        assertRecorded(out, 0, 10, Protocol.PERMISSION_GRANTED, false);
        assertRecorded(out, 1, 11, Protocol.PERMISSION_GRANTED, false);
        assertEquals(2, host.requests.size(), "microphone started next");
        assertArrayEquals(new String[] { Permissions.RECORD_AUDIO }, host.requests.get(1), "second is mic");

        // Deny microphone.
        perms.permissionResult(new String[] { Permissions.RECORD_AUDIO }, new int[] { -1 });
        assertEquals(3, out.size(), "mic answered");
        assertRecorded(out, 2, 12, Protocol.PERMISSION_DENIED, false);
    }

    private static void noActivityFailsCleanly() {
        FakeHost host = new FakeHost();
        host.declared.add(Permissions.CAMERA);
        host.requestSucceeds = false;
        Permissions perms = new Permissions(host);
        List<Recorded> out = new ArrayList<>();

        perms.handle(20, Protocol.PERMISSION_ASK, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, 0, 20, Protocol.PERMISSION_NOT_ASKED, false);
        if (host.asked.contains("camera")) throw new AssertionError("failed prompt start must not mark asked");
    }

    private static void openSettings() {
        FakeHost host = new FakeHost();
        host.declared.add(Permissions.CAMERA);
        Permissions perms = new Permissions(host);
        List<Recorded> out = new ArrayList<>();

        // Supported and declared -> opens.
        perms.handle(1, Protocol.PERMISSION_OPEN_SETTINGS, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, 0, 1, 0, true);
        assertEquals(1, host.settingsOpened, "settings opened for camera");

        // Sensor permissions -> opens.
        for (String sensor : new String[] { "location", "background-location", "motion" }) {
            perms.handle(2, Protocol.PERMISSION_OPEN_SETTINGS, sensor, (c, s, o) -> out.add(new Recorded(c, s, o)));
            assertRecorded(out, out.size() - 1, 2, 0, true);
        }
        assertEquals(4, host.settingsOpened, "settings opened for sensors");

        // Undeclared Android permission -> does not open.
        perms.handle(3, Protocol.PERMISSION_OPEN_SETTINGS, "microphone", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, out.size() - 1, 3, 0, false);

        // Unsupported permission -> does not open.
        perms.handle(4, Protocol.PERMISSION_OPEN_SETTINGS, "reminders", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, out.size() - 1, 4, 0, false);
        assertEquals(4, host.settingsOpened, "unsupported did not open settings");

        // Activity launch failure -> reports false.
        host.settingsSucceeds = false;
        perms.handle(5, Protocol.PERMISSION_OPEN_SETTINGS, "camera", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, out.size() - 1, 5, 0, false);
    }

    private static void guideAndStopGuide() {
        FakeHost host = new FakeHost();
        host.declared.add(Permissions.BLUETOOTH_CONNECT);
        host.granted.add(Permissions.BLUETOOTH_CONNECT);
        Permissions perms = new Permissions(host);
        List<Recorded> out = new ArrayList<>();

        perms.handle(30, Protocol.PERMISSION_GUIDE, "bluetooth", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertRecorded(out, 0, 30, Protocol.PERMISSION_GRANTED, false);

        perms.handle(31, Protocol.PERMISSION_STOP_GUIDE, "", (c, s, o) -> out.add(new Recorded(c, s, o)));
        assertEquals(1, out.size(), "stop guide must not answer");
    }

    private static void assertRecorded(List<Recorded> list, int index, long count, int status, boolean opened) {
        if (index >= list.size()) throw new AssertionError("missing recorded answer at " + index);
        Recorded r = list.get(index);
        if (r.count != count || r.status != status || r.opened != opened) {
            throw new AssertionError("answer[" + index + "] = (" + r.count + ", " + r.status + ", " + r.opened
                    + "), want (" + count + ", " + status + ", " + opened + ")");
        }
    }

    private static void assertEquals(int want, int got, String what) {
        if (want != got) throw new AssertionError(what + ": got " + got + ", want " + want);
    }

    private static void assertArrayEquals(String[] want, String[] got, String what) {
        if (!Arrays.equals(want, got)) {
            throw new AssertionError(what + ": got " + Arrays.toString(got) + ", want " + Arrays.toString(want));
        }
    }
}
