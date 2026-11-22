package dev.oneo.nsgo;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Context;
import android.content.ContextWrapper;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.Insets;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import android.util.Base64;
import android.view.InputDevice;
import android.view.KeyCharacterMap;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.PixelCopy;
import android.view.View;
import android.view.ViewGroup;
import android.view.Window;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputConnection;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Iterator;
import java.util.List;
import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

/**
 * The test agent of nsgo test -target android and nsgo dev -target android -automation:
 * the test or automation agent in the app's process asks it for input over the pipe
 * (Protocol.TEST), and it acts on the UI thread as a user would: MotionEvents dispatched
 * to the window as a finger's, drags that rest before they lift, KeyEvents as a
 * keyboard's, and what no key types committed through the focused field's input
 * connection. It answers each step with Protocol.TEST_REPLY once done.
 */
final class TestAgent {
    /** The launch intent's extra that names the test: base64 JSON. */
    static final String EXTRA = "nsgo.test";
    /** The launch intent's boolean extra that starts the test agent for nsgo dev -automation. */
    static final String AUTOMATION_EXTRA = "nsgo.automation";

    /** An alert on screen: its dialog, buttons and list items' actions. */
    static final class Alert {
        final AlertDialog dialog;
        final int[] buttons;
        final int[] items;

        Alert(AlertDialog dialog, int[] buttons, int[] items) {
            this.dialog = dialog;
            this.buttons = buttons;
            this.items = items;
        }
    }

    private final NsgoView view;
    private final Handler ui = new Handler(Looper.getMainLooper());

    TestAgent(NsgoView view) {
        this.view = view;
    }

    /** Sets the test's arguments and environment on the app's process. */
    static void apply(String encoded, ProcessBuilder builder) {
        try {
            JSONObject test = new JSONObject(new String(Base64.decode(encoded, Base64.DEFAULT), StandardCharsets.UTF_8));
            JSONArray args = test.optJSONArray("args");
            for (int i = 0; args != null && i < args.length(); i++) builder.command().add(args.getString(i));
            JSONObject env = test.optJSONObject("env");
            if (env != null) {
                for (Iterator<String> keys = env.keys(); keys.hasNext(); ) {
                    String k = keys.next();
                    builder.environment().put(k, env.getString(k));
                }
            }
        } catch (JSONException | IllegalArgumentException e) {
            android.util.Log.w("nsgo", "unreadable test: " + e);
        }
    }

    /** Takes a step from the pipe's reader and runs it on the UI thread. */
    void handle(DataInputStream in) throws IOException {
        int id = in.readInt();
        String op = Protocol.readString(in);
        ui.post(() -> {
            try {
                run(id, new JSONObject(op));
            } catch (Exception e) {
                view.testReply(id, e.toString(), "");
            }
        });
    }

    private Activity activity() {
        Context c = view.getContext();
        while (c instanceof ContextWrapper && !(c instanceof Activity)) c = ((ContextWrapper) c).getBaseContext();
        if (!(c instanceof Activity)) throw new IllegalStateException("no activity shows the app");
        return (Activity) c;
    }

