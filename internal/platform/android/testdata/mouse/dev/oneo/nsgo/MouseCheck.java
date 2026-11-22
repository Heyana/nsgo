package dev.oneo.nsgo;

import android.view.InputDevice;
import android.view.KeyEvent;
import android.view.MotionEvent;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

/** Exercises Mouse.java on the host JVM against minimal Android view stubs (#1553). */
public final class MouseCheck {
    private static final long ALL_MOUSE = (1L << Protocol.EVENT_HOVER_IN)
            | (1L << Protocol.EVENT_HOVER_OUT)
            | (1L << Protocol.EVENT_HOVER_MOVE);

    static final class Box {
        final int id;
        float x, y, w, h;
        long events;
        int pointerEvents;
        boolean visible = true;
        float alpha = 1f;
        boolean clips;
        boolean modal;
        boolean disabled;
        float[] slop;
        final List<Box> children = new ArrayList<>();

        Box(int id, float x, float y, float w, float h, long events) {
            this.id = id;
            this.x = x;
            this.y = y;
            this.w = w;
            this.h = h;
            this.events = events;
        }
    }

    static final class FakeHost implements Mouse.Host {
        final float width, height, density;
        final Box root;
        final List<String> log = new ArrayList<>();

        FakeHost(float width, float height, float density, Box root) {
            this.width = width;
            this.height = height;
            this.density = density;
            this.root = root;
        }

        @Override
        public void hitTest(float x, float y, List<Integer> under) {
            Box topModal = findTopModal(root);
            if (topModal != null) {
                float[] local = toLocal(root, topModal, x, y);
                if (local != null) walk(topModal, local[0], local[1], under);
                return;
            }
            walk(root, x, y, under);
        }

        private Box findTopModal(Box b) {
            if (b == null || !b.visible || b.pointerEvents == Protocol.POINTER_EVENTS_NONE) return null;
            for (int i = b.children.size() - 1; i >= 0; i--) {
                Box m = findTopModal(b.children.get(i));
                if (m != null) return m;
            }
            return b.modal ? b : null;
        }

        private float[] toLocal(Box cur, Box target, float x, float y) {
            float lx = x - cur.x, ly = y - cur.y;
            if (cur == target) return new float[] {lx, ly};
            for (Box c : cur.children) {
                float[] p = toLocal(c, target, lx, ly);
                if (p != null) return p;
            }
            return null;
        }

        private boolean walk(Box b, float x, float y, List<Integer> out) {
            if (!b.visible) return false;
            if (!b.modal && b.alpha <= 0f) return false;
            if (b.pointerEvents == Protocol.POINTER_EVENTS_NONE) return false;
            boolean self = Mouse.reachSelf(true, b.pointerEvents, x, y, b.w, b.h, b.slop, density);
            if (self && Mouse.follows(b.events)) out.add(b.id);
            if (Mouse.reachChildren(true, b.pointerEvents, b.clips, x, y, b.w, b.h)) {
                for (int i = b.children.size() - 1; i >= 0; i--) {
                    Box c = b.children.get(i);
                    if (walk(c, x - c.x, y - c.y, out)) return true;
                }
            }
            boolean inside = x >= 0 && y >= 0 && x < b.w && y < b.h;
            return self || (b.modal && inside);
        }

        private Box findById(Box b, int id) {
            if (b == null) return null;
            if (b.id == id) return b;
            for (Box c : b.children) {
                Box f = findById(c, id);
                if (f != null) return f;
            }
            return null;
        }

        @Override
        public void report(int id, int kind, float x, float y, int mods, int button) {
            Box b = findById(root, id);
            if (b == null || (b.events & (1L << kind)) == 0) return;
            float[] local = toLocal(root, b, x, y);
            float lx = local != null ? local[0] / density : 0;
            float ly = local != null ? local[1] / density : 0;
            String k = kind == Protocol.EVENT_HOVER_IN ? "IN"
                    : kind == Protocol.EVENT_HOVER_OUT ? "OUT"
                    : kind == Protocol.EVENT_HOVER_MOVE ? "MOVE" : "?" + kind;
            log.add(k + ":" + id + "@" + fmt(lx) + "," + fmt(ly) + "|page=" + fmt(x / density) + "," + fmt(y / density)
                    + "|m=" + mods + "|b=" + button);
        }

