package dev.oneo.nsgo;

import android.view.InputDevice;
import android.view.KeyEvent;
import android.view.MotionEvent;
import java.util.ArrayList;
import java.util.List;

/**
 * Follows a pointing device over an NsgoView: hover enter, move and leave with
 * coordinates, modifier keys and held buttons, button-down mouse drags, and
 * modifier key changes under a still pointer.
 */
final class Mouse {
    /** Surface operations Mouse drives without coupling to NsgoView's Android view tree. */
    interface Host {
        /** Appends the node IDs under surface pixel point (x, y) that follow the mouse, outermost first. */
        void hitTest(float x, float y, List<Integer> under);

        /** Reports a mouse event (EVENT_HOVER_IN, EVENT_HOVER_OUT or EVENT_HOVER_MOVE) for node id at surface pixels (x, y). */
        void report(int id, int kind, float x, float y, int mods, int button);

        /** Whether surface pixel point (x, y) lies inside the surface bounds. */
        boolean contains(float x, float y);

        /** Whether any mounted node currently follows mouse events. */
        boolean hasListeners();
    }

    private final Host host;
    /** Currently hovered node IDs, outermost first. */
    private final ArrayList<Integer> over = new ArrayList<>();
    private boolean inside;
    private boolean dragging;
    private float lastX, lastY;
    private int lastMods;
    private int lastButton;

    Mouse(Host host) {
        this.host = host;
    }

    /** Whether an event mask follows the mouse (enter, leave or move). */
    static boolean follows(long events) {
        return (events & ((1L << Protocol.EVENT_HOVER_IN)
                | (1L << Protocol.EVENT_HOVER_OUT)
                | (1L << Protocol.EVENT_HOVER_MOVE))) != 0;
    }

    /** The bits core.Modifiers is sent as: control, alt, shift and meta (the platform key). */
    static int modifiers(int meta) {
        return ((meta & KeyEvent.META_CTRL_ON) != 0 ? 1 : 0)
                | ((meta & KeyEvent.META_ALT_ON) != 0 ? 2 : 0)
                | ((meta & KeyEvent.META_SHIFT_ON) != 0 ? 4 : 0)
                | ((meta & KeyEvent.META_META_ON) != 0 ? 8 : 0);
    }

    /** The first held mouse button in buttonState, as Protocol.MOUSE_BUTTON_* (Left, Right, Middle, Back, Forward). */
    static int button(int buttonState) {
        if ((buttonState & MotionEvent.BUTTON_PRIMARY) != 0) return Protocol.MOUSE_BUTTON_LEFT;
        if ((buttonState & MotionEvent.BUTTON_SECONDARY) != 0) return Protocol.MOUSE_BUTTON_RIGHT;
        if ((buttonState & MotionEvent.BUTTON_TERTIARY) != 0) return Protocol.MOUSE_BUTTON_MIDDLE;
        if ((buttonState & MotionEvent.BUTTON_BACK) != 0) return Protocol.MOUSE_BUTTON_BACK;
        if ((buttonState & MotionEvent.BUTTON_FORWARD) != 0) return Protocol.MOUSE_BUTTON_FORWARD;
        return Protocol.MOUSE_BUTTON_NONE;
    }

    /**
     * Whether a hover MotionEvent comes from a pointing device (mouse, touchpad
     * or hovering stylus) rather than TalkBack touch exploration on a touchscreen.
     */
    static boolean isHoverPointer(MotionEvent e) {
        int tool = e.getPointerCount() > 0 ? e.getToolType(0) : MotionEvent.TOOL_TYPE_UNKNOWN;
        if (tool == MotionEvent.TOOL_TYPE_MOUSE
                || tool == MotionEvent.TOOL_TYPE_STYLUS
                || tool == MotionEvent.TOOL_TYPE_ERASER) {
            return true;
        }
        return e.isFromSource(InputDevice.SOURCE_MOUSE)
                || e.isFromSource(InputDevice.SOURCE_MOUSE_RELATIVE)
                || e.isFromSource(InputDevice.SOURCE_TOUCHPAD)
                || e.isFromSource(InputDevice.SOURCE_STYLUS);
    }

