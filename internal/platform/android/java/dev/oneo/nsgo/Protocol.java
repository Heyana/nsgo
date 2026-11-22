package dev.oneo.nsgo;

import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.EOFException;
import java.io.IOException;
import java.nio.charset.StandardCharsets;

/**
 * The pipe protocol between the runtime and the app's Go process: messages
 * of a type byte, a big-endian length and a payload. Kept in sync with
 * internal/platform/android/android.go, whose writers define each payload.
 */
final class Protocol {
    /** Bounds an allocation before reading a frame, including images and font data; Go refuses a longer message before writing it (maxMessage in protocol.go). */
    static final int MAX_MESSAGE_SIZE = 64 * 1024 * 1024;

    static final class Message {
        final int kind;
        final byte[] payload;

        Message(int kind, byte[] payload) {
            this.kind = kind;
            this.payload = payload;
        }
    }

    /** Reads Go's next message, or null at a clean end of the pipe. */
    static Message readMessage(DataInputStream in) throws IOException {
        int kind = in.read();
        if (kind == -1) return null;
        if (kind < BATCH || kind > EXTENDED) throw invalidMessage("unexpected message kind " + kind);
        int length;
        try {
            length = in.readInt();
        } catch (EOFException e) {
            throw invalidMessage("incomplete message header");
        }
        if (length < 0 || length > MAX_MESSAGE_SIZE) {
            throw invalidMessage("message length " + Integer.toUnsignedLong(length) + " exceeds " + MAX_MESSAGE_SIZE + " bytes");
        }
        byte[] payload = new byte[length];
        try {
            in.readFully(payload);
        } catch (EOFException e) {
            throw invalidMessage("incomplete message payload (expected " + length + " bytes)");
        }
        if (kind == EXTENDED) {
            if (length == 0) throw invalidMessage("extended message without its kind");
            int extended = payload[0] & 0xff;
            if (!isExtended(extended)) throw invalidMessage("unexpected extended message kind " + extended);
        }
        return new Message(kind, payload);
    }

    /** Whether Go writes extended messages of this kind: each EXT_* constant has its case. */
    static boolean isExtended(int kind) {
        switch (kind) {
            case EXT_AUDIO_FILE:
                return true;
            case EXT_DISCOVERY:
            case EXT_SURFACE_RESET:
                return true;
            case EXT_ASSET_READ:
                return true;
            case EXT_FRAME_RATE:
                return true;
            case EXT_GAME_SERVICES:
                return true;
            case EXT_DOWNLOAD:
                return true;
            default:
                return false;
        }
    }

    private static IOException invalidMessage(String reason) {
        return new IOException("NSGo pipe: " + reason + "; stdout is reserved for the runtime protocol; " +
                "write logs to stderr and call nsgotest.Main or nsgotest.MainFunc from TestMain when testing");
    }

    // Go to Java.
    static final int BATCH = 1;
    static final int RELEASE = 5;
    static final int IMAGE = 6;
    static final int CANVAS_IMAGE = 8;
    static final int FONTS = 9;
    static final int TEST = 10;
    static final int HOST_CALL = 11;
    /** Requests a frame from the surface named by its ID, or the first surface without a payload. */
    static final int WANT_FRAME = 13;
    static final int FONT_DATA = 14;
    static final int FOLDER = 15;
    static final int FETCH = 16;
    /** Starts, updates or ends a Live Activity: JSON (LiveUpdates). */
    static final int LIVE_UPDATE = 17;
    /** Starts or stops a recording or asks for the microphone: JSON (RecordingService). */
    static final int RECORDING = 18;
    /** Reads, writes or deletes a secret: its number, a SECRET_* op, the service, the account and the value (Secrets). */
    static final int SECRET = 19;
    /** Sets, lists or clears the web views' cookies and storage: the surface, the request's count and a COOKIES_* op, then for COOKIES_ADD each cookie's URL and Set-Cookie value, for COOKIES_LIST the URL (webcookie.go). */
    static final int COOKIES = 20;
    /** Starts or stops the code scanner: the surface, the request's count and a SCAN_START or SCAN_STOP (scan.go, Scanner). */
    static final int SCAN = 21;
    /** A bitmap of the next batch: item index, key, width, height and premultiplied RGBA bytes. */
    static final int PIXEL_FRAME = 22;