        @Override
        public boolean contains(float x, float y) {
            return x >= 0 && y >= 0 && x < width && y < height;
        }

        @Override
        public boolean hasListeners() {
            return anyListener(root);
        }

        private boolean anyListener(Box b) {
            if (b == null) return false;
            if (Mouse.follows(b.events)) return true;
            for (Box c : b.children) {
                if (anyListener(c)) return true;
            }
            return false;
        }
    }

    private static String fmt(float v) {
        if (v == (int) v) return Integer.toString((int) v);
        return Float.toString(v);
    }

    private static void check(boolean cond, String msg) {
        if (!cond) throw new AssertionError(msg);
    }

    private static void checkLog(FakeHost host, String... want) {
        List<String> expected = Arrays.asList(want);
        if (!host.log.equals(expected)) {
            throw new AssertionError("events mismatch:\n  got:  " + host.log + "\n  want: " + expected);
        }
        host.log.clear();
    }

    public static void main(String[] args) {
        testModifiersAndButtons();
        testPointerClassification();
        testReachRules();
        testHoverAncestorTransitionsAndExit();
        testButtonDownDragMovesAndHoverTransitions();
        testModifierKeyChangesUnderStillMouse();
        testPointerEventsDisabledModalAndDeletedViews();
    }

    private static void testModifiersAndButtons() {
        check(Mouse.modifiers(0) == 0, "no mods");
        check(Mouse.modifiers(KeyEvent.META_CTRL_ON) == 1, "ctrl");
        check(Mouse.modifiers(KeyEvent.META_ALT_ON) == 2, "alt");
        check(Mouse.modifiers(KeyEvent.META_SHIFT_ON) == 4, "shift");
        check(Mouse.modifiers(KeyEvent.META_META_ON) == 8, "meta");
        check(Mouse.modifiers(KeyEvent.META_CTRL_ON | KeyEvent.META_SHIFT_ON | KeyEvent.META_META_ON) == (1 | 4 | 8),
                "combined mods");

        check(Mouse.button(0) == Protocol.MOUSE_BUTTON_NONE, "no button");
        check(Mouse.button(MotionEvent.BUTTON_PRIMARY) == Protocol.MOUSE_BUTTON_LEFT, "left");
        check(Mouse.button(MotionEvent.BUTTON_SECONDARY) == Protocol.MOUSE_BUTTON_RIGHT, "right");
        check(Mouse.button(MotionEvent.BUTTON_TERTIARY) == Protocol.MOUSE_BUTTON_MIDDLE, "middle");
        check(Mouse.button(MotionEvent.BUTTON_BACK) == Protocol.MOUSE_BUTTON_BACK, "back");
        check(Mouse.button(MotionEvent.BUTTON_FORWARD) == Protocol.MOUSE_BUTTON_FORWARD, "forward");
        // Priority order: Left > Right > Middle > Back > Forward
        check(Mouse.button(MotionEvent.BUTTON_SECONDARY | MotionEvent.BUTTON_PRIMARY) == Protocol.MOUSE_BUTTON_LEFT, "left over right");
        check(Mouse.button(MotionEvent.BUTTON_TERTIARY | MotionEvent.BUTTON_SECONDARY) == Protocol.MOUSE_BUTTON_RIGHT, "right over middle");
        check(Mouse.button(MotionEvent.BUTTON_BACK | MotionEvent.BUTTON_TERTIARY) == Protocol.MOUSE_BUTTON_MIDDLE, "middle over back");
        check(Mouse.button(MotionEvent.BUTTON_FORWARD | MotionEvent.BUTTON_BACK) == Protocol.MOUSE_BUTTON_BACK, "back over forward");
        // Stylus barrel buttons are not mouse buttons.
        check(Mouse.button(MotionEvent.BUTTON_STYLUS_PRIMARY | MotionEvent.BUTTON_STYLUS_SECONDARY) == Protocol.MOUSE_BUTTON_NONE,
                "stylus buttons ignored");

        check(Mouse.follows(1L << Protocol.EVENT_HOVER_IN), "follows enter");
        check(Mouse.follows(1L << Protocol.EVENT_HOVER_OUT), "follows leave");
        check(Mouse.follows(1L << Protocol.EVENT_HOVER_MOVE), "follows move");
        check(!Mouse.follows(1L << Protocol.EVENT_PRESS), "press does not follow mouse");
    }

