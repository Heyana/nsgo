package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.drawable.GradientDrawable;
import android.os.Handler;
import android.os.Looper;
import android.text.Editable;
import android.text.InputType;
import android.text.StaticLayout;
import android.text.TextWatcher;
import android.util.Log;
import android.util.SparseArray;
import android.util.TypedValue;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewGroup;
import android.view.ViewParent;
import android.view.accessibility.AccessibilityNodeInfo;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputMethodManager;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.HorizontalScrollView;
import android.widget.ScrollView;
import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.BufferedReader;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.File;
import java.io.IOException;
import java.io.InputStreamReader;
import java.util.concurrent.ConcurrentHashMap;

/**
 * A surface showing one NSGo app. It starts the app's Go executable
 * (libnsgo.so in the native library directory), serves its text engine on
 * a reader thread and mounts its batches on the UI thread, one Android view
 * per node placed at the frame the engine computed. Points are
 * density-independent pixels. It needs API 28.
 */
public class NsgoView extends FrameLayout {
    private static final String TAG = "nsgo";

    private final float density;
    private final Handler ui = new Handler(Looper.getMainLooper());
    private final InputMethodManager ime;
    /** Mounted nodes by ID, on the UI thread. */
    private final SparseArray<Node> nodes = new SparseArray<>();
    /** Prepared text by ID, on the reader thread. */
    private final SparseArray<Text.Prepared> prepared = new SparseArray<>();
    /** Laid-out text by ID: the reader thread adds it, the batch that draws it takes it. */
    private final ConcurrentHashMap<Integer, StaticLayout> layouts = new ConcurrentHashMap<>();
    private Process process;
    private DataOutputStream out;

    public NsgoView(Context context) {
        super(context);
        density = context.getResources().getDisplayMetrics().density;
        ime = context.getSystemService(InputMethodManager.class);
    }

    @Override
    protected void onSizeChanged(int w, int h, int oldw, int oldh) {
        super.onSizeChanged(w, h, oldw, oldh);
        boolean first = process == null;
        if (first) start();
        send(first ? Protocol.START : Protocol.RESIZE, w / density, h / density);
    }

    private void start() {
        File exe = new File(getContext().getApplicationInfo().nativeLibraryDir, "libnsgo.so");
        try {
            process = new ProcessBuilder(exe.getAbsolutePath()).start();
        } catch (IOException e) {
            throw new IllegalStateException("cannot start " + exe, e);
        }
        out = new DataOutputStream(new BufferedOutputStream(process.getOutputStream()));
        Thread reader = new Thread(this::read, "nsgo-reader");
        reader.setDaemon(true);
        reader.start();
        Thread logger = new Thread(this::log, "nsgo-log");
        logger.setDaemon(true);
        logger.start();
    }

    /** Ends the app's process. */
    public void stop() {
        if (process != null) process.destroy();
    }

    private void read() {
        try (DataInputStream in = new DataInputStream(new BufferedInputStream(process.getInputStream()))) {
            while (true) {
                int kind = in.readUnsignedByte();
                byte[] payload = new byte[in.readInt()];
                in.readFully(payload);
                if (kind == Protocol.BATCH) {
                    ui.post(() -> mount(payload));
                } else {
                    text(kind, new DataInputStream(new ByteArrayInputStream(payload)));
                }
            }
        } catch (IOException e) {
            Log.i(TAG, "app process ended: " + e);
        }
    }

    private void log() {
        try (BufferedReader err = new BufferedReader(new InputStreamReader(process.getErrorStream()))) {
            for (String line; (line = err.readLine()) != null; ) Log.i(TAG, line);
        } catch (IOException ignored) {
            // The process ended.
        }
    }

    /** Serves the engine's text engine. Only a measurement has an answer, which the engine waits for. */
    private void text(int kind, DataInputStream in) throws IOException {
        int id = in.readInt();
        switch (kind) {
            case Protocol.PREPARE: {
                String text = Protocol.readString(in);
                Text.Style style = new Text.Style(in, density);
                prepared.put(id, new Text.Prepared(text, style, in.readInt()));
                break;
            }
            case Protocol.MEASURE: {
                float maxWidth = in.readFloat(); // NaN does not wrap
                float[] size = prepared.get(id).measure(maxWidth * density);
                send(Protocol.MEASURED, size[0] / density, size[1] / density);
                break;
            }
            case Protocol.LAYOUT: {
                int layout = in.readInt();
                layouts.put(layout, prepared.get(id).layout(in.readFloat() * density));
                break;
            }
            case Protocol.RELEASE:
                prepared.remove(id);
                break;
            default:
                Log.w(TAG, "unknown message " + kind);
        }
    }