    /** Reads native geometry and typography, including views placed outside core's layout roots. */
    private void viewMetrics(View target, JSONObject query, int[] origin, JSONArray out) throws JSONException {
        boolean match = query.has("testID") && query.getString("testID").equals(target.getTag());
        if (query.has("text") && target instanceof android.widget.TextView) {
            match |= query.getString("text").contentEquals(((android.widget.TextView) target).getText());
        }
        if (match) {
            int[] at = new int[2];
            target.getLocationInWindow(at);
            float d = view.density(), alpha = target.getAlpha();
            android.view.ViewParent parent = target.getParent();
            while (parent instanceof View) {
                alpha *= ((View) parent).getAlpha();
                parent = parent.getParent();
            }
            android.graphics.Rect visible = new android.graphics.Rect();
            target.getGlobalVisibleRect(visible);
            JSONObject m = new JSONObject().put("direction", target.getLayoutDirection())
                    .put("rootDirection", view.getLayoutDirection()).put("rtlSupported", (view.getContext().getApplicationInfo().flags & android.content.pm.ApplicationInfo.FLAG_SUPPORTS_RTL) != 0)
                    .put("visibleHeight", visible.height() / d).put("x", (at[0] - origin[0]) / d).put("y", (at[1] - origin[1]) / d)
                    .put("width", target.getWidth() / d).put("height", target.getHeight() / d)
                    .put("alpha", alpha).put("visible", target.isShown())
                    .put("paddingBottom", target.getPaddingBottom() / d).put("paddingTop", target.getPaddingTop() / d);
            if (target instanceof android.widget.TextView) {
                android.widget.TextView text = (android.widget.TextView) target;
                android.graphics.Typeface font = text.getTypeface();
                m.put("size", text.getTextSize() / target.getResources().getDisplayMetrics().scaledDensity)
                        .put("weight", font.getWeight()).put("italic", font.isItalic());
                if (query.has("family")) {
                    android.graphics.Typeface expected = android.graphics.Typeface.create(
                            android.graphics.Typeface.create(query.getString("family"), android.graphics.Typeface.NORMAL),
                            font.getWeight(), font.isItalic());
                    m.put("familyMatches", font.equals(expected));
                }
            }
            out.put(m);
        }
        if (target instanceof ViewGroup) {
            ViewGroup group = (ViewGroup) target;
            for (int i = 0; i < group.getChildCount(); i++) viewMetrics(group.getChildAt(i), query, origin, out);
        }
    }

    /** x, y of the surface's points, in the window's pixels. */
    private float[] window(double x, double y) {
        int[] at = new int[2];
        view.getLocationInWindow(at);
        float d = view.density();
        return new float[] {at[0] + (float) x * d, at[1] + (float) y * d};
    }