    private static void testPointerClassification() {
        // Real hover devices: mouse, touchpad, mouse relative, stylus, eraser.
        check(Mouse.isHoverPointer(new MotionEvent(MotionEvent.ACTION_HOVER_MOVE, 10, 10, 0, 0,
                InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE)), "mouse hover");
        check(Mouse.isHoverPointer(new MotionEvent(MotionEvent.ACTION_HOVER_MOVE, 10, 10, 0, 0,
                InputDevice.SOURCE_TOUCHPAD, MotionEvent.TOOL_TYPE_FINGER)), "touchpad hover");
        check(Mouse.isHoverPointer(new MotionEvent(MotionEvent.ACTION_HOVER_MOVE, 10, 10, 0, 0,
                InputDevice.SOURCE_MOUSE_RELATIVE, MotionEvent.TOOL_TYPE_UNKNOWN)), "relative mouse hover");
        check(Mouse.isHoverPointer(new MotionEvent(MotionEvent.ACTION_HOVER_MOVE, 10, 10, 0, 0,
                InputDevice.SOURCE_STYLUS | InputDevice.SOURCE_TOUCHSCREEN, MotionEvent.TOOL_TYPE_STYLUS)), "stylus hover");
        check(Mouse.isHoverPointer(new MotionEvent(MotionEvent.ACTION_HOVER_MOVE, 10, 10, 0, 0,
                InputDevice.SOURCE_TOUCHSCREEN, MotionEvent.TOOL_TYPE_ERASER)), "eraser hover");
        // TalkBack touch exploration uses SOURCE_TOUCHSCREEN + TOOL_TYPE_FINGER: must NOT be treated as mouse hover.
        check(!Mouse.isHoverPointer(new MotionEvent(MotionEvent.ACTION_HOVER_MOVE, 10, 10, 0, 0,
                InputDevice.SOURCE_TOUCHSCREEN, MotionEvent.TOOL_TYPE_FINGER)), "talkback finger hover rejected");

        // Mouse touch drags vs finger/stylus touches.
        check(Mouse.isMouseTouch(new MotionEvent(MotionEvent.ACTION_MOVE, 10, 10, 0, MotionEvent.BUTTON_PRIMARY,
                InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE)), "mouse drag");
        check(Mouse.isMouseTouch(new MotionEvent(MotionEvent.ACTION_MOVE, 10, 10, 0, MotionEvent.BUTTON_SECONDARY,
                InputDevice.SOURCE_TOUCHPAD, MotionEvent.TOOL_TYPE_FINGER)), "touchpad button drag");
        check(!Mouse.isMouseTouch(new MotionEvent(MotionEvent.ACTION_MOVE, 10, 10, 0, 0,
                InputDevice.SOURCE_TOUCHSCREEN, MotionEvent.TOOL_TYPE_FINGER)), "touchscreen finger rejected");
        check(!Mouse.isMouseTouch(new MotionEvent(MotionEvent.ACTION_MOVE, 10, 10, 0, 0,
                InputDevice.SOURCE_STYLUS | InputDevice.SOURCE_TOUCHSCREEN, MotionEvent.TOOL_TYPE_STYLUS)), "stylus touch rejected");
    }

