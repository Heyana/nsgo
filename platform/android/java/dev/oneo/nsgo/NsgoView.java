package dev.oneo.nsgo;

import android.animation.ValueAnimator;
import android.app.Activity;
import android.app.AlertDialog;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.ContextWrapper;
import android.content.DialogInterface;
import android.content.res.Configuration;
import android.database.ContentObserver;
import android.graphics.Bitmap;
import android.graphics.Insets;
import android.net.ConnectivityManager;
import android.net.LinkProperties;
import android.net.Network;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.provider.Settings;
import android.text.Editable;
import android.text.InputType;
import android.text.StaticLayout;
import android.text.TextWatcher;
import android.util.DisplayMetrics;
import android.util.Log;
import android.util.SparseArray;
import android.util.TypedValue;
import android.view.DisplayCutout;
import android.view.Gravity;
import android.view.KeyCharacterMap;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.ScaleGestureDetector;
import android.view.VelocityTracker;
import android.view.View;
import android.view.ViewGroup;
import android.view.ViewParent;
import android.view.ViewTreeObserver;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.view.accessibility.AccessibilityManager;
import android.view.accessibility.AccessibilityNodeInfo;
import android.view.animation.DecelerateInterpolator;
import android.view.animation.LinearInterpolator;
import android.view.animation.PathInterpolator;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputMethodManager;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.HorizontalScrollView;
import android.widget.ScrollView;
import android.window.OnBackInvokedCallback;
import android.window.OnBackInvokedDispatcher;
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
import java.net.InetAddress;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.function.IntConsumer;

/**
 * A surface showing one NSGo app. It starts the app's Go executable
 * (libnsgo.so in the native library directory), serves its text engine on
 * a reader thread and mounts its batches on the UI thread, one Android view
 * per node placed at the frame the engine computed. Points are
 * density-independent pixels. It reports the environment as it changes,
 * consumes back while the app runs and decodes the images the app fetches.
 * It needs API 28, and its activity should handle configuration changes
 * itself and draw edge to edge, as NsgoActivity does.
 */
public class NsgoView extends FrameLayout {
    private static final String TAG = "nsgo";
    private static final int DESTRUCTIVE = 0xffe53935;

    /** Pixels per point; the reader thread reads it too. */
    private volatile float density;
    private final Handler ui = new Handler(Looper.getMainLooper());
    private final InputMethodManager ime;
    /** Mounted nodes by ID, on the UI thread. */
    private final SparseArray<Node> nodes = new SparseArray<>();
    /** Prepared text by ID, on the reader thread. */
    private final SparseArray<Text.Prepared> prepared = new SparseArray<>();
    /** Laid-out text by ID: the reader thread adds it, the batch that draws it takes it. */
    private final ConcurrentHashMap<Integer, StaticLayout> layouts = new ConcurrentHashMap<>();
    /** Decodes images off the UI thread, one at a time. */
    private final ExecutorService decoder = Executors.newSingleThreadExecutor(r -> {
        Thread t = new Thread(r, "nsgo-images");
        t.setDaemon(true);
        return t;
    });
    /** Nodes reporting presses outside them, on the UI thread. */
    private final ArrayList<Node> outside = new ArrayList<>();
    private final int[] location = new int[2];
    private Process process;
    private DataOutputStream out;

    // The environment, on the UI thread: the insets in pixels and what was last sent.
    private final AccessibilityManager accessibility;
    private final AccessibilityManager.TouchExplorationStateChangeListener screenReader = on -> environment();
    private final ContentObserver motion = new ContentObserver(ui) {
        @Override
        public void onChange(boolean self) {
            environment();
        }
    };
    private int safeTop, safeRight, safeBottom, safeLeft, keyboard;
    private byte[] environment;
    private ConnectivityManager.NetworkCallback network;
    /** The OnBackInvokedDispatcher and OnBackInvokedCallback of back on API 33 and later. */
    private Object backDispatcher, backCallback;
    /** The keymap's bindings, and the keys whose presses it consumed, on the UI thread. */
    private Set<String> keys = new HashSet<>();
    private final Set<Integer> consumedKeys = new HashSet<>();
    private int statusBarStyle;
    private boolean statusBarHidden;
    /** The pan, pinch and rotation of the touch in progress, on the UI thread. */
    private final Gestures gestures;

    public NsgoView(Context context) {
        super(context);
        density = context.getResources().getDisplayMetrics().density;
        ime = context.getSystemService(InputMethodManager.class);
        accessibility = context.getSystemService(AccessibilityManager.class);
        gestures = new Gestures(context);
        setOnApplyWindowInsetsListener((v, insets) -> {
            insets(insets);
            environment();
            return insets;
        });
    }

    @Override
    protected void onSizeChanged(int w, int h, int oldw, int oldh) {
        super.onSizeChanged(w, h, oldw, oldh);
        boolean first = process == null;
        if (first) start();
        environment();
        send(first ? Protocol.START : Protocol.RESIZE, w / density, h / density);
    }

    @Override
    protected void onConfigurationChanged(Configuration config) {
        super.onConfigurationChanged(config);
        // Text prepared before a change of density keeps its pixel size
        // until the engine prepares it again.
        float d = getResources().getDisplayMetrics().density;
        if (d != density && process != null) {
            density = d;
            send(Protocol.RESIZE, getWidth() / density, getHeight() / density);
        }
        environment();
        systemBars();
    }

    @Override
    protected void onAttachedToWindow() {
        super.onAttachedToWindow();
        accessibility.addTouchExplorationStateChangeListener(screenReader);
        getContext().getContentResolver().registerContentObserver(
                Settings.Global.getUriFor(Settings.Global.ANIMATOR_DURATION_SCALE), false, motion);
        if (Build.VERSION.SDK_INT >= 33) registerBack();
        systemBars();
    }

    @Override
    protected void onDetachedFromWindow() {
        accessibility.removeTouchExplorationStateChangeListener(screenReader);
        getContext().getContentResolver().unregisterContentObserver(motion);
        unregisterBack();
        super.onDetachedFromWindow();
    }