    private void run(int id, JSONObject op) throws Exception {
        switch (op.getString("op")) {
            case "notification-intents": {
                Context context = view.getContext();
                Activity host = op.optBoolean("launcher") ? null : activity();
                android.app.PendingIntent first = Notifications.pressIntent(context, host, op.getString("first"));
                android.app.PendingIntent second = Notifications.pressIntent(context, host, op.getString("second"));
                view.testReply(id, "", Boolean.toString(first.equals(second)));
                return;
            }
            case "press": {
                float[] p = window(op.getDouble("x"), op.getDouble("y"));
                long hold = Math.max(op.optLong("hold"), 0) + 50;
                long down = SystemClock.uptimeMillis();
                JSONObject mods = op.optJSONObject("modifiers");
                int meta = 0;
                if (mods != null) {
                    if (mods.optBoolean("Control")) meta |= KeyEvent.META_CTRL_ON | KeyEvent.META_CTRL_LEFT_ON;
                    if (mods.optBoolean("Shift")) meta |= KeyEvent.META_SHIFT_ON | KeyEvent.META_SHIFT_LEFT_ON;
                    if (mods.optBoolean("Alt")) meta |= KeyEvent.META_ALT_ON | KeyEvent.META_ALT_LEFT_ON;
                    if (mods.optBoolean("Platform")) meta |= KeyEvent.META_META_ON;
                }
                final int held = meta;
                touch(down, SystemClock.uptimeMillis(), MotionEvent.ACTION_DOWN, p, held);
                ui.postDelayed(() -> {
                    touch(down, SystemClock.uptimeMillis(), MotionEvent.ACTION_UP, p, held);
                    // Wait for what the release posts, such as View.PerformClick, so OnPress reaches Go before the reply.
                    ui.post(() -> view.testReply(id, "", ""));
                }, hold);
                return;
            }
            case "drag":
            case "swipe": {
                JSONArray path = op.getJSONArray("path");
                List<float[]> points = new ArrayList<>();
                for (int i = 0; i < path.length(); i++) {
                    JSONObject p = path.getJSONObject(i);
                    points.add(window(p.getDouble("X"), p.getDouble("Y")));
                }
                finger(id, points, op.getString("op").equals("drag"));
                return;
            }
            case "scroll-ready": {
                View scroll = scrollAt(view, window(op.getDouble("x"), op.getDouble("y")));
                view.testReply(id, "", Boolean.toString(scroll != null &&
                        (scroll.canScrollHorizontally(1) || scroll.canScrollVertically(1))));
                return;
            }
            case "hold-scroll": {
                float[] p = window(op.getDouble("x"), op.getDouble("y"));
                View scroll = scrollAt(view, p);
                if (scroll == null) throw new IllegalArgumentException("no scroll view under the finger");
                long down = SystemClock.uptimeMillis();
                touch(down, MotionEvent.ACTION_DOWN, p);
                int x = scroll.getScrollX(), y = scroll.getScrollY();
                scroll.postOnAnimation(new Runnable() {
                    int frames, drift;

                    @Override
                    public void run() {
                        drift = Math.max(drift, Math.max(Math.abs(scroll.getScrollX() - x), Math.abs(scroll.getScrollY() - y)));
                        if (++frames < 20) {
                            scroll.postOnAnimation(this);
                            return;
                        }
                        // The finger lifts where it held, and the reply waits for what the release posts, such as a click.
                        touch(down, MotionEvent.ACTION_UP, p);
                        int reply = drift;
                        scroll.post(() -> view.testReply(id, "", Integer.toString(reply)));
                    }
                });
                return;
            }
            case "scroll": {
                float[] from = window(op.getDouble("x"), op.getDouble("y"));
                float d = view.density();
                float[] to = {from[0] - (float) op.getDouble("dx") * d, from[1] - (float) op.getDouble("dy") * d};
                List<float[]> points = new ArrayList<>();
                points.add(from);
                points.add(to);
                finger(id, points, true);
                return;
            }
            case "type":
                type(op.getString("text"));
                break;
            case "compose":
                compose(op.getString("text"), false);
                break;
            case "commit":
                compose(op.getString("text"), true);
                break;
            case "key":
                key(op.getString("key"));
                break;
            case "select-range": {
                View focused = activity().getCurrentFocus();
                if (!(focused instanceof InputField)) throw new IllegalStateException("no text field has the keyboard focus");
                InputField field = (InputField) focused;
                EditorInfo info = new EditorInfo();
                InputConnection connection = field.onCreateInputConnection(info);
                if (connection == null || !connection.setSelection(op.getInt("start"), op.getInt("end"))) {
                    throw new IllegalStateException("the focused field refused its native selection");
                }
                view.testReply(id, "", new JSONObject().put("Start", field.getSelectionStart()).put("End", field.getSelectionEnd()).toString());
                return;
            }
            case "editor-action": {
                View focused = activity().getCurrentFocus();
                EditorInfo info = new EditorInfo();
                InputConnection connection = focused == null ? null : focused.onCreateInputConnection(info);
                if (connection == null) throw new IllegalStateException("no focused input connection");
                if (op.optBoolean("hardware")) {
                    // Enter delivered to the field by an IME, after the root's hardware-key stage.
                    long now = SystemClock.uptimeMillis();
                    int meta = op.optBoolean("shift") ? KeyEvent.META_SHIFT_ON | KeyEvent.META_SHIFT_LEFT_ON : 0;
                    focused.dispatchKeyEvent(new KeyEvent(now, now, KeyEvent.ACTION_DOWN, KeyEvent.KEYCODE_ENTER, 0, meta));
                    focused.dispatchKeyEvent(new KeyEvent(now, now, KeyEvent.ACTION_UP, KeyEvent.KEYCODE_ENTER, 0, meta));
                } else {
                    connection.performEditorAction(info.imeOptions & EditorInfo.IME_MASK_ACTION);
                }
                break;
            }
            case "back":
                view.back();
                break;
            case "answer": {
                int alert = op.getInt("id");
                answer(alert, op.getInt("index"));
                // The dialog calls its listener from its handler: the reply
                // waits for the answer, which then reaches the app first.
                long until = SystemClock.uptimeMillis() + 2000;
                Runnable[] check = new Runnable[1];
                check[0] = () -> {
                    if (view.alerts.get(alert) != null && SystemClock.uptimeMillis() < until) {
                        ui.postDelayed(check[0], 16);
                    } else {
                        view.testReply(id, "", "");
                    }
                };
                ui.post(check[0]);
                return;
            }
            case "select-tab":
                view.selectTestTab(op.getInt("id"), op.getInt("index"));
                break;
            case "recreate-activity":
                activity().recreate();
                break;
            case "recording-probe": {
                int descriptors = 0;
                String directory = new java.io.File(view.getContext().getFilesDir(), "recordings").getCanonicalPath() + "/";
                java.io.File[] fds = new java.io.File("/proc/self/fd").listFiles();
                if (fds != null) for (java.io.File fd : fds) {
                    try {
                        if (fd.getCanonicalPath().startsWith(directory)) descriptors++;
                    } catch (IOException ignored) {}
                }
                boolean notification = false;
                android.app.NotificationManager notifications = view.getContext().getSystemService(android.app.NotificationManager.class);
                for (android.service.notification.StatusBarNotification n : notifications.getActiveNotifications()) {
                    if ("nsgo.recording".equals(n.getNotification().getChannelId())) notification = true;
                }
                view.testReply(id, "", new JSONObject().put("notification", notification)
                        .put("descriptors", descriptors).put("pssKB", android.os.Debug.getPss())
                        .put("visible", activity().getWindow().getDecorView().getWindowVisibility() == View.VISIBLE).toString());
                return;
            }
            case "system-bars":
                view.testReply(id, "", systemBars());
                return;
            case "view-metrics": {
                JSONArray metrics = new JSONArray();
                int[] origin = new int[2];
                view.getLocationInWindow(origin);
                viewMetrics(view, op, origin, metrics);
                view.testReply(id, "", metrics.toString());
                return;
            }
            case "screenshot":
                screenshot(id, 3);
                return;
            case "signature":
                view.testReply(id, "", signature());
                return;
            case "canvas-images":
                view.testReply(id, "", new JSONObject()
                        .put("count", view.canvasImages.size())
                        .put("ready", view.canvasImages.ready(op.getString("key"))).toString());
                return;
            case "detach-view":
                view.detachFromEngine();
                view.testReply(id, "", viewState().toString());
                return;
            case "reattach-view":
                view.attachToEngine((NsgoEngine) field(view, "engine"));
                break;
            case "view-state":
                view.testReply(id, "", viewState().toString());
                return;
            default:
                throw new IllegalArgumentException("unknown step " + op);
        }
        view.testReply(id, "", "");
    }

