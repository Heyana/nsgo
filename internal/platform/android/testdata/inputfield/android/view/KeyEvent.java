package android.view;

public class KeyEvent {
    public static final int ACTION_DOWN = 0;
    public static final int KEYCODE_DEL = 67;
    public static final int KEYCODE_ENTER = 66;
    public int getAction() { return ACTION_DOWN; }
    public boolean isShiftPressed() { return false; }
    public boolean isAltPressed() { return false; }
    public int getUnicodeChar() { return 0; }
}