    private void start() {
        File exe = new File(getContext().getApplicationInfo().nativeLibraryDir, "libnsgo.so");
        try {
            ProcessBuilder builder = new ProcessBuilder(exe.getAbsolutePath());
            builder.environment().put("NSGO_DATA_DIR", getContext().getFilesDir().getAbsolutePath());
            builder.environment().put("NSGO_CACHE_DIR", getContext().getCacheDir().getAbsolutePath());
            process = builder.start();
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
        watchNetwork();
    }

    /** Ends the app's process. */
    public void stop() {
        if (network != null) {
            getContext().getSystemService(ConnectivityManager.class).unregisterNetworkCallback(network);
        }
        decoder.shutdownNow();
        if (process != null) process.destroy();
    }

    private void read() {
        try (DataInputStream in = new DataInputStream(new BufferedInputStream(process.getInputStream()))) {
            while (true) {
                int kind = in.readUnsignedByte();
                byte[] payload = new byte[in.readInt()];
                in.readFully(payload);
                DataInputStream message = new DataInputStream(new ByteArrayInputStream(payload));
                switch (kind) {
                    case Protocol.BATCH:
                        ui.post(() -> mount(payload));
                        break;
                    case Protocol.IMAGE:
                        image(message);
                        break;
                    default:
                        text(kind, message);
                }
            }
        } catch (IOException e) {
            Log.i(TAG, "app process ended: " + e);
        }
        // Leave back to the system from now on.
        ui.post(this::unregisterBack);
    }

    private void log() {
        try (BufferedReader err = new BufferedReader(new InputStreamReader(process.getErrorStream()))) {
            for (String line; (line = err.readLine()) != null; ) Log.i(TAG, line);
        } catch (IOException ignored) {
            // The process ended.
        }
    }

    /**
     * Sends the environment if it changed: before START, then whenever the
     * insets, the configuration or an accessibility setting change. The
     * keyboard takes the system's default duration.
     */
    private void environment() {
        if (process == null) return;
        Configuration config = getResources().getConfiguration();
        boolean dark = dark();
        // Android's reduce-motion setting turns animations off.
        boolean reduceMotion = Settings.Global.getFloat(getContext().getContentResolver(),
                Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f;
        boolean reader = accessibility.isTouchExplorationEnabled();
        boolean rtl = config.getLayoutDirection() == View.LAYOUT_DIRECTION_RTL;
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeFloat(safeTop / density);
            d.writeFloat(safeRight / density);
            d.writeFloat(safeBottom / density);
            d.writeFloat(safeLeft / density);
            d.writeFloat(keyboard / density);
            d.writeFloat(config.fontScale);
            d.writeFloat(density);
            d.writeByte((dark ? 1 : 0) | (reduceMotion ? 2 : 0) | (reader ? 4 : 0) | (rtl ? 8 : 0));
            d.writeByte(config.smallestScreenWidthDp >= 600 ? Protocol.IDIOM_TABLET : Protocol.IDIOM_PHONE);
            Protocol.writeString(d, config.getLocales().get(0).toLanguageTag());
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        byte[] payload = bytes.toByteArray();
        if (Arrays.equals(payload, environment)) return;
        environment = payload;
        write(Protocol.ENVIRONMENT, payload);
    }

    private boolean dark() {
        int night = getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK;
        return night == Configuration.UI_MODE_NIGHT_YES;
    }

    /** Reads the safe area and the keyboard's height, in pixels, from the window's insets. */
    private void insets(WindowInsets insets) {
        if (Build.VERSION.SDK_INT >= 30) {
            Insets bars = insets.getInsets(WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout());
            safeTop = bars.top;
            safeRight = bars.right;
            safeBottom = bars.bottom;
            safeLeft = bars.left;
            keyboard = insets.getInsets(WindowInsets.Type.ime()).bottom;
            return;
        }
        // Before API 30 the bottom inset grows by the keyboard while it
        // shows, and the stable one never does.
        int bottom = insets.getSystemWindowInsetBottom(), stable = insets.getStableInsetBottom();
        safeTop = insets.getSystemWindowInsetTop();
        safeRight = insets.getSystemWindowInsetRight();
        safeBottom = Math.min(bottom, stable);
        safeLeft = insets.getSystemWindowInsetLeft();
        keyboard = bottom > stable ? bottom : 0;
        DisplayCutout cutout = insets.getDisplayCutout();
        if (cutout != null) {
            safeTop = Math.max(safeTop, cutout.getSafeInsetTop());
            safeRight = Math.max(safeRight, cutout.getSafeInsetRight());
            safeBottom = Math.max(safeBottom, cutout.getSafeInsetBottom());
            safeLeft = Math.max(safeLeft, cutout.getSafeInsetLeft());
        }
    }

    /**
     * Tells the app the default network's DNS servers as they change: Go
     * outside the runtime has no resolver of the system's to ask. It needs
     * the ACCESS_NETWORK_STATE permission.
     */
    private void watchNetwork() {
        ConnectivityManager connectivity = getContext().getSystemService(ConnectivityManager.class);
        network = new ConnectivityManager.NetworkCallback() {
            @Override
            public void onLinkPropertiesChanged(Network n, LinkProperties properties) {
                dns(properties.getDnsServers());
            }
        };
        try {
            connectivity.registerDefaultNetworkCallback(network);
        } catch (SecurityException e) {
            Log.w(TAG, "the app resolves no host names without ACCESS_NETWORK_STATE", e);
            network = null;
        }
    }

    private void dns(List<InetAddress> servers) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeInt(servers.size());
            for (InetAddress server : servers) Protocol.writeString(d, server.getHostAddress());
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        write(Protocol.DNS, bytes.toByteArray());
    }

    /** Serves the engine's text engine. Only a measurement has an answer, which the engine waits for. */
    private void text(int kind, DataInputStream in) throws IOException {
        int id = in.readInt();
        switch (kind) {
            case Protocol.PREPARE: {
                String text = Protocol.readString(in);
                Text.Style style = new Text.Style(in, density);
                int maxLines = in.readInt();
                List<Text.Span> spans = new ArrayList<>();
                for (int i = 0, n = in.readInt(); i < n; i++) spans.add(new Text.Span(in, density));
                prepared.put(id, new Text.Prepared(text, style, maxLines, spans));
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

    /**
     * Shows the bytes Go fetched for an image node's request: decoded on
     * the decoder thread at about the node's size, and dropped once the
     * node has moved on to another request.
     */
    private void image(DataInputStream in) throws IOException {
        int id = in.readInt(), request = in.readInt(), part = in.readUnsignedByte();
        float scale = in.readFloat();
        byte[] data = new byte[in.readInt()];
        in.readFully(data);
        ui.post(() -> {
            Node n = nodes.get(id);
            if (n == null || n.image == null || n.image.request != request || decoder.isShutdown()) return;
            ViewGroup.LayoutParams lp = n.view.getLayoutParams();
            DisplayMetrics screen = getResources().getDisplayMetrics();
            int width = lp != null && lp.width > 0 ? lp.width : screen.widthPixels;
            int height = lp != null && lp.height > 0 ? lp.height : screen.heightPixels;
            decoder.execute(() -> {
                int[] size = new int[2];
                Bitmap bitmap = ImageNode.decode(data, scale, width, height, size);
                ui.post(() -> {
                    if (nodes.get(id) != n || n.image.request != request) return;
                    if (part == Protocol.IMAGE_PLACEHOLDER) {
                        if (bitmap != null) n.image.showPlaceholder(bitmap);
                        return;
                    }
                    if (bitmap == null) {
                        report(n, Protocol.EVENT_ERROR, 0, 0, "the image cannot be decoded");
                    } else {
                        n.image.showSource(bitmap);
                        if (n.has(Protocol.EVENT_LOAD)) {
                            event(Protocol.EVENT_LOAD, id, 0, 0, 0, "", size[0], size[1], 0);
                        }
                    }
                    report(n, Protocol.EVENT_LOAD_END);
                });
            });
        });
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

    private void event(int kind, int id, long count, float x, float y, String text, float width, float height,
            int index) {
        event(kind, id, count, x, y, text, width, height, index, 0, 0, 0, 0);
    }

    /**
     * Sends an event to the engine: its kind, node, count, point, text,
     * size and index, then a gesture's phase, velocity and value.
     */
    private void event(int kind, int id, long count, float x, float y, String text, float width, float height,
            int index, int phase, float vx, float vy, float value) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeByte(kind);
            d.writeInt(id);
            d.writeInt((int) count);
            d.writeFloat(x);
            d.writeFloat(y);
            Protocol.writeString(d, text);
            d.writeFloat(width);
            d.writeFloat(height);
            d.writeInt(index);
            d.writeByte(phase);
            d.writeFloat(vx);
            d.writeFloat(vy);
            d.writeFloat(value);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        write(Protocol.EVENT, bytes.toByteArray());
    }

    private void event(int kind, int id, long count, float x, float y, String text) {
        event(kind, id, count, x, y, text, 0, 0, 0);
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

    /**
     * Reports back to the engine, which routes it to the topmost overlay,
     * modal or stack. While the app runs it takes back even when nothing
     * there does, for the engine cannot answer; false once the app ended.
     */
    public boolean back() {
        if (process == null || !process.isAlive()) return false;
        event(Protocol.EVENT_BACK, 0, 0, 0, 0, "");
        return true;
    }

    /** Takes back on API 33 and later, where predictive back skips Activity.onBackPressed. */
    private void registerBack() {
        OnBackInvokedDispatcher dispatcher = findOnBackInvokedDispatcher();
        if (dispatcher == null) return;
        OnBackInvokedCallback callback = this::back;
        dispatcher.registerOnBackInvokedCallback(OnBackInvokedDispatcher.PRIORITY_DEFAULT, callback);
        backDispatcher = dispatcher;
        backCallback = callback;
    }

    private void unregisterBack() {
        if (backCallback == null || Build.VERSION.SDK_INT < 33) return;
        OnBackInvokedDispatcher dispatcher = (OnBackInvokedDispatcher) backDispatcher;
        dispatcher.unregisterOnBackInvokedCallback((OnBackInvokedCallback) backCallback);
        backDispatcher = null;
        backCallback = null;
    }

    /**
     * Sees each touch before the views below: a touch starting reports
     * presses outside overlays and collects the gesture views under it as
     * they see it; each later one moves the gestures first, so a pan that
     * activates stops the scroll views around it from taking the move.
     */
    @Override
    public boolean dispatchTouchEvent(MotionEvent e) {
        boolean down = e.getActionMasked() == MotionEvent.ACTION_DOWN;
        if (down) {
            pressOutside(e);
            gestures.reset();
        } else {
            gestures.track(e);
        }
        boolean handled = super.dispatchTouchEvent(e);
        if (down) gestures.down(e);
        return handled;
    }

    /** Reports a touch starting outside a node that listens for one, at its point in points. */
    private void pressOutside(MotionEvent e) {
        int x = Math.round(e.getRawX()), y = Math.round(e.getRawY());
        for (Node n : outside) {
            if (n.view.isShown() && !contains(n.view, x, y)) {
                event(Protocol.EVENT_PRESS_OUTSIDE, n.id, 0, e.getX() / density, e.getY() / density, "");
            }
        }
    }

    /** Whether the screen point (x, y) is on view or on a descendant it lets draw outside it. */
    private boolean contains(View view, int x, int y) {
        if (view.getVisibility() != VISIBLE) return false;
        view.getLocationOnScreen(location);
        if (x >= location[0] && y >= location[1]
                && x < location[0] + view.getWidth() && y < location[1] + view.getHeight()) {
            return true;
        }
        if (!(view instanceof ViewGroup) || view.getClipToOutline() || ((ViewGroup) view).getClipChildren()) {
            return false;
        }
        ViewGroup group = (ViewGroup) view;
        for (int i = 0; i < group.getChildCount(); i++) {
            if (contains(group.getChildAt(i), x, y)) return true;
        }
        return false;
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
                if (n.modal >= 0 && !n.presented) {
                    n.presented = true;
                    present(n);
                }
                break;
            }
            case Protocol.ITEM_REMOVE:
                detach(n.view);
                break;
            case Protocol.ITEM_DELETE:
                // Dropping the node drops its views, its text layout and its bitmaps.
                detach(n.view);
                nodes.remove(n.id);
                outside.remove(n);
                if (n.animator != null) n.animator.cancel();
                break;
            case Protocol.ITEM_FRAME:
                n.view.setLayoutParams(frame(in));
                break;
            case Protocol.ITEM_VIEW_STYLE:
                style(n, in);
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
            case Protocol.ITEM_IMAGE: {
                int request = in.readInt();
                int mode = in.readUnsignedByte();
                float blur = in.readFloat() * density;
                n.image.configure(request, mode, blur, in.readInt());
                break;
            }
            case Protocol.ITEM_MODAL:
                n.modal = in.readUnsignedByte();
                break;
            case Protocol.ITEM_ALERT:
                alert(in);
                break;
            case Protocol.ITEM_DISMISS_KEYBOARD: {
                View focused = findFocus();
                if (focused != null) {
                    ime.hideSoftInputFromWindow(focused.getWindowToken(), 0);
                    focused.clearFocus();
                }
                break;
            }
            case Protocol.ITEM_SCROLL_TO: {
                int x = px(in.readFloat()), y = px(in.readFloat());
                boolean animated = Protocol.flag(in.readUnsignedByte(), 0);
                FrameLayout scroller = n.scroller;
                // Scroll views clamp to their content, which takes its new
                // size at the next layout.
                afterLayout(scroller, () -> {
                    if (animated) {
                        if (scroller instanceof ScrollView) ((ScrollView) scroller).smoothScrollTo(x, y);
                        else ((HorizontalScrollView) scroller).smoothScrollTo(x, y);
                    } else {
                        scroller.scrollTo(x, y);
                    }
                });
                break;
            }
            case Protocol.ITEM_CANVAS: {
                byte[] ops = new byte[in.readInt()];
                in.readFully(ops);
                n.canvas.setOps(ops);
                break;
            }
            case Protocol.ITEM_SET_CLIPBOARD:
                clipboard().setPrimaryClip(ClipData.newPlainText("", Protocol.readString(in)));
                break;
            case Protocol.ITEM_READ_CLIPBOARD: {
                long count = in.readInt() & 0xffffffffL;
                // Android answers null unless the app has focus.
                ClipData clip = clipboard().getPrimaryClip();
                CharSequence text = clip != null && clip.getItemCount() > 0
                        ? clip.getItemAt(0).coerceToText(getContext()) : null;
                event(Protocol.EVENT_CLIPBOARD, 0, count, 0, 0, text != null ? text.toString() : "");
                break;
            }
            case Protocol.ITEM_ANNOUNCE:
                announceForAccessibility(Protocol.readString(in));
                break;
            case Protocol.ITEM_KEYMAP: {
                Set<String> bindings = new HashSet<>();
                for (int i = 0, count = in.readInt(); i < count; i++) bindings.add(Protocol.readString(in));
                keys = bindings;
                break;
            }
            case Protocol.ITEM_STATUS_BAR:
                statusBarStyle = in.readUnsignedByte();
                statusBarHidden = Protocol.flag(in.readUnsignedByte(), 0);
                systemBars();
                break;
            case Protocol.ITEM_TITLE: {
                // Phones show no title; the window's names it to TalkBack.
                String title = Protocol.readString(in);
                Activity activity = activity();
                if (activity != null) activity.setTitle(title);
                break;
            }
            case Protocol.ITEM_WINDOW:
            case Protocol.ITEM_MENU:
                // Desktop windows' and menus'; Go writes no fields.
                break;
            case Protocol.ITEM_BACK_UNHANDLED: {
                // As the system's back does from API 31: a task's root
                // activity goes to the background, keeping the app; any
                // other finishes.
                Activity activity = activity();
                if (activity != null && !(activity.isTaskRoot() && activity.moveTaskToBack(true))) {
                    activity.finish();
                }
                break;
            }
            default:
                // Go writes no fields for the kinds it does not encode.
                Log.w(TAG, "unknown mount item " + kind);
        }
    }

    private static void detach(View view) {
        ViewParent parent = view.getParent();
        if (parent instanceof ViewGroup) ((ViewGroup) parent).removeView(view);
    }

    /** Runs r once view has been laid out with what it was last given. */
    private static void afterLayout(View view, Runnable r) {
        if (view.isLaidOut() && !view.isLayoutRequested()) {
            r.run();
            return;
        }
        view.getViewTreeObserver().addOnGlobalLayoutListener(new ViewTreeObserver.OnGlobalLayoutListener() {
            @Override
            public void onGlobalLayout() {
                view.getViewTreeObserver().removeOnGlobalLayoutListener(this);
                r.run();
            }
        });
    }

    private ClipboardManager clipboard() {
        return getContext().getSystemService(ClipboardManager.class);
    }

    private Activity activity() {
        for (Context c = getContext(); c instanceof ContextWrapper; c = ((ContextWrapper) c).getBaseContext()) {
            if (c instanceof Activity) return (Activity) c;
        }
        return null;
    }

    /**
     * Lays the view out behind the system bars and draws their icons to
     * contrast with the status bar style or, for Auto, the appearance; and
     * hides the status bar when asked.
     */
    private void systemBars() {
        boolean dark = dark();
        boolean darkIcons = statusBarStyle == Protocol.STATUS_BAR_DARK
                || statusBarStyle != Protocol.STATUS_BAR_LIGHT && !dark;
        if (Build.VERSION.SDK_INT >= 30) {
            WindowInsetsController bars = getWindowInsetsController();
            if (bars == null) return;
            int light = WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS
                    | WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS;
            int appearance = (darkIcons ? WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS : 0)
                    | (dark ? 0 : WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS);
            bars.setSystemBarsAppearance(appearance, light);
            bars.setSystemBarsBehavior(WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE);
            if (statusBarHidden) bars.hide(WindowInsets.Type.statusBars());
            else bars.show(WindowInsets.Type.statusBars());
            return;
        }
        int flags = SYSTEM_UI_FLAG_LAYOUT_STABLE | SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                | SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION;
        if (darkIcons) flags |= SYSTEM_UI_FLAG_LIGHT_STATUS_BAR;
        if (!dark) flags |= SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR;
        if (statusBarHidden) flags |= SYSTEM_UI_FLAG_FULLSCREEN;
        getRootView().setSystemUiVisibility(flags);
    }

    /**
     * Consumes a key press matching a keymap binding, named with "mod" for
     * Control or with the modifiers as they are, and reports it as EventKey.
     */
    @Override
    public boolean dispatchKeyEvent(KeyEvent e) {
        int code = e.getKeyCode();
        if (e.getAction() == KeyEvent.ACTION_UP && consumedKeys.remove(code)) return true;
        if (e.getAction() == KeyEvent.ACTION_DOWN && !keys.isEmpty()) {
            String binding = binding(e);
            if (binding != null) {
                consumedKeys.add(code);
                event(Protocol.EVENT_KEY, 0, 0, 0, 0, binding);
                return true;
            }
        }
        return super.dispatchKeyEvent(e);
    }

    private String binding(KeyEvent e) {
        String key = keyName(e);
        if (key == null) return null;
        String rest = (e.isAltPressed() ? "alt+" : "") + (e.isShiftPressed() ? "shift+" : "")
                + (e.isMetaPressed() ? "meta+" : "") + key;
        if (e.isCtrlPressed()) {
            if (keys.contains("mod+" + rest)) return "mod+" + rest;
            rest = "ctrl+" + rest;
        }
        return keys.contains(rest) ? rest : null;
    }

    /** The key's name as core.NormalizeKey writes it, or null for keys it has none for. */
    private static String keyName(KeyEvent e) {
        int code = e.getKeyCode();
        if (code >= KeyEvent.KEYCODE_A && code <= KeyEvent.KEYCODE_Z) {
            return String.valueOf((char) ('a' + code - KeyEvent.KEYCODE_A));
        }
        if (code >= KeyEvent.KEYCODE_0 && code <= KeyEvent.KEYCODE_9) {
            return String.valueOf((char) ('0' + code - KeyEvent.KEYCODE_0));
        }
        if (code >= KeyEvent.KEYCODE_F1 && code <= KeyEvent.KEYCODE_F12) return "f" + (code - KeyEvent.KEYCODE_F1 + 1);
        switch (code) {
            case KeyEvent.KEYCODE_ENTER: case KeyEvent.KEYCODE_NUMPAD_ENTER: return "enter";
            case KeyEvent.KEYCODE_ESCAPE: return "escape";
            case KeyEvent.KEYCODE_TAB: return "tab";
            case KeyEvent.KEYCODE_SPACE: return "space";
            case KeyEvent.KEYCODE_DEL: return "backspace";
            case KeyEvent.KEYCODE_FORWARD_DEL: return "delete";
            case KeyEvent.KEYCODE_DPAD_UP: return "up";
            case KeyEvent.KEYCODE_DPAD_DOWN: return "down";
            case KeyEvent.KEYCODE_DPAD_LEFT: return "left";
            case KeyEvent.KEYCODE_DPAD_RIGHT: return "right";
            case KeyEvent.KEYCODE_MOVE_HOME: return "home";
            case KeyEvent.KEYCODE_MOVE_END: return "end";
            case KeyEvent.KEYCODE_PAGE_UP: return "pageup";
            case KeyEvent.KEYCODE_PAGE_DOWN: return "pagedown";
            default: break;
        }
        // Other keys by the character they type unmodified, such as "/".
        int c = e.getKeyCharacterMap().get(code, 0);
        if (c <= ' ' || (c & KeyCharacterMap.COMBINING_ACCENT) != 0) return null;
        return new String(Character.toChars(c)).toLowerCase(Locale.ROOT);
    }

    /**
     * Animates a modal in as it is first inserted. Android's reduce-motion
     * setting, no animations, makes either animation instant.
     */
    private void present(Node n) {
        View view = n.view;
        if (n.modal == Protocol.MODAL_SLIDE) {
            ViewGroup.LayoutParams lp = view.getLayoutParams();
            view.setTranslationY(lp != null && lp.height > 0 ? lp.height : getHeight());
            view.animate().translationY(0).setDuration(300).setInterpolator(new DecelerateInterpolator());
        } else if (n.modal == Protocol.MODAL_FADE) {
            float opacity = view.getAlpha();
            view.setAlpha(0);
            view.animate().alpha(opacity).setDuration(200);
        }
    }

    /**
     * Shows an alert without waiting for it and answers with the index of
     * the action chosen, or, when dismissed, of the cancel action or -1.
     */
    private void alert(DataInputStream in) throws IOException {
        int id = in.readInt();
        String title = Protocol.readString(in), message = Protocol.readString(in);
        int count = in.readInt();
        String[] labels = new String[count];
        int[] roles = new int[count];
        int cancel = -1;
        for (int i = 0; i < count; i++) {
            labels[i] = Protocol.readString(in);
            roles[i] = in.readUnsignedByte();
            if (cancel < 0 && roles[i] == Protocol.ALERT_CANCEL) cancel = i;
        }
        boolean[] answered = {false};
        IntConsumer answer = index -> {
            if (answered[0]) return;
            answered[0] = true;
            event(Protocol.EVENT_ALERT, 0, id, 0, 0, "", 0, 0, index);
        };
        AlertDialog.Builder b = new AlertDialog.Builder(getContext(), dark()
                ? android.R.style.Theme_DeviceDefault_Dialog_Alert
                : android.R.style.Theme_DeviceDefault_Light_Dialog_Alert);
        int[] buttons = count <= 3 ? buttons(count, cancel) : null;
        if (buttons == null) {
            // A message hides a list, so the title takes it.
            String heading = title.isEmpty() || message.isEmpty() ? title + message : title + "\n\n" + message;
            if (!heading.isEmpty()) b.setTitle(heading);
            b.setItems(labels, (d, which) -> answer.accept(which));
        } else {
            if (!title.isEmpty()) b.setTitle(title);
            if (!message.isEmpty()) b.setMessage(message);
            for (int i = 0; i < count; i++) {
                int index = i;
                DialogInterface.OnClickListener click = (d, which) -> answer.accept(index);
                switch (buttons[i]) {
                    case DialogInterface.BUTTON_POSITIVE: b.setPositiveButton(labels[i], click); break;
                    case DialogInterface.BUTTON_NEGATIVE: b.setNegativeButton(labels[i], click); break;
                    default: b.setNeutralButton(labels[i], click);
                }
            }
        }
        // Back and touches outside cancel it.
        int dismissed = cancel;
        b.setOnCancelListener(d -> answer.accept(dismissed));
        AlertDialog dialog = b.show();
        for (int i = 0; buttons != null && i < count; i++) {
            if (roles[i] == Protocol.ALERT_DESTRUCTIVE) dialog.getButton(buttons[i]).setTextColor(DESTRUCTIVE);
        }
    }

    /**
     * The button of each of count actions: the cancel action's is the
     * negative one, and the others take the positive, the neutral, then the
     * negative one in order.
     */
    private static int[] buttons(int count, int cancel) {
        int[] order = {
            DialogInterface.BUTTON_POSITIVE, DialogInterface.BUTTON_NEUTRAL, DialogInterface.BUTTON_NEGATIVE,
        };
        int[] buttons = new int[count];
        for (int i = 0, next = 0; i < count; i++) {
            buttons[i] = i == cancel ? DialogInterface.BUTTON_NEGATIVE : order[next++];
        }
        return buttons;
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
                : kind == Protocol.KIND_IMAGE ? new ImageNode(context)
                : kind == Protocol.KIND_CANVAS ? new CanvasNode(context)
                : kind == Protocol.KIND_SCROLL ? new FrameLayout(context)
                : kind == Protocol.KIND_MODAL ? new Layer(context) : new Group(context);
        Node n = new Node(id, view);
        if (view instanceof Group) ((Group) view).node = n;
        switch (kind) {
            case Protocol.KIND_TEXT:
                break;
            case Protocol.KIND_IMAGE:
                n.image = (ImageNode) view;
                break;
            case Protocol.KIND_CANVAS:
                n.canvas = (CanvasNode) view;
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

    /**
     * Applies an ItemViewStyle. With a transition, the properties it lists
     * move from what the view shows to the new look on a ValueAnimator,
     * which Android's animator scale, its reduce-motion setting, also
     * scales; the rest change at once, as on a view's first style.
     */
    private void style(Node n, DataInputStream in) throws IOException {
        View view = n.view;
        Look look = new Look();
        look.background = Text.argb(in.readInt());
        look.radius = in.readFloat() * density;
        look.border = in.readFloat() * density;
        look.borderColor = Text.argb(in.readInt());
        for (int i = 0; i < 4; i++) look.borders[i] = in.readFloat() * density;
        for (int i = 0; i < 4; i++) look.borderColors[i] = Text.argb(in.readInt());
        look.sides = look.borders[0] > 0 || look.borders[1] > 0 || look.borders[2] > 0 || look.borders[3] > 0;
        look.opacity = in.readFloat();
        look.shadowColor = Text.argb(in.readInt());
        look.shadowRadius = in.readFloat() * density;
        int tint = in.readInt();
        look.translateX = in.readFloat() * density;
        look.translateY = in.readFloat() * density;
        look.scaleX = in.readFloat();
        look.scaleY = in.readFloat();
        look.rotation = in.readFloat();
        long duration = in.readInt() & 0xffffffffL, delay = in.readInt() & 0xffffffffL;
        float x1 = in.readFloat(), y1 = in.readFloat(), x2 = in.readFloat(), y2 = in.readFloat();
        int properties = in.readUnsignedByte();
        int flags = in.readUnsignedByte();

        if (n.image != null) n.image.setTint(Text.argb(tint));
        // The outline is the background's, corners included.
        view.setClipToOutline(Protocol.flag(flags, 0));
        view.setVisibility(Protocol.flag(flags, 1) ? GONE : VISIBLE);
        if (n.background == null) {
            n.background = new Box();
            view.setBackground(n.background);
        }
        if (n.animator != null) n.animator.cancel(); // n.shown is where it stopped
        if (duration == 0 || n.shown == null) {
            n.shown = look;
            look.apply(view, n.background);
            return;
        }
        Look from = n.shown;
        n.shown = Look.mix(from, look, 0, properties);
        n.shown.apply(view, n.background);
        ValueAnimator animator = ValueAnimator.ofFloat(0, 1);
        animator.setDuration(duration);
        animator.setStartDelay(delay);
        boolean linear = x1 == 0 && y1 == 0 && x2 == 0 && y2 == 0;
        animator.setInterpolator(linear ? new LinearInterpolator() : new PathInterpolator(x1, y1, x2, y2));
        animator.addUpdateListener(a -> {
            n.shown = Look.mix(from, look, (float) a.getAnimatedValue(), properties);
            n.shown.apply(view, n.background);
        });
        n.animator = animator;
        animator.start();
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
     * reports presses or follows gestures (Group), so other touches reach
     * the pressable view under it.
     */
    private void events(Node n, long mask) {
        n.events = mask;
        View view = n.view;
        boolean press = n.has(Protocol.EVENT_PRESS);
        boolean longPress = n.has(Protocol.EVENT_LONG_PRESS);
        boolean downUp = n.has(Protocol.EVENT_PRESS_DOWN) || n.has(Protocol.EVENT_PRESS_UP);
        boolean hover = n.has(Protocol.EVENT_HOVER_IN) || n.has(Protocol.EVENT_HOVER_OUT);
        // A text view's press carries the byte offset of the character
        // under it, for its link spans.
        view.setOnClickListener(press ? v -> event(Protocol.EVENT_PRESS, n.id, 0, 0, 0, "", 0, 0,
                v instanceof TextNode ? ((TextNode) v).pressedIndex() : 0) : null);
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
        // A secondary click is no touch; it comes as a generic motion.
        view.setOnGenericMotionListener(n.has(Protocol.EVENT_CONTEXT_MENU) ? (v, e) -> {
            if (e.getActionMasked() != MotionEvent.ACTION_BUTTON_PRESS
                    || e.getActionButton() != MotionEvent.BUTTON_SECONDARY) {
                return false;
            }
            report(n, Protocol.EVENT_CONTEXT_MENU, e.getX() / density, e.getY() / density, "");
            return true;
        } : null);
        view.setClickable(press || longPress || downUp);
        view.setLongClickable(longPress);
        outside.remove(n);
        if (n.has(Protocol.EVENT_PRESS_OUTSIDE)) outside.add(n);
    }

    private void props(Node n, DataInputStream in) throws IOException {
        n.role = in.readUnsignedByte();
        int flags = in.readUnsignedByte();
        n.disabled = Protocol.flag(flags, 0);
        n.selected = Protocol.flag(flags, 1);
        n.checked = Protocol.flag(flags, 2);
        n.mixed = Protocol.flag(flags, 3);
        n.label = Protocol.readString(in);
        n.testID = Protocol.readString(in);
        n.activeX = in.readFloat();
        n.activeY = in.readFloat();
        n.failX = in.readFloat();
        n.failY = in.readFloat();
        View view = n.accessible();
        // A text field's ItemInput enables it.
        if (n.field == null) view.setEnabled(!n.disabled);
        view.setSelected(n.selected);
        view.setTag(n.testID.isEmpty() ? null : n.testID);
        // A dialog announces its label as it appears, an alert its content.
        view.setAccessibilityPaneTitle(n.role == Protocol.ROLE_DIALOG && !n.label.isEmpty() ? n.label : null);
        view.setAccessibilityLiveRegion(n.role == Protocol.ROLE_ALERT
                ? View.ACCESSIBILITY_LIVE_REGION_ASSERTIVE : View.ACCESSIBILITY_LIVE_REGION_NONE);
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
            case Protocol.ROLE_RADIO_GROUP: className = "android.widget.RadioGroup"; break;
            case Protocol.ROLE_TAB_LIST: className = "android.widget.TabWidget"; break;
            case Protocol.ROLE_MENU: role = "menu"; break;
            case Protocol.ROLE_MENU_ITEM: role = "menu item"; break;
            case Protocol.ROLE_DIALOG: role = "dialog"; break;
            case Protocol.ROLE_ALERT: role = "alert"; break;
            default: break;
        }
        if (className != null) info.setClassName(className);
        // The key AndroidX and TalkBack read a role description under.
        if (role != null) info.getExtras().putCharSequence("AccessibilityNodeInfo.roleDescription", role);
        if (checkable) {
            info.setCheckable(true);
            info.setChecked(n.checked && !n.mixed);
            // The checked state has no mixed value to set, so describe it.
            if (n.mixed && Build.VERSION.SDK_INT >= 30) info.setStateDescription("partially checked");
        }
        info.setSelected(n.selected);
        if (!n.testID.isEmpty()) info.setViewIdResourceName(n.testID);
    }

    /**
     * A view node's container, whose children may draw outside it, as
     * without Clip. While its node follows gestures it takes the touches
     * its children leave, so the gestures see them, and while one of them
     * is active it takes its children's too, cancelling their presses.
     */
    private final class Group extends FrameLayout {
        Node node;

        Group(Context context) {
            super(context);
            setClipChildren(false);
        }

        @Override
        public boolean dispatchTouchEvent(MotionEvent e) {
            if (node != null && node.followsGestures()) {
                int action = e.getActionMasked();
                if (action == MotionEvent.ACTION_DOWN) gestures.candidate(node);
                // An ancestor, such as a scroll view, took the touch.
                else if (action == MotionEvent.ACTION_CANCEL) gestures.lost(node, true);
            }
            return super.dispatchTouchEvent(e);
        }

        @Override
        public void requestDisallowInterceptTouchEvent(boolean disallow) {
            // A descendant, such as a scroll view, took the touch first.
            if (disallow && node != null && !gestures.claiming) gestures.lost(node, false);
            super.requestDisallowInterceptTouchEvent(disallow);
        }

        @Override
        public boolean onInterceptTouchEvent(MotionEvent e) {
            return gestures.owns(node) || super.onInterceptTouchEvent(e);
        }

        @Override
        public boolean onTouchEvent(MotionEvent e) {
            if (gestures.owns(node)) {
                // The gesture took the press: end the view's own once.
                if (gestures.takePress(node)) {
                    MotionEvent cancel = MotionEvent.obtain(e);
                    cancel.setAction(MotionEvent.ACTION_CANCEL);
                    super.onTouchEvent(cancel);
                    cancel.recycle();
                }
                return true;
            }
            boolean handled = super.onTouchEvent(e);
            return handled || node != null && node.followsGestures();
        }
    }

    /**
     * Recognizes the pan, pinch and rotation of one touch from the root's
     * motion events, all pointers in its coordinates, and reports them.
     * The innermost view under the first finger that follows each kind is
     * its target. A pan activates as its view's PanActivation decides
     * (core.PanActivation.Decide), on the move before the views below see
     * it; until then a scroll view that takes the touch fails it. An
     * active gesture keeps the scroll views around it from taking the
     * touch and takes it from the views inside its own.
     */
    private final class Gestures implements ScaleGestureDetector.OnScaleGestureListener {
        /** The distance a pan without active offsets moves before it activates, in points. */
        private static final float PAN_SLOP = 10;
        /** The angle a rotation turns before it begins, in radians. */
        private static final float ROTATION_SLOP = (float) (Math.PI / 36);

        /** Set while a gesture disallows its ancestors' interception, which is no other claim. */
        boolean claiming;
        private boolean collecting;
        private final ArrayList<Node> candidates = new ArrayList<>();
        private final ScaleGestureDetector scaler;
        private VelocityTracker velocity;
        private boolean cancelling;

        // The pan: its node, its pointer, where that went down, its last
        // translation in points, its state (0 undecided, 1 active, -1
        // failed or over) and whether it took the view's own press.
        private Node pan;
        private int panPointer;
        private float downX, downY, panX, panY;
        private int panState;
        private boolean panTookPress;

        // The pinch: its node, whether it is in progress, its scale and velocity.
        private Node pinch;
        private boolean pinching, pinchTookPress;
        private float scale = 1, scaleVelocity;

        // The rotation: its node, its two pointers (-1: none), its state as
        // a pan's, its angle so far, the last angle between the pointers,
        // its velocity and the time of the last move.
        private Node rotation;
        private int first = -1, second = -1;
        private int rotationState;
        private boolean rotationTookPress;
        private float angle, lastAngle, angleVelocity;
        private long angleTime;

        Gestures(Context context) {
            scaler = new ScaleGestureDetector(context, this);
            scaler.setQuickScaleEnabled(false);
        }

        /** Forgets the last touch as a new one starts. */
        void reset() {
            candidates.clear();
            collecting = true;
            pan = pinch = rotation = null;
            panState = rotationState = 0;
            panTookPress = pinchTookPress = rotationTookPress = false;
            pinching = false;
            first = second = -1;
            if (velocity != null) {
                velocity.recycle();
                velocity = null;
            }
        }

        /** Notes a gesture view the new touch went down on, outermost first. */
        void candidate(Node n) {
            if (collecting) candidates.add(n);
        }

        /** Takes the targets of the touch that went down. */
        void down(MotionEvent e) {
            collecting = false;
            for (Node n : candidates) {
                if (n.has(Protocol.EVENT_PAN)) pan = n;
                if (n.has(Protocol.EVENT_PINCH)) pinch = n;
                if (n.has(Protocol.EVENT_ROTATION)) rotation = n;
            }
            candidates.clear();
            if (pan == null && pinch == null && rotation == null) return;
            panPointer = e.getPointerId(0);
            downX = e.getX(0);
            downY = e.getY(0);
            panX = panY = 0;
            velocity = VelocityTracker.obtain();
            velocity.addMovement(e);
            if (pinch != null) scaler.onTouchEvent(e);
        }

        /** Follows a touch after it went down, before the views below see it. */
        void track(MotionEvent e) {
            if (velocity == null) return; // no gesture views under it
            int action = e.getActionMasked();
            cancelling = action == MotionEvent.ACTION_CANCEL;
            velocity.addMovement(e);
            trackPan(e, action);
            trackRotation(e, action);
            if (pinch != null) scaler.onTouchEvent(e);
            if (action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL) {
                velocity.recycle();
                velocity = null;
                pan = pinch = rotation = null;
            }
        }

        private void trackPan(MotionEvent e, int action) {
            if (pan == null || panState < 0) return;
            int i = e.findPointerIndex(panPointer);
            boolean lifted = action == MotionEvent.ACTION_UP
                    || action == MotionEvent.ACTION_POINTER_UP && e.getPointerId(e.getActionIndex()) == panPointer;
            if (i >= 0 && action != MotionEvent.ACTION_CANCEL) {
                panX = (e.getX(i) - downX) / density;
                panY = (e.getY(i) - downY) / density;
            }
            if (i < 0 || action == MotionEvent.ACTION_CANCEL || lifted) {
                if (panState > 0) panEvent(lifted ? Protocol.GESTURE_ENDED : Protocol.GESTURE_CANCELLED);
                panState = -1;
                return;
            }
            if (action != MotionEvent.ACTION_MOVE) return;
            if (panState > 0) {
                panEvent(Protocol.GESTURE_CHANGED);
                return;
            }
            int decision = decide(pan, panX, panY);
            if (decision < 0) {
                panState = -1;
            } else if (decision > 0) {
                panState = 1;
                claim(pan);
                panEvent(Protocol.GESTURE_BEGAN);
            }
        }

        private void panEvent(int phase) {
            float vx = 0, vy = 0;
            if (velocity != null) {
                velocity.computeCurrentVelocity(1000);
                vx = velocity.getXVelocity(panPointer) / density;
                vy = velocity.getYVelocity(panPointer) / density;
            }
            gesture(pan, Protocol.EVENT_PAN, phase, panX, panY, vx, vy, 0);
        }

        private void trackRotation(MotionEvent e, int action) {
            if (rotation == null || rotationState < 0) return;
            if (action == MotionEvent.ACTION_POINTER_DOWN && second < 0) {
                int index = e.getActionIndex();
                first = e.getPointerId(index == 0 ? 1 : 0);
                second = e.getPointerId(index);
                lastAngle = angleOf(e);
                angle = angleVelocity = 0;
                angleTime = e.getEventTime();
                return;
            }
            if (second < 0) return;
            int a = e.findPointerIndex(first), b = e.findPointerIndex(second);
            int up = action == MotionEvent.ACTION_POINTER_UP ? e.getPointerId(e.getActionIndex()) : -1;
            boolean lifted = action == MotionEvent.ACTION_UP || up == first || up == second;
            if (a < 0 || b < 0 || action == MotionEvent.ACTION_CANCEL || lifted) {
                if (rotationState > 0) {
                    rotationEvent(lifted ? Protocol.GESTURE_ENDED : Protocol.GESTURE_CANCELLED);
                    rotationState = 0; // another pair may rotate again
                    rotationTookPress = false;
                }
                first = second = -1;
                return;
            }
            if (action != MotionEvent.ACTION_MOVE) return;
            float now = angleOf(e);
            float delta = now - lastAngle;
            // The shorter way round, across the atan2 seam.
            if (delta > Math.PI) delta -= (float) (2 * Math.PI);
            else if (delta < -Math.PI) delta += (float) (2 * Math.PI);
            lastAngle = now;
            angle += delta;
            long time = e.getEventTime();
            if (time > angleTime) angleVelocity = delta * 1000f / (time - angleTime);
            angleTime = time;
            if (rotationState > 0) {
                rotationEvent(Protocol.GESTURE_CHANGED);
            } else if (Math.abs(angle) >= ROTATION_SLOP) {
                rotationState = 1;
                claim(rotation);
                rotationEvent(Protocol.GESTURE_BEGAN);
            }
        }

        /** The angle of the line from the first pointer to the second, clockwise on screen. */
        private float angleOf(MotionEvent e) {
            int a = e.findPointerIndex(first), b = e.findPointerIndex(second);
            return (float) Math.atan2(e.getY(b) - e.getY(a), e.getX(b) - e.getX(a));
        }

        private void rotationEvent(int phase) {
            gesture(rotation, Protocol.EVENT_ROTATION, phase, 0, 0, angleVelocity, 0, angle);
        }

        @Override
        public boolean onScaleBegin(ScaleGestureDetector d) {
            if (pinch == null) return false;
            pinching = true;
            scale = 1;
            scaleVelocity = 0;
            claim(pinch);
            gesture(pinch, Protocol.EVENT_PINCH, Protocol.GESTURE_BEGAN, 0, 0, 0, 0, scale);
            return true;
        }

        @Override
        public boolean onScale(ScaleGestureDetector d) {
            if (!pinching) return true;
            float next = scale * d.getScaleFactor();
            long dt = d.getTimeDelta();
            if (dt > 0) scaleVelocity = (next - scale) * 1000f / dt;
            scale = next;
            gesture(pinch, Protocol.EVENT_PINCH, Protocol.GESTURE_CHANGED, 0, 0, scaleVelocity, 0, scale);
            return true;
        }

        @Override
        public void onScaleEnd(ScaleGestureDetector d) {
            if (!pinching) return;
            pinching = false;
            pinchTookPress = false;
            int phase = cancelling ? Protocol.GESTURE_CANCELLED : Protocol.GESTURE_ENDED;
            gesture(pinch, Protocol.EVENT_PINCH, phase, 0, 0, scaleVelocity, 0, scale);
        }

        /**
         * Notes that something else took n's touch: its gestures that have
         * not begun fail, and with active, those in progress are cancelled.
         */
        void lost(Node n, boolean active) {
            if (pan == n && (panState == 0 || active && panState > 0)) {
                if (panState > 0) panEvent(Protocol.GESTURE_CANCELLED);
                panState = -1;
            }
            if (pinch == n && (!pinching || active)) {
                if (pinching) {
                    gesture(pinch, Protocol.EVENT_PINCH, Protocol.GESTURE_CANCELLED, 0, 0, scaleVelocity, 0, scale);
                }
                pinching = false;
                pinch = null;
            }
            if (rotation == n && (rotationState == 0 || active && rotationState > 0)) {
                if (rotationState > 0) rotationEvent(Protocol.GESTURE_CANCELLED);
                rotationState = -1;
            }
        }

        /** Whether an active gesture of n's holds the touch. */
        boolean owns(Node n) {
            return n != null && (pan == n && panState > 0 || pinch == n && pinching
                    || rotation == n && rotationState > 0);
        }

        /** Whether n's own press is still to cancel for the gesture holding the touch, once. */
        boolean takePress(Node n) {
            boolean took = panTookPress && pan == n || pinchTookPress && pinch == n
                    || rotationTookPress && rotation == n;
            if (took) return false;
            if (pan == n) panTookPress = true;
            if (pinch == n) pinchTookPress = true;
            if (rotation == n) rotationTookPress = true;
            return true;
        }

        /** Keeps the scroll views and gesture views around n from taking the touch. */
        private void claim(Node n) {
            ViewParent parent = n.view.getParent();
            if (parent == null) return;
            claiming = true;
            parent.requestDisallowInterceptTouchEvent(true);
            claiming = false;
        }

        private void gesture(Node n, int kind, int phase, float x, float y, float vx, float vy, float value) {
            if (n.has(kind)) event(kind, n.id, 0, x, y, "", 0, 0, 0, phase, vx, vy, value);
        }
    }

    /** Whether a drag of n translated by (dx, dy) points activates its pan (1), fails it (-1) or is undecided (0). */
    private static int decide(Node n, float dx, float dy) {
        if (past(dx, n.failX) || past(dy, n.failY)) return -1;
        if (n.activeX == 0 && n.activeY == 0) {
            return Math.abs(dx) >= Gestures.PAN_SLOP || Math.abs(dy) >= Gestures.PAN_SLOP ? 1 : 0;
        }
        return past(dx, n.activeX) || past(dy, n.activeY) ? 1 : 0;
    }

    private static boolean past(float v, float limit) {
        return limit != 0 && Math.abs(v) >= Math.abs(limit);
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

    /** A modal's layer: it takes the touches its children leave, so none reach the views it covers. */
    private static final class Layer extends FrameLayout {
        Layer(Context context) {
            super(context);
            setClipChildren(false);
        }

        @Override
        public boolean onTouchEvent(MotionEvent e) {
            super.onTouchEvent(e);
            return true;
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
        ImageNode image;
        CanvasNode canvas;
        /** The view's background, the look it shows and the transition moving it. */
        Box background;
        Look shown;
        ValueAnimator animator;
        /** A modal's animation, -1 for other nodes, and whether it was presented. */
        int modal = -1;
        boolean presented;
        long events;
        /** User edits of field, numbered as EventChangeText numbers them. */
        long edits;
        /** Set while a batch sets field's text, which is no user edit. */
        boolean applying;
        int role;
        boolean disabled, selected, checked, mixed;
        String label = "", text = "", testID = "";
        /** The pan's activation offsets in points, as in core.PanActivation; 0 is unset. */
        float activeX, activeY, failX, failY;

        Node(int id, View view) {
            this.id = id;
            this.view = view;
        }

        boolean has(int kind) {
            return (events & (1L << kind)) != 0;
        }

        /** Whether the node follows a pan, a pinch or a rotation. */
        boolean followsGestures() {
            return has(Protocol.EVENT_PAN) || has(Protocol.EVENT_PINCH) || has(Protocol.EVENT_ROTATION);
        }

        /** The view assistive technology sees: an input's text field, else the view. */
        View accessible() {
            return field != null ? field : view;
        }
    }
}