    private static Object field(Object owner, String name) throws Exception {
        java.lang.reflect.Field field = owner.getClass().getDeclaredField(name);
        field.setAccessible(true);
        return field.get(owner);
    }

    /** Reads the real native tree, including while its Go graph is detached. */
    private JSONObject viewState() throws Exception {
        android.util.SparseArray<?> nodes = (android.util.SparseArray<?>) field(view, "nodes");
        JSONArray text = new JSONArray(), inputs = new JSONArray();
        int drawings = 0;
        for (int i = 0; i < nodes.size(); i++) {
            Object node = nodes.valueAt(i);
            Object value = field(node, "text");
            if (value instanceof String) text.put(value);
            Object input = field(node, "field");
            if (input instanceof android.widget.EditText) inputs.put(((android.widget.EditText) input).getText().toString());
            Object canvas = field(node, "canvas");
            if (canvas != null && field(canvas, "ops") != null) drawings++;
        }
        NsgoEngine engine = (NsgoEngine) field(view, "engine");
        return new JSONObject().put("nodes", nodes.size()).put("children", view.getChildCount())
                .put("alive", engine != null && engine.isAlive()).put("text", text).put("inputs", inputs)
                .put("drawings", drawings).put("awake", view.getKeepScreenOn())
                .put("displayHolds", ((java.util.Set<?>) field(view, "displayHolds")).size());
    }

    private void touch(long down, int action, float[] p) {
        touch(down, SystemClock.uptimeMillis(), action, p, 0);
    }