    private static void testReachRules() {
        float[] slop = new float[] {5, 10, 5, 10}; // top, right, bottom, left in points
        // density = 2 -> slop in px: top/bottom 10, left/right 20
        check(Mouse.reachSelf(true, 0, 5, 5, 40, 40, null, 2f), "inside");
        check(!Mouse.reachSelf(true, 0, -5, 5, 40, 40, null, 2f), "outside without slop");
        check(Mouse.reachSelf(true, 0, -15, 5, 40, 40, slop, 2f), "inside left slop");
        check(!Mouse.reachSelf(true, 0, -25, 5, 40, 40, slop, 2f), "outside left slop");
        check(!Mouse.reachSelf(true, Protocol.POINTER_EVENTS_NONE, 5, 5, 40, 40, slop, 2f), "none blocks self");
        check(!Mouse.reachSelf(true, Protocol.POINTER_EVENTS_BOX_NONE, 5, 5, 40, 40, slop, 2f), "box-none blocks self");
        check(Mouse.reachSelf(true, Protocol.POINTER_EVENTS_BOX_ONLY, 5, 5, 40, 40, slop, 2f), "box-only allows self");

        check(Mouse.reachChildren(true, 0, true, 5, 5, 40, 40), "clipped inside");
        check(!Mouse.reachChildren(true, 0, true, 50, 5, 40, 40), "clipped outside");
        check(Mouse.reachChildren(true, 0, false, 50, 5, 40, 40), "unclipped outside");
        check(!Mouse.reachChildren(true, Protocol.POINTER_EVENTS_NONE, false, 5, 5, 40, 40), "none blocks children");
        check(!Mouse.reachChildren(true, Protocol.POINTER_EVENTS_BOX_ONLY, false, 5, 5, 40, 40), "box-only blocks children");
        check(Mouse.reachChildren(true, Protocol.POINTER_EVENTS_BOX_NONE, false, 5, 5, 40, 40), "box-none allows children");
    }

    private static void testHoverAncestorTransitionsAndExit() {
        // density = 2: 200x100 points surface (400x200 px)
        Box root = new Box(1, 0, 0, 400, 200, ALL_MOUSE);
        Box parent = new Box(2, 20, 20, 360, 160, ALL_MOUSE);
        Box childA = new Box(3, 20, 20, 100, 100, ALL_MOUSE); // surface x=40..140, y=40..140
        Box childB = new Box(4, 160, 20, 100, 100, ALL_MOUSE); // surface x=180..280, y=40..140
        parent.children.add(childA);
        parent.children.add(childB);
        root.children.add(parent);

        FakeHost host = new FakeHost(400, 200, 2f, root);
        Mouse mouse = new Mouse(host);

        // 1. Hover enters over childA at surface (60, 80) px = (30, 40) pt, Shift held.
        // Local: root (30,40), parent (20,30), childA (10,20).
        check(mouse.hover(new MotionEvent(MotionEvent.ACTION_HOVER_ENTER, 60, 80,
                KeyEvent.META_SHIFT_ON, 0, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE)), "enter handled");
        checkLog(host,
                "IN:1@30,40|page=30,40|m=4|b=0",
                "IN:2@20,30|page=30,40|m=4|b=0",
                "IN:3@10,20|page=30,40|m=4|b=0",
                "MOVE:3@10,20|page=30,40|m=4|b=0",
                "MOVE:2@20,30|page=30,40|m=4|b=0",
                "MOVE:1@30,40|page=30,40|m=4|b=0");

        // 2. Move within childA to (80, 100) px = (40, 50) pt, Ctrl+Alt held: no enter/leave, only move innermost-first.
        mouse.hover(new MotionEvent(MotionEvent.ACTION_HOVER_MOVE, 80, 100,
                KeyEvent.META_CTRL_ON | KeyEvent.META_ALT_ON, 0, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE));
        checkLog(host,
                "MOVE:3@20,30|page=40,50|m=3|b=0",
                "MOVE:2@30,40|page=40,50|m=3|b=0",
                "MOVE:1@40,50|page=40,50|m=3|b=0");

        // 3. Move across to childB at (200, 80) px = (100, 40) pt: leaves childA only, enters childB only, moves B->parent->root.
        // Local in childA: (200-40)/2=80, (80-40)/2=20. Local in childB: (200-180)/2=10, (80-40)/2=20.
        mouse.hover(new MotionEvent(MotionEvent.ACTION_HOVER_MOVE, 200, 80,
                0, 0, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE));
        checkLog(host,
                "OUT:3@80,20|page=100,40|m=0|b=0",
                "IN:4@10,20|page=100,40|m=0|b=0",
                "MOVE:4@10,20|page=100,40|m=0|b=0",
                "MOVE:2@90,30|page=100,40|m=0|b=0",
                "MOVE:1@100,40|page=100,40|m=0|b=0");

        // 4. Hover exit INSIDE surface with BUTTON_PRIMARY (Android's hover-to-touch transition): must NOT fire spurious leave!
        mouse.hover(new MotionEvent(MotionEvent.ACTION_HOVER_EXIT, 200, 80,
                0, MotionEvent.BUTTON_PRIMARY, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE));
        checkLog(host);

        // 5. Hover exit OUTSIDE surface with BUTTON_SECONDARY + Meta: leaves innermost-first with coords, mods & button!
        mouse.hover(new MotionEvent(MotionEvent.ACTION_HOVER_EXIT, -10, 80,
                KeyEvent.META_META_ON, MotionEvent.BUTTON_SECONDARY, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE));
        check(!mouse.inside(), "outside after exit");
        checkLog(host,
                "OUT:4@-95,20|page=-5,40|m=8|b=2",
                "OUT:2@-15,30|page=-5,40|m=8|b=2",
                "OUT:1@-5,40|page=-5,40|m=8|b=2");

        // 6. TalkBack touchscreen finger hover must be completely ignored.
        check(!mouse.hover(new MotionEvent(MotionEvent.ACTION_HOVER_ENTER, 60, 80,
                0, 0, InputDevice.SOURCE_TOUCHSCREEN, MotionEvent.TOOL_TYPE_FINGER)), "talkback ignored");
        checkLog(host);
    }

