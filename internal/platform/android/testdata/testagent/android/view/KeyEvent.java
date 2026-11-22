package android.view;

public class KeyEvent {
    public static final int ACTION_DOWN = 0, ACTION_UP = 1;
    public static final int META_ALT_ON = 0x02, META_ALT_LEFT_ON = 0x10;
    public static final int META_SHIFT_ON = 0x01, META_SHIFT_LEFT_ON = 0x40;
    public static final int META_CTRL_ON = 0x1000, META_CTRL_LEFT_ON = 0x2000;
    public static final int META_META_ON = 0x10000;
    public static final int KEYCODE_ENTER = 66, KEYCODE_DEL = 67, KEYCODE_FORWARD_DEL = 112;
    public static final int KEYCODE_ESCAPE = 111, KEYCODE_TAB = 61, KEYCODE_SPACE = 62;
    public static final int KEYCODE_DPAD_LEFT = 21, KEYCODE_DPAD_RIGHT = 22, KEYCODE_DPAD_UP = 19, KEYCODE_DPAD_DOWN = 20;
    public static final int KEYCODE_MOVE_HOME = 122, KEYCODE_MOVE_END = 123;
    public static final int KEYCODE_A = 29, KEYCODE_0 = 7;

    public KeyEvent(long down, long event, int action, int code, int repeat, int meta) {}
    public KeyEvent(long down, long event, int action, int code, int repeat, int meta, int dev, int scan) {}
}
