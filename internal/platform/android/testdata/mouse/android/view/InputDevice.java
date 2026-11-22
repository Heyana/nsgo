package android.view;

public final class InputDevice {
    public static final int SOURCE_CLASS_POINTER = 0x00000002;
    public static final int SOURCE_TOUCHSCREEN = 0x00001000 | SOURCE_CLASS_POINTER;
    public static final int SOURCE_MOUSE = 0x00002000 | SOURCE_CLASS_POINTER;
    public static final int SOURCE_STYLUS = 0x00004000 | SOURCE_CLASS_POINTER;
    public static final int SOURCE_TOUCHPAD = 0x00100000 | SOURCE_CLASS_POINTER;
    public static final int SOURCE_MOUSE_RELATIVE = 0x00020000 | 0x00000004;

    private InputDevice() {}
}