    /**
     * Whether a touch-stream MotionEvent (ACTION_DOWN/MOVE/UP/CANCEL) belongs
     * to a mouse press or drag rather than a touchscreen finger or stylus touch.
     */
    static boolean isMouseTouch(MotionEvent e) {
        if (e.getPointerCount() <= 0) return false;
        if (e.getToolType(0) == MotionEvent.TOOL_TYPE_MOUSE) return true;
        return e.isFromSource(InputDevice.SOURCE_MOUSE)
                || e.isFromSource(InputDevice.SOURCE_MOUSE_RELATIVE)
                || button(e.getButtonState()) != Protocol.MOUSE_BUTTON_NONE;
    }

    /** Whether local point (x, y) reaches the node itself, honoring PointerEvents and hit slop. */
    static boolean reachSelf(boolean hasNode, int pointerEvents, float x, float y, float width, float height,
            float[] slop, float density) {
        if (!hasNode || pointerEvents == Protocol.POINTER_EVENTS_NONE || pointerEvents == Protocol.POINTER_EVENTS_BOX_NONE) {
            return false;
        }
        if (x >= 0 && y >= 0 && x < width && y < height) return true;
        return slop != null
                && x >= -slop[3] * density && y >= -slop[0] * density
                && x < width + slop[1] * density && y < height + slop[2] * density;
    }

    /** Whether local point (x, y) may descend into a view's children, honoring PointerEvents and clipping. */
    static boolean reachChildren(boolean hasNode, int pointerEvents, boolean clips, float x, float y, float width, float height) {
        if (hasNode && (pointerEvents == Protocol.POINTER_EVENTS_NONE || pointerEvents == Protocol.POINTER_EVENTS_BOX_ONLY)) {
            return false;
        }
        if (x >= 0 && y >= 0 && x < width && y < height) return true;
        return !clips;
    }

    /** Handles an Android hover event (ACTION_HOVER_ENTER/MOVE/EXIT). Returns whether listeners keep hover active. */
    boolean hover(MotionEvent e) {
        if (!isHoverPointer(e)) return false;
        int action = e.getActionMasked();
        float x = e.getX(), y = e.getY();
        int mods = modifiers(e.getMetaState());
        int btn = button(e.getButtonState());
        switch (action) {
            case MotionEvent.ACTION_HOVER_ENTER:
            case MotionEvent.ACTION_HOVER_MOVE:
                inside = true;
                lastX = x;
                lastY = y;
                lastMods = mods;
                lastButton = btn;
                updateOver(x, y, mods, btn, true);
                return host.hasListeners();
            case MotionEvent.ACTION_HOVER_EXIT:
                lastX = x;
                lastY = y;
                lastMods = mods;
                lastButton = btn;
                // Android sends ACTION_HOVER_EXIT right before ACTION_DOWN when a mouse button is pressed;
                // keep the hovered chain intact while the pointer stays inside with a button held.
                if (btn != Protocol.MOUSE_BUTTON_NONE && host.contains(x, y)) {
                    return host.hasListeners();
                }
                inside = false;
                leaveAll(x, y, mods, btn);
                return host.hasListeners();
            default:
                return false;
        }
    }

    /**
     * Handles a mouse button press, drag or release coming through dispatchTouchEvent.
     * Returns true while a mouse drag is active on a surface with mouse listeners so
     * Android keeps delivering ACTION_MOVE even when no view is clickable.
     */
    boolean touch(MotionEvent e) {
        if (!dragging && !isMouseTouch(e)) return false;
        int action = e.getActionMasked();
        float x = e.getX(), y = e.getY();
        int mods = modifiers(e.getMetaState());
        int btn = button(e.getButtonState());
        switch (action) {
            case MotionEvent.ACTION_DOWN:
                inside = host.contains(x, y);
                lastX = x;
                lastY = y;
                lastMods = mods;
                lastButton = btn;
                dragging = host.hasListeners();
                updateOver(x, y, mods, btn, false);
                return dragging;
            case MotionEvent.ACTION_MOVE:
                inside = host.contains(x, y);
                lastX = x;
                lastY = y;
                lastMods = mods;
                lastButton = btn;
                updateOver(x, y, mods, btn, true);
                return dragging;
            case MotionEvent.ACTION_UP:
                inside = host.contains(x, y);
                lastX = x;
                lastY = y;
                lastMods = mods;
                lastButton = btn;
                dragging = false;
                updateOver(x, y, mods, btn, false);
                return false;
            case MotionEvent.ACTION_CANCEL:
                dragging = false;
                lastX = x;
                lastY = y;
                lastMods = mods;
                lastButton = Protocol.MOUSE_BUTTON_NONE;
                if (!host.contains(x, y)) {
                    inside = false;
                    leaveAll(x, y, mods, Protocol.MOUSE_BUTTON_NONE);
                }
                return false;
            default:
                return dragging;
        }
    }

