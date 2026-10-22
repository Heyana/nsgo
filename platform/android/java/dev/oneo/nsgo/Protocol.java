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
    static final int IMAGE = 6;

    // Java to Go.
    static final int START = 1;
    static final int RESIZE = 2;
    static final int EVENT = 3;
    static final int MEASURED = 4;
    static final int ENVIRONMENT = 5;
    static final int DNS = 6;

    // The parts of an image request an IMAGE message carries.
    static final int IMAGE_PLACEHOLDER = 0;
    static final int IMAGE_SOURCE = 1;

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
    static final int ITEM_IMAGE = 16;
    static final int ITEM_MODAL = 17;
    static final int ITEM_ALERT = 18;
    static final int ITEM_DISMISS_KEYBOARD = 19;
    static final int ITEM_SCROLL_TO = 20;
    static final int ITEM_CANVAS = 21;
    static final int ITEM_SET_CLIPBOARD = 22;
    static final int ITEM_READ_CLIPBOARD = 23;
    static final int ITEM_ANNOUNCE = 24;
    static final int ITEM_KEYMAP = 25;
    static final int ITEM_STATUS_BAR = 26;
    static final int ITEM_TITLE = 27;
    static final int ITEM_BACK_UNHANDLED = 28;
    static final int ITEM_WINDOW = 29;
    static final int ITEM_MENU = 30;

    // View kinds, as in core/mount.go.
    static final int KIND_VIEW = 1;
    static final int KIND_TEXT = 2;
    static final int KIND_INPUT = 3;
    static final int KIND_SCROLL = 4;
    static final int KIND_IMAGE = 5;
    static final int KIND_MODAL = 6;
    static final int KIND_CANVAS = 7;

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
    static final int EVENT_LOAD_START = 12;
    static final int EVENT_PROGRESS = 13;
    static final int EVENT_LOAD = 14;
    static final int EVENT_ERROR = 15;
    static final int EVENT_LOAD_END = 16;
    static final int EVENT_PRESS_OUTSIDE = 17;
    static final int EVENT_BACK = 18;
    static final int EVENT_ALERT = 19;
    static final int EVENT_CLIPBOARD = 20;
    static final int EVENT_CONTEXT_MENU = 21;
    static final int EVENT_KEY = 22;
    static final int EVENT_PAN = 23;
    static final int EVENT_PINCH = 24;
    static final int EVENT_ROTATION = 25;

    // Gesture phases, as in core/gesture.go.
    static final int GESTURE_BEGAN = 1;
    static final int GESTURE_CHANGED = 2;
    static final int GESTURE_ENDED = 3;
    static final int GESTURE_CANCELLED = 4;

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
    static final int ROLE_RADIO_GROUP = 11;
    static final int ROLE_TAB_LIST = 12;
    static final int ROLE_MENU = 13;
    static final int ROLE_MENU_ITEM = 14;
    static final int ROLE_DIALOG = 15;
    static final int ROLE_ALERT = 16;

    // Keyboard types, as in core/mount.go.
    static final int KEYBOARD_EMAIL = 1;
    static final int KEYBOARD_NUMBER = 2;
    static final int KEYBOARD_DECIMAL = 3;
    static final int KEYBOARD_PHONE = 4;
    static final int KEYBOARD_URL = 5;

    // Resize modes, as in core/element.go; Cover is 0.
    static final int RESIZE_CONTAIN = 1;
    static final int RESIZE_STRETCH = 2;
    static final int RESIZE_CENTER = 3;

    // Modal animations, as in core/element.go.
    static final int MODAL_SLIDE = 0;
    static final int MODAL_FADE = 1;

    // Alert action roles, as in core/services.go; Default is 0.
    static final int ALERT_CANCEL = 1;
    static final int ALERT_DESTRUCTIVE = 2;

    // Transition properties, as in core/animate.go; 0 is all of them.
    static final int TRANSITION_OPACITY = 1;
    static final int TRANSITION_BACKGROUND = 2;
    static final int TRANSITION_TRANSFORM = 4;
    static final int TRANSITION_BORDER = 8;
    static final int TRANSITION_SHADOW = 16;

    // Status bar styles, as in core/keys.go; Auto is 0.
    static final int STATUS_BAR_DARK = 1;
    static final int STATUS_BAR_LIGHT = 2;

    // Drawing operations, as in core/canvas.go.
    static final int DRAW_SAVE = 1;
    static final int DRAW_RESTORE = 2;
    static final int DRAW_TRANSLATE = 3;
    static final int DRAW_SCALE = 4;
    static final int DRAW_ROTATE = 5;
    static final int DRAW_BEGIN_PATH = 6;
    static final int DRAW_MOVE_TO = 7;
    static final int DRAW_LINE_TO = 8;
    static final int DRAW_QUAD_TO = 9;
    static final int DRAW_CUBIC_TO = 10;
    static final int DRAW_ARC = 11;
    static final int DRAW_RECT = 12;
    static final int DRAW_ROUND_RECT = 13;
    static final int DRAW_ELLIPSE = 14;
    static final int DRAW_CLOSE_PATH = 15;
    static final int DRAW_FILL = 16;
    static final int DRAW_STROKE = 17;
    static final int DRAW_CLIP = 18;
    static final int DRAW_FILL_COLOR = 19;
    static final int DRAW_STROKE_COLOR = 20;
    static final int DRAW_LINE_WIDTH = 21;
    static final int DRAW_LINE_CAP = 22;
    static final int DRAW_LINE_JOIN = 23;
    static final int DRAW_ALPHA = 24;
    static final int DRAW_FILL_TEXT = 25;
    static final int DRAW_CLEAR = 26;

    // Line caps and joins, as in core/canvas.go; Butt and Miter are 0.
    static final int CAP_ROUND = 1;
    static final int CAP_SQUARE = 2;
    static final int JOIN_ROUND = 1;
    static final int JOIN_BEVEL = 2;

    // Device idioms, as in core/env.go.
    static final int IDIOM_PHONE = 0;
    static final int IDIOM_TABLET = 1;

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