    /** The requesting surface, the request's count and the app's data directory. */
    static final int DISK_SPACE = 23;

    /** Starts frame sampling (with or without a visible panel), or updates the panel. */
    static final int PERF_PANEL = 24;

    /** The shader programs no view draws any more: their count, then each one's number (ShaderNode.release). */
    static final int SHADER_RELEASE = 25;

    /** A correlated text request: request ID and operation. */
    static final int TEXT_REQUEST = 26;
    /** Starts or stops rumble on a game controller: device id, lowFreq, highFreq, durationMs (gamepad.go, Gamepads). */
    static final int GAMEPAD_RUMBLE = 27;

    /** A message of an extended kind: an EXT_* byte, then that message's payload, which NsgoView.extended reads. The kinds stop here so that stray text on stdout is refused rather than read as a message; new Go-to-Java messages are extended kinds. */
    static final int EXTENDED = 31;
    /** The environment's third capability byte's bit for a runtime that reads EXTENDED messages (capExtendedMessages in protocol.go): Go sends none to a runtime without it, which would refuse them as a corrupt pipe. */
    static final int CAP_EXTENDED_MESSAGES = 1;
    /** The third capability byte's bit for a runtime that plays sounds and players (Audio.java), capAudio in protocol.go. */
    static final int CAP_AUDIO = 2;

    // Extended kinds, the first byte of an EXTENDED message, kept in sync with android.go's ext* constants: each is EXT_NAME = n with n from 1 up, and has its case in isExtended.
    /** A staged audio file, or a failure: surface, object, path, error code and message (audio.go, Audio.file). */
    static final int EXT_AUDIO_FILE = 2;
    /** Starts or stops DNS-SD on a surface (Discovery): surface, request count, then a DISCOVERY_* operation and its service. */
    static final int EXT_DISCOVERY = 10;
    static final int DISCOVERY_ADVERTISE = 1;
    static final int DISCOVERY_BROWSE = 2;
    static final int DISCOVERY_STOP = 3;
    /** The environment's third capability byte's bit for a runtime that runs Discovery (capDiscoverServices in protocol.go). */
    static final int CAP_DISCOVER_SERVICES = 32;
    /** A full native tree follows: surface ID and its attachment generation (ViewAttachment). */
    static final int EXT_SURFACE_RESET = 11;
    /** The environment's third capability byte's bit for a device that plays timed vibration patterns (capHapticPatterns in protocol.go). */
    static final int CAP_HAPTIC_PATTERNS = 16;
    /** The environment's third capability byte's bit for a runtime that streams a FETCH_TO_FILE fetch to its file (Downloads.java), capFetchToFile in protocol.go: Go sends FETCH_TO_FILE only to a runtime with it. */
    static final int CAP_FETCH_TO_FILE = 64;
    /** Sets the surface's preferred frame rate: its ID and a float in Hz. */
    static final int EXT_FRAME_RATE = 1;
    /** A process-wide background download command, JSON, which NsgoEngine hands to BackgroundDownloadService. */
    static final int EXT_DOWNLOAD = 5;

    /** Reads a bundled file: its request ID and the path under assets/. */
    static final int EXT_ASSET_READ = 9;
    /** A JSON command for package gameservices to the Play Games Services hook (NsgoView answers it unavailable when the runtime has none). */
    static final int EXT_GAME_SERVICES = 8;

