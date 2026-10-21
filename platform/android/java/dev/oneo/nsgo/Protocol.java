package dev.oneo.nsgo;

import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;

/**
 * The pipe protocol between the runtime and the app's Go process: messages
 * of a type byte, a big-endian length and a payload. Kept in sync with
 * platform/android/android.go, whose writers define each payload.
 */
final class Protocol {
    // Go to Java.
    static final int BATCH = 1;
    static final int PREPARE = 2;
    static final int MEASURE = 3;
    static final int LAYOUT = 4;
    static final int RELEASE = 5;

    // Java to Go.
    static final int START = 1;
    static final int RESIZE = 2;
    static final int EVENT = 3;
    static final int MEASURED = 4;

    // Mount item kinds, as in core/mount.go.
    static final int ITEM_CREATE = 1;
    static final int ITEM_INSERT = 2;
    static final int ITEM_REMOVE = 3;
    static final int ITEM_DELETE = 4;
    static final int ITEM_FRAME = 5;
    static final int ITEM_VIEW_STYLE = 6;
    static final int ITEM_TEXT = 7;
    static final int ITEM_INPUT = 8;
    static final int ITEM_INPUT_VALUE = 9;
    static final int ITEM_EVENTS = 10;
    static final int ITEM_VIEW_PROPS = 11;
    static final int ITEM_SCROLL_CONFIG = 12;
    static final int ITEM_SCROLL_CONTENT = 13;
    static final int ITEM_FOCUS = 14;
    static final int ITEM_BLUR = 15;

    // View kinds, as in core/mount.go.
    static final int KIND_VIEW = 1;
    static final int KIND_TEXT = 2;
    static final int KIND_INPUT = 3;
    static final int KIND_SCROLL = 4;

    // Event kinds, as in core/events.go. An event mask holds 1 << kind.
    static final int EVENT_PRESS = 1;
    static final int EVENT_PRESS_DOWN = 2;
    static final int EVENT_PRESS_UP = 3;
    static final int EVENT_LONG_PRESS = 4;
    static final int EVENT_HOVER_IN = 5;
    static final int EVENT_HOVER_OUT = 6;
    static final int EVENT_FOCUS = 7;
    static final int EVENT_BLUR = 8;
    static final int EVENT_CHANGE_TEXT = 9;
    static final int EVENT_SUBMIT_EDITING = 10;
    static final int EVENT_SCROLL = 11;

    // Accessibility roles, as in core/mount.go.
    static final int ROLE_BUTTON = 1;
    static final int ROLE_LINK = 2;
    static final int ROLE_HEADER = 3;
    static final int ROLE_IMAGE = 4;
    static final int ROLE_CHECKBOX = 5;
    static final int ROLE_SWITCH = 6;
    static final int ROLE_RADIO = 7;
    static final int ROLE_TAB = 8;
    static final int ROLE_TEXT = 9;
    static final int ROLE_TEXT_FIELD = 10;

    // Keyboard types, as in core/mount.go.
    static final int KEYBOARD_EMAIL = 1;
    static final int KEYBOARD_NUMBER = 2;
    static final int KEYBOARD_DECIMAL = 3;
    static final int KEYBOARD_PHONE = 4;
    static final int KEYBOARD_URL = 5;

    // Text alignment, as in core/style.go.
    static final int ALIGN_CENTER = 2;
    static final int ALIGN_END = 3;

    private Protocol() {}

    static String readString(DataInputStream in) throws IOException {
        byte[] bytes = new byte[in.readInt()];
        in.readFully(bytes);
        return new String(bytes, StandardCharsets.UTF_8);
    }

    static void writeString(DataOutputStream out, String s) throws IOException {
        byte[] bytes = s.getBytes(StandardCharsets.UTF_8);
        out.writeInt(bytes.length);
        out.write(bytes);
    }

    /** Whether bit i of a flags byte is set; Go writes the first flag lowest. */
    static boolean flag(int flags, int i) {
        return (flags & (1 << i)) != 0;
    }
}