    private synchronized void write(int kind, byte[] payload) {
        if (out == null) return;
        try {
            out.writeByte(kind);
            out.writeInt(payload.length);
            out.write(payload);
            out.flush();
        } catch (IOException e) {
            Log.w(TAG, "app process unreachable", e);
        }
    }

    private void send(int kind, float a, float b) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream(8);
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeFloat(a);
            d.writeFloat(b);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        write(kind, bytes.toByteArray());
    }

    /** Sends an event to the engine: its kind, node, count, point and text. */
    private void event(int kind, int id, long count, float x, float y, String text) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeByte(kind);
            d.writeInt(id);
            d.writeInt((int) count);
            d.writeFloat(x);
            d.writeFloat(y);
            Protocol.writeString(d, text);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        write(Protocol.EVENT, bytes.toByteArray());
    }

    /** Sends an event of kind about n if n reports that kind. */
    private void report(Node n, int kind, float x, float y, String text) {
        if (n.has(kind)) event(kind, n.id, 0, x, y, text);
    }

    private void report(Node n, int kind) {
        report(n, kind, 0, 0, "");
    }

    private int px(float points) {
        return Math.round(points * density);
    }

    private void mount(byte[] batch) {
        DataInputStream in = new DataInputStream(new ByteArrayInputStream(batch));
        try {
            int root = in.readInt();
            for (int i = 0, count = in.readInt(); i < count; i++) {
                int kind = in.readUnsignedByte();
                int id = in.readInt();
                if (kind == Protocol.ITEM_CREATE) {
                    Node n = create(id, in.readUnsignedByte());
                    // Show the root at once, so text fields can take focus
                    // while the rest of its first batch mounts.
                    if (id == root) show(n.view);
                } else {
                    apply(kind, nodes.get(id), in);
                }
            }
        } catch (IOException e) {
            Log.e(TAG, "bad batch", e);
        }
    }

    private void show(View root) {
        if (root.getParent() == this) return;
        removeAllViews();
        addView(root);
    }

    private void apply(int kind, Node n, DataInputStream in) throws IOException {
        switch (kind) {
            case Protocol.ITEM_INSERT: {
                Node parent = nodes.get(in.readInt());
                Node before = nodes.get(in.readInt()); // none for ID 0: last
                detach(n.view);
                ViewGroup group = parent.children;
                group.addView(n.view, before == null ? -1 : group.indexOfChild(before.view));
                break;
            }
            case Protocol.ITEM_REMOVE:
                detach(n.view);
                break;
            case Protocol.ITEM_DELETE:
                // Dropping the node drops its views and its text layout.
                detach(n.view);
                nodes.remove(n.id);
                break;
            case Protocol.ITEM_FRAME:
                n.view.setLayoutParams(frame(in));
                break;
            case Protocol.ITEM_VIEW_STYLE:
                style(n.view, in);
                break;
            case Protocol.ITEM_TEXT: {
                n.text = Protocol.readString(in);
                StaticLayout layout = layouts.remove(in.readInt());
                int x = px(in.readFloat()), y = px(in.readFloat());
                ((TextNode) n.view).setLayout(layout, x, y);
                describe(n);
                break;
            }
            case Protocol.ITEM_INPUT:
                input(n.field, in);
                break;
            case Protocol.ITEM_INPUT_VALUE: {
                String value = Protocol.readString(in);
                long count = in.readInt() & 0xffffffffL;
                // Skip values the user has typed past.
                if (n.edits <= count && !n.field.getText().toString().equals(value)) {
                    n.applying = true;
                    n.field.setText(value);
                    n.field.setSelection(n.field.length());
                    n.applying = false;
                }
                break;
            }
            case Protocol.ITEM_EVENTS:
                events(n, in.readLong());
                break;
            case Protocol.ITEM_VIEW_PROPS:
                props(n, in);
                break;
            case Protocol.ITEM_SCROLL_CONFIG: {
                int flags = in.readUnsignedByte();
                scroller(n, Protocol.flag(flags, 0));
                n.scroller.setVerticalScrollBarEnabled(!Protocol.flag(flags, 1));
                n.scroller.setHorizontalScrollBarEnabled(!Protocol.flag(flags, 1));
                break;
            }
            case Protocol.ITEM_SCROLL_CONTENT: {
                int w = px(in.readFloat()), h = px(in.readFloat());
                n.children.setLayoutParams(new LayoutParams(w, h, Gravity.LEFT | Gravity.TOP));
                break;
            }
            case Protocol.ITEM_FOCUS:
                if (n.field != null && n.field.requestFocus()) {
                    EditText field = n.field;
                    field.post(() -> ime.showSoftInput(field, InputMethodManager.SHOW_IMPLICIT));
                }
                break;
            case Protocol.ITEM_BLUR:
                if (n.field != null && n.field.hasFocus()) {
                    n.field.clearFocus();
                    ime.hideSoftInputFromWindow(n.field.getWindowToken(), 0);
                }
                break;
            default:
                throw new IOException("unknown mount item " + kind);
        }
    }

    private static void detach(View view) {
        ViewParent parent = view.getParent();
        if (parent instanceof ViewGroup) ((ViewGroup) parent).removeView(view);
    }

    /** Reads a frame in points into layout params in pixels, rounding its edges. */
    private LayoutParams frame(DataInputStream in) throws IOException {
        float x = in.readFloat(), y = in.readFloat(), w = in.readFloat(), h = in.readFloat();
        int left = px(x), top = px(y);
        LayoutParams lp = new LayoutParams(px(x + w) - left, px(y + h) - top, Gravity.LEFT | Gravity.TOP);
        lp.leftMargin = left;
        lp.topMargin = top;
        return lp;
    }

    private Node create(int id, int kind) {
        Context context = getContext();
        // A scroll view leaves clipping its content to its parent, so a
        // scroll node clips it.
        View view = kind == Protocol.KIND_TEXT ? new TextNode(context)
                : kind == Protocol.KIND_SCROLL ? new FrameLayout(context) : group(context);
        Node n = new Node(id, view);
        switch (kind) {
            case Protocol.KIND_TEXT:
                break;
            case Protocol.KIND_INPUT:
                n.field = field(n);
                ((ViewGroup) view).addView(n.field);
                break;
            case Protocol.KIND_SCROLL:
                n.children = new Content(context);
                scroller(n, false);
                break;
            default:
                n.children = (ViewGroup) view;
        }
        n.accessible().setAccessibilityDelegate(new View.AccessibilityDelegate() {
            @Override
            public void onInitializeAccessibilityNodeInfo(View host, AccessibilityNodeInfo info) {
                super.onInitializeAccessibilityNodeInfo(host, info);
                describe(n, info);
            }
        });
        nodes.put(id, n);
        return n;
    }

    /** A container whose children may draw outside it, as without Clip. */
    private static FrameLayout group(Context context) {
        FrameLayout group = new FrameLayout(context);
        group.setClipChildren(false);
        return group;
    }

    private EditText field(Node n) {
        EditText field = new EditText(getContext());
        field.setBackground(null);
        field.setPadding(0, 0, 0, 0);
        field.setSingleLine(true);
        field.setInputType(InputType.TYPE_CLASS_TEXT);
        field.setImeOptions(EditorInfo.IME_ACTION_DONE);
        field.addTextChangedListener(new TextWatcher() {
            @Override public void beforeTextChanged(CharSequence s, int a, int b, int c) {}
            @Override public void onTextChanged(CharSequence s, int a, int b, int c) {}
            @Override public void afterTextChanged(Editable s) {
                if (n.applying) return;
                n.edits++;
                event(Protocol.EVENT_CHANGE_TEXT, n.id, n.edits, 0, 0, s.toString());
            }
        });
        field.setOnEditorActionListener((v, action, key) -> {
            // A hardware Enter calls this on key down and again on key up.
            if (key == null || key.getAction() == KeyEvent.ACTION_DOWN) {
                report(n, Protocol.EVENT_SUBMIT_EDITING, 0, 0, field.getText().toString());
            }
            return false;
        });
        field.setOnFocusChangeListener((v, focused) ->
                report(n, focused ? Protocol.EVENT_FOCUS : Protocol.EVENT_BLUR));
        return field;
    }

    /** Gives scroll node n a scroll view of the orientation, moving its content into it. */
    private void scroller(Node n, boolean horizontal) {
        if (n.scroller != null && (n.scroller instanceof HorizontalScrollView) == horizontal) return;
        FrameLayout scroller = horizontal ? new HorizontalScrollView(getContext()) : new ScrollView(getContext());
        scroller.setOnScrollChangeListener((v, x, y, oldX, oldY) ->
                report(n, Protocol.EVENT_SCROLL, x / density, y / density, ""));
        if (n.scroller != null) {
            n.scroller.removeView(n.children);
            ((ViewGroup) n.view).removeView(n.scroller);
        }
        scroller.addView(n.children);
        ((ViewGroup) n.view).addView(scroller, new LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT));
        n.scroller = scroller;
    }

    private void style(View view, DataInputStream in) throws IOException {
        int background = in.readInt();
        float radius = in.readFloat() * density;
        float border = in.readFloat() * density;
        int borderColor = in.readInt();
        float opacity = in.readFloat();
        int shadow = in.readInt();
        float shadowRadius = in.readFloat() * density;
        int flags = in.readUnsignedByte();

        GradientDrawable bg = new GradientDrawable();
        bg.setColor(Text.argb(background));
        bg.setCornerRadius(radius);
        if (border > 0) bg.setStroke(Math.max(1, Math.round(border)), Text.argb(borderColor));
        view.setBackground(bg);
        view.setAlpha(opacity);
        // The outline is the background's, corners included.
        view.setClipToOutline(Protocol.flag(flags, 0));
        view.setVisibility(Protocol.flag(flags, 1) ? GONE : VISIBLE);
        boolean shadowed = (shadow & 0xff) != 0 && shadowRadius > 0;
        view.setElevation(shadowed ? shadowRadius : 0);
        if (shadowed) {
            view.setOutlineAmbientShadowColor(Text.argb(shadow));
            view.setOutlineSpotShadowColor(Text.argb(shadow));
        }
    }

    private void input(EditText field, DataInputStream in) throws IOException {
        Text.Style style = new Text.Style(in, density);
        String placeholder = Protocol.readString(in);
        int keyboard = in.readUnsignedByte();
        int flags = in.readUnsignedByte();
        boolean masked = Protocol.flag(flags, 0), readonly = Protocol.flag(flags, 1);
        boolean disabled = Protocol.flag(flags, 2);
        LayoutParams content = frame(in);

        // Setting the input type restarts the keyboard, so only on change.
        int type = inputType(keyboard, masked);
        if (field.getInputType() != type) field.setInputType(type);
        // Without a key listener the field takes no input but still selects.
        if (readonly) field.setKeyListener(null);
        field.setEnabled(!disabled);
        field.setHint(placeholder);
        field.setTypeface(style.typeface());
        field.setTextSize(TypedValue.COMPLEX_UNIT_PX, style.size);
        field.setTextColor(style.color);
        field.setLetterSpacing(style.letterSpacingEm());
        field.setPaintFlags(style.paintFlags(field.getPaintFlags()));
        field.setGravity(style.gravity() | Gravity.CENTER_VERTICAL);
        field.setLayoutParams(content);
    }

    private static int inputType(int keyboard, boolean masked) {
        boolean decimal = keyboard == Protocol.KEYBOARD_DECIMAL;
        boolean numeric = decimal || keyboard == Protocol.KEYBOARD_NUMBER || keyboard == Protocol.KEYBOARD_PHONE;
        if (masked) {
            return numeric
                    ? InputType.TYPE_CLASS_NUMBER | InputType.TYPE_NUMBER_VARIATION_PASSWORD
                            | (decimal ? InputType.TYPE_NUMBER_FLAG_DECIMAL : 0)
                    : InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_PASSWORD;
        }
        switch (keyboard) {
            case Protocol.KEYBOARD_EMAIL:
                return InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_EMAIL_ADDRESS;
            case Protocol.KEYBOARD_NUMBER:
                return InputType.TYPE_CLASS_NUMBER;
            case Protocol.KEYBOARD_DECIMAL:
                return InputType.TYPE_CLASS_NUMBER | InputType.TYPE_NUMBER_FLAG_DECIMAL;
            case Protocol.KEYBOARD_PHONE:
                return InputType.TYPE_CLASS_PHONE;
            case Protocol.KEYBOARD_URL:
                return InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI;
            default:
                return InputType.TYPE_CLASS_TEXT;
        }
    }

    /**
     * Listens for the events in mask. A view takes touches only while it
     * reports presses, so other touches reach the pressable view under it.
     */
    private void events(Node n, long mask) {
        n.events = mask;
        View view = n.view;
        boolean press = n.has(Protocol.EVENT_PRESS);
        boolean longPress = n.has(Protocol.EVENT_LONG_PRESS);
        boolean downUp = n.has(Protocol.EVENT_PRESS_DOWN) || n.has(Protocol.EVENT_PRESS_UP);
        boolean hover = n.has(Protocol.EVENT_HOVER_IN) || n.has(Protocol.EVENT_HOVER_OUT);
        view.setOnClickListener(press ? v -> report(n, Protocol.EVENT_PRESS) : null);
        view.setOnLongClickListener(longPress ? v -> {
            report(n, Protocol.EVENT_LONG_PRESS);
            return true;
        } : null);
        view.setOnTouchListener(downUp ? (v, e) -> {
            int action = e.getActionMasked();
            if (action == MotionEvent.ACTION_DOWN) {
                report(n, Protocol.EVENT_PRESS_DOWN);
            } else if (action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL) {
                report(n, Protocol.EVENT_PRESS_UP);
            }
            return false;
        } : null);
        view.setOnHoverListener(hover ? (v, e) -> {
            int action = e.getActionMasked();
            if (action == MotionEvent.ACTION_HOVER_ENTER) {
                report(n, Protocol.EVENT_HOVER_IN);
            } else if (action == MotionEvent.ACTION_HOVER_EXIT) {
                report(n, Protocol.EVENT_HOVER_OUT);
            }
            return false;
        } : null);
        view.setClickable(press || longPress || downUp);
        view.setLongClickable(longPress);
    }

    private void props(Node n, DataInputStream in) throws IOException {
        n.role = in.readUnsignedByte();
        int flags = in.readUnsignedByte();
        n.disabled = Protocol.flag(flags, 0);
        n.selected = Protocol.flag(flags, 1);
        n.checked = Protocol.flag(flags, 2);
        n.label = Protocol.readString(in);
        n.testID = Protocol.readString(in);
        View view = n.accessible();
        // A text field's ItemInput enables it.
        if (n.field == null) view.setEnabled(!n.disabled);
        view.setSelected(n.selected);
        view.setTag(n.testID.isEmpty() ? null : n.testID);
        describe(n);
    }

    /** Sets what assistive technology reads: the label, else a text node's text. */
    private static void describe(Node n) {
        String d = !n.label.isEmpty() ? n.label : n.text;
        n.accessible().setContentDescription(d.isEmpty() ? null : d);
    }

    private static void describe(Node n, AccessibilityNodeInfo info) {
        String className = null, role = null;
        boolean checkable = false;
        switch (n.role) {
            case Protocol.ROLE_BUTTON: className = "android.widget.Button"; break;
            case Protocol.ROLE_LINK: role = "link"; break;
            case Protocol.ROLE_HEADER: info.setHeading(true); break;
            case Protocol.ROLE_IMAGE: className = "android.widget.ImageView"; break;
            case Protocol.ROLE_CHECKBOX: className = "android.widget.CheckBox"; checkable = true; break;
            case Protocol.ROLE_SWITCH: className = "android.widget.Switch"; checkable = true; break;
            case Protocol.ROLE_RADIO: className = "android.widget.RadioButton"; checkable = true; break;
            case Protocol.ROLE_TAB: role = "tab"; break;
            case Protocol.ROLE_TEXT: className = "android.widget.TextView"; break;
            case Protocol.ROLE_TEXT_FIELD: className = "android.widget.EditText"; break;
            default: break;
        }
        if (className != null) info.setClassName(className);
        // The key AndroidX and TalkBack read a role description under.
        if (role != null) info.getExtras().putCharSequence("AccessibilityNodeInfo.roleDescription", role);
        if (checkable) {
            info.setCheckable(true);
            info.setChecked(n.checked);
        }
        info.setSelected(n.selected);
        if (!n.testID.isEmpty()) info.setViewIdResourceName(n.testID);
    }

    /**
     * A scroll view's content. Scroll views leave its length along their
     * axis unbounded, so it takes the size ItemScrollContent gave it.
     */
    private static final class Content extends FrameLayout {
        Content(Context context) {
            super(context);
            setClipChildren(false);
        }

        @Override
        protected void onMeasure(int widthSpec, int heightSpec) {
            ViewGroup.LayoutParams lp = getLayoutParams();
            super.onMeasure(MeasureSpec.makeMeasureSpec(Math.max(lp.width, 0), MeasureSpec.EXACTLY),
                    MeasureSpec.makeMeasureSpec(Math.max(lp.height, 0), MeasureSpec.EXACTLY));
        }
    }

    /** A mounted node: its views and what batches told it. UI thread only. */
    private static final class Node {
        final int id;
        /** The view placed at the node's frame in its parent. */
        final View view;
        /** Where children go: the view, or a scroll view's content. */
        ViewGroup children;
        FrameLayout scroller;
        EditText field;
        long events;
        /** User edits of field, numbered as EventChangeText numbers them. */
        long edits;
        /** Set while a batch sets field's text, which is no user edit. */
        boolean applying;
        int role;
        boolean disabled, selected, checked;
        String label = "", text = "", testID = "";

        Node(int id, View view) {
            this.id = id;
            this.view = view;
        }

        boolean has(int kind) {
            return (events & (1L << kind)) != 0;
        }

        /** The view assistive technology sees: an input's text field, else the view. */
        View accessible() {
            return field != null ? field : view;
        }
    }
}