    // Java to Go.
    static final int START = 1;
    static final int RESIZE = 2;
    static final int EVENT = 3;
    static final int ENVIRONMENT = 5;
    static final int DNS = 6;
    static final int TEST_REPLY = 9;
    static final int HOST = 10;
    /** A Choreographer frame began, answering WANT_FRAME: its frameTimeNanos as a long, then the optional surface ID. */
    static final int FRAME = 12;
    static final int FOLDER_REPLY = 13;
    static final int FETCHED = 14;
    /** What LiveUpdates reports: JSON. */
    static final int LIVE_REPORT = 15;
    /** A surface's state, end, detach or resync, after its ID (overlay.go). */
    static final int SURFACE = 16;
    /** The device's time zone: its ID, then its offset from UTC now in seconds (timezone.go). */
    static final int TIME_ZONE = 17;
    /** What RecordingService reports: JSON. */
    static final int RECORDING_REPORT = 18;
    /** Answers a SECRET: its number, an error ("" for none), whether a value was found and the value. */
    static final int SECRET_REPLY = 19;
    /** A sensor session's report or coalesced sample: JSON (sensors.go). */
    static final int SENSOR_REPORT = 20;
    /** Answers COOKIES: its surface and count, an error, "" for none, and its op, then for COOKIES_LIST the URL and the Cookie header CookieManager gave for it. */
    static final int COOKIES_SET = 21;
    /** Requests or cancels a canvas image: surface, key, generation, cancel. */
    static final int CANVAS_REQUEST = 22;
    /** What the code scanner reports: the surface, the request's count and a SCAN_FRAME with the frame's width, height and luminance, a SCAN_CLOSED, or a SCAN_FAILED with why (scan.go). */
    static final int SCAN_REPORT = 23;
    static final int DOWNLOAD_REPORT = 36;

    /** Game controller connect, disconnect or button/axis state (gamepad.go, Gamepads). */
    static final int GAMEPAD = 24;

    // What a GAMEPAD message reports, as gamepad.go has them.
    static final int GAMEPAD_CONNECT = 0;
    static final int GAMEPAD_DISCONNECT = 1;
    static final int GAMEPAD_STATE = 2;

    /** Answers DISK_SPACE with its surface and count, an error ("" for none), total and allocatable bytes. */
    static final int DISK_SPACE_REPLY = 25;

    /** Answers TEXT_REQUEST with its request ID, its operation, and the reply bytes or an error. */
    static final int TEXT_REPLY = 27;
    /** Frame timestamp, display period, app UI work in nanoseconds, and whether the app changed. */
    static final int PERF_FRAME = 28;
    /** A hardware key: surface, view, scan code, Android key code, flags and name. */
    static final int PHYSICAL_KEY = 30;
    /** Reports a resolved service, its removal or an error (discovery.go). */
    static final int DISCOVERY_REPLY = 40;
    /** Streams a bundled file: its request ID, an ASSET_* status byte and the chunk or error text. */
    static final int ASSET_REPLY = 29;
    static final int ASSET_CHUNK = 0;
    static final int ASSET_DONE = 1;
    static final int ASSET_MISSING = 2;
    static final int ASSET_FAILED = 3;
    /** What the Play Games Services hook reports: JSON (driver.ReportGameServices). */
    static final int GAME_SERVICES_REPORT = 31;

    /** A sound or player report (Audio.java, audio.go). */
    static final int AUDIO_REPORT = 33;

    // TEXT_REQUEST carries a request ID and one of these operations.
    static final int TEXT_PREPARE = 1;
    static final int TEXT_MEASURE = 2;
    static final int TEXT_LAYOUT = 3;
    static final int TEXT_LAYOUT_LINES = 4;
    static final int TEXT_CARETS = 5;

    /** Writes a message's part after its head. */
    interface Body {
        void write(DataOutputStream out) throws IOException;
    }

    // What a SCAN message asks and a SCAN_REPORT reports, as scan.go has them.
    static final int SCAN_START = 0;
    static final int SCAN_STOP = 1;
    static final int SCAN_FRAME = 0;
    static final int SCAN_CLOSED = 1;
    static final int SCAN_FAILED = 2;

    // What a COOKIES message asks, as driver.WebCookiesOp has them.
    static final int COOKIES_ADD = 0;
    static final int COOKIES_LIST = 1;
    static final int COOKIES_CLEAR = 2;

    // What a SECRET message asks, as secrets.go has them.
    static final int SECRET_READ = 0;
    static final int SECRET_WRITE = 1;
    static final int SECRET_DELETE = 2;

    // What a SURFACE message reports, as overlay.go has them: the window's size and place in points, then the environment, or its end, an OVERLAY_END_*.
    static final int SURFACE_STATE = 1;
    static final int SURFACE_ENDED = 2;
    static final int SURFACE_DETACH = 3;
    static final int SURFACE_RESYNC = 4;