    /**
     * Reports a modifier key press or release while the mouse rests over the surface
     * as an EVENT_HOVER_MOVE to the hovered views, innermost first, with the new modifiers.
     */
    void keyEvent(KeyEvent e) {
        if (!inside || dragging || lastButton != Protocol.MOUSE_BUTTON_NONE) return;
        if (!KeyEvent.isModifierKey(e.getKeyCode())) return;
        int action = e.getAction();
        if ((action != KeyEvent.ACTION_DOWN && action != KeyEvent.ACTION_UP) || e.getRepeatCount() != 0) return;
        int mods = modifiers(e.getMetaState());
        if (mods == lastMods) return;
        lastMods = mods;
        for (int i = over.size() - 1; i >= 0; i--) {
            host.report(over.get(i), Protocol.EVENT_HOVER_MOVE, lastX, lastY, lastMods, lastButton);
        }
    }

    /** Drops a deleted or unlistening node from the hovered chain without reporting a leave. */
    void forget(int id) {
        for (int i = over.size() - 1; i >= 0; i--) {
            if (over.get(i) == id) {
                over.remove(i);
            }
        }
    }

    /** Re-evaluates what lies under the resting pointer after layout, visibility or modal changes. */
    void refresh() {
        if (!inside) return;
        updateOver(lastX, lastY, lastMods, lastButton, false);
    }

    /** Clears all tracked hover and drag state without emitting events (e.g. on detach or release). */
    void reset() {
        over.clear();
        inside = false;
        dragging = false;
        lastMods = 0;
        lastButton = Protocol.MOUSE_BUTTON_NONE;
    }

    boolean inside() {
        return inside;
    }

    boolean dragging() {
        return dragging;
    }

    int mods() {
        return lastMods;
    }

    int button() {
        return lastButton;
    }

    private void updateOver(float x, float y, int mods, int btn, boolean reportMove) {
        ArrayList<Integer> under = new ArrayList<>();
        if (inside) {
            host.hitTest(x, y, under);
        }
        // Leave views no longer under the pointer, innermost first.
        for (int i = over.size() - 1; i >= 0; i--) {
            int id = over.get(i);
            if (!containsId(under, id)) {
                host.report(id, Protocol.EVENT_HOVER_OUT, x, y, mods, btn);
            }
        }
        // Enter newly reached views, outermost first.
        for (int i = 0; i < under.size(); i++) {
            int id = under.get(i);
            if (!containsId(over, id)) {
                host.report(id, Protocol.EVENT_HOVER_IN, x, y, mods, btn);
            }
        }
        over.clear();
        over.addAll(under);
        // Report moves innermost first.
        if (reportMove) {
            for (int i = over.size() - 1; i >= 0; i--) {
                host.report(over.get(i), Protocol.EVENT_HOVER_MOVE, x, y, mods, btn);
            }
        }
    }

    private void leaveAll(float x, float y, int mods, int btn) {
        for (int i = over.size() - 1; i >= 0; i--) {
            host.report(over.get(i), Protocol.EVENT_HOVER_OUT, x, y, mods, btn);
        }
        over.clear();
    }

    private static boolean containsId(List<Integer> ids, int target) {
        for (int i = 0; i < ids.size(); i++) {
            if (ids.get(i) == target) return true;
        }
        return false;
    }
}