    private static void testButtonDownDragMovesAndHoverTransitions() {
        Box root = new Box(1, 0, 0, 200, 100, ALL_MOUSE);
        Box left = new Box(2, 0, 0, 100, 100, ALL_MOUSE);
        Box right = new Box(3, 100, 0, 100, 100, ALL_MOUSE);
        root.children.add(left);
        root.children.add(right);

        FakeHost host = new FakeHost(200, 100, 1f, root);
        Mouse mouse = new Mouse(host);

        // Touchscreen finger press/drag/up must NOT emit mouse events or claim drag.
        check(!mouse.touch(new MotionEvent(MotionEvent.ACTION_DOWN, 20, 20, 0, 0,
                InputDevice.SOURCE_TOUCHSCREEN, MotionEvent.TOOL_TYPE_FINGER)), "finger down ignored");
        check(!mouse.touch(new MotionEvent(MotionEvent.ACTION_MOVE, 120, 20, 0, 0,
                InputDevice.SOURCE_TOUCHSCREEN, MotionEvent.TOOL_TYPE_FINGER)), "finger move ignored");
        check(!mouse.touch(new MotionEvent(MotionEvent.ACTION_UP, 120, 20, 0, 0,
                InputDevice.SOURCE_TOUCHSCREEN, MotionEvent.TOOL_TYPE_FINGER)), "finger up ignored");
        checkLog(host);

        // Mouse hovers over left at (20, 30), then presses left button and drags across into right, then outside!
        mouse.hover(new MotionEvent(MotionEvent.ACTION_HOVER_ENTER, 20, 30,
                0, 0, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE));
        host.log.clear();

        // Android sends HOVER_EXIT (in-bounds, BUTTON_PRIMARY) then ACTION_DOWN (BUTTON_PRIMARY).
        mouse.hover(new MotionEvent(MotionEvent.ACTION_HOVER_EXIT, 20, 30,
                0, MotionEvent.BUTTON_PRIMARY, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE));
        check(mouse.touch(new MotionEvent(MotionEvent.ACTION_DOWN, 20, 30,
                KeyEvent.META_SHIFT_ON, MotionEvent.BUTTON_PRIMARY, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE)),
                "mouse down keeps drag");
        check(mouse.dragging(), "dragging active");
        checkLog(host); // Already over [1, 2]: no spurious leave/enter or duplicate move!

        // Drag within left with Left button + Shift held.
        check(mouse.touch(new MotionEvent(MotionEvent.ACTION_MOVE, 40, 35,
                KeyEvent.META_SHIFT_ON, MotionEvent.BUTTON_PRIMARY, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE)),
                "drag move stays active");
        checkLog(host,
                "MOVE:2@40,35|page=40,35|m=4|b=1",
                "MOVE:1@40,35|page=40,35|m=4|b=1");

        // Drag into right with Middle button + Ctrl held: leaves left, enters right, moves right -> root.
        mouse.touch(new MotionEvent(MotionEvent.ACTION_MOVE, 120, 45,
                KeyEvent.META_CTRL_ON, MotionEvent.BUTTON_TERTIARY, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE));
        checkLog(host,
                "OUT:2@120,45|page=120,45|m=1|b=3",
                "IN:3@20,45|page=120,45|m=1|b=3",
                "MOVE:3@20,45|page=120,45|m=1|b=3",
                "MOVE:1@120,45|page=120,45|m=1|b=3");

        // Drag outside surface with Right button held: leaves right and root with Right button!
        mouse.touch(new MotionEvent(MotionEvent.ACTION_MOVE, 210, 45,
                0, MotionEvent.BUTTON_SECONDARY, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE));
        check(!mouse.inside(), "outside during out-of-bounds drag");
        checkLog(host,
                "OUT:3@110,45|page=210,45|m=0|b=2",
                "OUT:1@210,45|page=210,45|m=0|b=2");

        // Drag back into right with Back button held, then release.
        mouse.touch(new MotionEvent(MotionEvent.ACTION_MOVE, 130, 50,
                0, MotionEvent.BUTTON_BACK, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE));
        checkLog(host,
                "IN:1@130,50|page=130,50|m=0|b=4",
                "IN:3@30,50|page=130,50|m=0|b=4",
                "MOVE:3@30,50|page=130,50|m=0|b=4",
                "MOVE:1@130,50|page=130,50|m=0|b=4");

        check(!mouse.touch(new MotionEvent(MotionEvent.ACTION_UP, 130, 50,
                0, 0, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE)), "up ends drag");
        check(!mouse.dragging(), "dragging cleared on up");
        checkLog(host);
    }