    // The ITEM_WINDOW commands an overlay takes, as protocol.go's writeWindow has them.
    static final int WINDOW_NONE = 0;
    static final int WINDOW_FRAME = 1;
    static final int WINDOW_SHOW = 2;
    static final int WINDOW_HIDE = 3;
    static final int WINDOW_FRONT = 4;
    static final int WINDOW_DESTROY = 5;

    // What a FOLDER message asks, as folders.go has them.
    static final int FOLDER_LIST = 0;
    static final int FOLDER_READ = 1;

    // What a FETCH message asks and the parts of a FETCHED, as fetch.go
    // has them.
    static final int FETCH_START = 0;
    static final int FETCH_CANCEL = 1;
    static final int FETCH_TO_FILE = 2;
    static final int FETCHED_PROGRESS = 0;
    static final int FETCHED_DONE = 1;

    // The parts of a HOST message, and of a HOST_CALL, as host.go has them.
    static final int HOST_EVENT = 1;
    static final int HOST_HANDLE = 2;
    static final int HOST_REPLY = 3;
    static final int HOST_TOKENS = 4;
    static final int HOST_URL_HANDLER = 5;
    static final int HOST_URL_ANSWER = 6;
    static final int CALL_CALL = 1;
    static final int CALL_OPEN_URL = 2;
    static final int CALL_ERROR = 3;

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
    static final int ITEM_KEYMAP = 26;
    static final int ITEM_STATUS_BAR = 27;
    static final int ITEM_TITLE = 28;
    static final int ITEM_BACK_UNHANDLED = 29;
    static final int ITEM_WINDOW = 30;
    static final int ITEM_MENU = 31;
    static final int ITEM_WEB = 32;
    static final int ITEM_WEB_COMMAND = 33;
    static final int ITEM_VIDEO = 34;
    static final int ITEM_VIDEO_SEEK = 35;
    static final int ITEM_NATIVE = 36;
    static final int ITEM_NATIVE_COMMAND = 37;
    static final int ITEM_ANIMATION = 38;
    static final int ITEM_ZOOM_TO = 39;
    static final int ITEM_SELECTION = 41;
    static final int ITEM_SCROLL_BY = 42;
    static final int ITEM_STICKY_CELLS = 43;
    static final int ITEM_ACCESSIBILITY_LABEL = 44;
    static final int ITEM_ACCESSIBILITY_FOCUS = 45;
    static final int ITEM_SCREEN = 46;
    static final int ITEM_OVERFLOW = 47;
    static final int ITEM_BLOCK_NATIVE = 48;
    static final int ITEM_SHADER = 49;
    static final int ITEM_SET_CLIPBOARD_IMAGE = 50;
    static final int ITEM_OPEN_URL = 51;
    static final int ITEM_PICK_IMAGES = 52;
    static final int ITEM_APPEARANCE = 53;
    static final int ITEM_POPUP_MENU = 54;
    static final int ITEM_QUIT = 55;
    static final int ITEM_BACK_PREVIEW = 56;
    static final int ITEM_HISTORY = 57;
    static final int ITEM_PICK_FILES = 58;
    static final int ITEM_SAVE_FILE = 59;
    static final int ITEM_DIRECTION = 60;
    static final int ITEM_PICK_DIRECTORIES = 62;
    static final int ITEM_OPEN_FILE = 63;
    static final int ITEM_SHARE = 64;
    static final int ITEM_INSTALL_PACKAGE = 65;
    static final int ITEM_CAN_INSTALL_PACKAGES = 66;
    static final int ITEM_INSTALL_SETTINGS = 67;
    static final int ITEM_TAKE_PHOTO = 78;
    static final int ITEM_MATCH = 79;
    static final int ITEM_HAPTIC = 81;
    static final int ITEM_NOTIFICATION_PERMISSION = 82;
    static final int ITEM_SHOW_NOTIFICATION = 83;
    static final int ITEM_CLOSE_NOTIFICATION = 84;
    static final int ITEM_BADGE = 85;
    static final int ITEM_BIOMETRIC_AVAILABILITY = 86;
    static final int ITEM_BIOMETRIC_AUTHENTICATE = 87;
    static final int ITEM_OPEN_SURFACE = 68;
    static final int ITEM_OVERLAY_PERMISSION = 89;
    static final int ITEM_PERMISSION = 90;
    static final int ITEM_KEEP_AWAKE = 93;
    static final int ITEM_RELEASE_AWAKE = 94;
    static final int ITEM_AUDIO = 96;
    static final int ITEM_SHADER_TRANSITION = 75;
    static final int ITEM_SHADER_INPUT = 76;
    static final int ITEM_SPEAK = 102;
    static final int ITEM_SENSOR = 103;
    static final int ITEM_ORIENTATION = 111;

