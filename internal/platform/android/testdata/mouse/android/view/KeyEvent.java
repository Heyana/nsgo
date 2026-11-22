package android.view;

public class KeyEvent {
    public static final int ACTION_DOWN = 0;
    public static final int ACTION_UP = 1;

    public static final int META_ALT_ON = 0x02;
    public static final int META_SHIFT_ON = 0x01;
    public static final int META_CTRL_ON = 0x1000;
    public static final int META_META_ON = 0x10000;

    public static final int KEYCODE_A = 29;
    public static final int KEYCODE_ALT_LEFT = 57;
    public static final int KEYCODE_SHIFT_LEFT = 59;
    public static final int KEYCODE_CTRL_LEFT = 113;
    public static final int KEYCODE_META_LEFT = 117;

    private final int action;
    private final int keyCode;
    private final int metaState;
    private final int repeatCount;

    public KeyEvent(int action, int keyCode, int metaState, int repeatCount) {
        this.action = action;
        this.keyCode = keyCode;
        this.metaState = metaState;
        this.repeatCount = repeatCount;
    }

    public int getAction() {
        return action;
    }

    public int getKeyCode() {
        return keyCode;
    }

    public int getMetaState() {
        return metaState;
    }

    public int getRepeatCount() {
        return repeatCount;
    }

    public static boolean isModifierKey(int keyCode) {
        return keyCode == KEYCODE_SHIFT_LEFT
                || keyCode == KEYCODE_ALT_LEFT
                || keyCode == KEYCODE_CTRL_LEFT
                || keyCode == KEYCODE_META_LEFT;
    }
}
