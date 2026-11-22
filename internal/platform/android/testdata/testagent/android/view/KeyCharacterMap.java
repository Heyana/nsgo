package android.view;

public class KeyCharacterMap {
    public static final int VIRTUAL_KEYBOARD = -1;
    public static KeyCharacterMap load(int id) { return new KeyCharacterMap(); }
    public KeyEvent[] getEvents(char[] chars) { return null; }
}
