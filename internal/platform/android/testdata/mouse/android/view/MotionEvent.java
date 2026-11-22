package android.view;

public class MotionEvent {
    public static final int ACTION_DOWN = 0;
    public static final int ACTION_UP = 1;
    public static final int ACTION_MOVE = 2;
    public static final int ACTION_CANCEL = 3;
    public static final int ACTION_HOVER_MOVE = 7;
    public static final int ACTION_HOVER_ENTER = 9;
    public static final int ACTION_HOVER_EXIT = 10;

    public static final int TOOL_TYPE_UNKNOWN = 0;
    public static final int TOOL_TYPE_FINGER = 1;
    public static final int TOOL_TYPE_STYLUS = 2;
    public static final int TOOL_TYPE_MOUSE = 3;
    public static final int TOOL_TYPE_ERASER = 4;

    public static final int BUTTON_PRIMARY = 1 << 0;
    public static final int BUTTON_SECONDARY = 1 << 1;
    public static final int BUTTON_TERTIARY = 1 << 2;
    public static final int BUTTON_BACK = 1 << 3;
    public static final int BUTTON_FORWARD = 1 << 4;
    public static final int BUTTON_STYLUS_PRIMARY = 1 << 5;
    public static final int BUTTON_STYLUS_SECONDARY = 1 << 6;

    private final int action;
    private final float x, y;
    private final int metaState;
    private final int buttonState;
    private final int source;
    private final int toolType;
    private final int pointerCount;

    public MotionEvent(int action, float x, float y, int metaState, int buttonState, int source, int toolType) {
        this(action, x, y, metaState, buttonState, source, toolType, 1);
    }

    public MotionEvent(int action, float x, float y, int metaState, int buttonState, int source, int toolType, int pointerCount) {
        this.action = action;
        this.x = x;
        this.y = y;
        this.metaState = metaState;
        this.buttonState = buttonState;
        this.source = source;
        this.toolType = toolType;
        this.pointerCount = pointerCount;
    }

    public int getActionMasked() {
        return action;
    }

    public float getX() {
        return x;
    }

    public float getY() {
        return y;
    }

    public int getMetaState() {
        return metaState;
    }

    public int getButtonState() {
        return buttonState;
    }

    public int getPointerCount() {
        return pointerCount;
    }

    public int getToolType(int pointerIndex) {
        return toolType;
    }

    public boolean isFromSource(int mask) {
        return (source & mask) == mask;
    }
}