    /** A touch stamped at time, so a velocity read from it does not depend on when a loaded device runs the post that sends it. */
    private void touch(long down, long time, int action, float[] p) {
        touch(down, time, action, p, 0);
    }

    /** A touch stamped at time with the meta state of the modifier keys held. */
    private void touch(long down, long time, int action, float[] p, int meta) {
        MotionEvent e = MotionEvent.obtain(down, time, action, p[0], p[1], meta);
        e.setSource(InputDevice.SOURCE_TOUCHSCREEN);
        activity().dispatchTouchEvent(e);
        e.recycle();
    }

    /**
     * A finger along points: it rests where it lands, moves 10 points a frame, and rests still before it lifts when rest is set. Drags rest, so they end without a fling; swipes lift while moving, so they fling.
     */
    private void finger(int id, List<float[]> points, boolean rest) {
        if (points.isEmpty()) {
            view.testReply(id, "an empty drag", "");
            return;
        }
        long down = SystemClock.uptimeMillis();
        List<float[]> moves = new ArrayList<>();
        float step = 10 * view.density();
        for (int i = 1; i < points.size(); i++) {
            float[] a = points.get(i - 1), b = points.get(i);
            int n = (int) (Math.max(Math.abs(b[0] - a[0]), Math.abs(b[1] - a[1])) / step) + 1;
            for (int k = 1; k <= n; k++) {
                float f = (float) k / n;
                moves.add(new float[] {a[0] + (b[0] - a[0]) * f, a[1] + (b[1] - a[1]) * f});
            }
        }
        float[] last = points.get(points.size() - 1);
        if (rest) for (int i = 0; i < 6; i++) moves.add(last);
        touch(down, MotionEvent.ACTION_DOWN, points.get(0));
        long at = 100;
        for (float[] p : moves) {
            long time = down + at;
            ui.postDelayed(() -> touch(down, time, MotionEvent.ACTION_MOVE, p), at);
            at += 16;
        }
        long up = down + at;
        ui.postDelayed(() -> {
            touch(down, up, MotionEvent.ACTION_UP, last);
            view.testReply(id, "", "");
        }, at);
    }

    /** The innermost native scroll view containing a point in the window. */
    private static View scrollAt(View v, float[] p) {
        if (v.getVisibility() != View.VISIBLE) return null;
        int[] at = new int[2];
        v.getLocationInWindow(at);
        if (p[0] < at[0] || p[1] < at[1] || p[0] >= at[0] + v.getWidth() || p[1] >= at[1] + v.getHeight()) return null;
        if (v instanceof ViewGroup) {
            ViewGroup group = (ViewGroup) v;
            for (int i = group.getChildCount() - 1; i >= 0; i--) {
                View scroll = scrollAt(group.getChildAt(i), p);
                if (scroll != null) return scroll;
            }
        }
        return v instanceof Scrolling.Scroller ? v : null;
    }

    /** Types text as a keyboard's keys, or commits what no key types. */
    private void type(String text) {
        Activity a = activity();
        KeyCharacterMap keys = KeyCharacterMap.load(KeyCharacterMap.VIRTUAL_KEYBOARD);
        for (int i = 0; i < text.length(); ) {
            int cp = text.codePointAt(i);
            String s = new String(Character.toChars(cp));
            i += s.length();
            KeyEvent[] events = cp == '\n' ? null : keys.getEvents(s.toCharArray());
            if (cp == '\n') {
                key("enter");
            } else if (events != null) {
                for (KeyEvent e : events) a.dispatchKeyEvent(e);
            } else {
                View f = a.getCurrentFocus();
                InputConnection ic = f == null ? null : f.onCreateInputConnection(new EditorInfo());
                if (ic != null) ic.commitText(s, 1);
            }
        }
    }