    private static void testModifierKeyChangesUnderStillMouse() {
        Box root = new Box(1, 0, 0, 200, 100, 1L << Protocol.EVENT_HOVER_MOVE);
        Box child = new Box(2, 10, 10, 80, 80, ALL_MOUSE);
        root.children.add(child);

        FakeHost host = new FakeHost(200, 100, 1f, root);
        Mouse mouse = new Mouse(host);

        // Modifier key while outside surface: nothing emitted.
        mouse.keyEvent(new KeyEvent(KeyEvent.ACTION_DOWN, KeyEvent.KEYCODE_SHIFT_LEFT, KeyEvent.META_SHIFT_ON, 0));
        checkLog(host);

        // Hover enters child at (30, 40).
        mouse.hover(new MotionEvent(MotionEvent.ACTION_HOVER_ENTER, 30, 40,
                0, 0, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE));
        host.log.clear();

        // Press Shift under still mouse: emits MOVE innermost-first with Shift.
        mouse.keyEvent(new KeyEvent(KeyEvent.ACTION_DOWN, KeyEvent.KEYCODE_SHIFT_LEFT, KeyEvent.META_SHIFT_ON, 0));
        checkLog(host,
                "MOVE:2@20,30|page=30,40|m=4|b=0",
                "MOVE:1@30,40|page=30,40|m=4|b=0");

        // Auto-repeat or non-modifier key: ignored.
        mouse.keyEvent(new KeyEvent(KeyEvent.ACTION_DOWN, KeyEvent.KEYCODE_SHIFT_LEFT, KeyEvent.META_SHIFT_ON, 1));
        mouse.keyEvent(new KeyEvent(KeyEvent.ACTION_DOWN, KeyEvent.KEYCODE_A, KeyEvent.META_SHIFT_ON, 0));
        checkLog(host);

        // Release Shift under still mouse: emits MOVE innermost-first with mods=0.
        mouse.keyEvent(new KeyEvent(KeyEvent.ACTION_UP, KeyEvent.KEYCODE_SHIFT_LEFT, 0, 0));
        checkLog(host,
                "MOVE:2@20,30|page=30,40|m=0|b=0",
                "MOVE:1@30,40|page=30,40|m=0|b=0");
    }