    static final int ORIENTATION_PORTRAIT = 1;
    static final int ORIENTATION_PORTRAIT_UPSIDE_DOWN = 2;
    static final int ORIENTATION_LANDSCAPE_LEFT = 4;
    static final int ORIENTATION_LANDSCAPE_RIGHT = 8;
    static final int ORIENTATION_LANDSCAPE = 12;

    static final int ITEM_KEEP_IMAGE = 112;
    static final int ITEM_RELEASE_IMAGE = 113;
    static final int ITEM_REQUEST_REVIEW = 115;
    static final int ITEM_SAVE_PREFLIGHT_ANSWER = 117;
    static final int ITEM_SAVE_FILE_PREFLIGHT = 118;
    static final int ITEM_SYSTEM_GESTURES = 121;
    static final int ITEM_HAPTIC_PATTERN = 124;

    // What EVENT_OVERLAY_PERMISSION answers ITEM_OVERLAY_PERMISSION with, and why an overlay ends, as the driver's.
    static final int OVERLAY_DENIED = 0;
    static final int OVERLAY_GRANTED = 1;
    static final int OVERLAY_UNAVAILABLE = 2;
    static final int OVERLAY_END_DENIED = 1;
    static final int OVERLAY_END_UNAVAILABLE = 2;
    static final int OVERLAY_END_REVOKED = 3;
    static final int OVERLAY_END_REMOVED = 4;
    static final int OVERLAY_END_FAILED = 5;

    // What EVENT_NOTIFICATION answers ITEM_NOTIFICATION_PERMISSION with, as the driver's.
    static final int NOTIFICATIONS_NOT_ASKED = 0;
    static final int NOTIFICATIONS_GRANTED = 1;
    static final int NOTIFICATIONS_DENIED = 2;
    static final int NOTIFICATIONS_UNAVAILABLE = 3;

    // What EVENT_PERMISSION answers ITEM_PERMISSION with, as the driver's Permission* statuses.
    static final int PERMISSION_NOT_ASKED = 0;
    static final int PERMISSION_GRANTED = 1;
    static final int PERMISSION_DENIED = 2;
    static final int PERMISSION_UNKNOWN = 3;
    static final int PERMISSION_NOT_GATED = 4;
    static final int PERMISSION_UNAVAILABLE = 5;

    // What ITEM_PERMISSION asks, as driver.PermissionCommand.
    static final int PERMISSION_CHECK = 0;
    static final int PERMISSION_ASK = 1;
    static final int PERMISSION_OPEN_SETTINGS = 2;
    static final int PERMISSION_GUIDE = 3;
    static final int PERMISSION_STOP_GUIDE = 4;