    /**
     * Composes text in the focused field through its InputConnection, as an
     * input method does, or commits it; composing nothing ends the
     * composition.
     */
    private void compose(String text, boolean commit) {
        View f = activity().getCurrentFocus();
        InputConnection ic = f == null ? null : f.onCreateInputConnection(new EditorInfo());
        if (ic == null) throw new IllegalStateException("no field has the keyboard");
        ic.beginBatchEdit();
        if (commit) {
            ic.commitText(text, 1);
        } else if (text.isEmpty()) {
            ic.setComposingText("", 1);
            ic.finishComposingText();
        } else {
            ic.setComposingText(text, 1);
        }
        ic.endBatchEdit();
    }

    /** Reports the window's actual bar visibility, insets, and swipe behavior. */
    private String systemBars() throws JSONException {
        if (Build.VERSION.SDK_INT < 30) return "";
        WindowInsets insets = view.getRootWindowInsets();
        WindowInsetsController controller = view.getWindowInsetsController();
        if (insets == null || controller == null) return "{}";
        Insets safe = insets.getInsets(WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout());
        float density = view.density();
        return new JSONObject()
                .put("ready", true)
                .put("status", insets.isVisible(WindowInsets.Type.statusBars()))
                .put("navigation", insets.isVisible(WindowInsets.Type.navigationBars()))
                .put("transient", controller.getSystemBarsBehavior() == WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE)
                .put("safe", new JSONObject().put("Top", safe.top / density).put("Right", safe.right / density)
                        .put("Bottom", safe.bottom / density).put("Left", safe.left / density)).toString();
    }

    private static int keyCode(String name) {
        switch (name) {
            case "enter": case "return": return KeyEvent.KEYCODE_ENTER;
            case "backspace": return KeyEvent.KEYCODE_DEL;
            case "delete": return KeyEvent.KEYCODE_FORWARD_DEL;
            case "escape": return KeyEvent.KEYCODE_ESCAPE;
            case "tab": return KeyEvent.KEYCODE_TAB;
            case "space": return KeyEvent.KEYCODE_SPACE;
            case "left": return KeyEvent.KEYCODE_DPAD_LEFT;
            case "right": return KeyEvent.KEYCODE_DPAD_RIGHT;
            case "up": return KeyEvent.KEYCODE_DPAD_UP;
            case "down": return KeyEvent.KEYCODE_DPAD_DOWN;
            case "home": return KeyEvent.KEYCODE_MOVE_HOME;
            case "end": return KeyEvent.KEYCODE_MOVE_END;
            default:
                if (name.length() == 1) {
                    char c = name.charAt(0);
                    if (c >= 'a' && c <= 'z') return KeyEvent.KEYCODE_A + (c - 'a');
                    if (c >= '0' && c <= '9') return KeyEvent.KEYCODE_0 + (c - '0');
                }
                throw new IllegalArgumentException("unknown key " + name);
        }
    }

    /** Presses a key ("enter", "mod+a") as a hardware keyboard's. */
    private void key(String key) {
        String[] parts = key.toLowerCase().split("\\+");
        int meta = 0;
        for (int i = 0; i < parts.length - 1; i++) {
            switch (parts[i]) {
                case "mod": case "ctrl": case "control": meta |= KeyEvent.META_CTRL_ON | KeyEvent.META_CTRL_LEFT_ON; break;
                case "shift": meta |= KeyEvent.META_SHIFT_ON | KeyEvent.META_SHIFT_LEFT_ON; break;
                case "alt": case "option": meta |= KeyEvent.META_ALT_ON | KeyEvent.META_ALT_LEFT_ON; break;
                case "meta": case "super": meta |= KeyEvent.META_META_ON; break;
                default: throw new IllegalArgumentException("unknown modifier " + parts[i]);
            }
        }
        int code = keyCode(parts[parts.length - 1]);
        long now = SystemClock.uptimeMillis();
        Activity a = activity();
        KeyEvent down = new KeyEvent(now, now, KeyEvent.ACTION_DOWN, code, 0, meta, KeyCharacterMap.VIRTUAL_KEYBOARD, 0);
        // A key with Control is offered to the keymap first, as NsgoView.dispatchKeyEvent offers it before the window's input stage tries it as a shortcut, so a KeyAlways binding takes Control+A before the field selects all. A consumed key is released at once so its consumed state cannot leak into the next test.
        if ((meta & KeyEvent.META_CTRL_ON) != 0 && view.keyBindingShortcut(down)) {
            view.keyBindingShortcut(new KeyEvent(now, now, KeyEvent.ACTION_UP, code, 0, meta, KeyCharacterMap.VIRTUAL_KEYBOARD, 0));
            return;
        }
        if ((meta & KeyEvent.META_CTRL_ON) != 0 && a.dispatchKeyShortcutEvent(down)) return;
        a.dispatchKeyEvent(down);
        a.dispatchKeyEvent(new KeyEvent(now, now, KeyEvent.ACTION_UP, code, 0, meta, KeyCharacterMap.VIRTUAL_KEYBOARD, 0));
    }