    private static void testPointerEventsDisabledModalAndDeletedViews() {
        Box root = new Box(1, 0, 0, 200, 200, ALL_MOUSE);
        Box behind = new Box(2, 10, 10, 100, 100, ALL_MOUSE);
        Box disabledBox = new Box(3, 10, 10, 100, 100, ALL_MOUSE);
        disabledBox.disabled = true;
        Box noneOverlay = new Box(4, 10, 10, 100, 100, ALL_MOUSE);
        noneOverlay.pointerEvents = Protocol.POINTER_EVENTS_NONE;
        Box boxNoneWrap = new Box(5, 10, 10, 100, 100, ALL_MOUSE);
        boxNoneWrap.pointerEvents = Protocol.POINTER_EVENTS_BOX_NONE;
        Box innerBtn = new Box(6, 10, 10, 40, 40, ALL_MOUSE); // surface 20..60
        boxNoneWrap.children.add(innerBtn);

        root.children.add(behind);
        root.children.add(disabledBox);
        root.children.add(noneOverlay);
        root.children.add(boxNoneWrap);

        FakeHost host = new FakeHost(200, 200, 1f, root);
        Mouse mouse = new Mouse(host);

        // At (30, 30): boxNoneWrap skips self, hits innerBtn (6)! noneOverlay and disabledBox behind are blocked by innerBtn.
        mouse.hover(new MotionEvent(MotionEvent.ACTION_HOVER_ENTER, 30, 30,
                0, 0, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE));
        checkLog(host,
                "IN:1@30,30|page=30,30|m=0|b=0",
                "IN:6@10,10|page=30,30|m=0|b=0",
                "MOVE:6@10,10|page=30,30|m=0|b=0",
                "MOVE:1@30,30|page=30,30|m=0|b=0");

        // Move to (80, 80): outside innerBtn, boxNoneWrap passes through, noneOverlay passes through -> hits disabledBox (3)!
        // Disabled views still receive hover/move events (for hover: classes and tooltips).
        mouse.hover(new MotionEvent(MotionEvent.ACTION_HOVER_MOVE, 80, 80,
                0, 0, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE));
        checkLog(host,
                "OUT:6@60,60|page=80,80|m=0|b=0",
                "IN:3@70,70|page=80,80|m=0|b=0",
                "MOVE:3@70,70|page=80,80|m=0|b=0",
                "MOVE:1@80,80|page=80,80|m=0|b=0");

        // Open a modal over root: refresh() leaves disabledBox (3) AND root (1) behind the modal, and enters modal (7) + dialog (8)!
        Box modal = new Box(7, 0, 0, 200, 200, ALL_MOUSE);
        modal.modal = true;
        Box dialog = new Box(8, 50, 50, 100, 100, ALL_MOUSE);
        modal.children.add(dialog);
        root.children.add(modal);
        mouse.refresh();
        checkLog(host,
                "OUT:3@70,70|page=80,80|m=0|b=0",
                "OUT:1@80,80|page=80,80|m=0|b=0",
                "IN:7@80,80|page=80,80|m=0|b=0",
                "IN:8@30,30|page=80,80|m=0|b=0");

        // Delete dialog (8) while hovered: forget(8) drops it cleanly with NO leave event.
        modal.children.remove(dialog);
        mouse.forget(8);
        checkLog(host);

        // Next hover move stays on modal (7) without trying to leave deleted dialog (8).
        mouse.hover(new MotionEvent(MotionEvent.ACTION_HOVER_MOVE, 85, 85,
                0, 0, InputDevice.SOURCE_MOUSE, MotionEvent.TOOL_TYPE_MOUSE));
        checkLog(host, "MOVE:7@85,85|page=85,85|m=0|b=0");
    }
}