    // View kinds, as in core/mount.go.
    static final int KIND_VIEW = 1;
    static final int KIND_TEXT = 2;
    static final int KIND_INPUT = 3;
    static final int KIND_SCROLL = 4;
    static final int KIND_IMAGE = 5;
    static final int KIND_MODAL = 6;
    static final int KIND_CANVAS = 7;
    static final int KIND_WEB = 8;
    static final int KIND_VIDEO = 9;
    static final int KIND_NATIVE = 10;
    static final int KIND_SHADER = 11;

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
    /** A video paused by the system or its native controls; index is the reason. */
    static final int EVENT_VIDEO_PAUSE = 42;
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
    static final int EVENT_MENU = 26;
    static final int EVENT_MESSAGE = 27;
    static final int EVENT_NAVIGATION_STATE_CHANGE = 28;
    static final int EVENT_END = 29;
    static final int EVENT_BUFFER = 30;
    static final int EVENT_FILE_DRAG_ENTER = 31;
    static final int EVENT_FILE_DRAG_LEAVE = 32;
    static final int EVENT_FILE_DROP = 33;
    static final int EVENT_NATIVE = 34;
    static final int EVENT_NATIVE_SIZE = 35;
    static final int EVENT_ANIMATION = 36;
    static final int EVENT_SCROLL_DRAG_START = 37;
    static final int EVENT_SCROLL_DRAG_END = 38;
    static final int EVENT_MOMENTUM_START = 39;
    static final int EVENT_MOMENTUM_END = 40;
    static final int EVENT_REFRESH = 41;
    static final int EVENT_SELECTION_CHANGE = 43;
    static final int EVENT_KEY_PRESS = 44;
    static final int EVENT_COMPOSITION_CHANGE = 45;
    static final int EVENT_ACCESSIBILITY_ACTION = 46;
    static final int EVENT_HOVER_MOVE = 49;
    static final int EVENT_DOWNLOAD = 50;
    static final int EVENT_TOUCH = 51;
    static final int EVENT_CARET_MOVE = 52;
    static final int EVENT_PASTE = 53;
    static final int EVENT_SCREEN_HEADER = 54;
    static final int EVENT_OPEN_URL = 55;
    static final int EVENT_PICK = 56;
    /** A video previously paused by the system or its native controls started again. */
    static final int EVENT_VIDEO_PLAY = 57;
    static final int EVENT_BACK_STARTED = 58;
    static final int EVENT_BACK_CANCELLED = 59;
    static final int EVENT_SCROLL_END = 60;
    static final int EVENT_WHEEL = 61;
    static final int EVENT_FILES = 62;
    static final int EVENT_MODAL_SHOWN = 63;
    static final int EVENT_MODAL_DISMISSED = 64;
    static final int EVENT_SCREEN_INSETS = 65;
    static final int EVENT_INPUT_MODALITY = 75;
    static final int EVENT_NOTIFICATION = 76;
    static final int EVENT_NOTIFICATION_PRESS = 77;
    static final int EVENT_BIOMETRIC = 78;
    static final int EVENT_OVERLAY_PERMISSION = 80;
    static final int EVENT_PERMISSION = 82;
    static final int EVENT_OPEN_LINKS = 87;
    static final int EVENT_SPEAK = 90;

    static final int EVENT_SAVE_PREFLIGHT = 99;
    static final int EVENT_SHARED = 102;

    // Reasons carried in EVENT_VIDEO_PAUSE's index, as core.VideoPauseReason.
    static final int VIDEO_PAUSE_INTERRUPTION = 0;
    static final int VIDEO_PAUSE_ROUTE_LOST = 1;
    static final int VIDEO_PAUSE_CONTROLS = 2;

    static final int BIOMETRIC_NONE = 0;
    static final int BIOMETRIC_FACE = 1;
    static final int BIOMETRIC_FINGERPRINT = 2;
    static final int BIOMETRIC_SUCCESS = 1;
    static final int BIOMETRIC_FAILURE = 2;
    static final int BIOMETRIC_CANCELLED = 3;
    static final int BIOMETRIC_UNAVAILABLE = 4;

    // core.Haptic, what ITEM_HAPTIC plays.
    static final int HAPTIC_SELECTION = 1;
    static final int HAPTIC_LIGHT = 2;
    static final int HAPTIC_MEDIUM = 3;
    static final int HAPTIC_HEAVY = 4;
    static final int HAPTIC_SOFT = 5;
    static final int HAPTIC_RIGID = 6;
    static final int HAPTIC_SUCCESS = 7;
    static final int HAPTIC_WARNING = 8;
    static final int HAPTIC_ERROR = 9;

    // What EVENT_INPUT_MODALITY reports, as driver's Modality constants.
    static final int MODALITY_KEYBOARD = 1;
    static final int MODALITY_POINTER = 2;

    // core.PointerEvents.
    static final int POINTER_EVENTS_NONE = 1;
    static final int POINTER_EVENTS_BOX_NONE = 2;
    static final int POINTER_EVENTS_BOX_ONLY = 3;

    // driver.MouseButton; 0 is none.
    static final int MOUSE_BUTTON_NONE = 0;
    static final int MOUSE_BUTTON_LEFT = 1;
    static final int MOUSE_BUTTON_RIGHT = 2;
    static final int MOUSE_BUTTON_MIDDLE = 3;
    static final int MOUSE_BUTTON_BACK = 4;
    static final int MOUSE_BUTTON_FORWARD = 5;