    /** Taps the alert's button or list item for the action, or cancels it. */
    private void answer(int id, int index) {
        Alert a = view.alerts.get(id);
        if (a == null) throw new IllegalStateException("no alert " + id + " shows");
        if (index < 0) {
            a.dialog.cancel();
            return;
        }
        if (a.buttons != null && index < a.buttons.length) {
            a.dialog.getButton(a.buttons[index]).performClick();
            return;
        }
        for (int i = 0; i < a.items.length; i++) {
            if (a.items[i] == index) {
                android.widget.ListView list = a.dialog.getListView();
                list.performItemClick(list.getChildAt(i - list.getFirstVisiblePosition()), i, list.getItemIdAtPosition(i));
                return;
            }
        }
        throw new IllegalArgumentException("the alert has no action " + index);
    }

    /** The presented window, including shader and other texture-backed views. */
    private void screenshot(int id, int retries) {
        Window window = activity().getWindow();
        View root = window.getDecorView();
        Bitmap bitmap = Bitmap.createBitmap(Math.max(root.getWidth(), 1), Math.max(root.getHeight(), 1), Bitmap.Config.ARGB_8888);
        try {
            PixelCopy.request(window, bitmap, result -> {
                if (result == PixelCopy.SUCCESS) {
                    ByteArrayOutputStream png = new ByteArrayOutputStream();
                    bitmap.compress(Bitmap.CompressFormat.PNG, 100, png);
                    bitmap.recycle();
                    view.testReply(id, "", Base64.encodeToString(png.toByteArray(), Base64.NO_WRAP));
                } else {
                    bitmap.recycle();
                    if (result == PixelCopy.ERROR_SOURCE_NO_DATA && retries > 0) {
                        root.postOnAnimation(() -> screenshot(id, retries - 1));
                    } else {
                        view.testReply(id, "window screenshot failed: PixelCopy " + result, "");
                    }
                }
            }, ui);
        } catch (IllegalArgumentException e) {
            bitmap.recycle();
            view.testReply(id, "window screenshot failed: " + e.getMessage(), "");
        }
    }

    /**
     * Where every shown view is, how far it scrolled, how opaque it is
     * and which view holds focus: the test compares two of these to see
     * whether anything moved or shifted focus.
     */
    private String signature() {
        Activity a = activity();
        StringBuilder b = new StringBuilder();
        View focused = a.getCurrentFocus();
        b.append("f=").append(focused == null ? 0 : System.identityHashCode(focused)).append(';');
        walk(a.getWindow().getDecorView(), b);
        return Integer.toHexString(b.toString().hashCode());
    }

    private static void walk(View v, StringBuilder b) {
        if (v.getVisibility() != View.VISIBLE) return;
        b.append(v.getLeft()).append(',').append(v.getTop()).append(',')
                .append(v.getTranslationX()).append(',').append(v.getTranslationY()).append(',')
                .append(v.getScrollX()).append(',').append(v.getScrollY()).append(',')
                .append(v.getAlpha()).append(',').append(v.getScaleX()).append(';');
        if (v instanceof ViewGroup) {
            ViewGroup g = (ViewGroup) v;
            for (int i = 0; i < g.getChildCount(); i++) walk(g.getChildAt(i), b);
        }
    }
}