    // Web view commands, as in core/webview.go.
    static final int WEB_RELOAD = 1;
    static final int WEB_GO_BACK = 2;
    static final int WEB_GO_FORWARD = 3;
    static final int WEB_STOP_LOADING = 4;
    static final int WEB_POST_MESSAGE = 5;
    static final int WEB_INJECT_JAVASCRIPT = 6;
    // WebSendInput (7) never reaches Android, which has no trusted input.
    static final int WEB_LOAD = 8;

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
    static final int ROLE_ADJUSTABLE = 17;
    static final int ROLE_PROGRESS_BAR = 18;
    static final int ROLE_TABLE = 19;
    static final int ROLE_GRID = 20;
    static final int ROLE_ROW_GROUP = 21;
    static final int ROLE_ROW = 22;
    static final int ROLE_COLUMN_HEADER = 23;
    static final int ROLE_CELL = 24;
    static final int ROLE_LISTBOX = 25;
    static final int ROLE_OPTION = 26;
    static final int ROLE_TREE = 30;
    static final int ROLE_TREE_ITEM = 31;
    static final int ROLE_MENU_ITEM_CHECKBOX = 33;
    static final int ROLE_MENU_ITEM_RADIO = 34;
    static final int ROLE_SPIN_BUTTON = 35;
    static final int ROLE_STATUS = 38;
    static final int ROLE_SEARCH_FIELD = 39;

    // The keys a focused view uses itself, as driver.KeyUse: a keymap binding's flags carry the ones it yields from bit 2 on.
    static final int KEY_USE_PRESS = 1;
    static final int KEY_USE_ADJUST = 2;
    static final int KEY_USE_SELECT = 4;
    static final int KEY_USE_TEXT = 8;

    // How a view is an accessibility element, as core.Accessible.
    static final int ACCESSIBLE_YES = 1;
    static final int ACCESSIBLE_NO = 2;

    // The words ITEM_DIRECTION carries, in the language the app shows, for what TalkBack has no word of its own: roles and states, by their index, as protocol.go's spoken words.
    static final int SPOKEN_LINK = 0;
    static final int SPOKEN_TAB = 1;
    static final int SPOKEN_SEARCH_BOX = 2;
    static final int SPOKEN_STATUS = 3;
    static final int SPOKEN_MENU = 4;
    static final int SPOKEN_MENU_ITEM = 5;
    static final int SPOKEN_DIALOG = 6;
    static final int SPOKEN_ALERT = 7;
    static final int SPOKEN_TABLE = 8;
    static final int SPOKEN_GRID = 9;
    static final int SPOKEN_ROW = 10;
    static final int SPOKEN_COLUMN_HEADER = 11;
    static final int SPOKEN_CELL = 12;
    static final int SPOKEN_PARTIALLY_CHECKED = 13;
    static final int SPOKEN_EXPANDED = 14;
    static final int SPOKEN_COLLAPSED = 15;
    static final int SPOKEN_SEPARATOR = 16;
    static final int SPOKEN_SPIN_BUTTON = 17;
    static final int SPOKEN_HEADING_LEVEL = 18;
    static final int SPOKEN_LEVEL = 19;

    // Keyboard types, as in core/mount.go.
    static final int KEYBOARD_EMAIL = 1;
    static final int KEYBOARD_NUMBER = 2;
    static final int KEYBOARD_DECIMAL = 3;
    static final int KEYBOARD_PHONE = 4;
    static final int KEYBOARD_URL = 5;

    // Resize modes, as in core/element.go.
    static final int RESIZE_COVER = 0;
    static final int RESIZE_CONTAIN = 1;
    static final int RESIZE_STRETCH = 2;
    static final int RESIZE_CENTER = 3;
    static final int RESIZE_SCALE_DOWN = 4;

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

    // Status bar styles, as in core/statusbar.go; Auto is 1.
    static final int STATUS_BAR_DARK = 2;
    static final int STATUS_BAR_LIGHT = 3;

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
    static final int DRAW_FILL_GRADIENT = 27;
    static final int DRAW_STROKE_GRADIENT = 28;
    static final int DRAW_TRANSFORM = 29;
    static final int DRAW_SET_TRANSFORM = 30;
    static final int DRAW_COMPOSITE = 31;
    static final int DRAW_LINE_DASH = 32;
    static final int DRAW_LINE_DASH_OFFSET = 33;
    static final int DRAW_MITER_LIMIT = 34;
    static final int DRAW_SHADOW = 35;
    static final int DRAW_STROKE_TEXT = 36;
    static final int DRAW_IMAGE = 37;
    static final int DRAW_VERTICES = 40;
    static final int DRAW_ATLAS = 41;

    // Gradient spreads, as in core/canvas.go; Pad is 0.
    static final int SPREAD_REFLECT = 1;
    static final int SPREAD_REPEAT = 2;

    // Line caps and joins, as in core/canvas.go; Butt and Miter are 0.
    static final int CAP_ROUND = 1;
    static final int CAP_SQUARE = 2;
    static final int JOIN_ROUND = 1;
    static final int JOIN_BEVEL = 2;

    // Device idioms, as in core/env.go.
    static final int IDIOM_PHONE = 0;
    static final int IDIOM_TABLET = 1;
    static final int IDIOM_TV = 4;

    // driver.Selectable's off.
    static final int SELECTABLE_ON = 1;
    static final int SELECTABLE_OFF = 2;

    // Appearances, as driver.Appearance; 0 is the system's.
    static final int APPEARANCE_LIGHT = 1;
    static final int APPEARANCE_DARK = 2;

    // Text alignment, as in core/style.go.
    static final int ALIGN_CENTER = 2;
    static final int ALIGN_END = 3;
    static final int ALIGN_JUSTIFY = 4;

    // Where a line limit cuts text, as in core/text.go; 0 and Tail end
    // the last line with an ellipsis.
    static final int ELLIPSIZE_HEAD = 2;
    static final int ELLIPSIZE_MIDDLE = 3;
    static final int ELLIPSIZE_CLIP = 4;

    // Decoration styles, as in core/style.go; 0 and Solid are solid.
    static final int DECORATION_SOLID = 1;
    static final int DECORATION_DOUBLE = 2;
    static final int DECORATION_DOTTED = 3;
    static final int DECORATION_DASHED = 4;
    static final int DECORATION_WAVY = 5;

    // Border and outline styles, as in driver/effects.go; 0 is solid.
    static final int BORDER_SOLID = 0;
    static final int BORDER_DASHED = 2;
    static final int BORDER_DOTTED = 3;

    // Gradient kinds, as in driver/gradient.go.
    static final int GRADIENT_LINEAR = 0;
    static final int GRADIENT_RADIAL = 1;
    static final int GRADIENT_CONIC = 2;

    // Vertical alignments of attachments, as in core/style.go; 0 and
    // Baseline are the baseline.
    static final int VALIGN_TEXT_TOP = 2;
    static final int VALIGN_TEXT_BOTTOM = 3;
    static final int VALIGN_MIDDLE = 4;

    private Protocol() {}

    static byte[] readBytes(DataInputStream in) throws IOException {
        byte[] bytes = new byte[in.readInt()];
        in.readFully(bytes);
        return bytes;
    }

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

    /** Encodes pasted HTML as a base64 line in EventPaste's data, as Go's readPaste reads it. */
    static void appendPasteHTML(StringBuilder data, String html) {
        if (html == null || html.isEmpty()) return;
        data.append("html\t")
                .append(java.util.Base64.getEncoder().encodeToString(html.getBytes(StandardCharsets.UTF_8)))
                .append('\n');
    }

    /** Encodes a pasted file: URI as a line in EventPaste's data. */
    static void appendPasteURI(StringBuilder data, String uri) {
        if (uri == null || uri.isEmpty()) return;
        data.append("uri\t").append(uri).append('\n');
    }

    /** Encodes a pasted file's MIME type and bytes as a line in EventPaste's data. */
    static void appendPasteFile(StringBuilder data, String mime, byte[] bytes) {
        if (mime == null || mime.isEmpty()) return;
        data.append("file\t").append(mime).append('\t')
                .append(java.util.Base64.getEncoder().encodeToString(bytes == null ? new byte[0] : bytes))
                .append('\n');
    }
}
