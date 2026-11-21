package dev.oneo.nsgo;

import android.animation.ValueAnimator;
import android.app.Activity;
import android.app.AlertDialog;
import android.app.UiModeManager;
import android.content.BroadcastReceiver;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.ContextWrapper;
import android.content.DialogInterface;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.pm.PackageManager;
import android.content.res.AssetManager;
import android.content.res.ColorStateList;
import android.content.res.Configuration;
import android.database.ContentObserver;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.ColorFilter;
import android.graphics.Insets;
import android.graphics.Matrix;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.PixelFormat;
import android.graphics.Rect;
import android.graphics.RectF;
import android.graphics.drawable.Drawable;
import android.graphics.drawable.LayerDrawable;
import android.graphics.drawable.RippleDrawable;
import android.graphics.text.LineBreakConfig;
import android.hardware.biometrics.BiometricPrompt;
import android.hardware.biometrics.BiometricManager;
import android.hardware.display.DisplayManager;
import android.net.ConnectivityManager;
import android.net.NetworkCapabilities;
import android.net.LinkProperties;
import android.net.LocalSocket;
import android.net.Network;
import android.os.Build;
import android.os.StatFs;
import android.os.storage.StorageManager;
import android.os.CancellationSignal;
import android.os.Handler;
import android.os.Looper;
import android.os.PowerManager;
import android.provider.Settings;
import android.text.Editable;
import android.text.InputType;
import android.text.TextUtils;
import android.text.TextWatcher;
import android.util.DisplayMetrics;
import android.util.Log;
import android.util.SparseArray;
import android.util.TypedValue;
import android.view.Choreographer;
import android.view.Display;
import android.view.DisplayCutout;
import android.view.Gravity;
import android.view.InputDevice;
import android.view.KeyCharacterMap;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.ScaleGestureDetector;
import android.view.VelocityTracker;
import android.view.HapticFeedbackConstants;
import android.view.View;
import android.view.ViewGroup;
import android.view.ViewGroupOverlay;
import android.view.ViewConfiguration;
import android.view.ViewParent;
import android.view.ViewTreeObserver;
import android.view.Window;
import android.view.WindowManager;
import android.view.WindowInsets;
import android.view.WindowInsetsAnimation;
import android.view.WindowInsetsController;
import android.view.accessibility.AccessibilityManager;
import android.view.accessibility.AccessibilityNodeInfo;
import android.view.animation.DecelerateInterpolator;
import android.view.animation.LinearInterpolator;
import android.view.animation.PathInterpolator;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputMethodManager;
import android.webkit.CookieManager;
import android.webkit.WebStorage;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.HorizontalScrollView;
import android.window.OnBackInvokedCallback;
import android.window.OnBackInvokedDispatcher;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.io.InputStream;
import java.net.InetAddress;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashSet;
import java.util.HashMap;
import java.util.Map;
import java.util.Objects;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.FutureTask;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.function.IntConsumer;
import org.json.JSONException;
import org.json.JSONObject;

/**
 * A surface showing one NSGo app. It starts the app's Go executable
 * (libnsgo.so in the native library directory), serves its text engine on
 * a reader thread and mounts its batches on the UI thread, one Android view
 * per node placed at the frame the engine computed. Points are
 * density-independent pixels. It reports the environment as it changes,
 * consumes back while the app runs and decodes the images the app fetches.
 * It runs the app's animated values (Animator) and layout animations on
 * Choreographer frames, shows input accessory bars above the keyboard and
 * hosts native components' views (Embedded).
 * It needs API 28, and its activity should handle configuration changes
 * itself and draw edge to edge, as NsgoActivity does.
 */
public class NsgoView extends FrameLayout implements NsgoEngine.Listener {
    private final PlayUpdates playUpdates = new PlayUpdates();

    private static final String TAG = "nsgo";
    private static final int DESTRUCTIVE = 0xffe53935;

    /** Pixels per point; the reader thread reads it too. */
    private volatile float density;
    private final Handler ui = new Handler(Looper.getMainLooper());
    private final InputMethodManager ime;
    /** Mounted nodes by ID, on the UI thread. */
    private final SparseArray<Node> nodes = new SparseArray<>();
    /** The node each node's view is, for a group to find its children's hit slop and areas. */
    private final java.util.HashMap<View, Node> byView = new java.util.HashMap<>();
    /**
     * The first view, which the engine serves, for an overlay's view: null for the first view itself. The first view receives every message and hands an overlay's batches and images on to its view, whose events name its surface.
     */
    private final NsgoView primary;
    // The first view owns the engine's sounds and players, including its overlays'.
    private Audio audio;
    /** The surface this view shows, 0 for the first view's, else an overlay's (driver.SurfaceOverlay). */
    private final int surface;
    final CanvasImages<CanvasNode.Picture> canvasImages = new CanvasImages<>(this::requestCanvasImage, p -> p.bitmap.recycle(), ui::post);
    /** The first view's overlays over other apps, null on an overlay's view. */
    private final Overlays overlays;
    /** Canvas decodes can be removed from the queue when their cache entry is evicted. */
    private final Map<Integer, FutureTask<Void>> canvasDecodes = new HashMap<>();
    /** Decodes images off the UI thread, one at a time. */
    private final ThreadPoolExecutor decoder = new ThreadPoolExecutor(1, 1, 0, TimeUnit.MILLISECONDS, new LinkedBlockingQueue<>(), r -> {
        Thread t = new Thread(r, "nsgo-images");
        t.setDaemon(true);
        return t;
    });
    /** Runs filesystem requests from Go, a few at a time. */
    private final ExecutorService files = Executors.newFixedThreadPool(4, r -> {
        Thread t = new Thread(r, "nsgo-files");
        t.setDaemon(true);
        return t;
    });
    /** Nodes reporting presses outside them, on the UI thread. */
    private final ArrayList<Node> outside = new ArrayList<>();
    /** Mounted nodes with backdrop filters and the containers above them, which draw what is behind those nodes for them to blur, on the UI thread. */
    private final HashSet<Node> backdropNodes = new HashSet<>();
    private final HashSet<ViewGroup> backdropAncestors = new HashSet<>();
    private final Object backdropState = Backdrop.createState();
    private final Backdrop.Host backdropHost = new Backdrop.Host() {
        @Override
        public BackdropMath.Spec backdrop(View child) {
            Node n = byView.get(child);
            return n == null ? null : n.backdrop;
        }

        @Override
        public Box backdropBox(View child) {
            Node n = byView.get(child);
            return n == null ? null : n.background;
        }

        @Override
        public boolean hasBackdropDescendant(View child) {
            return child instanceof ViewGroup && backdropAncestors.contains(child);
        }

        @Override
        public void setInherited(View child, Object source) {
            Object state = backdropStateOf(child);
            if (state != null) {
                Backdrop.setInherited(state, source);
            } else if (child instanceof Scrolling.Scroller) {
                ((Scrolling.Scroller) child).setInheritedBackdrop(source, source == null ? null
                        : (content, from) -> setInherited(content, from == null ? null : Backdrop.forward(from, content)));
            }
        }
    };
    private final Backdrop.Callbacks backdropCallbacks = new Backdrop.Callbacks() {
        @Override public void superDraw(Canvas canvas) { NsgoView.super.draw(canvas); }
        @Override public boolean drawChild(Canvas canvas, View child, long drawingTime) {
            return NsgoView.this.drawChild(canvas, child, drawingTime);
        }
        @Override public void superOnDrawForeground(Canvas canvas) { NsgoView.super.onDrawForeground(canvas); }
    };
    private final int[] location = new int[2];
    private NsgoEngine engine;
    private final Set<Integer> displayHolds = new HashSet<>();
    private LocalSocket hostSocket;
    private volatile boolean hostStopped;
    /** The window's folds and hinges while the view is in one, null where the device reports none. */
    private Folds folds;
    private final Runnable foldsReported = this::foldsChanged;
    /** A report of the folds is waiting for the view's next layout (foldsChanged). */
    private boolean foldsPending;
    /** The app's start waits for the first report of the folds on a device that folds (Folds.waiting), so an app started half open builds its first tree with both regions. The library reports once the window has drawn its first frame, and until then the main thread may be too busy for a timer to mean anything: the start waits FOLDS_WAIT_MS from that frame, or FOLDS_WAIT_LONGEST_MS from the view's first size for a window that does not draw, then goes without the folds, which a RESIZE brings. */
    private boolean startHeld;
    private final Runnable startUnheld = this::releaseStart;
    private static final long FOLDS_WAIT_MS = 300, FOLDS_WAIT_LONGEST_MS = 3000;
    private final Discovery discovery;
    private Sensors sensors;
    private Permissions systemPermissions;
    private Gamepads gamepads;
    private boolean ownsEngine;
    private boolean attached;
    private boolean attachmentWanted = true;
    private final ViewAttachment attachment = new ViewAttachment();
    private boolean clearingNativeTree;
    /** Whether a service or overlay keeps the app running off screen after its activity ends, and whether the app quit, which keeps nothing. */
    private boolean keptOffScreen, quit;
    // The engine's flags the app starts with (--nsgo-fps).
    private final ArrayList<String> flags = new ArrayList<>();
    // The test nsgo test -target android runs, whether nsgo dev -automation drives the app, the agent, and the physical-input isolation gate.
    private String test;
    private boolean automation;
    private TestAgent agent;
    final TestInputGate inputGate = new TestInputGate();
    // The alerts shown, by their ID, for the test agent to answer.
    final SparseArray<TestAgent.Alert> alerts = new SparseArray<>();

    // The environment, on the UI thread: the insets in pixels and what was last sent.
    private final AccessibilityManager accessibility;
    private final UiModeManager uiMode;
    private final PowerManager power;
    private final BroadcastReceiver powerMode = new BroadcastReceiver() {
        @Override
        public void onReceive(Context context, Intent intent) {
            environment();
        }
    };
    // The thermal listener is an Object so versions before API 29 never resolve its class.
    private Object thermalListener;
    private final HapticPatterns hapticPatterns;
    private final AccessibilityManager.TouchExplorationStateChangeListener screenReader = on -> environment();
    /** The UiModeManager.ContrastChangeListener on API 34 and later: an Object, so older Androids, which have no such class, never resolve it. */
    private Object contrast;
    private final ContentObserver motion = new ContentObserver(ui) {
        @Override
        public void onChange(boolean self) {
            environment();
        }
    };
    private int safeTop, safeRight, safeBottom, safeLeft, keyboard;
    private byte[] environment;
    private DisplayManager displayManager;
    private final DisplayManager.DisplayListener displayListener = new DisplayManager.DisplayListener() {
        @Override public void onDisplayAdded(int id) {}
        @Override public void onDisplayRemoved(int id) {}
        @Override public void onDisplayChanged(int id) {
            Display display = getDisplay();
            if (display != null && display.getDisplayId() == id) environment();
        }
    };
    /** Whether new insets wait for the layout of their traversal, which sends them with a new size if they bring one (sendSize), else alone as the frame draws. */
    private boolean environmentHeld;
    private final ViewTreeObserver.OnPreDrawListener heldEnvironment = new ViewTreeObserver.OnPreDrawListener() {
        @Override
        public boolean onPreDraw() {
            releaseEnvironment();
            return true;
        }
    };
    private ConnectivityManager.NetworkCallback network;
    private int networkFlags;
    /**
     * The predictive back gesture, on the UI thread: whether one runs, its
     * progress, the screen following it and the one it uncovers, their
     * translations and visibility before it, how far off the screen is and
     * the animation settling it.
     */
    private boolean backGesture;
    private float backProgress, backBase, backBelowBase, backAt;
    private Node backScreen, backBelow;
    private int backBelowVisibility;
    private ValueAnimator backMove;
    /** The OnBackInvokedDispatcher and OnBackInvokedCallback of back on API 33 and later. */
    private Object backDispatcher, backCallback;
    /** The keymap's bindings, oldest first: each key with its flags (1 fires in text, 2 repeats). */
    private String[] keys = new String[0];
    private int[] keyFlags = new int[0];
    /** The keymap's binding IDs, which EventKey reports. */
    private int[] keyIds = new int[0];
    /** Set while a key Go did not consume re-enters the Activity's dispatch, so that no NSGo view asks Go about that same key again. UI thread only. */
    private static boolean deliveringHardwareContinuation;
    private final HardwareKeys hardwareKeys = new HardwareKeys(new HardwareKeys.Host() {
        @Override public int target() {
            if ((focusKeyUse() & Protocol.KEY_USE_TEXT) != 0) return -1;
            Node node = byView.get(findFocus());
            return node != null && node.has(Protocol.EVENT_KEY) ? node.id : 0;
        }
        @Override public boolean heldKeys() { return reportHeldKeys; }
        @Override public void send(int request, int target, HardwareKeys.Event event) {
            ByteArrayOutputStream bytes = new ByteArrayOutputStream();
            DataOutputStream out = new DataOutputStream(bytes);
            try {
                out.writeInt(surface);
                out.writeInt(target);
                out.writeInt(event.scan);
                out.writeInt(event.key);
                out.writeByte(event.modifiers | (!event.down ? 16 : 0) | (event.repeat ? 32 : 0));
                Protocol.writeString(out, event.name);
                // Appended, so that Go reads a report without it as before.
                if (request != 0) out.writeInt(request);
            } catch (IOException impossible) {
                throw new AssertionError(impossible);
            }
            write(Protocol.PHYSICAL_KEY, bytes.toByteArray());
        }
        @Override public void fallback(HardwareKeys.Event event) {
            KeyEvent key = (KeyEvent) event.nativeEvent;
            Activity activity = activity();
            // The key passed the test input gate as it arrived; its continuation may come after the scripted step that sent it.
            inputGate.enterScripted();
            try {
                if (surface == 0 && activity != null) {
                    deliveringHardwareContinuation = true;
                    try { activity.dispatchKeyEvent(key); }
                    finally { deliveringHardwareContinuation = false; }
                } else {
                    continueHardwareKey(key);
                }
            } finally {
                inputGate.leaveScripted();
            }
        }
        @Override public void schedule(Runnable timeout) { ui.postDelayed(timeout, 2000); }
        @Override public void unschedule(Runnable timeout) { ui.removeCallbacks(timeout); }
    });

    void whenKeysIdle(Runnable action) { hardwareKeys.whenIdle(action); }
    private final Set<Integer> consumedKeys = new HashSet<>();
    private int statusBarStyle;
    private boolean statusBarHidden, immersive, wasImmersive;
    /** The pan, pinch and rotation of the touch in progress, on the UI thread. */
    private final Gestures gestures;
    /** Mouse hover and button-down move tracker, on the UI thread. */
    private final Mouse mouse = new Mouse(new Mouse.Host() {
        @Override
        public void hitTest(float x, float y, List<Integer> under) {
            mouseHitTest(x, y, under);
        }

        @Override
        public void report(int id, int kind, float x, float y, int mods, int button) {
            reportMouse(id, kind, x, y, mods, button);
        }

        @Override
        public boolean contains(float x, float y) {
            return x >= 0 && y >= 0 && x < getWidth() && y < getHeight();
        }

        @Override
        public boolean hasListeners() {
            return listensMouse();
        }
    });
    private boolean hoverRefreshPending;
    private final Runnable hoverRefresh = () -> {
        hoverRefreshPending = false;
        mouse.refresh();
    };
    /** Tracks mouse wheels and touchpad scroll momentum for View.OnWheel, on the UI thread. */
    private final WheelScroll wheelScroll = new WheelScroll(this::emitWheel);
    private final Runnable wheelEnd = wheelScroll::end;
    /** The view the root node shows in, among the accessory bar. */
    private View rootView;
    private final Set<ShaderEffect> effects = new LinkedHashSet<>();
    private final ViewTreeObserver.OnPreDrawListener drawEffects = () -> {
        for (ShaderEffect effect : effects) effect.capture();
        return true;
    };
    private int rootID;

    // Animation, on the UI thread: the graph, whether a frame callback is
    // posted and the app waits for it, the nodes whose frames move or which appear, and the views
    // deleted views leave fading out in their parents' overlays.
    private final Animator animator = new Animator((id, value, finished) ->
            event(Protocol.EVENT_ANIMATION, 0, id, 0, 0, "", 0, 0, 0, 0, 0, 0, value, null, 0, 0, false, "",
                    finished));
    private final Choreographer.FrameCallback tick = this::tick;
    private boolean measuring;
    private long perfWork;
    private boolean perfDirty;
    private PerfPanel perfPanel;
    /** A new node's style and props, as the first batch encoded them (startDefault). */
    private byte[] defaultStyle, defaultProps;
    private boolean ticking, frameWanted;
    private float preferredFrameRate;
    private long lastFrameSent;
    private double requestedFramePeriod;
    private boolean geometryObserved;
    private long geometryVersion;
    private byte[] lastGeometry;
    private final android.view.ViewTreeObserver.OnPreDrawListener geometryDraw = () -> {
        reportGeometry();
        return true;
    };
    private final LinkedHashSet<Node> moving = new LinkedHashSet<>();
    private final ArrayList<Ghost> ghosts = new ArrayList<>();
    private final Set<View> ghostViews = new HashSet<>();
    // The layout animation of the batch being mounted: its phases, when it
    // was applied and the nodes it created.
    private LayoutTransition layoutCreate, layoutUpdate, layoutDelete;
    private long batchTime;
    /** The keyboard's last motion, from its WindowInsetsAnimation (API 30). */
    private final LayoutTransition.Keyboard keyboardMotion = new LayoutTransition.Keyboard();
    private final Set<Integer> created = new HashSet<>();

    // The input accessory bar: the input with focus, and the accessory
    // node shown above the keyboard.
    private Node focusedInput, accessoryShown;

    /** The channel to the embedding app, and its settings over the system's: the first view's. */
    private final Host host;
    /** Downloads the app's images, as its Go process asks. */
    private final Downloads downloads;
    private final AudioOutput audioOutputs;
    private java.util.Locale hostLocale;
    private float hostFontScale;
    private int hostAppearance;
    private File storage;

    /** The appearances setAppearance takes. */
    public static final int APPEARANCE_SYSTEM = 0, APPEARANCE_LIGHT = 1, APPEARANCE_DARK = 2;

    /** Answers the app's calls of a name (core.Scope.CallHost), on the UI thread. */
    public interface HostHandler {
        /** Answers data, as maps, lists and plain values, with reply: then or later, from any thread. */
        void call(Object data, HostReply reply);
    }

    /** The one answer to a call; a post's goes nowhere. */
    public interface HostReply {
        void resolve(Object result);

        void reject(String error);
    }

    /** Sees the URLs the app opens before the system, on the UI thread: true takes one. */
    public interface OpenUrlHandler {
        boolean openUrl(String url);
    }

    /** Hears each failure the app does not catch, on the UI thread. */
    public interface ErrorListener {
        void onError(String message);
    }

    public NsgoView(Context context) {
        this(context, (android.util.AttributeSet) null);
    }

    /** The constructor layouts inflate. */
    public NsgoView(Context context, android.util.AttributeSet attrs) {
        this(context, attrs, null, 0);
    }

    /** The view of overlay surface, in a window of its own, which first serves; Overlays makes it. */
    NsgoView(Context context, NsgoView first, int surface) {
        this(context, null, first, surface);
    }

    private NsgoView(Context context, android.util.AttributeSet attrs, NsgoView first, int surface) {
        super(context, attrs);
        primary = first;
        this.surface = surface;
        // Holds the window's focus while no app control does, as NSH's host does.
        setFocusable(true);
        setFocusableInTouchMode(true);
        setDescendantFocusability(FOCUS_BEFORE_DESCENDANTS);
        setDefaultFocusHighlightEnabled(false);
        setImportantForAccessibility(IMPORTANT_FOR_ACCESSIBILITY_NO);
        setClipChildren(false);
        setClipToPadding(false);
        if (first == null) {
            // Alarms a force stop cleared, which the boot receiver cannot set again.
            Notifications.rearm(context);
            host = new Host(this);
            downloads = new Downloads(this::write);
            audioOutputs = new AudioOutput(context.getApplicationContext(), this::write);
            overlays = new Overlays(this);
            discovery = new Discovery(context.getApplicationContext(), ui, this::write);
        } else {
            host = first.host;
            downloads = first.downloads;
            audioOutputs = first.audioOutputs;
            overlays = null;
            discovery = first.discovery;
            engine = first.engine;
        }
        density = context.getResources().getDisplayMetrics().density;
        ime = context.getSystemService(InputMethodManager.class);
        accessibility = context.getSystemService(AccessibilityManager.class);
        uiMode = context.getSystemService(UiModeManager.class);
        power = context.getSystemService(PowerManager.class);
        hapticPatterns = new HapticPatterns(context);
        gestures = new Gestures(context);
        setOnApplyWindowInsetsListener((v, insets) -> {
            insets(insets);
            // An overlay's usable area changes with the system bars and the cutout.
            if (primary != null) primary.overlays.fit(this);
            // A rotation's insets come before the size they go with, in the same traversal.
            holdEnvironment();
            updateAccessory();
            reportAllScreenInsets();
            return insets;
        });
        // The keyboard's curve, duration and start as it begins to move,
        // before its insets arrive: layout animations on its curve keep
        // pace with it however late their batch comes.
        if (Build.VERSION.SDK_INT >= 30) {
            setWindowInsetsAnimationCallback(new WindowInsetsAnimation.Callback(
                    WindowInsetsAnimation.Callback.DISPATCH_MODE_CONTINUE_ON_SUBTREE) {
                @Override
                public void onPrepare(WindowInsetsAnimation animation) {
                    if ((animation.getTypeMask() & WindowInsets.Type.ime()) == 0) return;
                    keyboardMotion.interpolator = animation.getInterpolator();
                    keyboardMotion.duration = animation.getDurationMillis() * 1_000_000;
                    keyboardMotion.start = System.nanoTime();
                }

                @Override
                public WindowInsets onProgress(WindowInsets insets, List<WindowInsetsAnimation> running) {
                    return insets;
                }
            });
        }
    }

    @Override
    public void draw(Canvas canvas) {
        Backdrop.drawContainer(this, canvas, backdropHost, backdropCallbacks, backdropState);
    }

    /** The state Backdrop keeps for a container that can draw what is behind a backdrop filter, null for other views and before Android 12. */
    private Object backdropStateOf(View view) {
        if (view == this) return backdropState;
        if (view instanceof Group) return ((Group) view).backdropState;
        if (view instanceof Layer) return ((Layer) view).backdropState;
        if (view instanceof Content) return ((Content) view).backdropState;
        if (view instanceof ScrollEdges) return ((ScrollEdges) view).backdropState;
        return null;
    }

    /** Sets n's backdrop filters from the batch's footer, null for none, redrawing the container under it. */
    private void setBackdrop(Node n, BackdropMath.Spec spec) {
        if (spec != null && !spec.active()) spec = null;
        if (spec == null ? n.backdrop == null : spec.sameAs(n.backdrop)) return;
        n.backdrop = spec;
        boolean had = backdropNodes.contains(n);
        if (spec != null) backdropNodes.add(n);
        else backdropNodes.remove(n);
        if (had != (spec != null)) updateBackdropAncestors();
        ViewParent p = n.view.getParent();
        if (p instanceof ViewGroup) ((ViewGroup) p).invalidate();
    }

    /** Rebuilds the set of containers holding a backdrop-filtered descendant, letting the new ones draw and releasing what those that no longer hold one kept. */
    private void updateBackdropAncestors() {
        HashSet<ViewGroup> prev = new HashSet<>(backdropAncestors);
        backdropAncestors.clear();
        for (Node n : backdropNodes) {
            ViewParent p = n.view.getParent();
            while (p instanceof ViewGroup) {
                ViewGroup g = (ViewGroup) p;
                backdropAncestors.add(g);
                if (g == this) break;
                p = g.getParent();
            }
        }
        for (ViewGroup g : backdropAncestors) {
            if (!prev.contains(g)) {
                // A container that draws nothing of its own skips draw(), where Backdrop records what is behind its children.
                if (backdropStateOf(g) != null) g.setWillNotDraw(false);
                g.invalidate();
            }
        }
        for (ViewGroup g : prev) {
            if (!backdropAncestors.contains(g)) {
                Backdrop.release(backdropStateOf(g));
                g.invalidate();
            }
        }
    }

    /** Releases the render nodes n's views keep for backdrop filters. */
    private void releaseBackdrops(Node n) {
        Backdrop.release(backdropStateOf(n.view));
        if (n.children != null && n.children != n.view) Backdrop.release(backdropStateOf(n.children));
    }

    /** Releases every render node kept for backdrop filters, as the view lets its tree go. */
    private void releaseAllBackdrops() {
        backdropNodes.clear();
        backdropAncestors.clear();
        Backdrop.release(backdropState);
        for (int i = 0; i < nodes.size(); i++) releaseBackdrops(nodes.valueAt(i));
    }

    @Override
    protected void onSizeChanged(int w, int h, int oldw, int oldh) {
        super.onSizeChanged(w, h, oldw, oldh);
        if (hostStopped) return;
        updateGestureRects();
        if (primary != null) {
            // An overlay's size goes with its environment.
            environment();
            return;
        }
        // The start sends the size the view has by then.
        if (startHeld) return;
        boolean first = false;
        if (!attached) {
            if (!attachmentWanted) return;
            if (engine == null) {
                engine = new NsgoEngine(getContext());
                ownsEngine = true;
            }
            // An engine whose process ended stays ended: a host decides whether to make another.
            if (engine.canStart()) {
                if (folds != null && folds.waiting()) {
                    startHeld = true;
                    postDelayed(startUnheld, FOLDS_WAIT_LONGEST_MS);
                    getViewTreeObserver().registerFrameCommitCallback(() -> {
                        if (!startHeld) return;
                        removeCallbacks(startUnheld);
                        postDelayed(startUnheld, FOLDS_WAIT_MS);
                    });
                    return;
                }
                start();
                first = true;
            } else if (engine.isAlive()) {
                attachRunningEngine();
            }
        }
        // A RESIZE carries the environment that changed with the size.
        if (first) environment();
        foldsPending = false;
        sendSize(first ? Protocol.START : Protocol.RESIZE);
    }

    @Override
    protected void onConfigurationChanged(Configuration config) {
        super.onConfigurationChanged(config);
        updateGestureRects();
        // Text prepared before a change of density keeps its pixel size
        // until the engine prepares it again.
        float d = getResources().getDisplayMetrics().density;
        if (d != density) {
            density = d;
            if (engine != null) {
                engine.text.setDensity(surface, d);
                if (primary == null && engine.isAlive()) sendSize(Protocol.RESIZE);
            }
        }
        // A rotation or a display change moves an overlay's usable area: its window moves back onto it before it reports its frame.
        if (primary != null) primary.overlays.fit(this);
        applyDirection();
        if (preferredFrameRate > 0) refreshRateHint();
        environment();
        if (primary == null) systemBars();
    }

    @Override
    protected void onAttachedToWindow() {
        super.onAttachedToWindow();
        updateGestureRects();
        if (preferredFrameRate > 0) refreshRateHint();
        if (!effects.isEmpty()) getViewTreeObserver().addOnPreDrawListener(drawEffects);
        displayManager = getContext().getSystemService(DisplayManager.class);
        if (displayManager != null) displayManager.registerDisplayListener(displayListener, ui);
        // Embedded hosts may already have a focused sibling; keep it there.
        if (getRootView().findFocus() == null) requestFocus();
        accessibility.addTouchExplorationStateChangeListener(screenReader);
        listenPower();
        if (Build.VERSION.SDK_INT >= 34 && uiMode != null && contrast == null) listenContrast();
        getContext().getContentResolver().registerContentObserver(
                Settings.Global.getUriFor(Settings.Global.ANIMATOR_DURATION_SCALE), false, motion);
        getContext().getContentResolver().registerContentObserver(
                Settings.System.getUriFor(Settings.System.TIME_12_24), false, motion);
        if (primary != null) return;
        // Back goes to the app only while it can answer, so not on a detached view or an ended engine.
        boolean answers = attached || engine == null || engine.canStart();
        if (Build.VERSION.SDK_INT >= 33 && answers) registerBack();
        systemBars();
        if (folds == null) {
            folds = Folds.listen(activity(), foldsReported);
            // The folds may have changed while the view was out of a window, and the report the library sends as it registers reaches no view.
            if (folds != null) foldsChanged();
        }
        if (gamepads == null) gamepads = new Gamepads(getContext(), payload -> write(Protocol.GAMEPAD, payload));
        gamepads.start(ui);
        // Play Games' sign-in, which says whether package cloud is available, is known only asynchronously: the probe reports it to package cloud and resends the environment with it.
        if (CloudStorage.isSdkPresent()) {
            CloudStorage.probeState(activity(), report -> {
                write(Protocol.CLOUD_STORAGE_REPORT, report);
                ui.post(this::environment);
            });
        }
        if (measuring) startTicking();
    }

    /** The app's lifecycle follows the window's focus and visibility; coming back to the front answers the overlay permission requests the setting held. */
    @Override
    public void onWindowFocusChanged(boolean hasFocus) {
        super.onWindowFocusChanged(hasFocus);
        if (!hasFocus) {
            hardwareKeys.cancel();
            activationKeys.clear();
        }
        environment();
        videoPictureInPicture.windowChanged();
        if (hasFocus && overlays != null) overlays.returned();
        if (hasFocus) (primary == null ? this : primary).playUpdates.resume(activity());
        // The user may have changed RECORD_AUDIO in the system's settings; the first view tells the app.
        if (primary == null) recordingState();
    }

    /** An overlay's window showing starts the service that keeps it, and one the system hides may have lost its grant. */
    @Override
    protected void onWindowVisibilityChanged(int visibility) {
        super.onWindowVisibilityChanged(visibility);
        if (videoPictureInPicture != null) videoPictureInPicture.windowChanged();
        if (visibility != View.VISIBLE && hapticPatterns != null) hapticPatterns.stop();
        environment();
        if (measuring && visibility == View.VISIBLE) startTicking();
        if (primary != null) primary.overlays.visibility(this, visibility);
    }

    @Override
    public void onVisibilityAggregated(boolean visible) {
        super.onVisibilityAggregated(visible);
        if (!visible) {
            pauseAudio();
            if (videoPictureInPicture != null && !videoPictureInPicture.isActive()) pauseVideos();
        }
    }

    private final VideoPictureInPicture videoPictureInPicture = new VideoPictureInPicture(this);

    /** Forward the hosting Activity's PiP callback, including in embedded hosts. */
    public void onPictureInPictureModeChanged(boolean active) {
        videoPictureInPicture.modeChanged(active);
    }

    /** Forward the hosting Activity's onUserLeaveHint for automatic PiP on Android 11 and earlier. */
    public void onUserLeaveHint() { videoPictureInPicture.userLeaving(); }

    /** Forward Android 15's early PiP transition callback to hide inline UI during the animation. */
    public void onPictureInPictureTransition(boolean entering) {
        if (entering) videoPictureInPicture.transitioning();
    }

    private boolean audioVisible() {
        return isAttachedToWindow() && isShown() && getWindowVisibility() == View.VISIBLE;
    }

    private void pauseVideos() {
        for (int i = 0; i < nodes.size(); i++) {
            Node node = nodes.valueAt(i);
            if (node.video != null) node.video.backgroundPause();
        }
    }

    private void pauseAudio() {
        Audio shared = (primary == null ? this : primary).audio;
        if (shared != null) shared.pauseSurface(surface);
    }

    @Override
    protected void onDetachedFromWindow() {
        hardwareKeys.cancel();
        pauseAudio();
        hapticPatterns.stop();
        getViewTreeObserver().removeOnPreDrawListener(drawEffects);
        for (ShaderEffect effect : effects) effect.detach();
        if (displayManager != null) {
            displayManager.unregisterDisplayListener(displayListener);
            displayManager = null;
        }
        ui.removeCallbacks(wheelEnd);
        wheelScroll.cancel(0);
        releaseEnvironment();
        // An app waiting for the folds starts as the view leaves its window, as it would have without them.
        releaseStart();
        accessibility.removeTouchExplorationStateChangeListener(screenReader);
        stopPower();
        if (contrast != null) stopContrast();
        getContext().getContentResolver().unregisterContentObserver(motion);
        unregisterBack();
        // A view out of its window keeps the geometry it last reported.
        if (folds != null) {
            folds.stop(foldsReported);
            folds = null;
        }
        if (gamepads != null) {
            gamepads.stop();
            gamepads = null;
        }
        foldsPending = false;
        removeCallbacks(hoverRefresh);
        hoverRefreshPending = false;
        mouse.reset();
        if (!ownsEngine && primary == null) detachFromEngine();
        super.onDetachedFromWindow();
    }

    /**
     * Shows a host-owned engine's app in this view, starting its process as the view first gets a size, or attaches this view again after detachFromEngine. Call on the UI thread.
     *
     * Until the surface protocol lands, an engine serves the one view that first attaches to it and a view shows one engine: this throws IllegalStateException for another view's engine, for a second engine, and for an engine whose process ended or was destroyed, which a host replaces with a new engine and a new view.
     */
    public void attachToEngine(NsgoEngine next) {
        if (next == null) throw new NullPointerException("engine");
        if (!next.isAlive() && !next.canStart()) throw new IllegalStateException("the engine's process has ended: make a new engine");
        attachmentWanted = true;
        if (engine != next) {
            if (engine != null) throw new IllegalStateException("this view already shows another engine's app");
            next.attach(this);
            engine = next;
            ownsEngine = false;
        }
        if (!attached && next.isAlive()) attachRunningEngine();
    }

    /**
     * Releases this view's native children and returns back to the system while its Go graph and process commands continue; the engine keeps answering text requests. Reattaching the same view reconstructs its current graph; queued view deltas from the old attachment are dropped. The engine still retains the view and its context until the process ends. The view detaches itself as it leaves its window.
     */
    public void detachFromEngine() {
        attachmentWanted = false;
        if (!attached) return;
        attached = false;
        attachment.detach();
        updateGestureRects();
        pauseAudio();
        unregisterBack();
        write(Protocol.SURFACE, java.nio.ByteBuffer.allocate(13).putInt(surface)
                .put((byte) Protocol.SURFACE_DETACH).putLong(0).array());
        clearNativeTree();
    }

    private void attachRunningEngine() {
        engine.text.registerSurface(surface, density);
        engine.attach(this);
        attached = true;
        attachmentWanted = true;
        long generation = attachment.attach();
        updateGestureRects();
        if (Build.VERSION.SDK_INT >= 33 && isAttachedToWindow() && backCallback == null) registerBack();
        host.start();
        if (attachment.waiting()) {
            sendSize(Protocol.RESIZE);
            write(Protocol.SURFACE, java.nio.ByteBuffer.allocate(13).putInt(surface)
                    .put((byte) Protocol.SURFACE_RESYNC).putLong(generation).array());
        }
    }

    /** Releases this surface's widgets while its graph and text service live on. */
    private void clearNativeTree() {
        clearingNativeTree = true;
        clearSelection();
        setKeepScreenOn(false);
        canvasImages.clear();
        layoutCreate = layoutUpdate = layoutDelete = null;
        ArrayList<Node> old = new ArrayList<>();
        for (int i = 0; i < nodes.size(); i++) old.add(nodes.valueAt(i));
        DataInputStream empty = new DataInputStream(new ByteArrayInputStream(new byte[0]));
        for (Node n : old) {
            try { apply(Protocol.ITEM_DELETE, n, empty, Collections.emptyMap()); }
            catch (IOException impossible) { throw new AssertionError(impossible); }
        }
        for (Ghost ghost : ghosts) ghost.overlay.remove(ghost.view);
        ghosts.clear();
        created.clear();
        touchViews.clear();
        rootView = null;
        removeAllViews();
        clearingNativeTree = false;
    }

    private boolean isDebuggable() {
        return (getContext().getApplicationInfo().flags
                & android.content.pm.ApplicationInfo.FLAG_DEBUGGABLE) != 0;
    }

    /**
     * Runs the test the launch intent names, as nsgo test -target android
     * starts it: its arguments and environment go to the app's process.
     * Only a debuggable build takes one, so no other app can set them.
     */
    void setTest(String encoded) {
        if (encoded != null && isDebuggable()) test = encoded;
        if (test != null) inputGate.setIsolated(true);
    }

    /**
     * Starts the test agent for nsgo dev -target android -automation so the
     * running app's agent can drive the UI over the pipe. Only a debuggable
     * build takes it, so release and non-debuggable builds never instantiate
     * TestAgent.
     */
    void setAutomation(boolean enabled) {
        if (enabled && isDebuggable()) automation = true;
    }

    /**
     * Takes the engine's flags from a launch intent's extras named nsgo-,
     * as nsgo dev passes them: a true boolean is a flag alone
     * (--ez nsgo-fps true is --nsgo-fps), anything else its value
     * (--es nsgo-lang de is --nsgo-lang=de).
     */
    void setFlags(android.os.Bundle extras) {
        flags.clear();
        if (extras == null) return;
        for (String key : extras.keySet()) {
            if (!key.startsWith("nsgo-")) continue;
            Object value = extras.get(key);
            if (Boolean.TRUE.equals(value)) flags.add("--" + key);
            else if (value != null && !(value instanceof Boolean)) flags.add("--" + key + "=" + value);
        }
    }

    /**
     * Sets the launch parameters the app reads (core.Scope.HostParams), as
     * maps, lists and plain values; before the app starts, as the view
     * first gets a size.
     */
    public void setParams(java.util.Map<String, ?> params) {
        host.params = params == null ? "" : Host.json(params);
    }

    /** Keeps the app's data, its storage included, in dir; before the app starts. The app's files directory by default. */
    public void setStorageDirectory(File dir) {
        storage = dir;
    }

    /** Has the app format for locale in place of the system's; null follows the system again. */
    public void setLocale(java.util.Locale locale) {
        hostLocale = locale;
        environment();
    }

    /** Sets the text size multiplier in place of the system's; 0 follows the system again. */
    public void setFontScale(float scale) {
        hostFontScale = scale;
        environment();
    }

    /**
     * Shows the app light or dark in place of the system's appearance
     * (APPEARANCE_LIGHT, APPEARANCE_DARK); APPEARANCE_SYSTEM follows the
     * system again. An appearance the app forces still wins.
     */
    public void setAppearance(int appearance) {
        hostAppearance = appearance;
        environment();
    }

    /**
     * Gives the app the values the host themes it with (core.Env.Token):
     * CSS colors and other strings, tokens in both appearances and
     * darkTokens over them while the app shows dark.
     */
    public void setTokens(java.util.Map<String, String> tokens, java.util.Map<String, String> darkTokens) {
        host.tokens(tokens, darkTokens);
    }

    /** Sends the app an event (core.Scope.OnHostEvent), kept until it has a listener. */
    public void send(String name, Object data) {
        host.event(name, data);
    }

    /** Answers the app's calls and posts of name; null removes the handler. */
    public void handle(String name, HostHandler handler) {
        host.handle(name, handler);
    }

    public void removeHandler(String name) {
        host.handle(name, null);
    }

    /** Sees the URLs the app opens before the system; null removes the handler. */
    public void setOpenUrlHandler(OpenUrlHandler handler) {
        host.openUrl(handler);
    }

    public void setErrorListener(ErrorListener listener) {
        host.errors = listener;
    }

    /**
     * Registers a component of the host's for this view's app, which shows
     * it with a Native element of name. Names under nsgo/ are NSGo's own.
     */
    public void registerComponent(String name, NsgoComponent.Factory factory) {
        if (name.startsWith("nsgo/")) {
            Log.w(TAG, "component names under nsgo/ are NSGo's own: " + name + " is not registered");
            return;
        }
        host.components.put(name, factory);
    }

    /** The component the host registered as name, or null. */
    Embedded hostComponent(String name, NsgoComponent.Events events) {
        NsgoComponent.Factory factory = host.components.get(name);
        if (factory == null) {
            Log.w(TAG, "no native component " + name + " is registered: it shows an empty box");
            return null;
        }
        NsgoComponent c = factory.create(getContext(), events);
        return c == null ? null : Embedded.hosted(c);
    }

    private void start() {
        if (hostSocket != null) {
            try {
                engine.start(hostSocket);
            } catch (IOException error) {
                throw new IllegalStateException("cannot attach the development process", error);
            }
            hostSocket = null;
        } else {
            startProcess();
        }
        // The first environment already carries the default network.
        watchNetwork();
        // The host's handlers and tokens come before the app starts.
        attachRunningEngine();
        write(Protocol.LIVE_REPORT, LiveUpdates.state(getContext()));
        if (livePressEarly != null) write(Protocol.LIVE_REPORT, livePressEarly);
        livePressEarly = null;
        recordingState();
        RecordingService.connected(engine);
    }

    void setHostSocket(LocalSocket socket) {
        hostSocket = socket;
    }

    /** Releases the previous engine's view before the Activity shows the new one. */
    void stopHostView() {
        hostStopped = true;
        startHeld = false;
        removeCallbacks(startUnheld);
        if (hostSocket != null) {
            AndroidHostDev.closeSocket(hostSocket);
            hostSocket = null;
        }
        stop();
        destroyNodes();
    }

    private void startProcess() {
        File exe = new File(getContext().getApplicationInfo().nativeLibraryDir, "libnsgo.so");
        ProcessBuilder builder = new ProcessBuilder(exe.getAbsolutePath());
        builder.command().addAll(flags);
        File data = storage != null ? storage : getContext().getFilesDir();
        builder.environment().put("NSGO_DATA_DIR", data.getAbsolutePath());
        AppWidgets.prepare(getContext(), builder);
        builder.environment().put("NSGO_CACHE_DIR", getContext().getCacheDir().getAbsolutePath());
        if (!host.params.isEmpty()) builder.environment().put("NSGO_HOST_PARAMS", host.params);
        String installer = installer();
        if (installer != null) builder.environment().put("NSGO_INSTALLER", installer);
        if (test != null || automation) {
            agent = new TestAgent(this);
            if (test != null) TestAgent.apply(test, builder);
        }
        try {
            engine.start(builder);
        } catch (IOException e) {
            throw new IllegalStateException("cannot start " + exe, e);
        }
    }

    /**
     * Keeps a standalone engine running after its activity ends while an overlay, background location stream or recording is active. The view detaches until the next activity takes it back or the last keeper ends.
     */
    boolean keepOffScreen() {
        if (!ownsEngine || quit || engine == null || !engine.isAlive() || !keeps()) return false;
        detachFromEngine();
        keptOffScreen = true;
        return true;
    }

    /** Whether an overlay or a foreground service keeps the app's process running without an activity. */
    private boolean keeps() {
        return (overlays != null && overlays.any()) || (sensors != null && sensors.hasBackground())
                || RecordingService.active(engine);
    }

    /** Whether keepOffScreen kept the app's process, which still runs. */
    boolean keptOffScreen() {
        return keptOffScreen && engine != null && engine.isAlive();
    }

    /** The Go app's request, which follows a kept view to its next launcher activity. */
    private int orientation;

    private void applyOrientation() {
        if (primary == null && activity() instanceof NsgoActivity) {
            ((NsgoActivity) activity()).requestOrientation(orientation);
        }
    }

    /** Shows the app keepOffScreen kept again, in the activity whose context this view's now wraps, before it goes into that activity's window: its appearance and direction restyle that window. */
    void showAgain() {
        keptOffScreen = false;
        themed = null;
        if (engine != null && engine.isAlive()) attachRunningEngine();
        applyDirection();
        applyForcedLanguage();
        applyOrientation();
    }

    /** Ends the app's process once its activity is gone and nothing keeps it running. */
    void keeperEnded() {
        if (keptOffScreen && !keeps()) stop();
    }

    @Override
    public void onRecordingEnded() {
        ui.post(this::keeperEnded);
    }

    /**
     * Ends the app's process and destroys its native components. On a view showing a host-owned engine it only detaches the view, as NsgoEngine.destroy ends that engine.
     */
    public void stop() {
        hapticPatterns.stop();
        if (engine != null && !ownsEngine) {
            detachFromEngine();
            return;
        }
        if (engine != null) engine.destroy();
        release();
    }

    /**
     * Lets go of what serves the app's process once it ends or the view stops, on the UI thread: the network callback, the decoders, back and the native components, as an ended app sends no deletes. It runs again harmlessly.
     */
    private void release() {
        if (audio != null) { audio.close(); audio = null; }
        AuthSession.dispose(this);
        hardwareKeys.cancel();
        measuring = false;
        if (perfPanel != null) {
            getOverlay().remove(perfPanel);
            perfPanel = null;
        }
        clearDisplayHolds();
        Speech.releaseView(this);
        if (sensors != null) { sensors.close(); sensors = null; }
        if (systemPermissions != null) systemPermissions.clear();
        if (audioOutputs != null) audioOutputs.closeAll();
        if (network != null) {
            getContext().getSystemService(ConnectivityManager.class).unregisterNetworkCallback(network);
            network = null;
        }
        canvasImages.close();
        decoder.shutdownNow();
        files.shutdownNow();
        // An ended process hears no answer, and a late grant starts nothing.
        recordingStarts.clear();
        recordingAsks.clear();
        attached = false;
        removeCallbacks(hoverRefresh);
        hoverRefreshPending = false;
        mouse.reset();
        gestureRects = Collections.emptyList();
        updateGestureRects();
        keptOffScreen = false;
        unregisterBack();
        // Overlays are the system's windows, not the activity's: nothing else removes them as the process ends, and they would stay above every app, frozen, with the service's notification.
        if (overlays != null) overlays.closeAll();
        discovery.closeSurface(surface);
        releaseAllBackdrops();
        destroyComponents();
    }

    private void clearDisplayHolds() {
        if (displayHolds.isEmpty()) return;
        displayHolds.clear();
        setKeepScreenOn(false);
    }

    /**
     * Lets go of an overlay's view as its window closes, on the UI thread: its decoders and every node's web view, video and native component, as a closed tree deletes none of its views.
     */
    void releaseOverlay() {
        discovery.closeSurface(surface);
        removeCallbacks(hoverRefresh);
        hoverRefreshPending = false;
        mouse.reset();
        ui.removeCallbacks(wheelEnd);
        wheelScroll.cancel(0);
        gestureRects = Collections.emptyList();
        updateGestureRects();
        clearDisplayHolds();
        if (primary.audio != null) primary.audio.closeSurface(surface);
        canvasImages.close();
        decoder.shutdownNow();
        files.shutdownNow();
        destroyNodes();
    }

    private void destroyNodes() {
        releaseAllBackdrops();
        for (int i = 0; i < nodes.size(); i++) {
            Node n = nodes.valueAt(i);
            if (engine != null) engine.text.hide(surface, n.laid);
            if (n.animator != null) n.animator.cancel();
            if (n.keyframer != null) n.keyframer.cancel();
            if (n.web != null) n.web.destroy();
            if (n.video != null) n.video.destroy();
            if (n.gpu != null) n.gpu.destroy();
            if (n.embedded != null) n.embedded.destroy();
            n.embedded = null;
        }
        if (engine != null) engine.text.removeSurface(surface);
        nodes.clear();
        byView.clear();
        outside.clear();
        moving.clear();
    }

    /** The surface this view shows. */
    int surface() {
        return surface;
    }

    /** Destroys the native components still showing, on the UI thread. */
    private void destroyComponents() {
        for (int i = 0; i < nodes.size(); i++) {
            Node n = nodes.valueAt(i);
            if (n.gpu != null) {
                n.gpu.destroy();
                n.gpu = null;
            }
            if (n.embedded != null) {
                n.embedded.destroy();
                n.embedded = null;
            }
        }
    }

    /** Pixel frames of the next batch. Only the pipe reader touches them until it posts the whole batch to the UI thread. */
    private SparseArray<Map<String, PixelFrame>> pendingPixels;

    @Override
    public void onMessage(int kind, byte[] payload) {
        if (hostStopped) return;
        DataInputStream message = new DataInputStream(new ByteArrayInputStream(payload));
        try {
            switch (kind) {
                case Protocol.BATCH: {
                    // Each batch names its surface: an overlay's goes to its view.
                    int to = message.readInt();
                    boolean control = message.readInt() == 0;
                    long generation = attachment.token();
                    SparseArray<Map<String, PixelFrame>> pixels = pendingPixels;
                    pendingPixels = null;
                    ui.post(() -> {
                        if (hostStopped) return;
                        NsgoView view = to == 0 ? this : overlays.view(to);
                        if (view != null && (view.primary != null || control || attachment.permits(generation))) view.mount(payload, pixels);
                    });
                    break;
                }
                case Protocol.PIXEL_FRAME: {
                    int item = message.readInt();
                    String key = Protocol.readString(message);
                    PixelFrame frame = PixelFrame.read(message, payload);
                    if (pendingPixels == null) pendingPixels = new SparseArray<>();
                    Map<String, PixelFrame> frames = pendingPixels.get(item);
                    if (frames == null) {
                        frames = new HashMap<>();
                        pendingPixels.put(item, frames);
                    }
                    frames.put(key, frame);
                    break;
                }
                case Protocol.IMAGE: {
                    int to = message.readInt();
                    if (to == 0) {
                        image(message);
                        break;
                    }
                    // The overlays are the UI thread's: an overlay's image finds its view there, unless it closed meanwhile.
                    ui.post(() -> {
                        NsgoView view = overlays.view(to);
                        if (view == null) return;
                        try {
                            view.image(message);
                        } catch (IOException e) {
                            Log.w(TAG, "invalid message from app", e);
                        }
                    });
                    break;
                }
                case Protocol.CANVAS_IMAGE: {
                    int to = message.readInt();
                    ui.post(() -> {
                        NsgoView view = to == 0 ? this : overlays.view(to);
                        if (view == null) return;
                        try { view.canvasImage(message); }
                        catch (IOException e) { Log.w(TAG, "canvas image: " + e); }
                    });
                    break;
                }
                case Protocol.FONTS:
                    fonts(message);
                    break;
                case Protocol.FONT_DATA:
                    fontData(message);
                    break;
                case Protocol.TEST:
                    if (agent != null) agent.handle(message);
                    break;
                case Protocol.HOST_CALL:
                    host.message(message);
                    break;
                case Protocol.FOLDER:
                    files.execute(() -> folder(message));
                    break;
                case Protocol.DISK_SPACE:
                    files.execute(() -> diskSpace(message));
                    break;
                case Protocol.SECRET:
                    Secrets.answer(getContext().getApplicationContext(), message, reply -> write(Protocol.SECRET_REPLY, reply));
                    break;
                case Protocol.FETCH:
                    downloads.message(message);
                    break;
                case Protocol.LIVE_UPDATE:
                    ui.post(() -> {
                        LiveUpdates.apply(getContext(), activity(), engine.liveActionToken(), payload);
                        write(Protocol.LIVE_REPORT, LiveUpdates.state(getContext()));
                    });
                    break;
                case Protocol.RECORDING:
                    recording(payload);
                    break;
                case Protocol.COOKIES:
                    cookies(message);
                    break;
                case Protocol.SCAN:
                    scan(message);
                    break;
                case Protocol.GAMEPAD_RUMBLE:
                    int gpId = message.readInt();
                    float gpLow = message.readFloat();
                    float gpHigh = message.readFloat();
                    int gpDurationMs = message.readInt();
                    ui.post(() -> {
                        if (gamepads != null) gamepads.rumble(gpId, gpLow, gpHigh, gpDurationMs);
                    });
                    break;
                case Protocol.WANT_FRAME: {
                    int id = payload.length == 0 ? 0 : message.readInt();
                    ui.post(() -> {
                        NsgoView view = id == 0 ? this : overlays.view(id);
                        if (view != null) {
                            view.frameWanted = true;
                            view.startTicking();
                        }
                    });
                    break;
                }
                case Protocol.PERF_PANEL:
                    if (payload.length == 1) {
                        boolean show = message.readBoolean();
                        ui.post(() -> startPerf(show));
                    } else {
                        PerfPanel.Reading reading = PerfPanel.read(message);
                        ui.post(() -> {
                            if (perfPanel != null) perfPanel.update(reading, getWidth(), safeTop, safeLeft, safeRight, density);
                        });
                    }
                    break;
                case Protocol.EXTENDED:
                    extended(message.readUnsignedByte(), message, payload);
                    break;
                case Protocol.SHADER_RELEASE: {
                    int[] ids = new int[message.readInt()];
                    for (int i = 0; i < ids.length; i++) ids[i] = message.readInt();
                    // After the batches that stopped drawing them.
                    ui.post(() -> ShaderNode.release(ids));
                    break;
                }
                default:
                    Log.w(TAG, "unknown message " + kind);
            }
        } catch (IOException e) {
            Log.w(TAG, "invalid message from app", e);
        }
    }

    /** Takes a message of an extended kind (Protocol.EXT_*), on the pipe's reader like onMessage: message is past its kind byte, and payload still holds it. Protocol.readMessage refuses a kind isExtended lacks. */
    private void extended(int kind, DataInputStream message, byte[] payload) throws IOException {
        switch (kind) {
            case Protocol.EXT_PLAY_UPDATE: {
                int from = message.readInt(), count = message.readInt();
                ui.post(() -> {
                    NsgoView view = from == surface ? this : overlays.view(from);
                    if (view == null) return;
                    playUpdates.check(view.activity(), (shown, error) -> {
                        try {
                            ByteArrayOutputStream bytes = new ByteArrayOutputStream();
                            DataOutputStream out = new DataOutputStream(bytes);
                            out.writeInt(from);
                            out.writeInt(count);
                            out.writeBoolean(shown);
                            Protocol.writeString(out, error);
                            write(Protocol.PLAY_UPDATE_REPLY, bytes.toByteArray());
                        } catch (IOException impossible) { throw new AssertionError(impossible); }
                    });
                });
                break;
            }

            case Protocol.EXT_KEY_REPLY: {
                int from = message.readInt(), request = message.readInt();
                boolean consumed = message.readUnsignedByte() != 0;
                ui.post(() -> {
                    NsgoView view = from == surface ? this : overlays == null ? null : overlays.view(from);
                    if (view != null) view.hardwareKeys.reply(request, consumed);
                });
                break;
            }

            case Protocol.EXT_SURFACE_RESET: {
                int to = message.readInt();
                long generation = message.readLong();
                if (to == surface && attachment.reset(generation)) {
                    ui.post(() -> {
                        if (attachment.permits(generation)) clearNativeTree();
                    });
                }
                break;
            }
            case Protocol.EXT_DISCOVERY: {
                int to = message.readInt();
                ui.post(() -> {
                    if (to == 0 || overlays.view(to) != null) discovery.request(to, message);
                });
                break;
            }
            case Protocol.EXT_AUDIO_FILE: {
                int from = message.readInt(), id = message.readInt();
                String path = Protocol.readString(message);
                String code = Protocol.readString(message), reason = Protocol.readString(message);
                ui.post(() -> { if (audio != null) audio.file(from, id, path, code, reason); });
                break;
            }
            case Protocol.EXT_ASSET_READ: {
                long id = message.readLong();
                String path = Protocol.readString(message);
                files.execute(() -> readAsset(id, path));
                break;
            }
            case Protocol.EXT_FRAME_RATE: {
                int id = message.readInt();
                float rate = message.readFloat();
                ui.post(() -> {
                    NsgoView view = id == 0 ? this : overlays.view(id);
                    if (view != null) view.preferFrameRate(rate);
                });
                break;
            }
            case Protocol.EXT_GAME_SERVICES:
                gameServicesUnavailable(payload);
                break;
            case Protocol.EXT_CLOUD_STORAGE:
                // The command's JSON follows the kind byte.
                CloudStorage.handle(activity(), Arrays.copyOfRange(payload, 1, payload.length), report -> write(Protocol.CLOUD_STORAGE_REPORT, report));
                break;
            case Protocol.EXT_GPU_FRAME: {
                int to = message.readInt();
                int viewId = message.readInt();
                // GpuView reads the frame from its start, past the extended kind byte.
                byte[] frame = Arrays.copyOfRange(payload, 1, payload.length);
                if (!GpuView.dispatch(to, viewId, frame)) {
                    ui.post(() -> GpuView.dispatch(to, viewId, frame));
                }
                break;
            }
            case Protocol.EXT_GEOMETRY: {
                int to = message.readInt();
                boolean on = message.readBoolean();
                ui.post(() -> {
                    NsgoView view = to == 0 ? this : overlays.view(to);
                    if (view != null) view.observeGeometry(on);
                });
                break;
            }
            case Protocol.EXT_AUDIO_OUTPUT:
                audioOutputs.message(message);
                break;
            default:
                Log.w(TAG, "unknown extended message " + kind);
        }
    }

    @Override
    public void onProcessEnded(IOException error) {
        pendingPixels = null;
        if (error != null) Log.i(TAG, "app process ended: " + error);
        // Leave back to the system from now on; the app's components, network callback and decoders go with it. The reader has passed on its last message, so nothing decodes after this.
        ui.post(this::release);
    }

    @Override
    public void onLog(String line) {
        Log.i(TAG, line);
    }

    /**
     * The package that installed the app, which Go's update checks leave a
     * store's app to (updates.go): null when the system does not say.
     */
    @SuppressWarnings("deprecation")
    private String installer() {
        PackageManager pm = getContext().getPackageManager();
        String app = getContext().getPackageName();
        try {
            if (Build.VERSION.SDK_INT >= 30) return pm.getInstallSourceInfo(app).getInstallingPackageName();
            return pm.getInstallerPackageName(app);
        } catch (Exception e) {
            return null;
        }
    }

    /**
     * Sends the environment if it changed: before START, then whenever the insets, the configuration, an accessibility setting or the window's focus and visibility change. New insets wait for the layout of their traversal (holdEnvironment), and a RESIZE carries what changed with the size, so the app never sees a safe area with a size it was not measured at. The keyboard takes the system's default duration. An overlay's view sends it with its size and place (SURFACE_STATE), the first of which attaches its tree, and takes the host's settings from the first view.
     */
    void environment() {
        byte[] payload = environmentPayload();
        if (payload == null) return;
        if (primary != null) {
            byte[] state = primary.overlays.state(this, payload);
            if (state == null || Arrays.equals(state, environment)) return;
            if (environment == null) engine.text.registerSurface(surface, density);
            environment = state;
            write(Protocol.SURFACE, state);
            return;
        }
        payload = environmentChange(payload);
        if (payload == null) return;
        write(Protocol.ENVIRONMENT, payload);
        // The app's forced appearance and the host's settings reach its overlays.
        overlays.environment();
    }

    /** The first view's environment payload if it differs from the one last sent, which it then counts as sent; null if it does not. */
    private byte[] environmentChange(byte[] payload) {
        if (payload == null || Arrays.equals(payload, environment)) return null;
        environment = payload;
        return payload;
    }

    /** Holds the environment until the traversal that applied new insets has laid the view out: a rotation's safe area comes before its size, and the two reach the app together. */
    private void holdEnvironment() {
        if (environmentHeld) return;
        environmentHeld = true;
        getViewTreeObserver().addOnPreDrawListener(heldEnvironment);
    }

    /** Sends the environment holdEnvironment held, if the layout's RESIZE did not carry it. */
    private void releaseEnvironment() {
        if (!environmentHeld) return;
        environmentHeld = false;
        getViewTreeObserver().removeOnPreDrawListener(heldEnvironment);
        environment();
    }

    /** The environment as ENVIRONMENT carries it, or null while no app runs. */
    private byte[] environmentPayload() {
        if (engine == null || !engine.isAlive()) return null;
        NsgoView first = primary != null ? primary : this;
        float hostFontScale = first.hostFontScale;
        java.util.Locale hostLocale = first.hostLocale;
        Configuration config = getResources().getConfiguration();
        boolean dark = dark();
        followAppearance(dark);
        // Android's reduce-motion setting turns animations off.
        boolean reduceMotion = Settings.Global.getFloat(getContext().getContentResolver(),
                Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f;
        boolean increaseContrast = Build.VERSION.SDK_INT >= 34 && uiMode != null && uiMode.getContrast() > 0f;
        boolean reader = accessibility.isTouchExplorationEnabled();
        boolean standalone = first.activity() instanceof NsgoActivity;
        boolean ownPin = hostLocale == null && standalone && AppLocale.isOwnPin(getContext());
        java.util.Locale locale = hostLocale != null ? hostLocale : AppLocale.userLocale(getContext(), config, standalone);
        boolean rtl = (hostLocale != null || ownPin)
                ? TextUtils.getLayoutDirectionFromLocale(locale) == View.LAYOUT_DIRECTION_RTL
                : config.getLayoutDirection() == View.LAYOUT_DIRECTION_RTL;
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeFloat(safeTop / density);
            d.writeFloat(safeRight / density);
            d.writeFloat(safeBottom / density);
            d.writeFloat(safeLeft / density);
            d.writeFloat(keyboard / density);
            d.writeFloat(hostFontScale > 0 ? hostFontScale : config.fontScale);
            d.writeFloat(density);
            // In the background while the window does not show, inactive
            // while it shows without the focus, as under a dialog of
            // another app or the notification shade.
            boolean background = getWindowVisibility() != View.VISIBLE;
            boolean inactive = !background && !hasWindowFocus();
            d.writeByte((dark ? 1 : 0) | (reduceMotion ? 2 : 0) | (reader ? 4 : 0) | (rtl ? 8 : 0)
                    | (increaseContrast ? 128 : 0)
                    | (inactive ? 16 : 0) | (background ? 32 : 0) | (Services.hasCamera(getContext()) ? 64 : 0));
            // The first byte's eight bits are taken: what else the device can do follows in a second.
            d.writeByte((Speaker.available(getContext()) ? 1 : 0) | (Scanner.available(getContext()) ? 2 : 0)
                    | (Services.canRequestReview(activity()) ? 4 : 0) | first.networkFlags
                    | (CloudStorage.signedIn ? 128 : 0));
            int idiom = config.smallestScreenWidthDp >= 600 ? Protocol.IDIOM_TABLET : Protocol.IDIOM_PHONE;
            if ((config.uiMode & Configuration.UI_MODE_TYPE_MASK) == Configuration.UI_MODE_TYPE_TELEVISION) {
                idiom = Protocol.IDIOM_TV;
            }
            d.writeByte(idiom);
            Protocol.writeString(d, locale.toLanguageTag());
            String deviceName = Settings.Global.getString(getContext().getContentResolver(), "device_name");
            Protocol.writeString(d, deviceName == null ? "" : deviceName);
            String manufacturer = Build.MANUFACTURER == null ? "" : Build.MANUFACTURER.trim();
            String model = Build.MODEL == null ? "" : Build.MODEL.trim();
            if (!manufacturer.isEmpty() && !model.isEmpty()
                    && !model.toLowerCase(java.util.Locale.ROOT).startsWith(manufacturer.toLowerCase(java.util.Locale.ROOT))) {
                model = manufacturer + " " + model;
            } else if (model.isEmpty()) {
                model = manufacturer;
            }
            Protocol.writeString(d, model);
            // The system header's heights, without and with a large title.
            d.writeFloat(Header.BAR);
            d.writeFloat(Header.LARGE_BAR);
            d.writeInt(Build.VERSION.SDK_INT);

            d.writeByte(power != null && Build.VERSION.SDK_INT >= 29 ? power.getCurrentThermalStatus() + 1 : 0);
            d.writeByte(power == null ? 0 : power.isPowerSaveMode() ? 2 : 1);
            java.util.Locale formatLocale = hostLocale != null ? hostLocale
                    : java.util.Locale.getDefault(java.util.Locale.Category.FORMAT);
            if (formatLocale == null) formatLocale = java.util.Locale.getDefault();
            if (formatLocale == null) formatLocale = locale;
            LocalePrefs.read(formatLocale, hostLocale != null ? null : locale,
                    android.text.format.DateFormat.is24HourFormat(getContext()),
                    measurementSystem(formatLocale)).write(d);
            // The second capability byte's bits are taken too: a third follows the locale settings, where readEnvironment reads it only if a runtime wrote it. Its bits say what this runtime reads and does: Protocol.CAP_EXTENDED_MESSAGES, Protocol.CAP_AUDIO, as Audio plays on every device, Protocol.CAP_SPEECH where the device recognizes speech, Protocol.CAP_DISCOVER_SERVICES and Protocol.CAP_FETCH_TO_FILE | Protocol.CAP_PLAY_UPDATES.
            int third = Protocol.CAP_EXTENDED_MESSAGES | Protocol.CAP_AUDIO | Protocol.CAP_DISCOVER_SERVICES | Protocol.CAP_FETCH_TO_FILE | Protocol.CAP_PLAY_UPDATES | (hapticPatterns.available ? Protocol.CAP_HAPTIC_PATTERNS : 0)
                    | (Speech.available(getContext()) ? Protocol.CAP_SPEECH : 0)
                    | (GpuView.available(getContext()) ? Protocol.CAP_GPU : 0);
            d.writeByte(third);
            writeDisplay(d);
            // Optional environment tail, after the display, for mobile.OpenedURL.
            Protocol.writeString(d, first.openedURL);
            // The third capability byte's bits are all taken: a fourth ends the environment, where readEnvironment reads it only if a runtime wrote it. Its bits: Protocol.CAP_EXTENDED_SENSORS, Protocol.CAP_AUTH_SESSION, Protocol.CAP_VIDEO_PIP and Protocol.CAP_CAPTURE.
            d.writeByte(Protocol.CAP_EXTENDED_SENSORS | Protocol.CAP_AUTH_SESSION | Protocol.CAP_VIDEO_PIP | Protocol.CAP_CAPTURE);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        return bytes.toByteArray();
    }


    private void listenPower() {
        if (power == null) return;
        // This receiver subscribes only to a system broadcast.
        getContext().registerReceiver(powerMode, new IntentFilter(PowerManager.ACTION_POWER_SAVE_MODE_CHANGED));
        if (Build.VERSION.SDK_INT >= 29) {
            PowerManager.OnThermalStatusChangedListener listener = state -> environment();
            power.addThermalStatusListener(getContext().getMainExecutor(), listener);
            thermalListener = listener;
        }
        environment();
    }

    private void stopPower() {
        if (power == null) return;
        getContext().unregisterReceiver(powerMode);
        if (Build.VERSION.SDK_INT >= 29 && thermalListener != null) {
            power.removeThermalStatusListener((PowerManager.OnThermalStatusChangedListener) thermalListener);
            thermalListener = null;
        }
    }

    /** ICU's measurement system for formatLocale when it names a region, else "". */
    private static String measurementSystem(java.util.Locale formatLocale) {
        if (Build.VERSION.SDK_INT < 28 || formatLocale == null || formatLocale.getCountry().isEmpty()) {
            return "";
        }
        try {
            android.icu.util.LocaleData.MeasurementSystem ms =
                    android.icu.util.LocaleData.getMeasurementSystem(android.icu.util.ULocale.forLocale(formatLocale));
            if (android.icu.util.LocaleData.MeasurementSystem.US.equals(ms)) return "us";
            if (android.icu.util.LocaleData.MeasurementSystem.UK.equals(ms)) return "uk";
            if (android.icu.util.LocaleData.MeasurementSystem.SI.equals(ms)) return "metric";
        } catch (Throwable ignored) {
        }
        return "";
    }

    private void writeDisplay(DataOutputStream d) throws IOException {
        Display display = getDisplay();
        d.writeBoolean(display != null);
        if (display == null) return;
        Display.Mode current = display.getMode();
        float rate = current.getRefreshRate(), maximum = rate;
        for (Display.Mode mode : display.getSupportedModes()) {
            if (mode.getPhysicalWidth() == current.getPhysicalWidth() && mode.getPhysicalHeight() == current.getPhysicalHeight()) {
                maximum = Math.max(maximum, mode.getRefreshRate());
            }
        }
        android.graphics.Point size = new android.graphics.Point();
        display.getRealSize(size);
        d.writeInt(display.getDisplayId());
        d.writeFloat(size.x / density);
        d.writeFloat(size.y / density);
        d.writeFloat(rate);
        d.writeFloat(maximum);
    }

    /** The appearance the app forces, as driver.Appearance; 0 follows the system's. */
    private int appearance;
    /**
     * The app's layout direction, as its language is written, or -1 before
     * Go sends it: the views, headers, dialogs and back gesture lay out in
     * it whatever the system's.
     */
    private int direction = -1;
    /** What TalkBack announces for the roles and states it has no word of its own for, by Protocol.SPOKEN_*, in the language the app shows; empty before Go sends them. */
    private String[] spoken = new String[0];
    /** The forced BCP 47 language tag sent by ITEM_DIRECTION ("" when following the user's). */
    private String forcedLanguage = "";
    private boolean hasForcedLanguage;

    /** The pick the photo picker answers. */
    private Services.Pick pick;

    /** The file pick or save the documents UI answers. */
    private Services.Documents documents;
    private final java.util.HashMap<Long, PendingSave> pendingSaves = new java.util.HashMap<>();
    private static final long SAVE_PREFLIGHT_TIMEOUT = 5000; // driver.SavePreflightTimeout
    private static final String SAVE_PREFLIGHT_TIMED_OUT = "the app did not approve the save before the deadline";

    private static final class PendingSave {
        final Services.Documents request;
        final android.net.Uri tree;
        final String destination;
        final long deadline;
        final Runnable timeout;

        PendingSave(Services.Documents request, android.net.Uri tree, Runnable timeout) {
            this.request = request;
            this.tree = tree;
            this.destination = tree.buildUpon().appendQueryParameter("name", request.saveName).build().toString();
            this.deadline = android.os.SystemClock.elapsedRealtime() + SAVE_PREFLIGHT_TIMEOUT;
            this.timeout = timeout;
        }
    }

    /** The photo the camera app takes, or that waits for the camera permission, and the file it writes. */
    private Services.Pick photo;
    private java.io.File photoFile;

    /** Dark as shown: the app's forced appearance, else the host's, else the system's; an overlay's, the first view's. */
    private boolean dark() {
        NsgoView first = primary != null ? primary : this;
        if (first.appearance != 0) return first.appearance == Protocol.APPEARANCE_DARK;
        if (first.hostAppearance != 0) return first.hostAppearance == APPEARANCE_DARK;
        int night = getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK;
        return night == Configuration.UI_MODE_NIGHT_YES;
    }

    /** The appearance the theme and the web views follow; null before the first. */
    private Boolean themed;

    /**
     * Gives the standalone activity's theme and the web views the
     * appearance shown, as it changes: a host's activity keeps its own.
     */
    private void followAppearance(boolean dark) {
        if (themed != null && themed == dark) return;
        themed = dark;
        if (activity() instanceof NsgoActivity) ((NsgoActivity) activity()).followAppearance(dark);
        NsgoView first = primary != null ? primary : this;
        boolean forced = first.appearance != 0 || first.hostAppearance != 0;
        for (int i = 0; i < nodes.size(); i++) {
            Node n = nodes.valueAt(i);
            if (n.web != null) n.web.appearance(dark, forced);
            if (n.header != null) n.header.colors(dark);
        }
    }

    /**
     * Hears the activity's results: the photo picker's images, the camera
     * app's photo, and the documents UI's files, folders and saves.
     */
    public void activityResult(int request, int result, android.content.Intent data) {
        (primary == null ? this : primary).playUpdates.activityResult(request);
        if (request == Services.PHOTO_REQUEST) {
            photoResult(result);
            return;
        }
        if (request == Services.FILES_REQUEST || request == Services.SAVE_REQUEST || request == Services.FOLDER_REQUEST) {
            documentsResult(request, result, data);
            return;
        }
        if (request != Services.PICK_REQUEST || pick == null) return;
        Services.Pick p = pick;
        pick = null;
        java.util.List<android.net.Uri> uris = result == Activity.RESULT_OK ? Services.picked(data, p)
                : new java.util.ArrayList<>();
        Context context = getContext().getApplicationContext();
        // Reading and encoding photos takes a while: off the UI thread.
        new Thread(() -> answerPick(p, Services.prepare(context, uris, p)), "nsgo-pick").start();
    }

    private void answerPick(Services.Pick p, String files) {
        answerPick(p, files, "");
    }

    private void answerPick(Services.Pick p, String files, String error) {
        event(Protocol.EVENT_PICK, 0, p.count, 0, 0, error, 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, files, false);
    }

    /**
     * Takes photo p: asks for the camera first where the app's manifest
     * asks for CAMERA, then starts the camera app, answering the photo it
     * replaces.
     */
    private void takePhoto(Services.Pick p) {
        if (photo != null) answerPick(photo, "");
        photo = null;
        Activity activity = activity();
        if (activity == null) {
            answerPick(p, "", "no activity shows the camera");
            return;
        }
        photo = p;
        if (Services.needsCameraPermission(activity)) {
            activity.requestPermissions(new String[] {Services.CAMERA_PERMISSION}, Services.CAMERA_PERMISSION_REQUEST);
            return;
        }
        startPhoto(activity);
    }

    private void startPhoto(Activity activity) {
        try {
            photoFile = Services.startPhoto(activity);
        } catch (java.io.IOException e) {
            Services.Pick p = photo;
            photo = null;
            answerPick(p, "", String.valueOf(e.getMessage()));
        }
    }

    /**
     * Hears the activity's permission prompts: the camera's, which a photo waits for, the notifications', the system permissions' and the microphone's, which a recording waits for. A host's activity passes its own here.
     */
    public void permissionResult(int request, String[] permissions, int[] results) {
        if (request == Permissions.PERMISSION_REQUEST) {
            if (systemPermissions != null) systemPermissions.permissionResult(permissions, results);
            return;
        }
        if (request == MEDIA_PERMISSION_REQUEST) {
            boolean allowed = results.length > 0;
            for (int r : results) allowed &= r == android.content.pm.PackageManager.PERMISSION_GRANTED;
            java.util.List<java.util.function.Consumer<Boolean>> answered = new java.util.ArrayList<>(mediaAsks);
            mediaAsks.clear();
            for (java.util.function.Consumer<Boolean> a : answered) a.accept(allowed);
            return;
        }
        if (request == RecordingService.PERMISSION_REQUEST) {
            // The prompt asked for POST_NOTIFICATIONS too where the recording's notification needed it, which is asked once, as Notifications asks it.
            if (java.util.Arrays.asList(permissions).contains(Notifications.PERMISSION)) Notifications.markAsked(getContext());
            recordingPermission(RecordingService.granted(getContext()));
            return;
        }
        if (request == Sensors.PERMISSION_REQUEST) { if (sensors != null) sensors.permissionResult(); return; }
        if (request == Speech.PERMISSION_REQUEST) {
            Speech.permissionResult(this, results.length > 0 && results[0] == android.content.pm.PackageManager.PERMISSION_GRANTED);
            return;
        }
        if (request == Scanner.PERMISSION_REQUEST) {
            if (scanner != null) scanner.permissionResult(results.length > 0 && results[0] == android.content.pm.PackageManager.PERMISSION_GRANTED);
            return;
        }
        if (request == Notifications.PERMISSION_REQUEST) {
            Notifications.markAsked(getContext());
            for (long count : permissionAsks) answerPermission(count);
            permissionAsks.clear();
            return;
        }
        if (request != Services.CAMERA_PERMISSION_REQUEST || photo == null) return;
        Activity activity = activity();
        boolean granted = results.length > 0 && results[0] == android.content.pm.PackageManager.PERMISSION_GRANTED;
        if (granted && activity != null) {
            startPhoto(activity);
            return;
        }
        Services.Pick p = photo;
        photo = null;
        answerPick(p, "", "camera access denied");
    }

    /** Prepares the photo the camera app wrote off the UI thread, then deletes its file. */
    private void photoResult(int result) {
        Services.Pick p = photo;
        java.io.File f = photoFile;
        photo = null;
        photoFile = null;
        if (p == null || f == null) return;
        if (result != Activity.RESULT_OK || f.length() == 0) {
            f.delete();
            answerPick(p, "");
            return;
        }
        Context context = getContext().getApplicationContext();
        new Thread(() -> {
            String files = "";
            try {
                java.util.List<android.net.Uri> uris = new java.util.ArrayList<>();
                uris.add(Services.photoURI(context, f));
                files = Services.prepare(context, uris, p);
            } catch (java.io.IOException e) {
                // Answered with nothing, as a photo that cannot be read.
            } finally {
                f.delete();
            }
            answerPick(p, files);
        }, "nsgo-photo").start();
    }

    /**
     * Copies the files picked into the cache, or the file saved to the
     * document created, off the UI thread; names the folder picked, which
     * Go reads through folder.
     */
    private void documentsResult(int request, int result, android.content.Intent data) {
        Services.Documents d = documents;
        if (d == null || d.request() != request) return;
        documents = null;
        android.net.Uri created = data != null ? data.getData() : null;
        if (result != Activity.RESULT_OK || (d.from != null || d.folder) && created == null) {
            answerDocuments(d, "", "", false);
            return;
        }
        Context context = getContext().getApplicationContext();
        if (d.saveName != null) {
            checkSaveDestination(d, created);
            return;
        }
        if (d.folder) {
            answerDocuments(d, Services.pickedTree(context, created), "", false);
            return;
        }
        java.util.List<android.net.Uri> uris = Services.picked(data, d.limit);
        if (d.from == null && uris.isEmpty()) {
            answerDocuments(d, "", "", false);
            return;
        }
        new Thread(() -> {
            try {
                if (d.from == null) answerDocuments(d, Services.copyPicked(context, uris), "", false);
                else {
                    Services.save(context, d.from, created);
                    answerDocuments(d, "", "", true);
                }
            } catch (java.io.IOException e) {
                answerDocuments(d, "", String.valueOf(e.getMessage()), false);
            }
        }, "nsgo-documents").start();
    }

    /** Answers a file pick with its files' lines, or a save with whether it saved; error says what failed. */
    private void answerDocuments(Services.Documents d, String files, String error, boolean saved) {
        event(Protocol.EVENT_FILES, 0, d.count, 0, 0, error, 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, files, saved);
    }

    private void checkSaveDestination(Services.Documents d, android.net.Uri tree) {
        PendingSave pending = new PendingSave(d, tree, () -> answerSavePreflight(d.count, false, SAVE_PREFLIGHT_TIMED_OUT));
        pendingSaves.put(d.count, pending);
        postDelayed(pending.timeout, SAVE_PREFLIGHT_TIMEOUT);
        event(Protocol.EVENT_SAVE_PREFLIGHT, 0, d.count, 0, 0, pending.destination, 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, "", false);
    }

    private void answerSavePreflight(long count, boolean approved, String reason) {
        PendingSave pending = pendingSaves.remove(count);
        if (pending == null) return;
        removeCallbacks(pending.timeout);
        if (android.os.SystemClock.elapsedRealtime() >= pending.deadline) {
            approved = false;
            reason = SAVE_PREFLIGHT_TIMED_OUT;
        }
        if (!approved) {
            answerDocuments(pending.request, pending.destination, reason, false);
            return;
        }
        Context context = getContext().getApplicationContext();
        new Thread(() -> {
            try {
                android.net.Uri saved = Services.createSave(context, pending.request, pending.tree);
                answerDocuments(pending.request, saved.toString(), "", true);
            } catch (java.io.IOException | RuntimeException e) {
                answerDocuments(pending.request, pending.destination, String.valueOf(e.getMessage()), false);
            }
        }, "nsgo-documents").start();
    }

    /** Starts d in the documents UI, answering the one it replaces, or answers d when no activity can. */
    private void startDocuments(Services.Documents d, java.util.function.Predicate<Activity> start) {
        if (documents != null && (documents.saveName != null || d.saveName != null)) {
            // Two live picker results share request codes. Keep the active
            // destination tied to the save that asked to check it.
            answerDocuments(d, "", "another document picker is open", false);
            return;
        }
        if (documents != null) answerDocuments(documents, "", "", false);
        documents = null;
        Activity activity = activity();
        if (activity != null && start.test(activity)) documents = d;
        else answerDocuments(d, "", "no activity shows documents", false);
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
        // shows, and the stable one never does. The decor view consumes
        // the stable insets it passes down, so they come from the window.
        WindowInsets root = getRootWindowInsets();
        int bottom = insets.getSystemWindowInsetBottom();
        int stable = (root != null ? root : insets).getStableInsetBottom();
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
     * Follows the default route's connectivity and DNS servers. Go outside
     * the runtime has no system resolver to ask. Both reports need
     * ACCESS_NETWORK_STATE.
     */
    private void watchNetwork() {
        ConnectivityManager connectivity = getContext().getSystemService(ConnectivityManager.class);
        if (connectivity == null) return;
        network = new ConnectivityManager.NetworkCallback() {
            @Override
            public void onCapabilitiesChanged(Network n, NetworkCapabilities caps) {
                changed(networkFlags(caps));
            }

            @Override
            public void onLost(Network n) {
                changed(networkFlags(null));
            }

            private void changed(int flags) {
                ui.post(() -> {
                    if (network != this || networkFlags == flags) return;
                    networkFlags = flags;
                    environment();
                });
            }

            @Override
            public void onLinkPropertiesChanged(Network n, LinkProperties properties) {
                dns(properties.getDnsServers());
            }
        };
        try {
            connectivity.registerDefaultNetworkCallback(network);
        } catch (SecurityException e) {
            Log.w(TAG, "the app cannot read its network state or DNS without ACCESS_NETWORK_STATE", e);
            network = null;
            return;
        }
        Network n = connectivity.getActiveNetwork();
        networkFlags = networkFlags(n == null ? null : connectivity.getNetworkCapabilities(n));
    }

    /** The known, online and expensive bits of the second environment flag byte. */
    private static int networkFlags(NetworkCapabilities caps) {
        boolean online = caps != null && caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
                && caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED);
        return 8 | (online ? 16 : 0)
                | (online && !caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED) ? 32 : 0);
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

    /**
     * Shows the bytes Go fetched for an image node's request: decoded on
     * the decoder thread at about the node's size, and dropped once the
     * node has moved on to another request.
     */
    /** Loads the app's font files from the assets as Typefaces of their families, before any text. */
    private void fonts(DataInputStream in) throws IOException {
        for (int i = 0, n = in.readInt(); i < n; i++) {
            String path = Protocol.readString(in), family = Protocol.readString(in);
            int weight = in.readUnsignedShort();
            boolean italic = in.readUnsignedByte() != 0;
            try {
                android.graphics.Typeface typeface = android.graphics.Typeface.createFromAsset(getContext().getAssets(), path);
                Object font = Text.assetFont(getContext().getAssets(), path, weight, italic);
                Text.register(family, weight, italic, typeface, font);
            } catch (RuntimeException e) {
                Log.w(TAG, "font " + path + ": " + e);
            }
        }
    }

    /**
     * Registers a font the app sends as it runs: Typeface reads files, so
     * its bytes go to the cache first, named by their hash.
     */
    private void fontData(DataInputStream in) throws IOException {
        String family = Protocol.readString(in);
        int weight = in.readUnsignedShort();
        boolean italic = in.readUnsignedByte() != 0;
        byte[] data = new byte[in.readInt()];
        in.readFully(data);
        try {
            java.io.File dir = new java.io.File(getContext().getCacheDir(), "nsgo-fonts");
            if (!dir.isDirectory() && !dir.mkdirs()) throw new IOException("cannot make " + dir);
            java.io.File file = new java.io.File(dir, Integer.toHexString(java.util.Arrays.hashCode(data)) + "-"
                    + data.length + ".font");
            if (!file.isFile()) {
                try (java.io.FileOutputStream out = new java.io.FileOutputStream(file)) {
                    out.write(data);
                }
            }
            android.graphics.Typeface typeface = android.graphics.Typeface.createFromFile(file);
            Object font = Text.fileFont(file, weight, italic);
            Text.register(family, weight, italic, typeface, font);
        } catch (IOException | RuntimeException e) {
            Log.w(TAG, "font " + family + ": " + e);
        }
    }

    /** Requests an evicted image again without waiting for another app commit. */
    private void requestCanvasImage(String key, int generation, boolean cancel) {
        if (cancel) {
            FutureTask<Void> task = canvasDecodes.remove(generation);
            if (task != null) {
                task.cancel(false);
                decoder.remove(task);
            }
        }
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream out = new DataOutputStream(bytes);
        try {
            out.writeInt(surface);
            Protocol.writeString(out, key);
            out.writeInt(generation);
            out.writeBoolean(cancel);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        write(Protocol.CANVAS_REQUEST, bytes.toByteArray());
    }

    /** Decodes only a current request; a late result cannot revive a closed cache. */
    private void canvasImage(DataInputStream in) throws IOException {
        String key = Protocol.readString(in);
        int generation = in.readInt();
        boolean unavailable = in.readBoolean();
        float scale = in.readFloat();
        byte[] data = new byte[in.readInt()];
        in.readFully(data);
        if (unavailable) {
            if (canvasImages.unavailable(key, generation)) redrawCanvases();
            return;
        }
        if (!canvasImages.wants(key, generation) || canvasDecodes.containsKey(generation)) return;
        FutureTask<Void> task = new FutureTask<>(() -> {
            if (!canvasImages.wants(key, generation)) return null;
            android.graphics.Bitmap bitmap = null;
            try {
                if (data.length != 0) bitmap = android.graphics.BitmapFactory.decodeByteArray(data, 0, data.length);
            } catch (RuntimeException | OutOfMemoryError e) {
                Log.w(TAG, "canvas: image " + key + ": " + e);
            }
            CanvasNode.Picture picture = bitmap == null ? null : new CanvasNode.Picture(bitmap, scale);
            ui.post(() -> {
                canvasDecodes.remove(generation);
                if (canvasImages.complete(key, generation, picture)) redrawCanvases();
            });
            return null;
        });
        canvasDecodes.put(generation, task);
        decoder.execute(task);
    }

    /** Answers ITEM_SPEAK count of this view's tree: an utterance's end, stopped or with error, one of driver's Speak errors, or the voices as JSON. */
    void answerSpeak(long count, boolean stopped, String error, String voices) {
        event(Protocol.EVENT_SPEAK, 0, count, 0, 0, error, 0, 0, stopped ? 1 : 0, 0, 0, 0, 0, null, 0, 0, false, voices, false);
    }

    /** Reports speech session count of this view's tree: its phase (Speech.STARTED, RESULT or ENDED), transcript text, finality and error. */
    void answerSpeech(long count, int phase, String text, boolean finalResult, String error) {
        event(Protocol.EVENT_SPEECH, 0, count, 0, 0, text, 0, 0, phase, 0, 0, 0, 0, null, 0, 0, false, error, finalResult);
    }

    /** Answers ITEM_OVERLAY_PERMISSION count of this view's tree with permission, an OVERLAY_* of Protocol's. */
    void answerOverlayPermission(long count, int permission) {
        event(Protocol.EVENT_OVERLAY_PERMISSION, 0, count, 0, 0, "", 0, 0, permission);
    }

    /** Answers ITEM_PERMISSION count of this view's tree with status (PERMISSION_*) or whether settings opened. */
    void answerSystemPermission(long count, int status, boolean opened) {
        event(Protocol.EVENT_PERMISSION, 0, count, 0, 0, "", 0, 0, status, 0, 0, 0, 0, null, 0, 0, false, "", opened);
    }

    /** Draws the canvases and shaders again, as an image they draw arrived. */
    void redrawCanvases() {
        for (int i = 0; i < nodes.size(); i++) {
            Node n = nodes.valueAt(i);
            if (n.canvas != null) n.canvas.invalidate();
            if (n.shader != null) n.shader.draw();
        }
    }

    private void image(DataInputStream in) throws IOException {
        int id = in.readInt(), request = in.readInt(), part = in.readUnsignedByte();
        float scale = in.readFloat();
        // The size EventLoad reports, as an SVG's in points; 0 for the bitmap's pixels.
        float reportWidth = in.readFloat(), reportHeight = in.readFloat();
        byte[] data = new byte[in.readInt()];
        in.readFully(data);
        ui.post(() -> {
            Node n = nodes.get(id);
            if (n == null || n.image == null || n.image.request != request || decoder.isShutdown()) return;
            ViewGroup.LayoutParams lp = n.view.getLayoutParams();
            DisplayMetrics screen = getResources().getDisplayMetrics();
            // Nearest-neighbor scaling needs the original raster pixels.
            boolean nearest = n.image.nearest;
            int width = nearest ? Integer.MAX_VALUE : (lp != null && lp.width > 0 ? lp.width : screen.widthPixels);
            int height = nearest ? Integer.MAX_VALUE : (lp != null && lp.height > 0 ? lp.height : screen.heightPixels);
            decoder.execute(() -> {
                ImageNode.Decoded image = ImageNode.decodeDrawable(getContext(), data, scale, width, height);
                ui.post(() -> {
                    if (nodes.get(id) != n || n.image.request != request) return;
                    if (part == Protocol.IMAGE_PLACEHOLDER) {
                        if (image != null) n.image.showPlaceholder(image.drawable);
                        return;
                    }
                    if (image == null) {
                        report(n, Protocol.EVENT_ERROR, 0, 0, "the image cannot be decoded");
                    } else {
                        n.image.showSource(image.drawable);
                        if (n.has(Protocol.EVENT_LOAD)) {
                            boolean given = reportWidth > 0 && reportHeight > 0;
                            event(Protocol.EVENT_LOAD, id, 0, 0, 0, "", given ? reportWidth : image.width,
                                    given ? reportHeight : image.height, 0);
                        }
                    }
                    report(n, Protocol.EVENT_LOAD_END);
                });
            });
        });
    }

    private void capture(int count, View target) {
        Activity activity = activity();
        SurfaceCapture.take(primary == null && activity != null ? activity.getWindow() : null, this, target, (png, error) -> {
            ByteArrayOutputStream bytes = new ByteArrayOutputStream();
            DataOutputStream out = new DataOutputStream(bytes);
            try {
                out.writeInt(surface);
                out.writeInt(count);
                Protocol.writeString(out, error);
                if (png != null) out.write(png);
                write(Protocol.CAPTURE_REPLY, bytes.toByteArray());
            } catch (IOException impossible) {
                throw new AssertionError(impossible);
            }
        });
    }

    void write(int kind, byte[] payload) {
        if (engine != null) engine.write(kind, payload);
    }

    private void diskSpace(DataInputStream in) {
        try {
            int from = in.readInt(), count = in.readInt();
            String path = Protocol.readString(in);
            long total = 0, available = 0;
            String error = "";
            try {
                StorageManager storage = getContext().getSystemService(StorageManager.class);
                available = storage.getAllocatableBytes(storage.getUuidForPath(new File(path)));
                total = new StatFs(path).getTotalBytes();
            } catch (IOException | RuntimeException failure) {
                error = failure.toString();
            }
            ByteArrayOutputStream bytes = new ByteArrayOutputStream();
            DataOutputStream out = new DataOutputStream(bytes);
            out.writeInt(from);
            out.writeInt(count);
            Protocol.writeString(out, error);
            out.writeLong(total);
            out.writeLong(available);
            write(Protocol.DISK_SPACE_REPLY, bytes.toByteArray());
        } catch (IOException failure) {
            Log.w(TAG, "disk space request", failure);
        }
    }

    /** Reads compressed and uncompressed APK assets off the pipe and UI threads. */
    private void readAsset(long id, String path) {
        // Keep a chunk small so input and text replies can use the same pipe between writes.
        byte[] chunk = new byte[64 * 1024];
        ByteArrayOutputStream bytes = new ByteArrayOutputStream(chunk.length + 9);
        DataOutputStream out = new DataOutputStream(bytes);
        int status = Protocol.ASSET_DONE;
        String error = "";
        try (InputStream in = getContext().getAssets().open(path, AssetManager.ACCESS_STREAMING)) {
            for (int size; (size = in.read(chunk)) != -1;) {
                bytes.reset();
                out.writeLong(id);
                out.writeByte(Protocol.ASSET_CHUNK);
                out.write(chunk, 0, size);
                write(Protocol.ASSET_REPLY, bytes.toByteArray());
            }
        } catch (FileNotFoundException e) {
            status = Protocol.ASSET_MISSING;
        } catch (IOException | SecurityException e) {
            status = Protocol.ASSET_FAILED;
            error = e.toString();
        }
        try {
            bytes.reset();
            out.writeLong(id);
            out.writeByte(status);
            if (status == Protocol.ASSET_FAILED) Protocol.writeString(out, error);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        write(Protocol.ASSET_REPLY, bytes.toByteArray());
    }
    /**
     * Answers a FOLDER message off the reader thread: lists a folder of a
     * tree the user picked or reads part of its file, as folders.go asks,
     * its answer after the request's number and an error, "" for none.
     */
    private void folder(DataInputStream in) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream out = new DataOutputStream(bytes);
        int id = 0;
        try {
            id = in.readInt();
            int op = in.readUnsignedByte();
            String tree = Protocol.readString(in), document = Protocol.readString(in);
            ByteArrayOutputStream body = new ByteArrayOutputStream();
            DataOutputStream b = new DataOutputStream(body);
            Context context = getContext().getApplicationContext();
            try {
                if (op == Protocol.FOLDER_LIST) {
                    Services.listFolder(context, tree, document, b);
                } else {
                    long off = in.readLong();
                    byte[] read = Services.readDocument(context, tree, document, off, in.readInt());
                    b.writeInt(read.length);
                    b.write(read);
                }
                out.writeInt(id);
                Protocol.writeString(out, "");
                body.writeTo(out);
            } catch (IOException | SecurityException e) {
                bytes.reset();
                out.writeInt(id);
                Protocol.writeString(out, String.valueOf(e.getMessage()));
            }
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        write(Protocol.FOLDER_REPLY, bytes.toByteArray());
    }

    /** Selects a tab through the same click listener as a tap. */
    void selectTestTab(int id, int index) {
        Node n = nodes.get(id);
        if (n == null || n.embedded == null || !(n.embedded.view instanceof TabBar bar)) {
            throw new IllegalArgumentException("view " + id + " has no native tab bar");
        }
        View item = bar.getChildAt(index);
        if (item == null || !item.performClick()) {
            throw new IllegalArgumentException("view " + id + " has no selectable tab at index " + index);
        }
    }

    /** Answers the test step id: an error, else the step's data. */
    void testReply(int id, String error, String data) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeInt(id);
            Protocol.writeString(d, error);
            Protocol.writeString(d, data);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        write(Protocol.TEST_REPLY, bytes.toByteArray());
    }

    float density() {
        return density;
    }

    /** A report of the window's folds is a resize to the size the view has, with the geometry measured at it, sent at the view's next layout: the library reports as a configuration is applied, before the traversal that gives the view the window's new size. */
    private void foldsChanged() {
        foldsPending = true;
        requestLayout();
    }

    @Override
    protected void onLayout(boolean changed, int left, int top, int right, int bottom) {
        super.onLayout(changed, left, top, right, bottom);
        // onSizeChanged has sent a new size with the folds already.
        if (!foldsPending) return;
        foldsPending = false;
        if (startHeld) {
            if (!folds.waiting()) releaseStart();
            return;
        }
        if (engine != null && engine.isAlive() && getWidth() > 0) sendSize(Protocol.RESIZE);
    }

    /** Starts the app onSizeChanged held for the folds, with the size the view has and the folds reported by now. */
    private void releaseStart() {
        if (!startHeld) return;
        startHeld = false;
        removeCallbacks(startUnheld);
        if (attached || !engine.canStart()) return;
        start();
        environment();
        foldsPending = false;
        sendSize(Protocol.START);
    }

    /**
     * Sends START or RESIZE: the view's size in points, then the folds and hinges measured at it (Folds), each a frame in the view's points, the extensions' type and state; none while the window has other bounds than the latest report was measured at. Frames are clipped to the view in pixels, so an edge the view cuts a feature at is the view's own edge in points to the bit, and one outside the view is left out. A RESIZE then carries the environment as ENVIRONMENT does if it changed, as a rotation's safe area does, so the app sees both in one change.
     */
    private void sendSize(int kind) {
        byte[] env = kind == Protocol.RESIZE ? environmentChange(environmentPayload()) : null;
        int[] f = folds != null ? folds.current() : new int[0];
        int[] at = new int[2];
        getLocationInWindow(at);
        int w = getWidth(), h = getHeight();
        int[] in = new int[f.length];
        int n = 0;
        for (int i = 0; i + 5 < f.length; i += 6) {
            int left = f[i] - at[0], top = f[i + 1] - at[1], right = f[i + 2] - at[0], bottom = f[i + 3] - at[1];
            if (right < 0 || bottom < 0 || left > w || top > h) continue;
            in[n++] = Math.max(left, 0);
            in[n++] = Math.max(top, 0);
            in[n++] = Math.min(right, w);
            in[n++] = Math.min(bottom, h);
            in[n++] = f[i + 4];
            in[n++] = f[i + 5];
        }
        ByteArrayOutputStream bytes = new ByteArrayOutputStream(12 + n / 6 * 18);
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            d.writeFloat(w / density);
            d.writeFloat(h / density);
            d.writeInt(n / 6);
            for (int i = 0; i < n; i += 6) {
                d.writeFloat(in[i] / density);
                d.writeFloat(in[i + 1] / density);
                d.writeFloat(in[i + 2] / density);
                d.writeFloat(in[i + 3] / density);
                d.writeByte(in[i + 4]);
                d.writeByte(in[i + 5]);
            }
            if (env != null) d.write(env);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        write(kind, bytes.toByteArray());
        if (env != null) overlays.environment();
    }

    private void event(int kind, int id, long count, float x, float y, String text, float width, float height,
            int index) {
        event(kind, id, count, x, y, text, width, height, index, 0, 0, 0, 0, null);
    }

    private void event(int kind, int id, long count, float x, float y, String text, float width, float height,
            int index, int phase, float vx, float vy, float value, WebNode.Navigation nav) {
        event(kind, id, count, x, y, text, width, height, index, phase, vx, vy, value, nav, 0, 0, false, "", false);
    }

    private void event(int kind, int id, long count, float x, float y, String text, float width, float height,
            int index, int phase, float vx, float vy, float value, WebNode.Navigation nav,
            long time, long duration, boolean buffering, String data, boolean finished) {
        event(kind, id, count, x, y, text, width, height, index, phase, vx, vy, value, nav, time, duration, buffering,
                data, finished, 0, 0, 0, 0, 0);
    }

    /**
     * Sends an event to the engine: its kind, node, count, point, text, size and index, then a gesture's phase, velocity and value, then a web view's navigation: flags of whether there is one, loading, canGoBack and canGoForward, and if there is, its URL and title; then a video's time and duration in milliseconds and whether it buffers; then a native view's callback arguments or a download as JSON, the URL of a message's page, or a web error's domain (with its code in index), and whether an animation finished; then a pointer's position in its root view, a pan's position in its view, the modifier keys (modifiers' bits) and the held mouse button (Protocol.MOUSE_BUTTON_*).
     */
    private void event(int kind, int id, long count, float x, float y, String text, float width, float height,
            int index, int phase, float vx, float vy, float value, WebNode.Navigation nav,
            long time, long duration, boolean buffering, String data, boolean finished,
            float pageX, float pageY, int mods) {
        event(kind, id, count, x, y, text, width, height, index, phase, vx, vy, value, nav, time, duration, buffering,
                data, finished, pageX, pageY, 0, 0, mods, Protocol.MOUSE_BUTTON_NONE);
    }

    private void event(int kind, int id, long count, float x, float y, String text, float width, float height,
            int index, int phase, float vx, float vy, float value, WebNode.Navigation nav,
            long time, long duration, boolean buffering, String data, boolean finished,
            float pageX, float pageY, float positionX, float positionY, int mods) {
        event(kind, id, count, x, y, text, width, height, index, phase, vx, vy, value, nav, time, duration, buffering,
                data, finished, pageX, pageY, positionX, positionY, mods, Protocol.MOUSE_BUTTON_NONE);
    }

    private void event(int kind, int id, long count, float x, float y, String text, float width, float height,
            int index, int phase, float vx, float vy, float value, WebNode.Navigation nav,
            long time, long duration, boolean buffering, String data, boolean finished,
            float pageX, float pageY, float positionX, float positionY, int mods, int button) {
        // Deleting native widgets must not blur or edit their live Go nodes.
        if (primary == null && id != 0 && (clearingNativeTree || !attachment.permits(attachment.token()))) return;
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream d = new DataOutputStream(bytes);
        try {
            // The surface first: an overlay's events go to its tree.
            d.writeInt(surface);
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
            if (nav == null) {
                d.writeByte(0);
            } else {
                d.writeByte(1 | (nav.loading ? 2 : 0) | (nav.canGoBack ? 4 : 0) | (nav.canGoForward ? 8 : 0));
                Protocol.writeString(d, nav.url);
                Protocol.writeString(d, nav.title);
            }
            d.writeLong(time);
            d.writeLong(duration);
            d.writeByte(buffering ? 1 : 0);
            Protocol.writeString(d, data);
            d.writeByte(finished ? 1 : 0);
            d.writeFloat(pageX);
            d.writeFloat(pageY);
            d.writeFloat(positionX);
            d.writeFloat(positionY);
            d.writeByte(mods);
            d.writeByte(button);
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
        if (nodes.get(n.id) == n && n.has(kind)) event(kind, n.id, 0, x, y, text);
    }

    private void report(Node n, int kind) {
        report(n, kind, 0, 0, "");
    }

    /** Sends a pointer event of kind about n at e if n reports that kind. */
    private void report(Node n, int kind, MotionEvent e) {
        if (!n.has(kind)) return;
        float[] page = page(n.view, e);
        event(kind, n.id, 0, e.getX() / density, e.getY() / density, "", 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, "",
                false, page[0], page[1], modifiers(e.getMetaState()));
    }

    /** Sends a press event of kind about n, with where its touch went down. */
    private void pressed(Node n, int kind, int index) {
        if (!n.has(kind)) return;
        event(kind, n.id, n.pressClicks, n.pressX, n.pressY, "", 0, 0, index, 0, 0, 0, 0, null, 0, 0, false, "",
                false, n.pressPageX, n.pressPageY, n.pressMods);
    }

    /** The UTF-8 byte offset of the character a text view's press went down on, or 0 for other views. */
    private static int pressedIndex(View v) {
        return v instanceof TextNode ? ((TextNode) v).pressedIndex() : 0;
    }

    /** Records where a touch went down on n, for its presses. */
    private void pressedAt(Node n, MotionEvent e) {
        float[] page = page(n.view, e);
        n.pressX = e.getX() / density;
        n.pressY = e.getY() / density;
        n.pressPageX = page[0];
        n.pressPageY = page[1];
        n.pressMods = modifiers(e.getMetaState());
        long now = e.getEventTime();
        int slop = ViewConfiguration.get(getContext()).getScaledDoubleTapSlop();
        if (n.id == lastPressID && now - lastPressTime <= ViewConfiguration.getDoubleTapTimeout()
                && Math.hypot(e.getRawX() - lastPressX, e.getRawY() - lastPressY) <= slop) {
            n.pressClicks = lastPressCount + 1;
        } else {
            n.pressClicks = 1;
        }
        lastPressID = n.id;
        lastPressCount = n.pressClicks;
        lastPressTime = now;
        lastPressX = e.getRawX();
        lastPressY = e.getRawY();
    }

    /** Stops timing n's long press. */
    private static void stopLongPress(Node n) {
        if (n.longTimer != null) {
            n.view.removeCallbacks(n.longTimer);
            n.longTimer = null;
        }
    }

    /** Forgets n's touch once its press is reported, so a screen reader's click reports none. */
    private static void pressDone(Node n) {
        n.pressX = n.pressY = n.pressPageX = n.pressPageY = 0;
        n.pressMods = n.pressClicks = 0;
    }

    /**
     * Presses at a screen point as a tap there would, for a canvas part a
     * screen reader activates: on the innermost shown view under it that
     * takes presses.
     */
    private boolean pressAtScreen(float rawX, float rawY) {
        Node best = null;
        int depth = -1;
        for (int i = 0; i < nodes.size(); i++) {
            Node o = nodes.valueAt(i);
            if (!(o.has(Protocol.EVENT_PRESS) || o.has(Protocol.EVENT_PRESS_DOWN)) || !under(o.view, rawX, rawY)) continue;
            int d = 0;
            for (Object p = o.view.getParent(); p instanceof View; p = ((View) p).getParent()) d++;
            if (d > depth) {
                best = o;
                depth = d;
            }
        }
        if (best == null) return false;
        int[] at = new int[2];
        best.view.getLocationOnScreen(at);
        float x = (rawX - at[0]) / density, y = (rawY - at[1]) / density;
        int[] page = new int[2];
        getLocationOnScreen(page);
        for (int kind : new int[] {Protocol.EVENT_PRESS_DOWN, Protocol.EVENT_PRESS_UP, Protocol.EVENT_PRESS}) {
            if (best.has(kind)) {
                event(kind, best.id, 1, x, y, "", 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, "", false,
                        (rawX - page[0]) / density, (rawY - page[1]) / density, 0);
            }
        }
        return true;
    }

    /** Where e is in the root view v is in, the modal around it or this surface, in points. */
    private float[] page(View v, MotionEvent e) {
        View root = this;
        for (View p = v; p != null && p != this; p = p.getParent() instanceof View ? (View) p.getParent() : null) {
            if (p instanceof Layer) {
                root = p;
                break;
            }
        }
        int[] at = new int[2];
        root.getLocationOnScreen(at);
        return new float[] {(e.getRawX() - at[0]) / density, (e.getRawY() - at[1]) / density};
    }

    /** Where surface pixel point (x, y) is in the root view v is in, the modal around it or this surface, in points. */
    private float[] pageAt(View v, float x, float y) {
        for (View p = v; p != null && p != this; p = p.getParent() instanceof View ? (View) p.getParent() : null) {
            if (p instanceof Layer) {
                float[] local = touchPoint(p, x, y);
                if (local != null) return new float[] {local[0] / density, local[1] / density};
                break;
            }
        }
        return new float[] {x / density, y / density};
    }

    /** Sends a mouse enter, leave or move about node id at surface pixel point (x, y) if it listens for kind. */
    private void reportMouse(int id, int kind, float x, float y, int mods, int button) {
        Node n = nodes.get(id);
        if (n == null || !n.has(kind)) return;
        float[] local = touchPoint(n.view, x, y);
        float lx = local != null ? local[0] / density : 0;
        float ly = local != null ? local[1] / density : 0;
        float[] pg = pageAt(n.view, x, y);
        event(kind, n.id, 0, lx, ly, "", 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, "", false,
                pg[0], pg[1], 0, 0, mods, button);
    }

    /** The bits core.Modifiers is sent as: control, alt, shift and meta (the platform key). */
    private static int modifiers(int meta) {
        return Mouse.modifiers(meta);
    }

    private int px(float points) {
        return Math.round(points * density);
    }

    /**
     * Reports back to the engine, which routes it to the topmost overlay, modal or stack. While the app runs it takes back even when nothing there does, for the engine cannot answer; false once the app ended or while the view is detached from its engine.
     */
    public boolean back() {
        if (!inputGate.allowInput()) return true;
        if (!attached || engine == null || !engine.isAlive()) return false;
        event(Protocol.EVENT_BACK, 0, 0, 0, 0, "");
        return true;
    }

    /** Takes back on API 33 and later, where predictive back skips Activity.onBackPressed. */
    private void registerBack() {
        OnBackInvokedDispatcher dispatcher = findOnBackInvokedDispatcher();
        if (dispatcher == null) return;
        OnBackInvokedCallback callback = Build.VERSION.SDK_INT >= 34 ? new BackGesture(this) : this::back;
        dispatcher.registerOnBackInvokedCallback(OnBackInvokedDispatcher.PRIORITY_DEFAULT, callback);
        backDispatcher = dispatcher;
        backCallback = callback;
    }

    /**
     * A predictive back gesture began: the engine answers which screen it
     * pops (ITEM_BACK_PREVIEW), which then follows the gesture.
     */
    void backStarted(float progress) {
        if (!inputGate.allowInput()) return;
        backGesture = true;
        backProgress = progress;
        event(Protocol.EVENT_BACK_STARTED, 0, 0, 0, 0, "");
    }

    void backProgressed(float progress) {
        if (!inputGate.allowInput()) return;
        backProgress = progress;
        if (backScreen != null) placeBack(backDrag(progress));
    }

    void backCancelled() {
        if (!inputGate.allowInput()) return;
        backGesture = false;
        if (backScreen != null) settleBack(false);
        event(Protocol.EVENT_BACK_CANCELLED, 0, 0, 0, 0, "");
    }

    /** The gesture went through: the screen leaves, then the engine pops it. */
    void backInvoked() {
        if (!inputGate.allowInput()) return;
        backGesture = false;
        if (backScreen != null) {
            settleBack(true);
        } else {
            back();
        }
    }

    /** How far off its screen the gesture's progress moves: part of the way, as Android's own does. */
    private static float backDrag(float progress) {
        return progress * 0.4f;
    }

    /** Answers backStarted with screen n, or null when back goes elsewhere first. */
    private void previewBack(Node n) {
        if (!backGesture || n == null || n.view.getParent() == null || backScreen != null || backMove != null) return;
        ViewGroup up = (ViewGroup) n.view.getParent();
        Node below = null;
        for (int i = up.indexOfChild(n.view) - 1; i >= 0 && below == null; i--) {
            View v = up.getChildAt(i);
            for (int j = 0; j < nodes.size(); j++) {
                Node o = nodes.valueAt(j);
                if (o.view == v && o.header != null) {
                    below = o;
                    break;
                }
            }
        }
        backScreen = n;
        backBelow = below;
        backBase = n.view.getTranslationX();
        if (below != null) {
            backBelowBase = below.view.getTranslationX();
            backBelowVisibility = below.view.getVisibility();
            below.view.setVisibility(VISIBLE);
        }
        placeBack(backDrag(backProgress));
    }

    /** Moves the screen a fraction t of the way off, and the one below from a quarter of the way in. */
    private void placeBack(float t) {
        backAt = t;
        float w = backScreen.view.getWidth();
        if (getLayoutDirection() == LAYOUT_DIRECTION_RTL) w = -w;
        backScreen.view.setTranslationX(backBase + t * w);
        if (backBelow != null) backBelow.view.setTranslationX(backBelowBase - (1 - t) * w * 0.25f);
    }

    /** Ends the gesture's motion: off, reporting the back, or back in place. */
    private void settleBack(boolean pop) {
        Node screen = backScreen, below = backBelow;
        float base = backBase, belowBase = backBelowBase;
        int belowVisibility = backBelowVisibility;
        ValueAnimator a = ValueAnimator.ofFloat(backAt, pop ? 1 : 0);
        a.setDuration(Math.round((pop ? 300 : 200) * Math.max(Math.abs((pop ? 1 : 0) - backAt), 0.2f)));
        a.setInterpolator(new DecelerateInterpolator(1.5f));
        a.addUpdateListener(x -> placeBack((float) x.getAnimatedValue()));
        a.addListener(new android.animation.AnimatorListenerAdapter() {
            @Override
            public void onAnimationEnd(android.animation.Animator animation) {
                backMove = null;
                backScreen = backBelow = null;
                screen.view.setTranslationX(base);
                if (below != null) {
                    below.view.setTranslationX(belowBase);
                    if (!pop) below.view.setVisibility(belowVisibility);
                }
                if (pop) {
                    // Until the engine hides it, as it pops it.
                    screen.view.setVisibility(GONE);
                    event(Protocol.EVENT_BACK, screen.id, 0, 0, 0, "");
                }
            }
        });
        backMove = a;
        a.start();
    }

    /** listenContrast hears the system's contrast change, on API 34 and later only. */
    private void listenContrast() {
        UiModeManager.ContrastChangeListener listener = value -> environment();
        uiMode.addContrastChangeListener(ui::post, listener);
        contrast = listener;
    }

    private void stopContrast() {
        uiMode.removeContrastChangeListener((UiModeManager.ContrastChangeListener) contrast);
        contrast = null;
    }

    private void unregisterBack() {
        if (backCallback == null || Build.VERSION.SDK_INT < 33) return;
        OnBackInvokedDispatcher dispatcher = (OnBackInvokedDispatcher) backDispatcher;
        dispatcher.unregisterOnBackInvokedCallback((OnBackInvokedCallback) backCallback);
        backDispatcher = null;
        backCallback = null;
    }

    @Override
    protected boolean dispatchHoverEvent(MotionEvent e) {
        if (!inputGate.allowInput()) return true;
        boolean mouseHandled = mouse.hover(e);
        boolean handled = super.dispatchHoverEvent(e);
        return handled || mouseHandled;
    }

    /**
     * Sees each touch before the views below: a touch starting reports
     * presses outside overlays and collects the gesture views under it as
     * they see it; each later one moves the gestures first, so a pan that
     * activates stops the scroll views around it from taking the move.
     */
    @Override
    public boolean dispatchTouchEvent(MotionEvent e) {
        if (!inputGate.allowInput()) return true;
        boolean down = e.getActionMasked() == MotionEvent.ACTION_DOWN;
        if (down) modality(Protocol.MODALITY_POINTER);
        boolean mouseHandled = mouse.touch(e);
        if (selectTouch(e)) return true;
        touches(e);
        if (down) {
            pressOutside(e);
            gestures.reset();
        } else {
            gestures.track(e);
        }
        boolean handled = super.dispatchTouchEvent(e);
        if (down) gestures.down(e);
        // While a node listens, the surface keeps the fingers though no
        // view took the down: a responder that claims on a move, as a
        // toast's swipe, hears the moves only then.
        return handled || touching || mouseHandled;
    }

    /**
     * The selection of an area, and its touch: a long press waited for at
     * the down, and whether the touch made the selection and now moves its
     * end (Selection.java).
     */
    private Selection selection;
    private Runnable selectLater;
    private boolean selectingTouch, selectMoved;
    private int lastPressID, lastPressCount;
    private long lastPressTime;
    private float lastPressX, lastPressY;
    private float selectDownX, selectDownY;
    private final ViewTreeObserver.OnScrollChangedListener selectionScrolled = this::refreshSelection;

    /**
     * Follows a touch for selections: a down elsewhere clears the
     * selection, and one held on an area selects the word there, then
     * takes the rest of the touch, which moves the selection's end, from
     * the views. It reports whether it took the event.
     */
    private boolean selectTouch(MotionEvent e) {
        float x = e.getX(), y = e.getY();
        switch (e.getActionMasked()) {
            case MotionEvent.ACTION_DOWN: {
                if (selection != null && !selection.onHandle(x, y)) clearSelection();
                View area = selection == null ? areaAt(x, y) : null;
                if (area == null) return false;
                selectDownX = x;
                selectDownY = y;
                long downTime = e.getDownTime();
                selectLater = () -> {
                    selectLater = null;
                    Selection s = new Selection(this, area, (text, bx, by, bw, bh) -> reportSelection(area, text, bx, by, bw, bh));
                    if (!s.selectWordAt(selectDownX, selectDownY)) return;
                    selection = s;
                    selectingTouch = true;
                    selectMoved = false;
                    getViewTreeObserver().addOnScrollChangedListener(selectionScrolled);
                    performHapticFeedback(android.view.HapticFeedbackConstants.LONG_PRESS);
                    // The views lose the touch the selection took.
                    MotionEvent cancel = MotionEvent.obtain(downTime, android.os.SystemClock.uptimeMillis(),
                            MotionEvent.ACTION_CANCEL, selectDownX, selectDownY, 0);
                    super.dispatchTouchEvent(cancel);
                    cancel.recycle();
                };
                postDelayed(selectLater, ViewConfiguration.getLongPressTimeout());
                return false;
            }
            case MotionEvent.ACTION_MOVE:
                if (selectingTouch) {
                    // The word stays until the finger moves on from it.
                    if (selectMoved || Math.hypot(x - selectDownX, y - selectDownY)
                            > ViewConfiguration.get(getContext()).getScaledTouchSlop()) {
                        selectMoved = true;
                        selection.moveHandle(1, x, y);
                    }
                    return true;
                }
                if (selectLater != null && Math.hypot(x - selectDownX, y - selectDownY)
                        > ViewConfiguration.get(getContext()).getScaledTouchSlop()) {
                    removeCallbacks(selectLater);
                    selectLater = null;
                }
                return false;
            case MotionEvent.ACTION_UP:
            case MotionEvent.ACTION_CANCEL:
                if (selectLater != null) {
                    removeCallbacks(selectLater);
                    selectLater = null;
                }
                if (selectingTouch) {
                    selectingTouch = false;
                    if (selection != null) selection.showToolbar();
                    return true;
                }
                return false;
            default:
                return selectingTouch;
        }
    }

    /**
     * The selection area a touch at x, y goes to: the outermost area around
     * the view hit, or the selectable text hit, unless a field, a page, a
     * native view, a pressable view or one that follows pans keeps it.
     */
    private View areaAt(float x, float y) {
        if (rootView == null) return null;
        List<View> under = new ArrayList<>();
        int[] at = new int[2], self = new int[2];
        getLocationInWindow(self);
        View v = rootView;
        while (v != null) {
            under.add(v);
            View next = null;
            if (v instanceof ViewGroup) {
                ViewGroup g = (ViewGroup) v;
                for (int i = g.getChildCount() - 1; i >= 0; i--) {
                    View c = g.getChildAt(i);
                    if (c.getVisibility() != VISIBLE) continue;
                    c.getLocationInWindow(at);
                    float cx = x + self[0] - at[0], cy = y + self[1] - at[1];
                    if (cx >= 0 && cy >= 0 && cx < c.getWidth() && cy < c.getHeight()) {
                        next = c;
                        break;
                    }
                }
            }
            v = next;
        }
        View area = null;
        boolean taken = false;
        for (int i = under.size() - 1; i >= 0; i--) {
            View u = under.get(i);
            Node n = byView.get(u);
            if (u instanceof EditText || u instanceof android.webkit.WebView) {
                taken = true;
                continue;
            }
            if (n == null) continue;
            boolean keeps = n.embedded != null || n.has(Protocol.EVENT_PRESS) || n.has(Protocol.EVENT_LONG_PRESS)
                    || n.has(Protocol.EVENT_PRESS_DOWN) || n.has(Protocol.EVENT_PAN);
            if (keeps) {
                if (area == null) taken = true;
            } else if (n.selectionArea) {
                if (area != null || !taken) area = u;
            } else if (u instanceof TextNode && ((TextNode) u).selectable == Protocol.SELECTABLE_ON && !taken && area == null) {
                area = u;
            }
        }
        return area;
    }

    /** Reports an area's selection change in points in the area when the area listens for it. */
    private void reportSelection(View area, String text, int x, int y, int width, int height) {
        Node n = byView.get(area);
        if (n != null && n.has(Protocol.EVENT_SELECTION_CHANGE)) {
            event(Protocol.EVENT_SELECTION_CHANGE, n.id, 0, x / density, y / density, text, width / density, height / density, 0);
        }
    }

    /** Follows the selection's texts after a batch or a scroll. */
    private void refreshSelection() {
        if (selection != null && !selection.refresh()) clearSelection();
    }

    private void clearSelection() {
        if (selection == null) return;
        selection.clear();
        selection = null;
        getViewTreeObserver().removeOnScrollChangedListener(selectionScrolled);
    }

    /** Whether the fingers are reported as EventTouch: from a first finger down while a node listens. */
    private boolean touching;
    /** The last fingers reported, for a cancel when a scroll view takes them. */
    private String fingers = "";
    /** Listening paths retained until the last finger lifts, including a former finger's responder. */
    private final LinkedHashSet<Node> touchViews = new LinkedHashSet<>();
    /** Whether the runtime itself is stopping the scroll views, for the touch responder. */
    private boolean blockingNative;

    /**
     * Reports the fingers of e as EventTouch while a node listens: each as
     * "id x y changed" in points in this view, joined by ";", with the
     * phase as core.GesturePhase.
     */
    private void touches(MotionEvent e) {
        int action = e.getActionMasked();
        if (action == MotionEvent.ACTION_DOWN) {
            touching = listensTouches();
            touchViews.clear();
        }
        if (!touching) return;
        int phase;
        int changed = -1; // every finger for a move
        switch (action) {
            case MotionEvent.ACTION_DOWN:
            case MotionEvent.ACTION_POINTER_DOWN:
                phase = 1;
                changed = e.getActionIndex();
                break;
            case MotionEvent.ACTION_MOVE:
                phase = 2;
                break;
            case MotionEvent.ACTION_UP:
            case MotionEvent.ACTION_POINTER_UP:
                phase = 3;
                changed = e.getActionIndex();
                break;
            case MotionEvent.ACTION_CANCEL:
                phase = 4;
                break;
            default:
                return;
        }
        StringBuilder b = new StringBuilder();
        for (int i = 0; i < e.getPointerCount(); i++) {
            if (i > 0) b.append(';');
            b.append(e.getPointerId(i)).append(' ').append(e.getX(i) / density).append(' ')
                    .append(e.getY(i) / density).append(' ').append(changed < 0 || changed == i ? 1 : 0);
        }
        fingers = b.toString();
        StringBuilder targets = new StringBuilder();
        if (phase == 1) {
            for (int i = 0; i < e.getPointerCount(); i++) {
                if (changed >= 0 && changed != i) continue;
                Node n = touchHit(this, e.getX(i), e.getY(i));
                targets.append("\nt ").append(e.getPointerId(i)).append(' ').append(n == null ? 0 : n.id);
                for (; n != null; n = n.up) {
                    if (n.has(Protocol.EVENT_TOUCH)) touchViews.add(n);
                }
            }
        }
        event(Protocol.EVENT_TOUCH, 0, 0, 0, 0, "", 0, 0, 0, phase, 0, 0, 0, null, e.getEventTime(), 0, false,
                touchGeometry(fingers, targets.toString()), false);
        if (action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL) {
            touching = false;
            touchViews.clear();
        }
    }

    /** A scroll view took the touches: the nodes hear them cancelled. */
    private void touchesTaken() {
        if (!touching || blockingNative) return;
        touching = false;
        event(Protocol.EVENT_TOUCH, 0, 0, 0, 0, "", 0, 0, 0, 4, 0, 0, 0, null, android.os.SystemClock.uptimeMillis(), 0,
                false, touchGeometry(fingers.replaceAll(" [01](;|$)", " 1$1"), ""), false);
        touchViews.clear();
    }

    /** Native hit testing, including ancestor transforms, clipping and NSGo's pointer policy. */
    private Node touchHit(View view, float x, float y) {
        if (view.getVisibility() != VISIBLE) return null;
        Node n = byView.get(view);
        if (n != null && n.pointerEvents == Protocol.POINTER_EVENTS_NONE) return null;
        boolean inside = x >= 0 && y >= 0 && x < view.getWidth() && y < view.getHeight();
        boolean slop = n != null && n.slop != null && x >= -n.slop[3] * density && y >= -n.slop[0] * density
                && x < view.getWidth() + n.slop[1] * density && y < view.getHeight() + n.slop[2] * density;
        boolean area = n != null && n.area != null && x >= n.area[0] * density && y >= n.area[1] * density
                && x < (n.area[0] + n.area[2]) * density && y < (n.area[1] + n.area[3]) * density;
        if (view instanceof ViewGroup && (n == null || n.pointerEvents != Protocol.POINTER_EVENTS_BOX_ONLY)) {
            ViewGroup group = (ViewGroup) view;
            if (inside || area || !group.getClipChildren() && !Box.clips(view)) {
                for (int i = group.getChildCount() - 1; i >= 0; i--) {
                    View child = group.getChildAt(i);
                    float[] p = intoChild(group, child, x, y);
                    if (p == null) continue;
                    Node hit = touchHit(child, p[0], p[1]);
                    if (hit != null) return hit;
                }
            }
        }
        return n != null && (inside || slop) && n.pointerEvents != Protocol.POINTER_EVENTS_BOX_NONE ? n : null;
    }

    /** The surface point in a descendant's viewport, using its current native matrices. */
    private float[] touchPoint(View view, float x, float y) {
        if (view == this) return new float[] {x, y};
        if (!(view.getParent() instanceof ViewGroup)) return null;
        ViewGroup parent = (ViewGroup) view.getParent();
        float[] p = touchPoint(parent, x, y);
        return p == null ? null : intoChild(parent, view, p[0], p[1]);
    }

    /** Snapshot positions before the pipe reader or a later frame can move these views. */
    private String touchGeometry(String points, String targets) {
        String[] fingers = points.split(";");
        float[][] at = new float[fingers.length][2];
        for (int i = 0; i < fingers.length; i++) {
            String[] fields = fingers[i].split(" ");
            at[i][0] = Float.parseFloat(fields[1]) * density;
            at[i][1] = Float.parseFloat(fields[2]) * density;
        }
        StringBuilder b = new StringBuilder(points).append(targets);
        // A newline also marks a snapshot with no listening views or no hit.
        b.append('\n');
        for (Node n : touchViews) {
            if (nodes.get(n.id) != n) continue;
            StringBuilder line = new StringBuilder("p ").append(n.id);
            boolean valid = true;
            for (float[] p : at) {
                float[] local = touchPoint(n.view, p[0], p[1]);
                if (local == null) { valid = false; break; }
                line.append(' ').append(local[0] / density).append(' ').append(local[1] / density);
            }
            if (valid) b.append(line).append('\n');
        }
        return b.toString();
    }

    private boolean listensTouches() {
        for (int i = 0; i < nodes.size(); i++) {
            if (nodes.valueAt(i).has(Protocol.EVENT_TOUCH)) return true;
        }
        return false;
    }

    private boolean listensMouse() {
        for (int i = 0; i < nodes.size(); i++) {
            if (Mouse.follows(nodes.valueAt(i).events)) return true;
        }
        return false;
    }

    /** The frontmost visible modal Layer under v that participates in pointer events, or null if none is shown. */
    private Layer topModal(View v) {
        if (v == null || v.getVisibility() != VISIBLE) return null;
        Node n = byView.get(v);
        if (n != null && n.pointerEvents == Protocol.POINTER_EVENTS_NONE) return null;
        if (v instanceof ViewGroup) {
            ViewGroup g = (ViewGroup) v;
            for (int i = g.getChildCount() - 1; i >= 0; i--) {
                Layer m = topModal(g.getChildAt(i));
                if (m != null) return m;
            }
        }
        return v instanceof Layer ? (Layer) v : null;
    }

    /** Appends the mouse-following node IDs along the frontmost pointer path at surface pixels (x, y), outermost first. */
    private void mouseHitTest(float x, float y, List<Integer> under) {
        Layer modal = topModal(this);
        if (modal != null) {
            float[] p = touchPoint(modal, x, y);
            if (p != null) mouseViewsAt(modal, p[0], p[1], under);
            return;
        }
        mouseViewsAt(this, x, y, under);
    }

    /** Walks the frontmost pointer path under view at local pixels (x, y), respecting PointerEvents, transforms, clipping, opacity and modal layers. */
    private boolean mouseViewsAt(View view, float x, float y, List<Integer> out) {
        if (view.getVisibility() != VISIBLE) return false;
        if (!(view instanceof Layer) && view.getAlpha() <= 0f) return false;
        Node n = byView.get(view);
        int pe = n != null ? n.pointerEvents : 0;
        if (n != null && pe == Protocol.POINTER_EVENTS_NONE) return false;
        boolean self = Mouse.reachSelf(n != null, pe, x, y, view.getWidth(), view.getHeight(),
                n != null ? n.slop : null, density);
        if (self && Mouse.follows(n.events)) out.add(n.id);
        if (view instanceof ViewGroup) {
            ViewGroup group = (ViewGroup) view;
            boolean clips = group.getClipChildren() || view.getClipToOutline();
            if (Mouse.reachChildren(n != null, pe, clips, x, y, view.getWidth(), view.getHeight())) {
                for (int i = group.getChildCount() - 1; i >= 0; i--) {
                    View child = group.getChildAt(i);
                    float[] p = intoChild(group, child, x, y);
                    if (p == null) continue;
                    if (mouseViewsAt(child, p[0], p[1], out)) return true;
                }
            }
        }
        boolean inside = x >= 0 && y >= 0 && x < view.getWidth() && y < view.getHeight();
        return self || (view instanceof Layer && inside);
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
        if (!(view instanceof ViewGroup) || Box.clips(view) || ((ViewGroup) view).getClipChildren()) {
            return false;
        }
        ViewGroup group = (ViewGroup) view;
        for (int i = 0; i < group.getChildCount(); i++) {
            if (contains(group.getChildAt(i), x, y)) return true;
        }
        return false;
    }

    /** Applies a batch of this view's surface, whose ID comes first. */
    private void mount(byte[] batch, SparseArray<Map<String, PixelFrame>> pixels) {
        long begin = measuring ? System.nanoTime() : 0;
        if (!mounted && primary == null) {
            // The engine runs: a press that started the app goes to it.
            mounted = true;
            notificationPressed(pressedEarly);
            pressedEarly = null;
            for (String link : linksEarly) openLink(link);
            linksEarly.clear();
            for (String share : sharesEarly) receiveShare(share);
            sharesEarly.clear();
        }
        DataInputStream in = new DataInputStream(new ByteArrayInputStream(batch));
        try {
            in.readInt();
            int root = in.readInt();
            if (root != 0) rootID = root;
            layoutCreate = layoutUpdate = layoutDelete = null;
            if (in.readUnsignedByte() != 0) {
                LayoutTransition create = LayoutTransition.read(in), update = LayoutTransition.read(in);
                LayoutTransition delete = LayoutTransition.read(in);
                // Reduced motion turns layout animations off.
                if (!reduceMotion()) {
                    layoutCreate = create;
                    layoutUpdate = update;
                    layoutDelete = delete;
                    batchTime = System.nanoTime();
                    for (LayoutTransition t : new LayoutTransition[] {create, update, delete}) {
                        if (t != null) t.alignToKeyboard(keyboardMotion, batchTime);
                    }
                }
            }
            // The first batch carries a new view's default style and props.
            if (in.readUnsignedByte() != 0) {
                defaultStyle = new byte[in.readInt()];
                in.readFully(defaultStyle);
                defaultProps = new byte[in.readInt()];
                in.readFully(defaultProps);
            }
            for (int i = 0, count = in.readInt(); i < count; i++) {
                int kind = in.readUnsignedByte();
                int id = in.readInt();
                if (kind == Protocol.ITEM_CREATE) {
                    Node n = create(id, in.readUnsignedByte());
                    startDefault(n);
                    if (layoutCreate != null || layoutUpdate != null) created.add(id);
                    // Show the root at once, so text fields can take focus
                    // while the rest of its first batch mounts.
                    if (id == root) show(n.view);
                } else {
                    Map<String, PixelFrame> frames = pixels == null ? null : pixels.get(i);
                    apply(kind, nodes.get(id), in, frames == null ? Collections.emptyMap() : frames);
                }
            }
            // Optional footer: old runtimes ignore it, and pixel-only
            // follow-up batches retain the last committed version.
            if (in.available() >= 8) {
                long version = in.readLong();
                if (version != 0) geometryVersion = version;
            }
            if (in.available() >= 4) {
                for (int i = 0, count = in.readInt(); i < count; i++) {
                    Node n = nodes.get(in.readInt());
                    TextCopy copy = TextCopy.read(in);
                    if (n != null && n.view instanceof TextNode) ((TextNode) n.view).copy = copy;
                }
            }
            // Then each restyled view's backdrop filters, which earlier runtimes stop short of.
            if (in.available() >= 4) {
                for (int i = 0, count = in.readInt(); i < count; i++) {
                    Node n = nodes.get(in.readInt());
                    BackdropMath.Spec spec = BackdropMath.read(in, density);
                    if (n != null) setBackdrop(n, spec);
                }
            }
        } catch (IOException e) {
            Log.e(TAG, "bad batch", e);
        }
        layoutCreate = layoutUpdate = layoutDelete = null;
        created.clear();
        // A control hidden, disabled or covered by a modal no longer holds keys.
        View focused = findFocus();
        Node focusedNode = focused == null ? null : byView.get(focused);
        if (focusedNode != null && focusedNode.field == null
                && (!focusStop(focusedNode) || !inFocusScope(focused))) requestFocus();
        // Bindings and values change in batches too.
        applyAnimated();
        setKeepScreenOn((primary != null || attached) && !displayHolds.isEmpty());
        if (animator.running()) startTicking();
        // The selection keeps to its texts, and goes with its area.
        if (selection != null) post(this::refreshSelection);
        // Views that moved, appeared, hid or opened a modal under a resting pointer.
        if (mouse.inside() && !hoverRefreshPending) {
            hoverRefreshPending = true;
            post(hoverRefresh);
        }
        if (measuring) {
            perfWork += System.nanoTime() - begin;
            perfDirty = true;
        }
        videoPictureInPicture.mounted();
        reportAllScreenInsets();
        reportGeometry();
    }

    private void observeGeometry(boolean on) {
        lastGeometry = null;
        if (geometryObserved != on) {
            geometryObserved = on;
            android.view.ViewTreeObserver observer = getViewTreeObserver();
            if (observer.isAlive()) {
                if (on) observer.addOnPreDrawListener(geometryDraw);
                else observer.removeOnPreDrawListener(geometryDraw);
            }
        }
        reportGeometry();
    }

    /** The native viewport's current matrix to screen coordinates, on every supported API. */
    private Matrix geometryMatrix(View view, java.util.IdentityHashMap<View, Matrix> cache) {
        Matrix previous = cache.get(view);
        if (previous != null) return previous;
        Matrix matrix = new Matrix();
        if (view.getParent() instanceof View) {
            View parent = (View) view.getParent();
            matrix.set(geometryMatrix(parent, cache));
            matrix.preTranslate(-parent.getScrollX(), -parent.getScrollY());
        } else {
            int[] screen = new int[2];
            view.getLocationOnScreen(screen);
            float[] origin = {0, 0};
            view.getMatrix().mapPoints(origin);
            matrix.preTranslate(screen[0] - view.getLeft() - origin[0],
                    screen[1] - view.getTop() - origin[1]);
        }
        matrix.preTranslate(view.getLeft(), view.getTop());
        matrix.preConcat(view.getMatrix());
        cache.put(view, matrix);
        return matrix;
    }

    /** Native pre-draw includes property animators, layout motion and sticky cells. */
    private void reportGeometry() {
        if (!geometryObserved) return;
        ArrayList<float[]> boxes = new ArrayList<>();
        ArrayList<Integer> ids = new ArrayList<>();
        java.util.IdentityHashMap<View, Matrix> matrices = new java.util.IdentityHashMap<>();
        Matrix inverse = new Matrix();
        if (!geometryMatrix(this, matrices).invert(inverse)) return;
        for (int i = 0; i < nodes.size(); i++) {
            Node n = nodes.valueAt(i);
            View view = n.view;
            if (view.getVisibility() != VISIBLE) continue;
            float[] box = {Float.POSITIVE_INFINITY, Float.POSITIVE_INFINITY,
                    Float.NEGATIVE_INFINITY, Float.NEGATIVE_INFINITY};
            float[] corners = {0, 0, view.getWidth(), 0,
                    view.getWidth(), view.getHeight(), 0, view.getHeight()};
            geometryMatrix(view, matrices).mapPoints(corners);
            inverse.mapPoints(corners);
            boolean valid = true;
            for (int at = 0; at < corners.length; at += 2) {
                float[] p = {corners[at], corners[at + 1]};
                if (!Float.isFinite(p[0]) || !Float.isFinite(p[1])) {
                    valid = false;
                    break;
                }
                box[0] = Math.min(box[0], p[0]);
                box[1] = Math.min(box[1], p[1]);
                box[2] = Math.max(box[2], p[0]);
                box[3] = Math.max(box[3], p[1]);
            }
            if (valid) {
                ids.add(n.id);
                boxes.add(new float[] {box[0] / density, box[1] / density,
                        (box[2] - box[0]) / density, (box[3] - box[1]) / density,
                        corners[0] / density, corners[1] / density, corners[2] / density, corners[3] / density,
                        corners[4] / density, corners[5] / density, corners[6] / density, corners[7] / density,
                        view.getWidth() / density, view.getHeight() / density});
            }
        }
        try {
            ByteArrayOutputStream bytes = new ByteArrayOutputStream();
            DataOutputStream out = new DataOutputStream(bytes);
            out.writeInt(surface);
            out.writeLong(geometryVersion);
            out.writeInt(boxes.size());
            for (int i = 0; i < boxes.size(); i++) {
                out.writeInt(ids.get(i));
                for (float value : boxes.get(i)) out.writeFloat(value);
            }
            byte[] payload = bytes.toByteArray();
            if (!Arrays.equals(lastGeometry, payload)) {
                lastGeometry = payload;
                write(Protocol.GEOMETRY_REPORT, payload);
            }
        } catch (IOException e) {
            Log.w(TAG, "geometry snapshot", e);
        }
    }

    /**
     * Starts new node n in the default style with no props and no events, as
     * android.go's first batch gave them: the core sends each only once it
     * differs.
     */
    private void startDefault(Node n) throws IOException {
        if (defaultStyle == null) return;
        style(n, new DataInputStream(new ByteArrayInputStream(defaultStyle)));
        n.fresh = true;
        props(n, new DataInputStream(new ByteArrayInputStream(defaultProps)));
        events(n, 0);
    }

    private void show(View root) {
        if (root.getParent() == this) return;
        if (rootView != null && rootView.getParent() == this) removeView(rootView);
        detach(root);
        // Under the accessory bar.
        addView(root, 0);
        rootView = root;
        if (!backdropNodes.isEmpty()) updateBackdropAncestors();
    }

    /** Whether Android's reduce-motion setting, no animations, is on. */
    private boolean reduceMotion() {
        return Settings.Global.getFloat(getContext().getContentResolver(),
                Settings.Global.ANIMATOR_DURATION_SCALE, 1f) == 0f;
    }

    private void apply(int kind, Node n, DataInputStream in, Map<String, PixelFrame> frames) throws IOException {
        switch (kind) {
            case Protocol.ITEM_INSERT: {
                Node parent = nodes.get(in.readInt());
                Node before = nodes.get(in.readInt()); // none for ID 0: last
                n.up = parent;
                detach(n.view);
                ViewGroup group = parent.children;
                group.addView(n.view, before == null ? -1 : group.indexOfChild(before.view));
                if (!backdropNodes.isEmpty()) updateBackdropAncestors();
                float parentScale = ancestorRasterScale(n.view);
                if (parentScale != n.inheritedRasterStep) {
                    n.inheritedRasterStep = parentScale;
                    walkRasterScale(n.view, parentScale);
                }
                if (n.modal >= 0 && !n.presented) {
                    n.presented = true;
                    present(n);
                }
                // The top view of each subtree the batch creates appears.
                if (layoutCreate != null && created.contains(n.id) && !created.contains(parent.id)) {
                    n.appear = layoutCreate;
                    n.appearStart = batchTime;
                    moving.add(n);
                    paint(n);
                    startTicking();
                }
                break;
            }
            case Protocol.ITEM_REMOVE:
                detach(n.view);
                if (!backdropNodes.isEmpty()) updateBackdropAncestors();
                break;
            case Protocol.ITEM_DELETE:
                if (n.effect != null) endShaderEffect(n);
                // Dropping the node drops its views, its text layout and its
                // bitmaps. A view inside one fading out goes with it.
                if (n.modal >= 0) n.view.animate().cancel();
                if (!vanish(n) && !inGhost(n.view)) detach(n.view);
                nodes.remove(n.id);
                if (wheelScroll.activeNode() == n.id) {
                    ui.removeCallbacks(wheelEnd);
                    wheelScroll.cancel(n.id);
                }
                byView.remove(n.view);
                releaseBackdrops(n);
                if (backdropNodes.remove(n) || !backdropAncestors.isEmpty()) updateBackdropAncestors();
                engine.text.hide(surface, n.laid);
                outside.remove(n);
                moving.remove(n);
                mouse.forget(n.id);
                animator.forget(n.id);
                if (n == focusedInput) focusedInput = null;
                if (n == accessoryShown) {
                    if (n.view.getParent() == this) removeView(n.view);
                    accessoryShown = null;
                }
                if (n.animator != null) n.animator.cancel();
                if (n.keyframer != null) n.keyframer.cancel();
                if (n.field != null) n.field.closeEditMenu();
                if (n.web != null) n.web.destroy();
                if (n.holder != null) {
                    detach(n.holder);
                    n.headerScreen = null;
                }
                if (n.video != null) n.video.destroy();
                if (n.gpu != null) n.gpu.destroy();
                if (n.embedded != null) n.embedded.destroy();
                if (n.modal >= 0) event(Protocol.EVENT_MODAL_DISMISSED, n.id, 0, 0, 0, "");
                break;
            case Protocol.ITEM_FRAME:
                setFrame(n, new float[] {in.readFloat(), in.readFloat(), in.readFloat(), in.readFloat()},
                        LayoutTransition.read(in));
                break;
            case Protocol.ITEM_MATCH:
                match(n, nodes.get(in.readInt()));
                break;
            case Protocol.ITEM_OVERFLOW: {
                float[] area = {in.readFloat(), in.readFloat(), in.readFloat(), in.readFloat()};
                n.area = area[2] > 0 && area[3] > 0 ? area : null;
                break;
            }
            case Protocol.ITEM_VIEW_STYLE:
                style(n, in);
                break;
            case Protocol.ITEM_TEXT: {
                ((TextNode) n.view).copy = TextCopy.EMPTY;
                n.text = Protocol.readString(in);
                int laid = in.readInt();
                if (n.laid != laid) engine.text.hide(surface, n.laid);
                TextCache.Layout layout = engine.text.show(surface, laid);
                n.laid = laid;
                int x = px(in.readFloat()), y = px(in.readFloat());
                ((TextNode) n.view).setLayout(layout != null ? layout.value : null, x, y,
                        layout != null ? layout.clip : 0, layout != null ? layout.shadow : null);
                describe(n);
                break;
            }
            case Protocol.ITEM_INPUT:
                input(n, in);
                updateAccessory();
                break;
            case Protocol.ITEM_INPUT_VALUE: {
                String value = Protocol.readString(in);
                long count = in.readInt() & 0xffffffffL;
                n.field.setValue(value, count);
                break;
            }
            case Protocol.ITEM_SELECTION: {
                int start = in.readInt(), end = in.readInt();
                if (n.field != null) n.field.select(start, end);
                break;
            }
            case Protocol.ITEM_EDIT_MENU: {
                float x = in.readFloat(), y = in.readFloat(), w = in.readFloat(), h = in.readFloat();
                int actions = in.readInt();
                if (n != null && n.field != null) {
                    if (actions == 0) {
                        n.field.closeEditMenu();
                    } else {
                        n.field.openEditMenu(this, x, y, w, h, actions, density);
                    }
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
                Scrolling.Config c = Scrolling.Config.read(in);
                scroller(n, c.horizontal, c.zooms());
                ((Scrolling.Scroller) n.scroller).configure(c);
                if (n.view instanceof ScrollEdges) ((ScrollEdges) n.view).setEdges(c.edges);
                n.insetTop = c.insetTop;
                n.headerInset = c.horizontal ? 0 : c.headerInset;
                break;
            }
            case Protocol.ITEM_SCROLL_CONTENT: {
                int w = px(in.readFloat()), h = px(in.readFloat());
                n.children.setLayoutParams(new LayoutParams(w, h, Gravity.LEFT | Gravity.TOP));
                break;
            }
            case Protocol.ITEM_FOCUS:
                if (n != null && n.field != null && n.field.requestFocus()) {
                    EditText field = n.field;
                    field.post(() -> ime.showSoftInput(field, InputMethodManager.SHOW_IMPLICIT));
                } else if (n != null && focusStop(n) && inFocusScope(n.view)) {
                    n.view.setFocusableInTouchMode(true);
                    if (!n.view.requestFocus()) n.view.setFocusableInTouchMode(false);
                }
                break;
            case Protocol.ITEM_BLUR:
                if (n != null && n.field != null && n.field.hasFocus()) {
                    blurInput(n.field);
                } else if (n != null && n.view.hasFocus()) {
                    requestFocus();
                }
                break;
            case Protocol.ITEM_IMAGE: {
                int request = in.readInt();
                int mode = in.readUnsignedByte();
                float blur = in.readFloat() * density;
                int fade = in.readInt();
                boolean nearest = in.readBoolean();
                // Each axis of the position: a fraction of the free space plus points.
                float xFraction = in.readFloat(), x = in.readFloat() * density;
                float yFraction = in.readFloat(), y = in.readFloat() * density;
                n.image.configure(request, mode, blur, fade, nearest);
                n.image.position(xFraction, x, yFraction, y);
                PixelFrame frame = frames.get("");
                if (frame != null) n.image.showPixels(frame);
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
                    blurInput(focused);
                }
                break;
            }
            case Protocol.ITEM_SCROLL_TO: {
                float x = in.readFloat(), y = in.readFloat();
                boolean animated = Protocol.flag(in.readUnsignedByte(), 0);
                if (n == null) break;
                if (n.field != null) {
                    InputField field = n.field;
                    afterLayout(field, () -> {
                        n.applied++;
                        field.scrollToPoints(x, y);
                    });
                    break;
                }
                if (!(n.scroller instanceof Scrolling.Scroller)) break;
                Scrolling.Scroller scroller = (Scrolling.Scroller) n.scroller;
                // Scroll views clamp to their content, which takes its new
                // size at the next layout. Counted as applied, the scroll
                // reports carry it.
                afterLayout(n.scroller, () -> {
                    n.applied++;
                    scroller.scrollToPoints(x, y, animated);
                });
                break;
            }
            case Protocol.ITEM_SCROLL_BY: {
                float x = in.readFloat(), y = in.readFloat();
                if (n == null || !(n.scroller instanceof Scrolling.Scroller)) break;
                Scrolling.Scroller scroller = (Scrolling.Scroller) n.scroller;
                afterLayout(n.scroller, () -> {
                    n.applied++;
                    scroller.scrollByPoints(x, y);
                });
                break;
            }
            case Protocol.ITEM_STICKY_CELLS: {
                // Each cell: its id, start and end, the animated node its
                // inset follows (0: none) and the inset start counts.
                float[] cells = new float[in.readInt() * 5];
                n.stickyBound = false;
                for (int i = 0; i < cells.length; i += 5) {
                    cells[i] = in.readInt();
                    cells[i + 1] = in.readFloat();
                    cells[i + 2] = in.readFloat();
                    cells[i + 3] = in.readInt();
                    cells[i + 4] = in.readFloat();
                    n.stickyBound |= cells[i + 3] != 0;
                }
                if (n.sticky != null) {
                    for (int i = 0; i < n.sticky.length; i += 5) stick(nodes.get((int) n.sticky[i]), 0, false);
                }
                n.sticky = cells;
                pinSticky(n);
                break;
            }
            case Protocol.ITEM_ZOOM_TO: {
                float scale = in.readFloat();
                boolean centered = Protocol.flag(in.readUnsignedByte(), 0);
                float x = in.readFloat(), y = in.readFloat();
                boolean animated = Protocol.flag(in.readUnsignedByte(), 0);
                if (n != null && n.scroller instanceof Scrolling.ZoomScroll) {
                    Scrolling.ZoomScroll z = (Scrolling.ZoomScroll) n.scroller;
                    afterLayout(z, () -> z.zoomTo(scale, centered, x, y, animated));
                }
                break;
            }
            case Protocol.ITEM_CANVAS: {
                byte[] ops = new byte[in.readInt()];
                in.readFully(ops);
                int slots = in.readInt();
                int[] offsets = new int[slots], nodes = new int[slots];
                boolean[] colors = new boolean[slots];
                for (int i = 0; i < slots; i++) {
                    offsets[i] = in.readInt();
                    colors[i] = in.readUnsignedByte() != 0;
                    nodes[i] = in.readInt();
                }
                CanvasNode.Part[] parts = new CanvasNode.Part[in.readInt()];
                for (int i = 0; i < parts.length; i++) {
                    float x = in.readFloat(), y = in.readFloat(), w = in.readFloat(), h = in.readFloat();
                    int role = in.readUnsignedByte();
                    String label = Protocol.readString(in), testID = Protocol.readString(in);
                    boolean selected = in.readUnsignedByte() != 0;
                    parts[i] = new CanvasNode.Part(x, y, w, h, role, label, testID, selected);
                }
                String[] keys = new String[in.readInt()];
                for (int i = 0; i < keys.length; i++) keys[i] = Protocol.readString(in);
                n.canvas.setPixels(keys, frames);
                n.canvas.setOps(ops, offsets, colors, parts);
                animator.bindCanvas(n.id, nodes);
                applyAnimated();
                break;
            }
            case Protocol.ITEM_SHADER:
                setShader(n, in);
                break;
            case Protocol.ITEM_SHADER_TRANSITION:
                if (n != null && Build.VERSION.SDK_INT >= 29) shaderEffect(n).start();
                break;
            case Protocol.ITEM_SHADER_INPUT:
                if (n != null && n.effect != null) n.effect.dirty();
                break;
            case Protocol.ITEM_KEEP_IMAGE:
                canvasImages.keep(Protocol.readString(in));
                break;
            case Protocol.ITEM_RELEASE_IMAGE:
                canvasImages.release(Protocol.readString(in));
                break;
            case Protocol.ITEM_GPU: {
                int colorFormat = in.readUnsignedByte();
                int depthFormat = in.readUnsignedByte();
                int sampleCount = in.readInt();
                if (n != null && n.gpu != null) n.gpu.configure(colorFormat, depthFormat, sampleCount);
                break;
            }
            case Protocol.ITEM_SET_CLIPBOARD:
                clipboard().setPrimaryClip(ClipData.newPlainText("", Protocol.readString(in)));
                break;
            case Protocol.ITEM_READ_CLIPBOARD: {
                long count = in.readInt() & 0xffffffffL;
                int what = in.readUnsignedByte();
                // Android answers null unless the app has focus.
                String[] text = {""}, data = {""};
                boolean[] has = {false};
                Services.readClipboard(getContext(), clipboard(), what, text, data, has);
                event(Protocol.EVENT_CLIPBOARD, 0, count, 0, 0, text[0], 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false,
                        data[0], has[0]);
                break;
            }
            case Protocol.ITEM_SET_CLIPBOARD_IMAGE:
                Services.setClipboardImage(getContext(), clipboard(), Protocol.readBytes(in));
                break;
            case Protocol.ITEM_OPEN_URL: {
                long count = in.readInt() & 0xffffffffL;
                Context context = activity() != null ? activity() : getContext();
                boolean opened = Services.openURL(context, Protocol.readString(in));
                event(Protocol.EVENT_OPEN_URL, 0, count, 0, 0, "", 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, "", opened);
                break;
            }
            case Protocol.ITEM_SYSTEM_GESTURES:
                readGestureRects(in);
                break;
            case Protocol.ITEM_HAPTIC:
                haptic(in.readInt());
                break;
            case Protocol.ITEM_REQUEST_REVIEW:
                Services.requestReview(activity());
                break;
            case Protocol.ITEM_KEEP_AWAKE: {
                int id = in.readInt();
                boolean display = in.readUnsignedByte() != 0;
                // Android folds visible views' requests into their window's FLAG_KEEP_SCREEN_ON.
                if (display && displayHolds.add(id) && displayHolds.size() == 1) setKeepScreenOn(primary != null || attached);
                break;
            }
            case Protocol.ITEM_RELEASE_AWAKE:
                if (displayHolds.remove(in.readInt()) && displayHolds.isEmpty()) setKeepScreenOn(false);
                break;
            case Protocol.ITEM_HAPTIC_PATTERN:
                hapticPatterns.apply(in, getWindowVisibility() == View.VISIBLE);
                break;
            case Protocol.ITEM_SENSOR: {
                if (sensors == null) sensors = new Sensors(getContext(), this::activity, engine, this::keeperEnded);
                sensors.handle(in);
                break;
            }
            case Protocol.ITEM_NOTIFICATION_PERMISSION: {
                long count = in.readInt() & 0xffffffffL;
                boolean ask = in.readUnsignedByte() != 0;
                Activity activity = activity();
                if (ask && activity != null && Notifications.needsPrompt(getContext())) {
                    permissionAsks.add(count);
                    if (permissionAsks.size() == 1) {
                        activity.requestPermissions(new String[] {Notifications.PERMISSION}, Notifications.PERMISSION_REQUEST);
                    }
                    break;
                }
                answerPermission(count);
                break;
            }
            case Protocol.ITEM_SHOW_NOTIFICATION: {
                long count = in.readInt() & 0xffffffffL;
                String nid = Protocol.readString(in), title = Protocol.readString(in), body = Protocol.readString(in);
                boolean silent = in.readUnsignedByte() != 0;
                long at = in.readLong();
                int repeat = in.readUnsignedByte();
                boolean shown = at == 0 ? Notifications.showNow(getContext(), activity(), nid, title, body, silent)
                        : Notifications.schedule(getContext(), nid, title, body, silent, at, repeat);
                event(Protocol.EVENT_NOTIFICATION, 0, count, 0, 0, "", 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, "", shown);
                break;
            }
            case Protocol.ITEM_CLOSE_NOTIFICATION:
                Notifications.close(getContext(), Protocol.readString(in));
                break;
            case Protocol.ITEM_BADGE:
                Notifications.badge = in.readInt();
                break;
            case Protocol.ITEM_BIOMETRIC_AVAILABILITY: {
                long count = in.readInt() & 0xffffffffL;
                event(Protocol.EVENT_BIOMETRIC, 0, count, 0, 0, "", 0, 0, biometricAvailability());
                break;
            }
            case Protocol.ITEM_ORIENTATION:
                orientation = in.readUnsignedByte();
                applyOrientation();
                break;
            case Protocol.ITEM_SPEECH: {
                long count = in.readInt() & 0xffffffffL;
                int command = in.readUnsignedByte();
                String lang = Protocol.readString(in);
                boolean interim = Protocol.flag(in.readUnsignedByte(), 0);
                Speech.handle(this, count, command, lang, interim);
                break;
            }
            case Protocol.ITEM_SPEAK: {
                long count = in.readInt() & 0xffffffffL;
                int command = in.readUnsignedByte();
                String text = Protocol.readString(in), lang = Protocol.readString(in), voice = Protocol.readString(in);
                float rate = in.readFloat(), pitch = in.readFloat();
                Speaker.handle(this, count, command, text, lang, voice, rate, pitch);
                break;
            }
            case Protocol.ITEM_AUDIO: {
                NsgoView first = primary == null ? this : primary;
                if (first.audio == null) first.audio = new Audio(first.getContext(), first::write);
                first.audio.command(surface, audioVisible(), in);
                break;
            }
            case Protocol.ITEM_AUTH_SESSION: {
                AuthSession.read(this, in);
                break;
            }
            case Protocol.ITEM_BIOMETRIC_AUTHENTICATE: {
                long count = in.readInt() & 0xffffffffL;
                authenticateBiometric(count, Protocol.readString(in));
                break;
            }
            case Protocol.ITEM_PICK_IMAGES: {
                Services.Pick p = new Services.Pick(in.readInt() & 0xffffffffL, in.readInt(), in.readInt(), in.readFloat(),
                        Protocol.flag(in.readUnsignedByte(), 0));
                if (pick != null) answerPick(pick, "");
                Activity activity = activity();
                if (activity != null && Services.startPick(activity, p)) pick = p;
                else answerPick(p, "");
                break;
            }
            case Protocol.ITEM_TAKE_PHOTO: {
                // As a pick of one, always encoded again so the photo comes
                // upright at the quality asked for.
                long count = in.readInt() & 0xffffffffL;
                int limit = in.readInt(), maxDimension = in.readInt();
                float quality = in.readFloat();
                in.readUnsignedByte();
                takePhoto(new Services.Pick(count, limit, maxDimension, quality > 0 ? quality : 0.9f, false));
                break;
            }
            case Protocol.ITEM_PICK_FILES: {
                long count = in.readInt() & 0xffffffffL;
                int limit = in.readInt();
                String[] types = new String[in.readInt()];
                for (int i = 0; i < types.length; i++) types[i] = Protocol.readString(in);
                startDocuments(new Services.Documents(count, limit, null, false), a -> Services.startFiles(a, limit, types));
                break;
            }
            case Protocol.ITEM_PICK_DIRECTORIES: {
                long count = in.readInt() & 0xffffffffL;
                // The tree picker picks one folder, whatever the limit.
                in.readInt();
                startDocuments(new Services.Documents(count, 1, null, true), Services::startFolder);
                break;
            }
            case Protocol.ITEM_SAVE_FILE:
            case Protocol.ITEM_SAVE_FILE_PREFLIGHT: {
                long count = in.readInt() & 0xffffffffL;
                String from = Protocol.readString(in), name = Protocol.readString(in), mime = Protocol.readString(in);
                // No file: Go failed the save already.
                if (!from.isEmpty()) {
                    if (kind == Protocol.ITEM_SAVE_FILE_PREFLIGHT) {
                        startDocuments(new Services.Documents(count, 1, from, false, name, mime), Services::startFolder);
                    } else {
                        startDocuments(new Services.Documents(count, 1, from, false), a -> Services.startSave(a, name, mime));
                    }
                }
                break;
            }
            case Protocol.ITEM_SAVE_PREFLIGHT_ANSWER: {
                long count = in.readInt() & 0xffffffffL;
                boolean approved = (in.readUnsignedByte() & 1) != 0;
                answerSavePreflight(count, approved, Protocol.readString(in));
                break;
            }
            case Protocol.ITEM_OPEN_FILE: {
                long count = in.readInt() & 0xffffffffL;
                String path = Protocol.readString(in), name = Protocol.readString(in), mime = Protocol.readString(in);
                Context context = activity() != null ? activity() : getContext();
                String[] error = {""};
                boolean opened = Services.openFile(context, path, name, mime, error);
                event(Protocol.EVENT_FILES, 0, count, 0, 0, error[0], 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, "", opened);
                break;
            }
            case Protocol.ITEM_INSTALL_PACKAGE: {
                long count = in.readInt() & 0xffffffffL;
                String path = Protocol.readString(in), name = Protocol.readString(in);
                Context context = activity() != null ? activity() : getContext();
                String error = Services.installPackage(context, path, name);
                event(Protocol.EVENT_FILES, 0, count, 0, 0, error, 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, "",
                        error.isEmpty());
                break;
            }
            case Protocol.ITEM_CAN_INSTALL_PACKAGES: {
                long count = in.readInt() & 0xffffffffL;
                boolean allowed = getContext().getPackageManager().canRequestPackageInstalls();
                event(Protocol.EVENT_FILES, 0, count, 0, 0, "", 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, "", allowed);
                break;
            }
            case Protocol.ITEM_INSTALL_SETTINGS: {
                long count = in.readInt() & 0xffffffffL;
                Context context = activity() != null ? activity() : getContext();
                boolean opened = Services.installSettings(context);
                event(Protocol.EVENT_FILES, 0, count, 0, 0, "", 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, "", opened);
                break;
            }
            case Protocol.ITEM_CAPTURE: {
                int count = in.readInt();
                boolean whole = Protocol.flag(in.readUnsignedByte(), 0);
                capture(count, whole ? this : n != null ? n.view : null);
                break;
            }
            case Protocol.ITEM_SHARE: {
                long count = in.readInt() & 0xffffffffL;
                String text = Protocol.readString(in);
                int files = in.readInt();
                String[] paths = new String[files], names = new String[files], mimes = new String[files];
                for (int i = 0; i < files; i++) {
                    paths[i] = Protocol.readString(in);
                    names[i] = Protocol.readString(in);
                    mimes[i] = Protocol.readString(in);
                }
                Context context = activity() != null ? activity() : getContext();
                String error = Services.share(context, text, paths, names, mimes);
                event(Protocol.EVENT_FILES, 0, count, 0, 0, error, 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, "",
                        error.isEmpty());
                break;
            }
            case Protocol.ITEM_APPEARANCE:
                appearance = in.readUnsignedByte();
                systemBars();
                environment();
                break;
            case Protocol.ITEM_DIRECTION: {
                direction = in.readUnsignedByte() != 0 ? LAYOUT_DIRECTION_RTL : LAYOUT_DIRECTION_LTR;
                String[] words = new String[in.readInt()];
                for (int i = 0; i < words.length; i++) words[i] = Protocol.readString(in);
                spoken = words;
                forcedLanguage = Protocol.readString(in);
                hasForcedLanguage = true;
                applyDirection();
                applyForcedLanguage();
                break;
            }
            case Protocol.ITEM_QUIT: {
                // Quitting ends the overlays with the app.
                quit = true;
                Activity activity = activity();
                if (activity != null) activity.finishAndRemoveTask();
                else if (keptOffScreen) stop();
                break;
            }
            case Protocol.ITEM_POPUP_MENU:
                // Desktops' only; Go writes no fields.
                break;
            case Protocol.ITEM_ANNOUNCE:
                announceForAccessibility(Protocol.readString(in));
                break;
            case Protocol.ITEM_BLOCK_NATIVE:
                // The touch responder stops the scroll views around it.
                if (n != null && n.view.getParent() != null) {
                    blockingNative = true;
                    n.view.getParent().requestDisallowInterceptTouchEvent(true);
                    blockingNative = false;
                }
                break;
            case Protocol.ITEM_KEYMAP: {
                int count = in.readInt();
                String[] bindings = new String[count];
                int[] flags = new int[count];
                int[] ids = new int[count];
                for (int i = 0; i < count; i++) {
                    bindings[i] = Protocol.readString(in);
                    flags[i] = in.readUnsignedByte();
                    ids[i] = in.readInt();
                }
                keys = bindings;
                keyFlags = flags;
                keyIds = ids;
                reportHeldKeys = in.readUnsignedByte() != 0;
                break;
            }
            case Protocol.ITEM_STATUS_BAR: {
                statusBarStyle = in.readUnsignedByte();
                int flags = in.readUnsignedByte();
                statusBarHidden = Protocol.flag(flags, 0);
                immersive = Protocol.flag(flags, 2);
                systemBars();
                break;
            }
            case Protocol.ITEM_TITLE: {
                // Phones show no title; the window's names it to TalkBack.
                String title = Protocol.readString(in);
                Activity activity = activity();
                if (activity != null) activity.setTitle(title);
                break;
            }
            case Protocol.ITEM_WEB:
                n.web.configure(in);
                break;
            case Protocol.ITEM_WEB_COMMAND: {
                int command = in.readUnsignedByte();
                String value = Protocol.readString(in);
                if (command == Protocol.WEB_LOAD) {
                    n.web.command(command, value, Protocol.readString(in), Protocol.readString(in));
                } else {
                    n.web.command(command, value, null, null);
                }
                break;
            }
            case Protocol.ITEM_VIDEO:
                n.video.configure(in);
                break;
            case Protocol.ITEM_VIDEO_SEEK:
                n.video.seek(in.readLong());
                break;
            case Protocol.ITEM_NATIVE:
                embed(n, in);
                break;
            case Protocol.ITEM_NATIVE_COMMAND: {
                String name = Protocol.readString(in);
                Object args = Embedded.parse(Protocol.readString(in));
                if (n.video != null && VideoPictureInPicture.EVENT.equals(name)) {
                    videoPictureInPicture.command(n.video, args);
                } else if (n.embedded != null) {
                    n.embedded.command(name, args);
                    nativeSize(n);
                }
                break;
            }
            case Protocol.ITEM_ACCESSIBILITY_LABEL:
                n.label = Protocol.readString(in);
                describe(n);
                break;
            case Protocol.ITEM_ACCESSIBILITY_FOCUS: {
                View view = n.accessible();
                view.performAccessibilityAction(AccessibilityNodeInfo.ACTION_ACCESSIBILITY_FOCUS, null);
                break;
            }
            case Protocol.ITEM_ANIMATION:
                for (int i = 0, count = in.readInt(); i < count; i++) animator.apply(Animator.readOp(in));
                break;
            case Protocol.ITEM_SCREEN:
                screen(n, in);
                break;
            case Protocol.ITEM_OPEN_SURFACE: {
                int id = in.readInt();
                float width = in.readFloat(), height = in.readFloat(), x = in.readFloat(), y = in.readFloat();
                int flags = in.readUnsignedByte();
                // Phones open no windows: only an overlay, from the first view's tree.
                if (Protocol.flag(flags, 0) && overlays != null) overlays.open(id, width, height, x, y, flags);
                break;
            }
            case Protocol.ITEM_WINDOW: {
                int op = in.readUnsignedByte();
                float x = in.readFloat(), y = in.readFloat(), width = in.readFloat(), height = in.readFloat();
                int flags = in.readUnsignedByte();
                // An overlay's commands; the first view has no window of the app's to command.
                if (primary != null && op != Protocol.WINDOW_NONE) {
                    primary.overlays.command(this, op, x, y, width, height, Protocol.flag(flags, 0), Protocol.flag(flags, 1));
                }
                break;
            }
            case Protocol.ITEM_OVERLAY_PERMISSION: {
                long count = in.readInt() & 0xffffffffL;
                boolean ask = in.readUnsignedByte() != 0;
                (primary != null ? primary : this).overlays.permission(this, count, ask);
                break;
            }
            case Protocol.ITEM_PERMISSION: {
                long count = in.readInt() & 0xffffffffL;
                int command = in.readUnsignedByte();
                String name = Protocol.readString(in);
                (primary != null ? primary : this).systemPermissions().handle(count, command, name,
                        this::answerSystemPermission);
                break;
            }
            case Protocol.ITEM_MENU:
            case Protocol.ITEM_HISTORY:
                // Desktop menus and the web's history; Go writes no fields.
                break;
            case Protocol.ITEM_BACK_PREVIEW:
                previewBack(n);
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

    /**
     * Draws n's look: its style, or the transition moving it, with its
     * animated values in place of their properties and a layout animation's
     * appearance over them.
     */
    private void paint(Node n) {
        if (n.shown == null) {
            if (n.values == null && n.appear == null) return;
            n.shown = new Look();
        }
        if (n.background == null) {
            n.background = new Box();
            n.view.setBackground(n.background);
        }
        Look l = n.shown;
        if (n.values != null) l = l.animated(n.values, density);
        if (n.keyframer != null) {
            l = n.keyframes.apply(l, (float) n.keyframer.getAnimatedValue(), n.view.getHeight());
        }
        if (l.cameraDistance == 0 && n.up != null && n.up.shown != null && n.up.shown.childPerspective > 0) {
            // Seen in its parent's perspective, about its own pivot.
            if (l == n.shown) l = l.copy();
            l.cameraDistance = n.up.shown.childPerspective;
        }
        if (n.appear != null) {
            if (l == n.shown) l = l.copy();
            float p = (float) n.appear.at(System.nanoTime() - n.appearStart);
            if (n.appear.property == LayoutTransition.SCALE) {
                l.scaleX *= p;
                l.scaleY *= p;
            } else {
                l.opacity *= p;
            }
        }
        l.apply(n.view, n.background);
        followScale(n, Math.max(Math.abs(l.scaleX), Math.abs(l.scaleY)));
        if (n.shift != 0) {
            if (n.shiftAcross) n.view.setTranslationX(n.view.getTranslationX() + n.shift * density);
            else n.view.setTranslationY(n.view.getTranslationY() + n.shift * density);
        }
        if (!backdropAncestors.isEmpty()) {
            // A container records what is behind its backdrop children as it draws, so a sibling that moves or changes redraws it.
            ViewParent p = n.view.getParent();
            if (p instanceof ViewGroup && backdropAncestors.contains(p)) ((ViewGroup) p).invalidate();
        }
    }

    /**
     * Redraws texts and canvases inside n at higher resolution when n's
     * style transform crosses a half-step (RasterScale.step), matching Apple.
     */
    private void followScale(Node n, float scale) {
        float step = RasterScale.step(scale);
        if (step == n.rasterStep) return;
        n.rasterStep = step;
        walkRasterScale(n.view, ancestorRasterScale(n.view));
    }

    /** Redraws texts and canvases inside zooming scroll node n when its zoom crosses a step. */
    private void followZoomScale(Node n, float zoom) {
        float step = RasterScale.step(zoom);
        if (step == n.zoomRasterStep) return;
        n.zoomRasterStep = step;
        walkRasterScale(n.children, ancestorRasterScale(n.children));
    }

    /** Cumulative stepped scale of v's ancestors up to this NsgoView. */
    private float ancestorRasterScale(View v) {
        float scale = 1f;
        for (ViewParent p = v.getParent(); p instanceof View && p != this; p = p.getParent()) {
            View pv = (View) p;
            float step = RasterScale.step(Math.max(Math.abs(pv.getScaleX()), Math.abs(pv.getScaleY())));
            scale = RasterScale.combine(scale, step);
        }
        return scale;
    }

    /** Applies the cumulative stepped raster scale to every TextNode and CanvasNode under v. */
    private void walkRasterScale(View v, float parentScale) {
        Node node = byView.get(v);
        if (node != null) node.inheritedRasterStep = parentScale;
        float own = RasterScale.step(Math.max(Math.abs(v.getScaleX()), Math.abs(v.getScaleY())));
        float scale = RasterScale.combine(parentScale, own);
        if (v instanceof CanvasNode) {
            ((CanvasNode) v).setRasterScale(scale);
        } else if (v instanceof TextNode) {
            ((TextNode) v).setRasterScale(scale);
        }
        if (v instanceof ViewGroup) {
            ViewGroup g = (ViewGroup) v;
            for (int i = 0, count = g.getChildCount(); i < count; i++) {
                walkRasterScale(g.getChildAt(i), scale);
            }
        }
    }

    /** Moves scroll node n's sticky cells within their signed travel from start to end. */
    private void pinSticky(Node n) {
        if (n.sticky == null || !(n.scroller instanceof Scrolling.Scroller)) return;
        Scrolling.Scroller s = (Scrolling.Scroller) n.scroller;
        boolean horizontal = n.scroller instanceof Scrolling.HScroll;
        float at = horizontal ? s.offsetX() : s.offsetY();
        for (int i = 0; i < n.sticky.length; i += 5) {
            int animatedNode = (int) n.sticky[i + 3];
            double inset = animatedNode == 0 ? Double.NaN : animator.valueOf(animatedNode);
            float shift = ScrollPhysics.stickyShift(n.sticky[i + 1], n.sticky[i + 2], at, inset, n.sticky[i + 4]);
            stick(nodes.get((int) n.sticky[i]), shift, horizontal);
        }
    }

    /** Moves sticky cell c by d points along its list from its frame. */
    private void stick(Node c, float d, boolean horizontal) {
        if (c == null || c.shift == d && c.shiftAcross == horizontal) return;
        c.shift = d;
        c.shiftAcross = horizontal;
        if (c.shown != null) {
            paint(c);
        } else {
            c.view.setTranslationX(horizontal ? d * density : 0);
            c.view.setTranslationY(horizontal ? 0 : d * density);
        }
    }

    private void setShader(Node node, DataInputStream in) throws IOException {
        if (node.shader == null) node.shader = new ShaderNode(getContext(), canvasImages);
        ShaderNode shader = node.shader;
        shader.set(in);
        animator.bindCanvas(node.id, shader.boundNodes);
        if (!shader.available()) {
            endShaderEffect(node);
            detach(shader);
        } else if (shader.role == ShaderNode.FILL) {
            endShaderEffect(node);
            if (shader.getParent() == null)
                node.children.addView(shader, 0, new LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT));
            shader.draw();
        } else if (Build.VERSION.SDK_INT >= 29) {
            shaderEffect(node).configure();
        } else {
            detach(shader);
        }
    }

    private ShaderEffect shaderEffect(Node node) {
        if (node.effect == null) {
            if (node.shader == null) node.shader = new ShaderNode(getContext(), canvasImages);
            node.effect = new ShaderEffect(node.view, node.shader, () -> rootView);
            if (effects.isEmpty() && isAttachedToWindow()) getViewTreeObserver().addOnPreDrawListener(drawEffects);
            effects.add(node.effect);
        }
        return node.effect;
    }

    private void endShaderEffect(Node node) {
        if (node.effect == null) return;
        node.effect.detach();
        effects.remove(node.effect);
        if (effects.isEmpty() && isAttachedToWindow()) getViewTreeObserver().removeOnPreDrawListener(drawEffects);
        node.effect = null;
    }

    @Override
    public void onDescendantInvalidated(View child, View target) {
        super.onDescendantInvalidated(child, target);
        // A sibling or the background can change without invalidating the effect's host.
        if (!(target instanceof ShaderNode)) {
            for (ShaderEffect effect : effects) {
                if (target != effect.host && (effect.shader.reads & (1 << ShaderNode.BACKDROP)) != 0) effect.stale = true;
            }
        }
    }

    /** Draws the views whose animated values changed. */
    private void applyAnimated() {
        animator.changed((id, values) -> {
            Node n = nodes.get(id);
            if (n == null) return;
            n.values = values.set != 0 ? values : null;
            if (measuring) perfDirty = true;
            if (n.view instanceof TextNode) {
                ((TextNode) n.view).setTint(values.has(Animator.PROP_TEXT_COLOR) ? Text.argb(values.textColor) : 0);
            }
            paint(n);
        });
        for (int i = 0; i < nodes.size(); i++) {
            Node s = nodes.valueAt(i);
            if (s.stickyBound) pinSticky(s);
        }
        animator.changedCanvases((id, c) -> {
            Node n = nodes.get(id);
            if (n == null) return;
            if (n.canvas != null) n.canvas.patch(c.nums, c.colors);
            if (n.shader != null) n.shader.patch(c.nums, c.colors);
            if (n.bound != null && n.embedded != null) n.embedded.animate(n.bound, c.nums, c.colors);
            if (measuring) perfDirty = true;
        });
    }

    private void startPerf(boolean show) {
        measuring = true;
        if (show && perfPanel == null) {
            perfPanel = new PerfPanel();
            getOverlay().add(perfPanel);
        }
        if (isAttachedToWindow() && getWindowVisibility() == View.VISIBLE) startTicking();
    }

    private void sampleFrame(long frameTime, long begin, boolean changed) {
        android.view.Display display = getDisplay();
        float hz = display == null ? 0 : display.getRefreshRate();
        long period = hz > 0 ? (long) (1_000_000_000.0 / hz) : 0;
        write(Protocol.PERF_FRAME, java.nio.ByteBuffer.allocate(25)
                .putLong(frameTime).putLong(period).putLong(perfWork + System.nanoTime() - begin)
                .put((byte) (perfDirty || changed ? 1 : 0)).array());
        perfWork = 0;
        perfDirty = false;
    }

    private void preferFrameRate(float rate) {
        if (preferredFrameRate == rate) return;
        preferredFrameRate = rate;
        lastFrameSent = 0;
        requestedFramePeriod = rate > 0 ? 1_000_000_000.0 / rate : 0;
        refreshRateHint();
    }

    /** A Window's refresh hint is the Surface.setFrameRate equivalent for a View hierarchy. */
    private void refreshRateHint() {
        float rate = preferredFrameRate;
        Display display = getDisplay();
        if (rate > 0 && Build.VERSION.SDK_INT < 34) {
            // Before API 34 the hint has to be a display-supported rate.
            float closest = 0;
            if (display != null) {
                for (float hz : display.getSupportedRefreshRates()) {
                    if (closest == 0 || Math.abs(hz - rate) < Math.abs(closest - rate)) closest = hz;
                }
            }
            rate = closest;
        }
        if (primary != null) {
            primary.overlays.preferFrameRate(surface, rate);
            return;
        }
        Activity activity = activity();
        if (activity instanceof NsgoActivity) {
            Window window = activity.getWindow();
            WindowManager.LayoutParams params = window.getAttributes();
            if (params.preferredRefreshRate != rate) {
                params.preferredRefreshRate = rate;
                window.setAttributes(params);
            }
        } else if (Build.VERSION.SDK_INT >= 35) {
            setRequestedFrameRate(rate > 0 ? rate : REQUESTED_FRAME_RATE_CATEGORY_DEFAULT);
        }
    }

    private boolean frameIsDue(long vsync) {
        double allowance = Math.min(requestedFramePeriod / 8, 500_000);
        return lastFrameSent == 0 || vsync - lastFrameSent >= requestedFramePeriod - allowance;
    }

    private void startTicking() {
        if (ticking) return;
        ticking = true;
        Choreographer.getInstance().postFrameCallback(tick);
    }

    /**
     * Tells the app a frame began if it waits for one, steps every animation
     * once a frame, and stops when none runs.
     */
    private void tick(long now) {
        ticking = false;
        if (frameWanted && frameIsDue(now)) {
            frameWanted = false;
            lastFrameSent = now;
            // The frame's vsync time, on System.nanoTime's clock, for RequestAnimationFrame's time.
            java.nio.ByteBuffer frame = java.nio.ByteBuffer.allocate(surface == 0 ? 8 : 12).putLong(now);
            if (surface != 0) frame.putInt(surface);
            write(Protocol.FRAME, frame.array());
        }
        long begin = measuring ? System.nanoTime() : 0;
        boolean animating = animator.running();
        boolean changed = measuring && (!moving.isEmpty() || !ghosts.isEmpty());
        if (animating) animator.step(now);
        applyAnimated();
        boolean more = animator.running();
        for (Node n : new ArrayList<>(moving)) {
            if (nodes.get(n.id) != n) {
                moving.remove(n);
                continue;
            }
            if (n.motion != null) {
                boolean finished = n.motion.step(now);
                place(n, n.motion.shown.clone());
                if (finished) n.motion = null;
            }
            if (n.appear != null) {
                if (n.appear.done(now - n.appearStart)) n.appear = null;
                paint(n);
            }
            if (n.motion == null && n.appear == null) moving.remove(n);
        }
        more |= !moving.isEmpty();
        for (int i = ghosts.size() - 1; i >= 0; i--) {
            Ghost g = ghosts.get(i);
            long elapsed = now - g.start;
            float p = (float) g.transition.at(elapsed);
            if (g.transition.property == LayoutTransition.SCALE) {
                float f = Math.max(1 - p, 0.0001f);
                g.view.setScaleX(g.scaleX * f);
                g.view.setScaleY(g.scaleY * f);
            } else {
                g.view.setAlpha(Math.max(0, g.alpha * (1 - p)));
            }
            if (g.transition.done(elapsed)) {
                g.overlay.remove(g.view);
                ghostViews.remove(g.view);
                ghosts.remove(i);
            }
        }
        more |= !ghosts.isEmpty();
        if (measuring && isAttachedToWindow() && getWindowVisibility() == View.VISIBLE) {
            sampleFrame(now, begin, changed);
            more = true;
        }
        if (more || frameWanted) startTicking();
    }

    /**
     * Moves n to frame f in points; under a layout animation's Update a
     * node that showed before the batch moves there from where it shows.
     */
    /**
     * Moves n to frame f, animating from where it shows with the batch's
     * Update, else with the item's own transition when it has one.
     */
    private void setFrame(Node n, float[] f, LayoutTransition own) {
        float[] target = n.motion != null ? n.motion.target : n.frame;
        if (target != null && Arrays.equals(target, f)) return;
        LayoutTransition motion = own;
        long start = System.nanoTime();
        if (layoutUpdate != null && !created.contains(n.id)) {
            motion = layoutUpdate;
            start = batchTime;
        }
        if (motion != null && n.frame != null && n != accessoryShown) {
            if (n.motion != null) {
                n.motion.retarget(f, motion, start);
            } else {
                n.motion = LayoutTransition.Motion.start(n.frame, f, motion, start);
            }
            if (n.motion.done) {
                place(n, f);
                n.motion = null;
                return;
            }
            moving.add(n);
            startTicking();
            return;
        }
        n.motion = null;
        place(n, f);
    }

    /**
     * Moves n, which the batch created, from where old, which it deletes,
     * shows, on the batch's Update rather than appearing, and hides old at
     * once (core's ItemMatch). The views the batch created are not laid
     * out yet, so n's parent is found through their frames from the
     * nearest view that is.
     */
    private void match(Node n, Node old) {
        if (layoutUpdate == null || old == null || n.frame == null || !old.view.isAttachedToWindow()) return;
        float x = 0, y = 0;
        Node p = n.up;
        for (; p != null && created.contains(p.id) && p.frame != null; p = p.up) {
            x += p.frame[0];
            y += p.frame[1];
        }
        if (p == null || !p.children.isAttachedToWindow()) return;
        int[] in = new int[2], was = new int[2];
        p.children.getLocationInWindow(in);
        old.view.getLocationInWindow(was);
        float[] from = {(was[0] - in[0]) / density - x, (was[1] - in[1]) / density - y,
                old.view.getWidth() / density, old.view.getHeight() / density};
        old.view.setVisibility(View.INVISIBLE);
        n.appear = null;
        n.motion = LayoutTransition.Motion.start(from, n.frame, layoutUpdate, batchTime);
        place(n, from);
        paint(n);
        if (n.motion.done) {
            n.motion = null;
            return;
        }
        moving.add(n);
        startTicking();
    }

    /**
     * Applies an ItemScreen: screen n's system header, over its content,
     * holding the header views the core sized.
     */
    private void screen(Node n, DataInputStream in) throws IOException {
        if (n.header == null) {
            n.header = new Header(getContext(), density, () -> event(Protocol.EVENT_BACK, n.id, 0, 0, 0, ""));
            n.header.colors(dark());
            n.children.addView(n.header.view);
        }
        n.header.configure(in, id -> {
            Node item = nodes.get(id);
            if (item == null) return null;
            if (item.holder == null) {
                item.holder = new Header.Holder(getContext());
                detach(item.view);
                item.holder.addView(item.view);
                float[] f = item.frame;
                Header.size(item.holder, f != null ? px(f[2]) : 1, f != null ? px(f[3]) : 1);
            }
            item.headerScreen = n;
            return item.holder;
        });
        measureHeader(n);
    }

    /**
     * Measures screen n's toolbar and reports its bar height and filling
     * titleView width to the engine when either changed.
     */
    private void measureHeader(Node n) {
        if (n.header == null || n.header.view.getVisibility() != View.VISIBLE) return;
        int width = n.frame != null ? px(n.frame[2]) : getWidth();
        if (width <= 0) return;
        float bar = n.header.measure(width);
        float titleWidth = n.header.titleWidth();
        if (bar != n.header.reported || titleWidth != n.header.reportedTitleWidth) {
            n.header.reported = bar;
            n.header.reportedTitleWidth = titleWidth;
            event(Protocol.EVENT_SCREEN_HEADER, n.id, 0, 0, 0, "", titleWidth, 0, 0, 0, 0, 0, bar, null);
        }
        reportScreenInsets(n);
    }

    /** Reports EventScreenInsets for every visible screen whose effective insets changed. */
    private void reportAllScreenInsets() {
        for (int i = 0; i < nodes.size(); i++) {
            Node n = nodes.valueAt(i);
            if (n != null && n.header != null) reportScreenInsets(n);
        }
    }

    /** Reports EventScreenInsets for screen n from its bounds in the surface and its header. */
    private void reportScreenInsets(Node n) {
        if (n.header == null || n.frame == null || n.hidden || n.frame[2] <= 0 || n.frame[3] <= 0) return;
        float rw = getWidth() / density, rh = getHeight() / density;
        Node rootNode = nodes.get(rootID);
        if ((rw <= 0 || rh <= 0) && rootNode != null && rootNode.frame != null) {
            rw = rootNode.frame[2];
            rh = rootNode.frame[3];
        }
        if (rw <= 0 || rh <= 0) return;
        float ox = 0, oy = 0;
        for (Node p = n; p != null && p.id != rootID; p = p.up) {
            if (p.frame != null) {
                ox += p.frame[0];
                oy += p.frame[1];
            }
        }
        float top = Math.max(0f, safeTop / density - oy);
        float left = Math.max(0f, safeLeft / density - ox);
        float right = Math.max(0f, safeRight / density - Math.max(0f, rw - (ox + n.frame[2])));
        float bottom = Math.max(0f, safeBottom / density - Math.max(0f, rh - (oy + n.frame[3])));
        if (!n.header.hidden()) {
            if (n.header.transparent()) {
                float bar = n.header.reported > 0 ? n.header.reported : Header.BAR;
                top = top + bar;
            } else {
                top = 0f;
            }
        }
        float[] prev = n.header.reportedInsets;
        if (prev != null && prev[0] == top && prev[1] == right && prev[2] == bottom && prev[3] == left) return;
        n.header.reportedInsets = new float[] {top, right, bottom, left};
        event(Protocol.EVENT_SCREEN_INSETS, n.id, 0, top, right, "", bottom, left, 0);
    }

    /** Places n's view at frame f in points, rounding its edges to pixels. */
    private void place(Node n, float[] f) {
        n.frame = f;
        if (n.holder != null) {
            Header.size(n.holder, px(f[0] + f[2]) - px(f[0]), px(f[1] + f[3]) - px(f[1]));
            if (n.headerScreen != null && nodes.get(n.headerScreen.id) == n.headerScreen) {
                measureHeader(n.headerScreen);
            }
        }
        if (n.header != null) {
            measureHeader(n);
        }
        if (n == accessoryShown) {
            placeAccessory();
            return;
        }
        int left = px(f[0]), top = px(f[1]);
        LayoutParams lp = new LayoutParams(px(f[0] + f[2]) - left, px(f[1] + f[3]) - top, Gravity.LEFT | Gravity.TOP);
        lp.leftMargin = left;
        lp.topMargin = top;
        n.view.setLayoutParams(lp);
    }

    /** A deleted view fading or shrinking out in its parent's overlay, its node gone. */
    private static final class Ghost {
        View view;
        ViewGroupOverlay overlay;
        float alpha, scaleX, scaleY;
        LayoutTransition transition;
        long start;
    }

    /**
     * Keeps a view the batch deletes drawn where it was while it fades or
     * shrinks out under the layout animation's Delete, and reports whether
     * it does. Only the top view of a deleted subtree does; the views it
     * holds go with it.
     */
    private boolean vanish(Node n) {
        if (layoutDelete == null || !(n.view.getParent() instanceof ViewGroup) || inGhost(n.view)) return false;
        ViewGroup parent = (ViewGroup) n.view.getParent();
        if (!parent.isAttachedToWindow() || n.view.getVisibility() != VISIBLE) return false;
        Ghost g = new Ghost();
        g.view = n.view;
        g.overlay = parent.getOverlay();
        g.alpha = n.view.getAlpha();
        g.scaleX = n.view.getScaleX();
        g.scaleY = n.view.getScaleY();
        g.transition = layoutDelete;
        g.start = batchTime;
        // The overlay draws it where it was and passes it no touches.
        g.overlay.add(n.view);
        ghosts.add(g);
        ghostViews.add(n.view);
        startTicking();
        return true;
    }

    /** Whether view is inside a view fading out. */
    private boolean inGhost(View view) {
        if (ghostViews.isEmpty()) return false;
        for (ViewParent p = view.getParent(); p instanceof View; p = p.getParent()) {
            if (ghostViews.contains(p)) return true;
        }
        return false;
    }

    /**
     * Shows the accessory bar of the input with focus above the keyboard
     * while the keyboard shows, as wide as the surface; hides it otherwise.
     * The core lays accessories out but never inserts them.
     */
    private void updateAccessory() {
        Node bar = null;
        if (focusedInput != null && focusedInput.accessory != 0 && keyboard > 0) {
            bar = nodes.get(focusedInput.accessory);
        }
        if (accessoryShown != null && accessoryShown != bar && accessoryShown.view.getParent() == this) {
            removeView(accessoryShown.view);
            if (!backdropNodes.isEmpty()) updateBackdropAncestors();
        }
        accessoryShown = bar;
        keepFocusedClear();
        if (bar == null) return;
        if (bar.view.getParent() != this) {
            detach(bar.view);
            addView(bar.view);
            if (!backdropNodes.isEmpty()) updateBackdropAncestors();
        }
        placeAccessory();
    }

    /** The scroll view keeping the focused field clear of the keyboard. */
    private Scrolling.VScroll keyboardScroll;

    /**
     * Keeps the focused field clear of the keyboard and the accessory bar
     * above it, through the innermost vertical scroll view around it.
     */
    private void keepFocusedClear() {
        View field = focusedInput != null && keyboard > 0 ? focusedInput.fieldHost() : null;
        Scrolling.VScroll scroll = null;
        for (android.view.ViewParent p = field != null ? field.getParent() : null; p != null; p = p.getParent()) {
            if (p instanceof Scrolling.VScroll) {
                scroll = (Scrolling.VScroll) p;
                break;
            }
        }
        if (keyboardScroll != null && keyboardScroll != scroll) keyboardScroll.d.keepClear(null, 0);
        keyboardScroll = scroll;
        if (scroll == null) return;
        int[] at = new int[2];
        getLocationInWindow(at);
        int bar = accessoryShown != null && accessoryShown.frame != null ? px(accessoryShown.frame[3]) : 0;
        scroll.d.keepClear(field, at[1] + getHeight() - keyboard - bar);
    }

    /** Places the accessory bar on the keyboard: its top keyboard pixels plus its height above the bottom. */
    private void placeAccessory() {
        Node bar = accessoryShown;
        int height = bar.frame != null ? px(bar.frame[3]) : 0;
        LayoutParams lp = new LayoutParams(getWidth(), height, Gravity.LEFT | Gravity.TOP);
        lp.topMargin = getHeight() - keyboard - height;
        bar.view.setLayoutParams(lp);
    }

    /**
     * Applies an ItemNative: the first one names the View class to build
     * by reflection, which fills the node's view; each sets the props that
     * changed through their setters. An empty class keeps the view.
     */
    private void embed(Node n, DataInputStream in) throws IOException {
        String className = Protocol.readString(in);
        if (!className.isEmpty() && !n.embedTried) {
            n.embedTried = true;
            Embedded.Emitter emit = (name, args) ->
                    event(Protocol.EVENT_NATIVE, n.id, 0, 0, 0, name, 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, args,
                            false);
            if (className.startsWith("host:")) {
                // A component of the embedding app's own: it emits one value.
                n.embedded = hostComponent(className.substring(5), (name, data) -> emit.emit(name, Host.json(data)));
            } else {
                n.embedded = Embedded.build(getContext(), className, emit);
            }
            if (n.embedded != null) {
                n.children.addView(n.embedded.view, new LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT));
            }
        }
        for (int i = 0, count = in.readInt(); i < count; i++) {
            String name = Protocol.readString(in);
            Object value = Embedded.parse(Protocol.readString(in));
            if (n.embedded != null) n.embedded.setProp(name, value);
        }
        if (Protocol.flag(in.readUnsignedByte(), 0)) {
            // Animated props, driven as a canvas's arguments are.
            int count = in.readInt();
            n.bound = new Embedded.Bound(count);
            int[] nodes = new int[count];
            for (int i = 0; i < count; i++) {
                n.bound.names[i] = Protocol.readString(in);
                nodes[i] = in.readInt();
                n.bound.colors[i] = Protocol.flag(in.readUnsignedByte(), 0);
            }
            animator.bindCanvas(n.id, nodes);
            applyAnimated();
        }
        nativeSize(n);
    }

    /** Reports a native view's natural size when it changed, for the element's open dimensions. */
    private void nativeSize(Node n) {
        if (n.embedded == null) return;
        int[] size = n.embedded.measure();
        if (size[0] == n.embedded.width && size[1] == n.embedded.height) return;
        n.embedded.width = size[0];
        n.embedded.height = size[1];
        event(Protocol.EVENT_NATIVE_SIZE, n.id, 0, 0, 0, "", size[0] / density, size[1] / density, 0);
    }

    static void detach(View view) {
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

    /**
     * Lays the view and the views inside it out in the app's direction, and
     * an activity of NSGo's own, its window too, which the system's back
     * animation follows. A host's activity keeps its own.
     */
    private void applyDirection() {
        if (direction < 0) return;
        setLayoutDirection(direction);
        Activity activity = activity();
        if (activity instanceof NsgoActivity && activity.getWindow() != null) {
            activity.getWindow().getDecorView().setLayoutDirection(direction);
        }
    }

    /**
     * Applies a standalone app's forced language to Android 13+'s per-app
     * locale so system text selection menus and dialogs follow SetLocale.
     * Embedded hosts own their own languages.
     */
    private void applyForcedLanguage() {
        if (!hasForcedLanguage || primary != null || !(activity() instanceof NsgoActivity)) return;
        AppLocale.pin(getContext(), forcedLanguage);
    }

    Activity activity() {
        for (Context c = getContext(); c instanceof ContextWrapper; c = ((ContextWrapper) c).getBaseContext()) {
            if (c instanceof Activity) return (Activity) c;
        }
        return null;
    }

    // The wire rectangles are in surface points. Keep them so resize and
    // display changes can recalculate the clipped pixel rectangles.
    private List<RectF> gestureRects = Collections.emptyList();
    private List<Rect> appliedGestureRects = Collections.emptyList();

    private void readGestureRects(DataInputStream in) throws IOException {
        int count = in.readInt();
        ArrayList<RectF> rects = new ArrayList<>();
        for (int i = 0; i < count; i++) {
            float x = in.readFloat(), y = in.readFloat();
            float width = in.readFloat(), height = in.readFloat();
            rects.add(new RectF(x, y, x + width, y + height));
        }
        gestureRects = rects;
        updateGestureRects();
    }

    private void updateGestureRects() {
        if (Build.VERSION.SDK_INT < 29 || (gestureRects.isEmpty() && appliedGestureRects.isEmpty())) return;
        List<Rect> pixels = new ArrayList<>();
        if (attached || primary != null) {
            float scale = getResources().getDisplayMetrics().density;
            for (RectF area : gestureRects) {
                Rect rect = new Rect((int) Math.floor(area.left * scale), (int) Math.floor(area.top * scale),
                        (int) Math.ceil(area.right * scale), (int) Math.ceil(area.bottom * scale));
                if (rect.intersect(0, 0, getWidth(), getHeight())) pixels.add(rect);
            }
        }
        if (!pixels.equals(appliedGestureRects)) {
            setSystemGestureExclusionRects(pixels);
            appliedGestureRects = pixels;
        }
    }

    /**
     * Lays the view out behind the system bars and draws their icons to
     * contrast with the status bar style or, for Auto, the appearance; and
     * hides the status bar or enters immersive mode when asked.
     */
    private void systemBars() {
        // The system bars are the activity's window's: an overlay's sets none.
        if (primary != null) return;
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
            int status = WindowInsets.Type.statusBars();
            int system = WindowInsets.Type.systemBars();
            if (immersive) {
                bars.hide(system);
            } else {
                if (wasImmersive) bars.show(system & ~status);
                if (statusBarHidden) bars.hide(status);
                else bars.show(status);
            }
            wasImmersive = immersive;
            return;
        }
        int flags = SYSTEM_UI_FLAG_LAYOUT_STABLE | SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                | SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION;
        if (darkIcons) flags |= SYSTEM_UI_FLAG_LIGHT_STATUS_BAR;
        if (!dark) flags |= SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR;
        if (immersive) flags |= SYSTEM_UI_FLAG_FULLSCREEN | SYSTEM_UI_FLAG_HIDE_NAVIGATION | SYSTEM_UI_FLAG_IMMERSIVE_STICKY;
        else if (statusBarHidden) flags |= SYSTEM_UI_FLAG_FULLSCREEN;
        getRootView().setSystemUiVisibility(flags);
    }

    /**
     * Plays core.Haptic {@code h} as its nearest haptic feedback constant,
     * which the view plays where the user's touch feedback setting allows:
     * CONFIRM and REJECT from API 30, SEGMENT_TICK from 34, older ones
     * before them.
     */
    private void haptic(int h) {
        boolean api30 = Build.VERSION.SDK_INT >= 30;
        int feedback;
        switch (h) {
            case Protocol.HAPTIC_SELECTION:
                feedback = Build.VERSION.SDK_INT >= 34 ? HapticFeedbackConstants.SEGMENT_TICK : HapticFeedbackConstants.CLOCK_TICK;
                break;
            case Protocol.HAPTIC_LIGHT: feedback = HapticFeedbackConstants.KEYBOARD_TAP; break;
            case Protocol.HAPTIC_MEDIUM: feedback = HapticFeedbackConstants.VIRTUAL_KEY; break;
            case Protocol.HAPTIC_HEAVY: feedback = HapticFeedbackConstants.LONG_PRESS; break;
            case Protocol.HAPTIC_SOFT: feedback = HapticFeedbackConstants.CLOCK_TICK; break;
            case Protocol.HAPTIC_RIGID: feedback = HapticFeedbackConstants.CONTEXT_CLICK; break;
            case Protocol.HAPTIC_SUCCESS:
                feedback = api30 ? HapticFeedbackConstants.CONFIRM : HapticFeedbackConstants.VIRTUAL_KEY;
                break;
            // A warning has no constant of its own.
            case Protocol.HAPTIC_WARNING: feedback = HapticFeedbackConstants.LONG_PRESS; break;
            case Protocol.HAPTIC_ERROR:
                feedback = api30 ? HapticFeedbackConstants.REJECT : HapticFeedbackConstants.LONG_PRESS;
                break;
            default: return;
        }
        performHapticFeedback(feedback);
    }

    /** Returns the enrolled biometric kind, without showing a prompt. */
    private int biometricAvailability() {
        if (Build.VERSION.SDK_INT < 28
                || getContext().checkSelfPermission("android.permission.USE_BIOMETRIC") != PackageManager.PERMISSION_GRANTED) {
            return Protocol.BIOMETRIC_NONE;
        }
        PackageManager pm = getContext().getPackageManager();
        if (Build.VERSION.SDK_INT >= 29) {
            BiometricManager manager = (BiometricManager) getContext().getSystemService(Context.BIOMETRIC_SERVICE);
            if (manager == null) return Protocol.BIOMETRIC_NONE;
            int state = Build.VERSION.SDK_INT >= 30
                    ? manager.canAuthenticate(BiometricManager.Authenticators.BIOMETRIC_STRONG)
                    : manager.canAuthenticate();
            if (state != BiometricManager.BIOMETRIC_SUCCESS) return Protocol.BIOMETRIC_NONE;
            // canAuthenticate does not say which kind is enrolled and FEATURE_FACE only says the hardware is there, so an enrolled fingerprint answers first; face is the best guess for a device with face hardware and no fingerprint enrolled.
            boolean fingerprint = pm.hasSystemFeature(PackageManager.FEATURE_FINGERPRINT);
            if (fingerprint && hasEnrolledFingerprints()) return Protocol.BIOMETRIC_FINGERPRINT;
            if (pm.hasSystemFeature(PackageManager.FEATURE_FACE)) return Protocol.BIOMETRIC_FACE;
            return fingerprint ? Protocol.BIOMETRIC_FINGERPRINT : Protocol.BIOMETRIC_NONE;
        }
        if (!pm.hasSystemFeature(PackageManager.FEATURE_FINGERPRINT)) return Protocol.BIOMETRIC_NONE;
        return hasEnrolledFingerprints() ? Protocol.BIOMETRIC_FINGERPRINT : Protocol.BIOMETRIC_NONE;
    }

    /** Says whether the device has a fingerprint enrolled, which FingerprintManager answers for USE_BIOMETRIC too. */
    private boolean hasEnrolledFingerprints() {
        android.hardware.fingerprint.FingerprintManager manager =
                (android.hardware.fingerprint.FingerprintManager) getContext().getSystemService(Context.FINGERPRINT_SERVICE);
        try {
            return manager != null && manager.isHardwareDetected() && manager.hasEnrolledFingerprints();
        } catch (SecurityException e) {
            return false;
        }
    }

    /**
     * Shows Android's biometric prompt and reports its result once: success, a cancel by the user, the
     * system or the prompt's button, unavailable when the hardware, the enrolment or a lockout stops it,
     * else a failure. The framework calls only the button's listener when the user presses it, not
     * onAuthenticationError, so the listener answers too.
     */
    private void authenticateBiometric(long count, String reason) {
        Activity activity = activity();
        if (activity == null || biometricAvailability() == Protocol.BIOMETRIC_NONE) {
            event(Protocol.EVENT_BIOMETRIC, 0, count, 0, 0, "", 0, 0, Protocol.BIOMETRIC_UNAVAILABLE);
            return;
        }
        java.util.concurrent.Executor main = getContext().getMainExecutor();
        java.util.concurrent.atomic.AtomicBoolean answered = new java.util.concurrent.atomic.AtomicBoolean();
        java.util.function.IntConsumer answer = result -> {
            if (answered.compareAndSet(false, true)) {
                event(Protocol.EVENT_BIOMETRIC, 0, count, 0, 0, "", 0, 0, result);
            }
        };
        BiometricPrompt.AuthenticationCallback callback = new BiometricPrompt.AuthenticationCallback() {
            @Override public void onAuthenticationSucceeded(BiometricPrompt.AuthenticationResult result) {
                answer.accept(Protocol.BIOMETRIC_SUCCESS);
            }
            @Override public void onAuthenticationError(int errorCode, CharSequence message) {
                switch (errorCode) {
                    case BiometricPrompt.BIOMETRIC_ERROR_USER_CANCELED:
                    case BiometricPrompt.BIOMETRIC_ERROR_CANCELED:
                    case 13: // ERROR_NEGATIVE_BUTTON, as androidx numbers it
                        answer.accept(Protocol.BIOMETRIC_CANCELLED);
                        break;
                    case BiometricPrompt.BIOMETRIC_ERROR_HW_UNAVAILABLE:
                    case BiometricPrompt.BIOMETRIC_ERROR_LOCKOUT:
                    case BiometricPrompt.BIOMETRIC_ERROR_LOCKOUT_PERMANENT:
                    case BiometricPrompt.BIOMETRIC_ERROR_NO_BIOMETRICS:
                    case BiometricPrompt.BIOMETRIC_ERROR_HW_NOT_PRESENT:
                        answer.accept(Protocol.BIOMETRIC_UNAVAILABLE);
                        break;
                    default:
                        answer.accept(Protocol.BIOMETRIC_FAILURE);
                }
            }
        };
        BiometricPrompt prompt = new BiometricPrompt.Builder(activity)
                .setTitle(getContext().getApplicationInfo().loadLabel(getContext().getPackageManager()))
                .setDescription(reason)
                .setNegativeButton(getContext().getString(android.R.string.cancel), main,
                        (dialog, which) -> answer.accept(Protocol.BIOMETRIC_CANCELLED))
                .build();
        prompt.authenticate(new CancellationSignal(), main, callback);
    }

    /** The notification permission requests waiting for the system's prompt. */
    private final java.util.ArrayList<Long> permissionAsks = new java.util.ArrayList<>();
    /** The web pages waiting for the microphone's and the camera's prompt, which one request answers. */
    private final java.util.ArrayList<java.util.function.Consumer<Boolean>> mediaAsks = new java.util.ArrayList<>();
    static final int MEDIA_PERMISSION_REQUEST = 0x6e7a;

    /** Asks for a web page's devices; a page asking while a prompt shows waits for its answer, then asks again for what it still lacks. */
    private void askMediaPermissions(String[] permissions, java.util.function.Consumer<Boolean> answer) {
        Activity activity = activity();
        if (activity == null) {
            answer.accept(false);
            return;
        }
        if (!mediaAsks.isEmpty()) {
            mediaAsks.add(allowed -> {
                for (String p : permissions) {
                    if (getContext().checkSelfPermission(p) != android.content.pm.PackageManager.PERMISSION_GRANTED) {
                        askMediaPermissions(permissions, answer);
                        return;
                    }
                }
                answer.accept(true);
            });
            return;
        }
        mediaAsks.add(answer);
        activity.requestPermissions(permissions, MEDIA_PERMISSION_REQUEST);
    }

    private void answerPermission(long count) {
        // A permission callback may immediately start a Live Activity. Its cached availability must be updated before the callback can run.
        write(Protocol.LIVE_REPORT, LiveUpdates.state(getContext()));
        event(Protocol.EVENT_NOTIFICATION, 0, count, 0, 0, "", 0, 0, Notifications.permission(getContext()));
    }

    /** Lazily creates the primary view's system permission controller. */
    private Permissions systemPermissions() {
        if (systemPermissions == null) {
            systemPermissions = new Permissions(new Permissions.Host() {
                @Override public int sdkInt() {
                    return Build.VERSION.SDK_INT;
                }

                @Override public boolean granted(String permission) {
                    return getContext().checkSelfPermission(permission) == PackageManager.PERMISSION_GRANTED;
                }

                @Override public boolean declared(String permission) {
                    try {
                        String[] requested = getContext().getPackageManager()
                                .getPackageInfo(getContext().getPackageName(), PackageManager.GET_PERMISSIONS)
                                .requestedPermissions;
                        return requested != null && java.util.Arrays.asList(requested).contains(permission);
                    } catch (PackageManager.NameNotFoundException e) {
                        return false;
                    }
                }

                @Override public boolean asked(String key) {
                    return getContext().getSharedPreferences(Permissions.PREFS, Context.MODE_PRIVATE)
                            .getBoolean(key, false);
                }

                @Override public void markAsked(String key) {
                    getContext().getSharedPreferences(Permissions.PREFS, Context.MODE_PRIVATE)
                            .edit().putBoolean(key, true).apply();
                }

                @Override public boolean request(String[] permissions) {
                    Activity activity = activity();
                    if (activity == null) return false;
                    try {
                        activity.requestPermissions(permissions, Permissions.PERMISSION_REQUEST);
                        return true;
                    } catch (RuntimeException e) {
                        Log.w(TAG, "requestPermissions failed", e);
                        return false;
                    }
                }

                @Override public boolean openSettings() {
                    Activity activity = activity();
                    Context context = activity != null ? activity : getContext();
                    Intent intent = new Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                            android.net.Uri.fromParts("package", context.getPackageName(), null));
                    if (activity == null) intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
                    try {
                        context.startActivity(intent);
                        return true;
                    } catch (RuntimeException e) {
                        Log.w(TAG, "the permission's setting does not open", e);
                        return false;
                    }
                }

                @Override public void warn(String message) {
                    Log.w(TAG, message);
                }
            });
        }
        return systemPermissions;
    }

    /**
     * Reports the press of a Live Activity's action, which brought the
     * activity back or started it: the activity passes its intent here.
     * Package widget keeps it until the app hears presses.
     */
    public void liveActionPressed(android.content.Intent intent) {
        byte[] press = intent == null ? null : LiveUpdates.press(intent);
        if (press == null) return;
        if (engine == null || !engine.isAlive()) {
            livePressEarly = press;
            return;
        }
        write(Protocol.LIVE_REPORT, press);
    }

    /** A Live Activity's press that came before the app's process started. */
    private byte[] livePressEarly;

    // Package microphone's recordings (RecordingService), on the UI thread: the starts and the authorizes waiting for RECORD_AUDIO's prompt, whether it shows, and the state last sent.
    private final ArrayList<JSONObject> recordingStarts = new ArrayList<>();
    private final ArrayList<Long> recordingAsks = new ArrayList<>();
    private boolean recordingAsking;
    private String recordingSent;

    /**
     * Takes a COOKIES message on the engine's reader thread and carries it out in CookieManager on the UI thread, whose looper its callbacks need: sets its cookies and flushes them to disk, reads the Cookie header of its URL, or removes every cookie and WebStorage's data, and answers with COOKIES_SET: the request's surface, count, an error, "" for none, and op, with the URL and the header for a list. CookieManager is the store every WebView of the app reads, so it needs no web view.
     */
    private void cookies(DataInputStream in) throws IOException {
        int surface = in.readInt(), count = in.readInt(), op = in.readByte();
        String[] urls = new String[0], values = new String[0];
        if (op == Protocol.COOKIES_LIST) {
            urls = new String[] {Protocol.readString(in)};
        } else if (op == Protocol.COOKIES_ADD) {
            urls = new String[in.readInt()];
            values = new String[urls.length];
            for (int i = 0; i < urls.length; i++) {
                urls[i] = Protocol.readString(in);
                values[i] = Protocol.readString(in);
            }
        }
        String[] u = urls, v = values;
        ui.post(() -> {
            CookieManager manager;
            try {
                manager = CookieManager.getInstance();
            } catch (RuntimeException e) {
                // Without a WebView provider, getInstance throws.
                cookiesSet(surface, count, "Android has no WebView: " + e.getMessage(), op, op == Protocol.COOKIES_LIST ? u[0] : "", "");
                return;
            }
            if (op == Protocol.COOKIES_LIST) {
                cookiesSet(surface, count, "", op, u[0], Objects.toString(manager.getCookie(u[0]), ""));
                return;
            }
            if (op == Protocol.COOKIES_CLEAR) {
                manager.removeAllCookies(ok -> {
                    manager.flush();
                    WebStorage.getInstance().deleteAllData();
                    cookiesSet(surface, count, "", op, "", "");
                });
                return;
            }
            int[] pending = {u.length};
            String[] failure = {""};
            for (int i = 0; i < u.length; i++) {
                String value = v[i];
                manager.setCookie(u[i], value, ok -> {
                    if (!Boolean.TRUE.equals(ok) && failure[0].isEmpty()) {
                        // The name alone: the value may be a credential.
                        failure[0] = "Android's CookieManager refused the cookie " + value.substring(0, value.indexOf('='));
                    }
                    if (--pending[0] == 0) {
                        manager.flush();
                        cookiesSet(surface, count, failure[0], op, "", "");
                    }
                });
            }
        });
    }

    /** The code scanner on screen, on the UI thread; null without one. */
    private Scanner scanner;

    /** Takes a SCAN message on the engine's reader thread: starts a scanner for the request, closing one that shows, or stops the one Go found a code with. */
    private void scan(DataInputStream in) throws IOException {
        int surface = in.readInt(), count = in.readInt(), op = in.readByte();
        ui.post(() -> {
            if (op == Protocol.SCAN_STOP) {
                if (scanner != null && scanner.answers(surface, count)) {
                    scanner.stop();
                    scanner = null;
                }
                return;
            }
            if (scanner != null) scanner.close();
            scanner = new Scanner(this, surface, count, () -> scanner = null);
            scanner.start(activity());
        });
    }

    /** Sends what the scanner of surface's request count reports: what, a Protocol.SCAN_* report, then body's. */
    void scanReport(int surface, int count, int what, Protocol.Body body) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream out = new DataOutputStream(bytes);
        try {
            out.writeInt(surface);
            out.writeInt(count);
            out.writeByte(what);
            if (body != null) body.write(out);
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        write(Protocol.SCAN_REPORT, bytes.toByteArray());
    }

    private void cookiesSet(int surface, int count, String error, int op, String url, String header) {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream out = new DataOutputStream(bytes);
        try {
            out.writeInt(surface);
            out.writeInt(count);
            Protocol.writeString(out, error);
            out.writeByte(op);
            if (op == Protocol.COOKIES_LIST) {
                Protocol.writeString(out, url);
                Protocol.writeString(out, header);
            }
        } catch (IOException impossible) {
            throw new AssertionError(impossible);
        }
        write(Protocol.COOKIES_SET, bytes.toByteArray());
    }

    /** Answers an EXT_GAME_SERVICES command, whose JSON follows the kind byte in payload: this runtime has no Play Games Services hook, so a command's request hears that game services are unavailable rather than waiting for a report. */
    private void gameServicesUnavailable(byte[] payload) {
        long id = 0;
        try {
            id = new JSONObject(new String(payload, 1, payload.length - 1, java.nio.charset.StandardCharsets.UTF_8)).optLong("id");
        } catch (JSONException e) {
            Log.w(TAG, "invalid game services message from app");
        }
        if (id == 0) return;
        try {
            JSONObject report = new JSONObject().put("type", "result").put("id", id).put("error", "unavailable");
            write(Protocol.GAME_SERVICES_REPORT, report.toString().getBytes(java.nio.charset.StandardCharsets.UTF_8));
        } catch (JSONException impossible) {
            throw new AssertionError(impossible);
        }
    }

    /** Takes a RECORDING message, on the engine's reader thread: a stop and the app's taking of an earlier recording go to the service at once, a start and an authorize to the UI thread. */
    private void recording(byte[] payload) {
        JSONObject m;
        try {
            m = new JSONObject(new String(payload, java.nio.charset.StandardCharsets.UTF_8));
        } catch (JSONException e) {
            Log.w(TAG, "invalid recording message from app");
            return;
        }
        long id = m.optLong("id");
        switch (m.optString("op")) {
            case "stop":
                RecordingService.stop(engine, id);
                break;
            case "start":
                ui.post(() -> recordingStart(m));
                break;
            case "authorize":
                ui.post(() -> recordingAuthorize(id));
                break;
            case "taken":
                RecordingService.taken(m.optString("path"));
                break;
            default:
        }
    }

    /**
     * Starts a recording, on the UI thread: only for an app that declares the service, while an activity of the app is visible, as Android lets only a foreground app start using the microphone, and once RECORD_AUDIO is granted, which it asks for first. From Android 13 it asks for POST_NOTIFICATIONS with it, or alone the first time, as without it the service's notification does not show; a refusal still records.
     */
    private void recordingStart(JSONObject m) {
        long id = m.optLong("id");
        String refusal = recordingRefusal();
        if (refusal != null) {
            write(Protocol.RECORDING_REPORT, RecordingService.refusal("ended", id, "unavailable", refusal));
        } else if (RecordingService.granted(getContext()) && !Notifications.needsPrompt(getContext())) {
            RecordingService.start(getContext().getApplicationContext(), engine, m);
        } else {
            recordingStarts.add(m);
            askRecording();
        }
    }

    /** Answers an authorize, on the UI thread: at once if the app holds RECORD_AUDIO or cannot ask, else once the user answers. */
    private void recordingAuthorize(long id) {
        String refusal = recordingRefusal();
        if (RecordingService.granted(getContext())) {
            write(Protocol.RECORDING_REPORT, RecordingService.refusal("authorized", id, "", ""));
        } else if (refusal != null) {
            write(Protocol.RECORDING_REPORT, RecordingService.refusal("authorized", id, "unavailable", refusal));
        } else {
            recordingAsks.add(id);
            askRecording();
        }
    }

    /** Why the app cannot ask for the microphone or start recording now, or null. */
    private String recordingRefusal() {
        if (!RecordingService.declared(getContext())) return "the app declares no recording service (nsgo.yaml's android.recording)";
        Activity activity = activity();
        if (activity == null || activity.isFinishing() || activity.isDestroyed()
                || activity.getWindow().getDecorView().getWindowVisibility() != View.VISIBLE) {
            return "no activity of the app is visible";
        }
        return null;
    }

    /** Asks for RECORD_AUDIO unless it is granted, with POST_NOTIFICATIONS where the recording's notification needs it and it was never asked. */
    private void askRecording() {
        if (recordingAsking) return;
        ArrayList<String> asked = new ArrayList<>();
        if (!RecordingService.granted(getContext())) asked.add(RecordingService.PERMISSION);
        if (Notifications.needsPrompt(getContext())) asked.add(Notifications.PERMISSION);
        if (asked.isEmpty()) {
            recordingPermission(RecordingService.granted(getContext()));
            return;
        }
        recordingAsking = true;
        activity().requestPermissions(asked.toArray(new String[0]), RecordingService.PERMISSION_REQUEST);
    }

    /** Hears RECORD_AUDIO's prompt answered, or POST_NOTIFICATIONS' alone: the authorizes and the starts waiting for it go on, whatever the user said of notifications. */
    private void recordingPermission(boolean granted) {
        recordingAsking = false;
        recordingState();
        for (long id : recordingAsks) {
            write(Protocol.RECORDING_REPORT, RecordingService.refusal("authorized", id, granted ? "" : "denied", ""));
        }
        recordingAsks.clear();
        ArrayList<JSONObject> starts = new ArrayList<>(recordingStarts);
        recordingStarts.clear();
        for (JSONObject m : starts) {
            long id = m.optLong("id");
            String refusal = recordingRefusal();
            if (!granted) {
                write(Protocol.RECORDING_REPORT, RecordingService.refusal("ended", id, "denied", ""));
            } else if (refusal != null) {
                write(Protocol.RECORDING_REPORT, RecordingService.refusal("ended", id, "unavailable", refusal));
            } else {
                RecordingService.start(getContext().getApplicationContext(), engine, m);
            }
        }
    }

    /** Tells an app that records whether it does and may, when that changed. */
    private void recordingState() {
        if (engine == null || !engine.isAlive() || !RecordingService.declared(getContext())) return;
        byte[] state = RecordingService.state(getContext());
        String sent = new String(state, java.nio.charset.StandardCharsets.UTF_8);
        if (sent.equals(recordingSent)) return;
        recordingSent = sent;
        write(Protocol.RECORDING_REPORT, state);
    }

    /** A notification's press that came before the engine mounted its first batch. */
    private String pressedEarly;
    private boolean mounted;

    /**
     * Reports that the user pressed the notification called id, which
     * brought the activity back or started it: the activity passes the
     * intent's extra here.
     */
    public void notificationPressed(String id) {
        if (id == null) return;
        if (!mounted) {
            pressedEarly = id;
            return;
        }
        event(Protocol.EVENT_NOTIFICATION_PRESS, 0, 0, 0, 0, id);
    }

    /** The last accepted invocation, available even before the first mount. */
    private String openedURL = "";
    private final ArrayList<String> linksEarly = new ArrayList<>();
    private final ArrayList<String> sharesEarly = new ArrayList<>();

    /** Reports a URL the host accepted from Android, an embedded host's forwarded links included: the active authentication callback takes it first, else it holds its open event until the first batch while starting. */
    public void openLink(String link) {
        if (link == null || link.isEmpty()) return;
        if (AuthSession.redirect(this, link)) return;
        openedURL = link;
        // On a warm link, update the tracked environment before OnOpenLinks. On a cold link, the initial environment carries it before rendering.
        environment();
        if (!mounted) {
            linksEarly.add(link);
            return;
        }
        event(Protocol.EVENT_OPEN_LINKS, 0, 0, 0, 0, link);
    }

    /** The two-phase authentication reply: redirect, or error and callback/message. */
    void authSessionEvent(long id, int phase, String text, String data) {
        event(Protocol.EVENT_AUTH_SESSION, 0, id, 0, 0, text, 0, 0, phase,
                0, 0, 0, 0, null, 0, 0, false, data, false);
    }

    /** Gives one incoming share to the engine after it has started. */
    void receiveShare(String items) {
        if (!mounted) {
            sharesEarly.add(items);
            return;
        }
        event(Protocol.EVENT_SHARED, 0, 0, 0, 0, items);
    }

    /** The input modality last reported (modality). */
    private int modality;

    /**
     * Reports what the user gives input with, a key or a finger, before
     * the focus moves it causes, as it changes: the engine shows the
     * focus by it.
     */
    private void modality(int m) {
        if (modality == m) return;
        modality = m;
        event(Protocol.EVENT_INPUT_MODALITY, 0, m, 0, 0, "");
    }

    /**
     * Consumes a key press matching a keymap binding, named with "mod" for
     * Control or with the modifiers as they are, and reports it as EventKey.
     */
    @Override
    public boolean dispatchKeyEvent(KeyEvent e) {
        if (!inputGate.allowInput()) return true;
        if (gamepads != null && gamepads.onKeyEvent(e)) return true;
        mouse.keyEvent(e);
        // A key the window delivers is the keyboard's input. An IME's action or committed Return, which a field offers to keyBinding, leaves the modality, as a software Return does on iOS.
        if (e.getAction() == KeyEvent.ACTION_DOWN && !KeyEvent.isModifierKey(e.getKeyCode())) modality(Protocol.MODALITY_KEYBOARD);
        if (!deliveringHardwareContinuation && e.isFromSource(android.view.InputDevice.SOURCE_KEYBOARD)
                && (e.getAction() == KeyEvent.ACTION_DOWN || e.getAction() == KeyEvent.ACTION_UP)) {
            int scan = e.getScanCode(), code = e.getKeyCode();
            long identity = ((long) e.getDeviceId() << 32) | (scan != 0 ? scan : 0x10000 | code);
            HardwareKeys.Event key = new HardwareKeys.Event(identity, scan, code, physicalKeyName(e), modifiers(e.getMetaState()),
                    e.getAction() == KeyEvent.ACTION_DOWN, e.getRepeatCount() > 0, new KeyEvent(e));
            if (hardwareKeys.dispatch(key)) return true;
        }
        return continueHardwareKey(e);
    }

    private boolean continueHardwareKey(KeyEvent event) {
        if (keyBinding(event) || activateKey(event) || super.dispatchKeyEvent(event)) return true;
        // ViewRoot searches for the next focus after an unhandled key; a key that waited for Go's answer comes back through the Activity after that dispatch has ended, so it searches here, with Android's own focus search.
        if (deliveringHardwareContinuation && event.getAction() == KeyEvent.ACTION_DOWN &&
                !event.isCtrlPressed() && !event.isAltPressed() && !event.isMetaPressed()) {
            int direction = 0;
            switch (event.getKeyCode()) {
                case KeyEvent.KEYCODE_TAB: direction = event.isShiftPressed() ? View.FOCUS_BACKWARD : View.FOCUS_FORWARD; break;
                case KeyEvent.KEYCODE_DPAD_LEFT: if (!event.isShiftPressed()) direction = View.FOCUS_LEFT; break;
                case KeyEvent.KEYCODE_DPAD_RIGHT: if (!event.isShiftPressed()) direction = View.FOCUS_RIGHT; break;
                case KeyEvent.KEYCODE_DPAD_UP: if (!event.isShiftPressed()) direction = View.FOCUS_UP; break;
                case KeyEvent.KEYCODE_DPAD_DOWN: if (!event.isShiftPressed()) direction = View.FOCUS_DOWN; break;
                default: break;
            }
            if (direction != 0) {
                View focused = findFocus();
                // Real hardware leaves touch mode before dispatch; a delayed continuation, and the test keyboard, must leave it before the search, or Android filters out the other plain controls.
                if (focused != null && focused.isInTouchMode()) focused.requestFocusFromTouch();
                View next = (focused == null ? this : focused).focusSearch(direction);
                if (next != null && next != focused) {
                    return next.isInTouchMode() ? next.requestFocusFromTouch() : next.requestFocus(direction);
                }
            }
        }
        return false;
    }

    @Override
    public boolean dispatchKeyShortcutEvent(KeyEvent e) {
        if (!inputGate.allowInput()) return true;
        return super.dispatchKeyShortcutEvent(e);
    }

    @Override
    public boolean dispatchGenericMotionEvent(MotionEvent e) {
        if (!inputGate.allowInput()) return true;
        if (gamepads != null && gamepads.onGenericMotionEvent(e)) return true;
        return super.dispatchGenericMotionEvent(e);
    }

    /** Whether the app follows held keys (core.KeysDown), from ItemKeymap. */
    private boolean reportHeldKeys;

    private static String physicalKeyName(KeyEvent e) {
        switch (e.getKeyCode()) {
            case KeyEvent.KEYCODE_CTRL_LEFT: case KeyEvent.KEYCODE_CTRL_RIGHT: return "ctrl";
            case KeyEvent.KEYCODE_ALT_LEFT: case KeyEvent.KEYCODE_ALT_RIGHT: return "alt";
            case KeyEvent.KEYCODE_SHIFT_LEFT: case KeyEvent.KEYCODE_SHIFT_RIGHT: return "shift";
            case KeyEvent.KEYCODE_META_LEFT: case KeyEvent.KEYCODE_META_RIGHT: return "meta";
            default: break;
        }
        int c = e.getUnicodeChar(0);
        if (c > ' ' && (c & KeyCharacterMap.COMBINING_ACCENT) == 0) {
            return new String(Character.toChars(c)).toLowerCase(Locale.ROOT);
        }
        String name = keyName(e);
        return name == null ? "" : name;
    }

    /** Lets the Android test agent offer a Control shortcut to the mounted keymap before native editor shortcuts. */
    boolean keyBindingShortcut(KeyEvent e) {
        return keyBinding(e);
    }

    /** Keys already used to activate a control: repeats and releases cannot activate it again. */
    private final java.util.Set<Integer> activationKeys = new java.util.HashSet<>();

    /** Return and Space activate the focused pressable after the keymap, preserving every modifier. */
    private boolean activateKey(KeyEvent e) {
        int code = e.getKeyCode();
        if (e.getAction() == KeyEvent.ACTION_UP) return activationKeys.remove(code);
        if (e.getAction() != KeyEvent.ACTION_DOWN) return false;
        if (activationKeys.contains(code)) return true;
        if (code != KeyEvent.KEYCODE_ENTER && code != KeyEvent.KEYCODE_NUMPAD_ENTER && code != KeyEvent.KEYCODE_SPACE) return false;
        View focused = findFocus();
        Node n = focused == null ? null : byView.get(focused);
        if (n == null || n.field != null || !focusStop(n) || !pressable(n) || !inFocusScope(focused)) return false;
        activationKeys.add(code);
        if (e.getRepeatCount() == 0) {
            int mods = modifiers(e.getMetaState());
            for (int kind : new int[]{Protocol.EVENT_PRESS_DOWN, Protocol.EVENT_PRESS_UP, Protocol.EVENT_PRESS}) {
                if (n.has(kind)) event(kind, n.id, 1, 0, 0, "", 0, 0, -1, 0, 0, 0, 0, null, 0, 0, false, "",
                        false, 0, 0, mods);
            }
        }
        return true;
    }

    private static boolean pressable(Node n) {
        return n.has(Protocol.EVENT_PRESS) || n.has(Protocol.EVENT_LONG_PRESS)
                || n.has(Protocol.EVENT_PRESS_DOWN) || n.has(Protocol.EVENT_PRESS_UP);
    }

    private static boolean focusStop(Node n) {
        return !n.disabled && !n.noFocus && (n.focusable || pressable(n));
    }

    /** The newest visible modal bounds keyboard focus as it bounds touch input. */
    private View focusModal(View view) {
        if (view == null || !view.isShown()) return null;
        if (view instanceof ViewGroup) {
            ViewGroup group = (ViewGroup) view;
            for (int i = group.getChildCount() - 1; i >= 0; i--) {
                View modal = focusModal(group.getChildAt(i));
                if (modal != null) return modal;
            }
        }
        return view instanceof Layer ? view : null;
    }

    private boolean inFocusScope(View view) {
        if (!view.isShown()) return false;
        View modal = focusModal(rootView);
        if (modal == null) return true;
        for (View v = view; v != null; ) {
            if (v == modal) return true;
            android.view.ViewParent parent = v.getParent();
            v = parent instanceof View ? (View) parent : null;
        }
        return false;
    }

    /** Native focus is reported independently of presses; touching a control still focuses no view. */
    private void focusable(Node n) {
        if (n.field != null) return;
        boolean stop = focusStop(n);
        if (!stop && n.view.hasFocus()) requestFocus();
        n.view.setFocusable(stop);
        if (!n.view.hasFocus()) n.view.setFocusableInTouchMode(false);
        n.view.setDefaultFocusHighlightEnabled(false);
    }

    @Override
    public boolean dispatchTrackballEvent(MotionEvent e) {
        if (!inputGate.allowInput()) return true;
        return super.dispatchTrackballEvent(e);
    }

    /** The one matcher for hardware keys and Return delivered through an IME. */
    private boolean keyBinding(KeyEvent e) {
        int code = e.getKeyCode();
        if (e.getAction() == KeyEvent.ACTION_UP && consumedKeys.remove(code)) return true;
        if (e.getAction() == KeyEvent.ACTION_DOWN && keys.length > 0) {
            String[] names = names(e);
            int focus = names == null ? 0 : focusKeyUse();
            // The newest binding of the key that fires here, as driver.FindKey decides.
            for (int i = keys.length - 1; names != null && i >= 0; i--) {
                if (!keys[i].equals(names[0]) && !keys[i].equals(names[1])) continue;
                if ((focus & Protocol.KEY_USE_TEXT) != 0 && (keyFlags[i] & 1) == 0) continue;
                if ((focus & (keyFlags[i] >> 2)) != 0) continue;
                consumedKeys.add(code);
                // The count carries the binding's ID, as driver.EventKey's.
                if (e.getRepeatCount() == 0 || (keyFlags[i] & 2) != 0) event(Protocol.EVENT_KEY, 0, keyIds[i], 0, 0, "");
                return true;
            }
        }
        return false;
    }

    /** The press's names: with "mod" for Control, and with the modifiers as they are. */
    private static String[] names(KeyEvent e) {
        String key = keyName(e);
        if (key == null) return null;
        String rest = (e.isAltPressed() ? "alt+" : "") + (e.isShiftPressed() ? "shift+" : "")
                + (e.isMetaPressed() ? "meta+" : "") + key;
        return e.isCtrlPressed() ? new String[] {"mod+" + rest, "ctrl+" + rest} : new String[] {rest, rest};
    }

    /** The keys the view with the keyboard focus uses itself, as driver.FocusKeyUse says: a text field's or a web view's editing keys, a pressable view's Return and Space, a slider's or a tab bar's arrows. */
    private int focusKeyUse() {
        View focused = findFocus();
        if (focused instanceof android.widget.EditText || focused instanceof android.webkit.WebView) return Protocol.KEY_USE_TEXT;
        Node n = focused == null ? null : byView.get(focused);
        if (n == null) return 0;
        int use = 0;
        if (n.has(Protocol.EVENT_PRESS) || n.has(Protocol.EVENT_LONG_PRESS) || n.has(Protocol.EVENT_PRESS_DOWN)
                || n.has(Protocol.EVENT_PRESS_UP)) use |= Protocol.KEY_USE_PRESS;
        switch (n.role) {
            case Protocol.ROLE_ADJUSTABLE: case Protocol.ROLE_SPIN_BUTTON:
                use |= Protocol.KEY_USE_ADJUST;
                break;
            case Protocol.ROLE_TAB: case Protocol.ROLE_TAB_LIST: case Protocol.ROLE_RADIO: case Protocol.ROLE_RADIO_GROUP:
            case Protocol.ROLE_LISTBOX: case Protocol.ROLE_OPTION: case Protocol.ROLE_MENU: case Protocol.ROLE_MENU_ITEM:
            case Protocol.ROLE_MENU_ITEM_CHECKBOX: case Protocol.ROLE_MENU_ITEM_RADIO: case Protocol.ROLE_TREE:
            case Protocol.ROLE_TREE_ITEM:
                use |= Protocol.KEY_USE_SELECT;
                break;
            default:
                break;
        }
        return use;
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
     * Animates a modal in as it is first inserted and reports EVENT_MODAL_SHOWN
     * once the entrance finishes (or at once when there is no animation).
     * Android's reduce-motion setting, no animations, makes either animation instant.
     */
    private void present(Node n) {
        View view = n.view;
        int id = n.id;
        Runnable shown = () -> {
            if (nodes.get(id) == n) {
                event(Protocol.EVENT_MODAL_SHOWN, id, 0, 0, 0, "");
            }
        };
        if (n.modal == Protocol.MODAL_SLIDE) {
            ViewGroup.LayoutParams lp = view.getLayoutParams();
            view.setTranslationY(lp != null && lp.height > 0 ? lp.height : getHeight());
            view.animate().translationY(0).setDuration(300).setInterpolator(new DecelerateInterpolator())
                    .withEndAction(shown);
        } else if (n.modal == Protocol.MODAL_FADE) {
            float opacity = view.getAlpha();
            view.setAlpha(0);
            view.animate().alpha(opacity).setDuration(200).withEndAction(shown);
        } else {
            shown.run();
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
        boolean sheet = in.readUnsignedByte() != 0, cancelable = in.readUnsignedByte() != 0;
        boolean[] answered = {false};
        IntConsumer answer = index -> {
            if (answered[0]) return;
            answered[0] = true;
            alerts.remove(id);
            event(Protocol.EVENT_ALERT, 0, id, 0, 0, "", 0, 0, index);
        };
        AlertDialog.Builder b = new AlertDialog.Builder(getContext(), dark()
                ? android.R.style.Theme_DeviceDefault_Dialog_Alert
                : android.R.style.Theme_DeviceDefault_Light_Dialog_Alert);
        int[] buttons = count <= 3 && !sheet ? buttons(count, cancel) : null;
        if (buttons == null) {
            // A message hides a list, so the title takes it.
            String heading = title.isEmpty() || message.isEmpty() ? title + message : title + "\n\n" + message;
            if (!heading.isEmpty()) b.setTitle(heading);
            // An action sheet's cancel action is its dismissal.
            int[] indices = new int[count];
            int n = 0;
            for (int i = 0; i < count; i++) {
                if (!sheet || roles[i] != Protocol.ALERT_CANCEL) indices[n++] = i;
            }
            String[] items = new String[n];
            for (int i = 0; i < n; i++) items[i] = labels[indices[i]];
            b.setItems(items, (d, which) -> answer.accept(indices[which]));
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
        // Back and touches outside choose the cancel action, or close it
        // without one when it is cancelable.
        int dismissed = cancel;
        b.setCancelable(sheet || cancelable || cancel >= 0);
        b.setOnCancelListener(d -> answer.accept(dismissed));
        AlertDialog dialog = b.create();
        if (direction >= 0 && dialog.getWindow() != null) dialog.getWindow().getDecorView().setLayoutDirection(direction);
        // An overlay has no activity to show a dialog in, nor the first view while its overlays keep the app without one: their alerts show over other apps.
        if (activity() == null && dialog.getWindow() != null) {
            dialog.getWindow().setType(android.view.WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY);
        }
        dialog.show();
        if (agent != null) alerts.put(id, new TestAgent.Alert(dialog, buttons, listIndices(count, sheet, roles)));
        for (int i = 0; buttons != null && i < count; i++) {
            if (roles[i] == Protocol.ALERT_DESTRUCTIVE) dialog.getButton(buttons[i]).setTextColor(DESTRUCTIVE);
        }
    }

    /** The action of each item of an alert shown as a list. */
    private static int[] listIndices(int count, boolean sheet, int[] roles) {
        int[] indices = new int[count];
        int n = 0;
        for (int i = 0; i < count; i++) {
            if (!sheet || roles[i] != Protocol.ALERT_CANCEL) indices[n++] = i;
        }
        return java.util.Arrays.copyOf(indices, n);
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
        View view = kind == Protocol.KIND_TEXT ? new TextNode(context) {
                    @Override public void draw(Canvas canvas) {
                        int save = Box.begin(canvas, this);
                        super.draw(canvas);
                        Box.end(canvas, this, save);
                    }
                } : kind == Protocol.KIND_IMAGE ? new ImageNode(context) {
                    @Override public void draw(Canvas canvas) {
                        int save = Box.begin(canvas, this);
                        super.draw(canvas);
                        Box.end(canvas, this, save);
                    }
                } : kind == Protocol.KIND_CANVAS ? new CanvasNode(context) {
                    @Override public void draw(Canvas canvas) {
                        int save = Box.begin(canvas, this);
                        super.draw(canvas);
                        Box.end(canvas, this, save);
                    }
                } : kind == Protocol.KIND_SCROLL ? new ScrollEdges(context, density)
                : kind == Protocol.KIND_MODAL ? new Layer(context) : new Group(context);
        Node n = new Node(id, view);
        if (view instanceof ScrollEdges) ((ScrollEdges) view).backdropHost = backdropHost;
        // Styles can arrive before Android lays the view out. Its pivot is relative to the actual size, so refresh it after a first layout or resize rather than retaining a pivot computed against zero bounds.
        view.addOnLayoutChangeListener((v, l, t, r, b, oldL, oldT, oldR, oldB) -> {
            if (r - l != oldR - oldL || b - t != oldB - oldT) paint(n);
        });
        if (view instanceof CanvasNode) {
            ((CanvasNode) view).presser = this::pressAtScreen;
            ((CanvasNode) view).pictures = canvasImages;
        }
        byView.put(view, n);
        view.setOnFocusChangeListener((v, focused) -> {
            if (!focused) v.setFocusableInTouchMode(false);
            report(n, focused ? Protocol.EVENT_FOCUS : Protocol.EVENT_BLUR);
        });
        if (view instanceof Group) ((Group) view).node = n;
        switch (kind) {
            case Protocol.KIND_TEXT:
                // Its children are the views of its attachments.
                n.children = (ViewGroup) view;
                break;
            case Protocol.KIND_IMAGE:
                n.image = (ImageNode) view;
                break;
            case Protocol.KIND_CANVAS:
                n.canvas = (CanvasNode) view;
                break;
            case Protocol.KIND_INPUT:
                n.field = field(n);
                n.children = (ViewGroup) view;
                n.children.addView(n.field);
                break;
            case Protocol.KIND_WEB:
                // The WebView fills the node's view, which draws its style.
                n.web = new WebNode(context, new WebNode.Reporter() {
                    @Override public void report(int k, String text, WebNode.Navigation nav) {
                        if (n.has(k)) event(k, n.id, 0, 0, 0, text, 0, 0, 0, 0, 0, 0, 0, nav);
                    }

                    @Override public void error(String text, WebNode.Navigation nav, String domain, int code) {
                        if (n.has(Protocol.EVENT_ERROR))
                            event(Protocol.EVENT_ERROR, n.id, 0, 0, 0, text, 0, 0, code, 0, 0, 0, 0, nav,
                                    0, 0, false, domain != null ? domain : "", false);
                    }

                    @Override public void message(String data, String url) {
                        if (n.has(Protocol.EVENT_MESSAGE))
                            event(Protocol.EVENT_MESSAGE, n.id, 0, 0, 0, data, 0, 0, 0, 0, 0, 0, 0, null,
                                    0, 0, false, url, false);
                    }

                    @Override public void download(String data) {
                        if (n.has(Protocol.EVENT_DOWNLOAD))
                            event(Protocol.EVENT_DOWNLOAD, n.id, 0, 0, 0, "", 0, 0, 0, 0, 0, 0, 0, null,
                                    0, 0, false, data, false);
                    }

                    @Override public void askPermissions(String[] permissions, java.util.function.Consumer<Boolean> answer) {
                        askMediaPermissions(permissions, answer);
                    }
                });
                n.web.appearance(dark(), appearance != 0 || hostAppearance != 0);
                n.children = (ViewGroup) view;
                n.children.addView(n.web.view, new LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT));
                break;
            case Protocol.KIND_VIDEO:
                // The player fills the node's view, which draws its style
                // and takes its presses.
                n.video = new VideoNode(context, new VideoNode.Reporter() {
                    @Override
                    public boolean wants(int k) {
                        return n.has(k);
                    }

                    @Override public void pictureInPictureChanged(VideoNode video) {
                        videoPictureInPicture.changed(video);
                    }

                    @Override public void pictureInPictureState(String json) {
                        if (n.has(Protocol.EVENT_NATIVE))
                            event(Protocol.EVENT_NATIVE, n.id, 0, 0, 0, VideoPictureInPicture.EVENT,
                                    0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, json, false);
                    }

                    @Override
                    public void report(int k, String text, long time, long duration, int width, int height, int index,
                            boolean buffering) {
                        event(k, n.id, 0, 0, 0, text, width / density, height / density, index, 0, 0, 0, 0, null,
                                time, duration, buffering, "", false);
                    }
                });
                n.children = (ViewGroup) view;
                n.children.addView(n.video, new LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT));
                break;
            case Protocol.KIND_SHADER:
                // A TextureView draws no background, so the shader fills the
                // node's view, which draws its style.
                n.shader = new ShaderNode(context, canvasImages);
                n.children = (ViewGroup) view;
                n.children.addView(n.shader, new LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT));
                break;
            case Protocol.KIND_GPU:
                // A TextureView draws the 3D GPU surface on the shared GL thread;
                // native children (HUD, buttons) sit above it in the same container.
                n.gpu = new GpuView(context, surface, n.id, payload -> write(Protocol.GPU_EVENT, payload));
                n.children = (ViewGroup) view;
                n.children.addView(n.gpu, new LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT));
                break;
            case Protocol.KIND_SCROLL:
                n.children = new Content(context);
                scroller(n, false, false);
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

            @Override
            public boolean performAccessibilityAction(View host, int action, android.os.Bundle args) {
                String name = actionName(n, action);
                if (name != null && n.offers(name) && n.has(Protocol.EVENT_ACCESSIBILITY_ACTION)) {
                    event(Protocol.EVENT_ACCESSIBILITY_ACTION, n.id, 0, 0, 0, name);
                    return true;
                }
                return super.performAccessibilityAction(host, action, args);
            }
        });
        nodes.put(id, n);
        return n;
    }

    private InputField field(Node n) {
        InputField field = new InputField(getContext(), new InputField.Listener() {
            @Override public boolean has(int kind) {
                return n.has(kind);
            }

            @Override public boolean keyBinding(KeyEvent event) {
                if (event != null) return NsgoView.this.keyBinding(event);
                // An IME action is one complete Return, with no later release to clear a held key.
                long now = android.os.SystemClock.uptimeMillis();
                KeyEvent down = new KeyEvent(now, now, KeyEvent.ACTION_DOWN, KeyEvent.KEYCODE_ENTER, 0);
                boolean consumed = NsgoView.this.keyBinding(down);
                if (consumed) NsgoView.this.keyBinding(new KeyEvent(now, now, KeyEvent.ACTION_UP, KeyEvent.KEYCODE_ENTER, 0));
                return consumed;
            }

            @Override public void send(int kind, long count, int index, String text, boolean finished) {
                // Text changes always go: the engine keeps the text.
                if (kind != Protocol.EVENT_CHANGE_TEXT && !n.has(kind)) return;
                event(kind, n.id, count, 0, 0, text, 0, 0, index, 0, 0, 0, 0, null, 0, 0, false, "", finished);
            }

            @Override public void caret(float x, float y, float w, float h) {
                if (n.has(Protocol.EVENT_CARET_MOVE)) {
                    event(Protocol.EVENT_CARET_MOVE, n.id, 0, x, y, "", w, h, 0);
                }
            }

            @Override public void paste(String text, String data) {
                if (n.has(Protocol.EVENT_PASTE)) {
                    event(Protocol.EVENT_PASTE, n.id, 0, 0, 0, text, 0, 0, 0, 0, 0, 0, 0, null, 0, 0, false, data, false);
                }
            }

            @Override public void scroll(float x, float y, float maxY) {
                if (n.has(Protocol.EVENT_SCROLL)) {
                    float w = n.view.getWidth() / density;
                    float h = n.view.getHeight() / density + maxY;
                    event(Protocol.EVENT_SCROLL, n.id, n.applied, x, y, "", w, h, 0, 0, 0, 0, 0, null);
                }
            }

            @Override public void editAction(int action) {
                event(Protocol.EVENT_EDIT_ACTION, n.id, action, 0, 0, "");
            }

            @Override public void blur() {
                if (n.field.hasFocus()) {
                    blurInput(n.field);
                }
            }
        });
        field.setOnFocusChangeListener((v, focused) -> {
            if (focused) focusedInput = n;
            else if (focusedInput == n) focusedInput = null;
            updateAccessory();
            report(n, focused ? Protocol.EVENT_FOCUS : Protocol.EVENT_BLUR);
        });
        return field;
    }

    /** Hides the IME and gives focus back to the host, so keys still reach it and no other field takes focus. */
    private void blurInput(View field) {
        ime.hideSoftInputFromWindow(field.getWindowToken(), 0);
        if (!requestFocus()) field.clearFocus();
    }

    /**
     * Gives scroll node n a scroll view of the orientation, or one that
     * zooms, moving its content into it.
     */
    private void scroller(Node n, boolean horizontal, boolean zooms) {
        if (n.scroller != null && (n.scroller instanceof Scrolling.ZoomScroll) == zooms
                && (zooms || (n.scroller instanceof HorizontalScrollView) == horizontal)) return;
        Scrolling.Host host = scrollHost(n);
        ViewGroup frame = (ViewGroup) n.view;
        FrameLayout scroller = zooms ? new Scrolling.ZoomScroll(getContext(), host, density, n.children)
                : horizontal ? new Scrolling.HScroll(getContext(), host, density, n.children, frame)
                : new Scrolling.VScroll(getContext(), host, density, n.children, frame);
        // A scroll view inside another one scrolls first, then hands the
        // rest to its parent (else the parent takes every drag along its axis).
        scroller.setNestedScrollingEnabled(true);
        if (!zooms) {
            Scrolling.Scroller s = (Scrolling.Scroller) scroller;
            scroller.setOnScrollChangeListener((v, x, y, oldX, oldY) -> host.scrolled(s.offsetX(), s.offsetY(), 1));
            n.children.setScaleX(1);
            n.children.setScaleY(1);
            if (n.zoomRasterStep > 1f) {
                n.zoomRasterStep = 1f;
                walkRasterScale(n.children, ancestorRasterScale(n.children));
            }
        } else {
            n.children.setScaleX(1);
            n.children.setScaleY(1);
        }
        n.children.setTranslationX(0);
        n.children.setTranslationY(0);
        if (n.scroller != null) {
            if (n.scroller instanceof Scrolling.Scroller) ((Scrolling.Scroller) n.scroller).detach();
            n.scroller.removeView(n.children);
            ((ViewGroup) n.view).removeView(n.scroller);
        }
        scroller.addView(n.children);
        ((ViewGroup) n.view).addView(scroller, new LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT));
        n.scroller = scroller;
        if (!backdropNodes.isEmpty()) updateBackdropAncestors();
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
        look.rawScale = in.readFloat();
        look.rawScaleX = in.readFloat();
        look.rawScaleY = in.readFloat();
        look.combineScales();
        look.rotation = in.readFloat();
        long duration = in.readInt() & 0xffffffffL, delay = in.readInt() & 0xffffffffL;
        float x1 = in.readFloat(), y1 = in.readFloat(), x2 = in.readFloat(), y2 = in.readFloat();
        int properties = in.readUnsignedByte();
        int flags = in.readUnsignedByte();
        look.continuous = Protocol.flag(flags, 2);
        look.backfaceHidden = Protocol.flag(flags, 3);
        readDecor(look, in);
        Keyframes keyframes = Keyframes.read(in);

        if (n.image != null) n.image.setTint(Text.argb(tint));
        // Clipping the render node would cut off its own shadows and outline.
        view.setClipToOutline(false);
        view.setVisibility(Protocol.flag(flags, 1) ? GONE : VISIBLE);
        if (n.background == null) {
            n.background = new Box();
            view.setBackground(n.background);
        }
        n.background.clip = Protocol.flag(flags, 0);
        if (n.animator != null) n.animator.cancel(); // n.shown is where it stopped
        runKeyframes(n, keyframes);
        boolean fresh = n.fresh;
        n.fresh = false;
        if (duration == 0 || n.shown == null || fresh) {
            n.shown = look;
            paint(n);
            return;
        }
        Look from = n.shown;
        n.shown = Look.mix(from, look, 0, properties);
        paint(n);
        ValueAnimator animator = ValueAnimator.ofFloat(0, 1);
        animator.setDuration(duration);
        animator.setStartDelay(delay);
        boolean linear = x1 == 0 && y1 == 0 && x2 == 0 && y2 == 0;
        animator.setInterpolator(linear ? new LinearInterpolator() : new PathInterpolator(x1, y1, x2, y2));
        animator.addUpdateListener(a -> {
            n.shown = Look.mix(from, look, (float) a.getAnimatedValue(), properties);
            paint(n);
        });
        n.animator = animator;
        animator.start();
    }

    /** Reads what android.go's writeDecor writes into look, in pixels. */
    private void readDecor(Look look, DataInputStream in) throws IOException {
        float[] corners = new float[4];
        boolean any = false;
        for (int i = 0; i < 4; i++) {
            corners[i] = in.readFloat() * density;
            any |= corners[i] > 0;
        }
        look.corners = any ? corners : null;
        look.borderStyle = in.readUnsignedByte();
        Look.Outline o = new Look.Outline();
        o.width = in.readFloat() * density;
        o.offset = in.readFloat() * density;
        o.color = Text.argb(in.readInt());
        o.style = in.readUnsignedByte();
        look.outline = o.width > 0 ? o : null;
        look.rotationX = in.readFloat();
        look.rotationY = in.readFloat();
        look.cameraDistance = in.readFloat() * density;
        look.originX = in.readFloat() * density;
        look.originY = in.readFloat() * density;
        look.childPerspective = in.readFloat() * density;
        look.blend = in.readUnsignedByte();
        Look.Shadow[] shadows = new Look.Shadow[in.readUnsignedByte()];
        for (int i = 0; i < shadows.length; i++) {
            Look.Shadow sh = new Look.Shadow();
            sh.dx = in.readFloat() * density;
            sh.dy = in.readFloat() * density;
            sh.blur = in.readFloat() * density;
            sh.spread = in.readFloat() * density;
            sh.color = Text.argb(in.readInt());
            sh.inset = Protocol.flag(in.readUnsignedByte(), 0);
            shadows[i] = sh;
        }
        look.shadows = shadows;
        Box.Gradient[] gradients = new Box.Gradient[in.readUnsignedByte()];
        for (int i = 0; i < gradients.length; i++) {
            Box.Gradient g = new Box.Gradient();
            g.kind = in.readUnsignedByte();
            g.x0 = in.readFloat();
            g.y0 = in.readFloat();
            g.x1 = in.readFloat();
            g.y1 = in.readFloat();
            g.cx = in.readFloat();
            g.cy = in.readFloat();
            g.rx = in.readFloat();
            g.ry = in.readFloat();
            g.angle = in.readFloat();
            int n = in.readInt();
            g.offsets = new float[n];
            g.colors = new int[n];
            for (int k = 0; k < n; k++) {
                g.offsets[k] = in.readFloat();
                g.colors[k] = Text.argb(in.readInt());
            }
            gradients[i] = g;
        }
        look.gradients = gradients;
        if (Protocol.flag(in.readUnsignedByte(), 0)) {
            look.colorMatrix = new float[20];
            for (int i = 0; i < 20; i++) look.colorMatrix[i] = in.readFloat();
        }
        look.blur = in.readFloat() * density;
        if (in.readUnsignedByte() != 0) {
            look.dropShadow = new Look.Shadow();
            look.dropShadow.dx = in.readFloat() * density;
            look.dropShadow.dy = in.readFloat() * density;
            look.dropShadow.blur = in.readFloat() * density;
            look.dropShadow.color = Text.argb(in.readInt());
        }
    }

    /** Runs keyframes over n's look, or stops what ran when they change. */
    private void runKeyframes(Node n, Keyframes keyframes) {
        if (keyframes.equals(n.keyframes)) return;
        if (n.keyframer != null) n.keyframer.cancel();
        n.keyframer = null;
        n.keyframes = keyframes.steps.length > 0 ? keyframes : null;
        if (n.keyframes == null) return;
        ValueAnimator a = ValueAnimator.ofFloat(0, 1);
        a.setDuration(keyframes.duration);
        a.setStartDelay(keyframes.delay);
        a.setInterpolator(new LinearInterpolator());
        a.setRepeatCount(keyframes.iterations > 0 ? keyframes.iterations - 1 : ValueAnimator.INFINITE);
        a.addUpdateListener(v -> paint(n));
        n.keyframer = a;
        a.start();
    }

    private void input(Node n, DataInputStream in) throws IOException {
        InputField field = n.field;
        Text.Style style = new Text.Style(in, density);
        String placeholder = Protocol.readString(in);
        int keyboard = in.readUnsignedByte();
        int flags = in.readUnsignedByte();
        boolean masked = Protocol.flag(flags, 0), readonly = Protocol.flag(flags, 1);
        boolean disabled = Protocol.flag(flags, 2);
        float cx = in.readFloat(), cy = in.readFloat(), cw = in.readFloat(), ch = in.readFloat();
        n.accessory = in.readInt();
        field.configure(in, keyboard, masked, readonly, disabled, density);
        field.setTypeface(style.typeface());
        field.setTextSize(TypedValue.COMPLEX_UNIT_PX, style.size);
        field.setTextColor(style.color);
        field.setLetterSpacing(style.letterSpacingEm());
        field.setFontFeatureSettings(style.features);
        field.setPaintFlags(style.paintFlags(field.getPaintFlags()));
        field.setTextLocale(style.locale != null ? style.locale : Locale.getDefault());
        if (Build.VERSION.SDK_INT >= 33) {
            field.setLineBreakWordStyle(style.keepAll
                    ? LineBreakConfig.LINE_BREAK_WORD_STYLE_PHRASE
                    : LineBreakConfig.LINE_BREAK_WORD_STYLE_NONE);
        }
        // A field's one line stays centered in its box.
        field.setLineHeights(field.multiline() ? Math.round(style.lineHeight) : 0, placeholder);
        field.setTextDirection(style.rtl ? View.TEXT_DIRECTION_RTL : View.TEXT_DIRECTION_LTR);
        field.setGravity(style.gravity() | (field.multiline() ? Gravity.TOP : Gravity.CENTER_VERTICAL));
        ViewGroup parent = (ViewGroup) n.view;
        if (field.multiline()) {
            // A multiline editor scrolls in the padding box, its text inset by
            // InputConfig.Padding, so its scroll bar runs along the edges and
            // its text scrolls under the padding.
            if (n.fieldScroll == null) {
                boolean hadFocus = field.hasFocus();
                parent.removeView(field);
                InputField.ScrollHost host = new InputField.ScrollHost(getContext(), field);
                host.addView(field, new FrameLayout.LayoutParams(
                        LayoutParams.MATCH_PARENT, LayoutParams.WRAP_CONTENT));
                parent.addView(host);
                field.setHost(host);
                n.fieldScroll = host;
                if (hadFocus) field.requestFocus();
            }
            float bx = cx - field.padLeft(), by = cy - field.padTop();
            float bw = cw + field.padLeft() + field.padRight(), bh = ch + field.padTop() + field.padBottom();
            int left = px(bx), top = px(by);
            int right = px(bx + bw), bottom = px(by + bh);
            LayoutParams hostLP = new LayoutParams(Math.max(right - left, 0), Math.max(bottom - top, 0), Gravity.LEFT | Gravity.TOP);
            hostLP.leftMargin = left;
            hostLP.topMargin = top;
            n.fieldScroll.setLayoutParams(hostLP);
            field.setPadding(Math.max(px(cx) - left, 0), Math.max(px(cy) - top, 0),
                    Math.max(right - px(cx + cw), 0), Math.max(bottom - px(cy + ch), 0));
        } else {
            if (n.fieldScroll != null) {
                boolean hadFocus = field.hasFocus();
                n.fieldScroll.removeView(field);
                parent.removeView(n.fieldScroll);
                n.fieldScroll = null;
                field.setHost(null);
                parent.addView(field);
                if (hadFocus) field.requestFocus();
            }
            field.setPadding(0, 0, 0, 0);
            int left = px(cx), top = px(cy);
            LayoutParams content = new LayoutParams(Math.max(px(cx + cw) - left, 0), Math.max(px(cy + ch) - top, 0), Gravity.LEFT | Gravity.TOP);
            content.leftMargin = left;
            content.topMargin = top;
            field.setLayoutParams(content);
        }
    }

    static int inputType(int keyboard, boolean masked) {
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
        if (!Mouse.follows(mask)) mouse.forget(n.id);
        // A text view's press and long press carry the byte offset of the
        // character under them, for its spans; every press where its touch
        // went down.
        view.setOnClickListener(press ? v -> {
            // A press held as a long press of the node's own delay ends there.
            if (!n.longFired) pressed(n, Protocol.EVENT_PRESS, pressedIndex(v));
            pressDone(n);
        } : null);
        // With a delay of its own the node times its long presses; the
        // system's, which comes at its fixed delay, only consumes the press.
        view.setOnLongClickListener(longPress ? v -> {
            if (n.delayLongPress > 0 && n.touching) {
                n.swallowed = true;
                return true;
            }
            lastPressID = 0;
            pressed(n, Protocol.EVENT_LONG_PRESS, pressedIndex(v));
            pressDone(n);
            return true;
        } : null);
        view.setOnTouchListener(press || longPress || downUp ? (v, e) -> {
            if (v instanceof Group && !((Group) v).ownTouch()) return false;
            int action = e.getActionMasked();
            if (action == MotionEvent.ACTION_DOWN) {
                pressedAt(n, e);
                n.touching = true;
                n.longFired = n.swallowed = false;
                n.downX = e.getX();
                n.downY = e.getY();
                if (n.delayLongPress > 0 && n.has(Protocol.EVENT_LONG_PRESS)) {
                    n.longTimer = () -> {
                        n.longTimer = null;
                        n.longFired = true;
                        lastPressID = 0;
                        pressed(n, Protocol.EVENT_LONG_PRESS, pressedIndex(v));
                    };
                    v.postDelayed(n.longTimer, n.delayLongPress);
                }
                pressed(n, Protocol.EVENT_PRESS_DOWN, 0);
            } else if (action == MotionEvent.ACTION_MOVE) {
                int slop = ViewConfiguration.get(getContext()).getScaledTouchSlop();
                if (Math.hypot(e.getX() - n.downX, e.getY() - n.downY) > slop) stopLongPress(n);
            } else if (action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL) {
                if (action == MotionEvent.ACTION_CANCEL) lastPressID = 0;
                n.touching = false;
                boolean timing = n.longTimer != null;
                stopLongPress(n);
                pressed(n, Protocol.EVENT_PRESS_UP, 0);
                // The system's long press took the click, but the node's own
                // had not come yet: the release is a press.
                if (action == MotionEvent.ACTION_UP && n.swallowed && timing && !n.longFired) {
                    pressed(n, Protocol.EVENT_PRESS, pressedIndex(v));
                    pressDone(n);
                } else if (action == MotionEvent.ACTION_CANCEL || n.longFired) {
                    pressDone(n);
                }
            }
            return false;
        } : null);
        // A secondary click is no touch, nor is a wheel's turn: they come as
        // generic motions.
        view.setOnGenericMotionListener(n.has(Protocol.EVENT_CONTEXT_MENU) || n.has(Protocol.EVENT_WHEEL) ? (v, e) -> {
            if (e.getActionMasked() == MotionEvent.ACTION_SCROLL && n.has(Protocol.EVENT_WHEEL)) {
                wheel(n, e);
                return true;
            }
            if (!n.has(Protocol.EVENT_CONTEXT_MENU) || e.getActionMasked() != MotionEvent.ACTION_BUTTON_PRESS
                    || e.getActionButton() != MotionEvent.BUTTON_SECONDARY) {
                return false;
            }
            report(n, Protocol.EVENT_CONTEXT_MENU, e);
            return true;
        } : null);
        view.setClickable(press || longPress || downUp);
        view.setLongClickable(longPress);
        outside.remove(n);
        if (n.has(Protocol.EVENT_PRESS_OUTSIDE)) outside.add(n);
        focusable(n);
    }

    private void props(Node n, DataInputStream in) throws IOException {
        n.role = in.readUnsignedByte();
        int flags = in.readUnsignedByte();
        n.disabled = Protocol.flag(flags, 0);
        n.selected = Protocol.flag(flags, 1);
        n.checked = Protocol.flag(flags, 2);
        n.mixed = Protocol.flag(flags, 3);
        n.focusable = Protocol.flag(flags, 4);
        n.noFocus = Protocol.flag(flags, 5);
        n.pressed = Protocol.flag(flags, 6);
        n.busy = Protocol.flag(flags, 7);
        n.label = Protocol.readString(in);
        n.testID = Protocol.readString(in);
        n.activeX = in.readFloat();
        n.activeY = in.readFloat();
        n.failX = in.readFloat();
        n.failY = in.readFloat();
        n.hint = Protocol.readString(in);
        int a11y = in.readUnsignedByte();
        n.expandable = Protocol.flag(a11y, 0);
        n.expanded = Protocol.flag(a11y, 1);
        n.hidden = Protocol.flag(a11y, 2);
        n.range = Protocol.flag(a11y, 3);
        n.accessibleMode = in.readUnsignedByte();
        n.liveRegion = in.readUnsignedByte();
        n.min = in.readFloat();
        n.max = in.readFloat();
        n.now = in.readFloat();
        n.valueText = Protocol.readString(in);
        int actions = in.readInt();
        String[] actionNames = new String[actions], actionLabels = new String[actions];
        for (int i = 0; i < actions; i++) {
            actionNames[i] = Protocol.readString(in);
            actionLabels[i] = Protocol.readString(in);
        }
        n.setActions(actionNames, actionLabels);
        n.pointerEvents = in.readUnsignedByte();
        n.slop = new float[] {in.readFloat(), in.readFloat(), in.readFloat(), in.readFloat()};
        n.delayLongPress = in.readInt();
        int ripple = in.readUnsignedByte();
        int rippleColor = ripple != 0 ? in.readInt() : 0;
        float rippleRadius = ripple != 0 ? in.readFloat() : 0;
        ripple(n, ripple, rippleColor, rippleRadius);
        n.selectionArea = in.readUnsignedByte() != 0;
        int selectable = in.readUnsignedByte(), selectionColor = in.readInt();
        int labelFlags = in.readUnsignedByte();
        n.invalid = Protocol.flag(labelFlags, 0);
        n.labelPress = Protocol.flag(labelFlags, 1);
        n.labelFor = in.readInt();
        n.labelledBy = new int[in.readInt()];
        for (int i = 0; i < n.labelledBy.length; i++) n.labelledBy[i] = in.readInt();
        n.level = in.readInt();
        if (n.view instanceof TextNode) ((TextNode) n.view).selectable(selectable, Text.argb(selectionColor));
        View view = n.accessible();
        // A text field's ItemInput enables it.
        if (n.field == null) view.setEnabled(!n.disabled);
        focusable(n);
        view.setSelected(n.selected);
        view.setTag(n.testID.isEmpty() ? null : n.testID);
        // A dialog announces its label as it appears, an alert its content.
        view.setAccessibilityPaneTitle(n.role == Protocol.ROLE_DIALOG && !n.label.isEmpty() ? n.label : null);
        int live = n.liveRegion == 2 || n.role == Protocol.ROLE_ALERT ? View.ACCESSIBILITY_LIVE_REGION_ASSERTIVE
                : n.liveRegion == 1 ? View.ACCESSIBILITY_LIVE_REGION_POLITE : View.ACCESSIBILITY_LIVE_REGION_NONE;
        view.setAccessibilityLiveRegion(live);
        if (n.hidden) {
            view.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO_HIDE_DESCENDANTS);
        } else if (n.accessibleMode == Protocol.ACCESSIBLE_NO) {
            view.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO);
        } else if (n.accessibleMode == Protocol.ACCESSIBLE_YES) {
            view.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_YES);
        } else {
            view.setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_AUTO);
        }
        // A group is one stop for TalkBack, which reads its label.
        view.setScreenReaderFocusable(n.accessibleMode == Protocol.ACCESSIBLE_YES);
        describe(n);
    }

    /**
     * Shows n's ripple, flags 1 set, 2 borderless and 4 foreground, as
     * Android's own press handling of a clickable view drives it: behind
     * the content in a layer over the background, or over it.
     */
    private void ripple(Node n, int flags, int color, float radius) {
        if (flags == n.rippleFlags && color == n.rippleColor && radius == n.rippleRadius) return;
        n.rippleFlags = flags;
        n.rippleColor = color;
        n.rippleRadius = radius;
        if (n.background == null) {
            n.background = new Box();
            n.view.setBackground(n.background);
        }
        if (n.view.getForeground() instanceof RippleDrawable) n.view.setForeground(null);
        if (!(n.view.getBackground() instanceof Box)) n.view.setBackground(n.background);
        if ((flags & 1) == 0) return;
        Box box = n.background;
        Drawable mask = (flags & 2) != 0 ? null : new Drawable() {
            private final Path path = new Path();
            private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);

            @Override
            public void draw(Canvas canvas) {
                paint.setColor(0xffffffff);
                canvas.drawPath(box.shape(getBounds(), path), paint);
            }

            @Override
            public void setAlpha(int alpha) {}

            @Override
            public void setColorFilter(ColorFilter filter) {}

            @Override
            public int getOpacity() {
                return PixelFormat.TRANSLUCENT;
            }
        };
        RippleDrawable drawable = new RippleDrawable(
                ColorStateList.valueOf(color != 0 ? Text.argb(color) : themeHighlight()), null, mask);
        if (radius > 0) drawable.setRadius(px(radius));
        if ((flags & 4) != 0) {
            n.view.setForeground(drawable);
        } else {
            LayerDrawable background = new LayerDrawable(new Drawable[] {box, drawable});
            n.view.setBackground(background);
            // Replacing the old background clears box's callback after the LayerDrawable took it. Restore it so restyles invalidate the view.
            box.setCallback(background);
        }
    }

    /** The theme's ripple color. */
    private int themeHighlight() {
        TypedValue value = new TypedValue();
        if (getContext().getTheme().resolveAttribute(android.R.attr.colorControlHighlight, value, true)) {
            return value.resourceId != 0 ? getContext().getColor(value.resourceId) : value.data;
        }
        return 0x1f000000;
    }

    /** The current name of a TalkBack action, if any. */
    private static String actionName(Node n, int action) {
        if (action >= CUSTOM_ACTION) {
            for (int i = 0; i < n.actionIDs.length; i++) {
                if (n.actionIDs[i] == action) return n.actionNames[i];
            }
            return null;
        }
        switch (action) {
            case AccessibilityNodeInfo.ACTION_CLICK: return "activate";
            case AccessibilityNodeInfo.ACTION_LONG_CLICK: return "longpress";
            case AccessibilityNodeInfo.ACTION_SCROLL_FORWARD: return "increment";
            case AccessibilityNodeInfo.ACTION_SCROLL_BACKWARD: return "decrement";
            case AccessibilityNodeInfo.ACTION_EXPAND: return "expand";
            case AccessibilityNodeInfo.ACTION_COLLAPSE: return "collapse";
            default: return null;
        }
    }

    /** Ids of custom accessibility actions, outside the framework's. */
    private static final int CUSTOM_ACTION = 0x3f000000;

    /** Actions TalkBack performs through its own gestures, not its actions menu. */
    private static boolean isStandardAction(String name) {
        switch (name) {
            case "activate": case "increment": case "decrement": case "longpress":
            case "expand": case "collapse": case "escape": case "magicTap":
                return true;
            default:
                return false;
        }
    }

    /** Sets what assistive technology reads: the label, else a text node's text. */
    private static void describe(Node n) {
        String d = !n.label.isEmpty() ? n.label : n.text;
        n.accessible().setContentDescription(d.isEmpty() ? null : d);
    }

    private void describe(Node n, AccessibilityNodeInfo info) {
        info.setContentInvalid(n.invalid);
        Node target = nodes.get(n.labelFor);
        if (target != null && Build.VERSION.SDK_INT < 36) info.setLabelFor(target.accessible());
        for (int id : n.labelledBy) {
            Node label = nodes.get(id);
            if (label == null) continue;
            if (Build.VERSION.SDK_INT >= 36) info.addLabeledBy(label.accessible());
            else { info.setLabeledBy(label.accessible()); break; }
        }
        if (n.labelPress) {
            info.setClassName("android.widget.TextView");
            info.setFocusable(false);
        }
        String fieldText = n.field != null && n.field.getText() != null ? n.field.getText().toString() : null;
        Accessibility.Description d = Accessibility.describe(
                n.role, n.field != null, fieldText, n.checked, n.mixed, n.pressed, n.busy,
                n.expandable, n.expanded, n.level, n.valueText, spoken);
        if (d.className != null) info.setClassName(d.className);
        if (d.heading) info.setHeading(true);
        // The key AndroidX and TalkBack read a role description under.
        if (d.roleDescription != null) {
            info.getExtras().putCharSequence("AccessibilityNodeInfo.roleDescription", d.roleDescription);
        }
        if (d.checkable) {
            info.setCheckable(true);
            info.setChecked(d.checked);
        }
        info.setSelected(n.selected);
        if (!n.testID.isEmpty()) info.setViewIdResourceName(n.testID);
        if (!n.hint.isEmpty()) info.setHintText(n.hint);
        if (n.expandable && n.offers(n.expanded ? "collapse" : "expand")) {
            info.addAction(n.expanded ? AccessibilityNodeInfo.AccessibilityAction.ACTION_COLLAPSE
                    : AccessibilityNodeInfo.AccessibilityAction.ACTION_EXPAND);
        }
        if (n.range) {
            info.setRangeInfo(AccessibilityNodeInfo.RangeInfo.obtain(
                    AccessibilityNodeInfo.RangeInfo.RANGE_TYPE_FLOAT, n.min, n.max, n.now));
        }
        if (d.stateDescription != null && Build.VERSION.SDK_INT >= 30) {
            info.setStateDescription(d.stateDescription);
        }
        if (n.offers("increment")) info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_SCROLL_FORWARD);
        if (n.offers("decrement")) info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_SCROLL_BACKWARD);
        if (n.offers("activate")) {
            info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_CLICK);
            info.setClickable(true);
        }
        if (n.offers("longpress")) {
            info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_LONG_CLICK);
            info.setLongClickable(true);
        }
        for (int i = 0; i < n.actionNames.length; i++) {
            if (n.actionIDs[i] == 0) continue;
            String label = n.actionLabels[i].isEmpty() ? n.actionNames[i] : n.actionLabels[i];
            info.addAction(new AccessibilityNodeInfo.AccessibilityAction(n.actionIDs[i], label));
        }
    }

    /**
     * A view node's container, whose children may draw outside it, as
     * without Clip. While its node follows gestures it takes the touches
     * its children leave, so the gestures see them, and while one of them
     * is active it takes its children's too, cancelling their presses.
     */
    final class Group extends DecoratedGroup {
        Node node;
        ShaderEffect effect;
        private boolean picturing;

        @Override
        public void onDescendantInvalidated(View child, View target) {
            super.onDescendantInvalidated(child, target);
            if (effect != null && target != effect.shader) effect.stale = true;
        }

        @Override
        protected void dispatchDraw(Canvas canvas) {
            if (canvas.isHardwareAccelerated() && effect != null && effect.shader.effectReady()) {
                super.drawChild(canvas, effect.shader, getDrawingTime());
            } else {
                super.dispatchDraw(canvas);
            }
        }

        @Override
        protected boolean drawChild(Canvas canvas, View child, long time) {
            if (effect != null && child == effect.shader && (picturing || effect.shader.isAvailable())) return false;
            return super.drawChild(canvas, child, time);
        }

        void picture(Canvas canvas, boolean previous) {
            if (previous && effect != null && effect.shader.effectReady()) {
                super.drawChild(canvas, effect.shader, getDrawingTime());
                return;
            }
            picturing = true;
            try {
                super.dispatchDraw(canvas);
            } finally {
                picturing = false;
            }
        }
        /** The child a touch outside every child went to by its hit slop or area, until it lifts. */
        private View routed;
        /** Whether the touch reached the group outside its bounds and slop: only children may take it. */
        boolean childrenOnly;

        final Object backdropState = Backdrop.createState();
        private final Backdrop.Callbacks backdropCallbacks = new Backdrop.Callbacks() {
            @Override public void superDraw(Canvas canvas) { Group.super.draw(canvas); }
            @Override public boolean drawChild(Canvas canvas, View child, long drawingTime) {
                return Group.this.drawChild(canvas, child, drawingTime);
            }
            @Override public void superOnDrawForeground(Canvas canvas) { Group.super.onDrawForeground(canvas); }
        };

        Group(Context context) {
            super(context);
            setClipChildren(false);
            setClipToPadding(false);
        }

        @Override
        public void draw(Canvas canvas) {
            // A shader effect draws its own picture of the children, so they blur only what they draw themselves.
            if (effect != null) {
                super.draw(canvas);
                return;
            }
            Backdrop.drawContainer(this, canvas, backdropHost, backdropCallbacks, backdropState);
        }

        /** Whether the group itself may take the touch it is handed, as core.Reach says. */
        boolean ownTouch() {
            int pe = node == null ? 0 : node.pointerEvents;
            return !childrenOnly && pe != Protocol.POINTER_EVENTS_NONE && pe != Protocol.POINTER_EVENTS_BOX_NONE;
        }

        @Override
        public boolean dispatchTouchEvent(MotionEvent e) {
            int action = e.getActionMasked();
            if (node != null && node.pointerEvents == Protocol.POINTER_EVENTS_NONE) return false;
            if (action == MotionEvent.ACTION_DOWN) routed = route(e);
            if (routed != null) {
                if (action != MotionEvent.ACTION_DOWN) deliver(routed, e);
                if (action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL) {
                    routed = null;
                    childrenOnly = false;
                }
                return true;
            }
            if (node != null && node.followsGestures()) {
                if (action == MotionEvent.ACTION_DOWN) gestures.candidate(node);
                // An ancestor, such as a scroll view, took the touch.
                else if (action == MotionEvent.ACTION_CANCEL) gestures.lost(node, true);
            }
            boolean handled = super.dispatchTouchEvent(e);
            if (action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL) childrenOnly = false;
            return handled;
        }

        /**
         * Hands a touch starting outside every child's bounds to the topmost
         * child whose hit slop or area holds it, as UIKit's and AppKit's
         * hit tests do in NSGo; null when a child's bounds hold it, or
         * none takes it.
         */
        private View route(MotionEvent e) {
            if (node != null && node.pointerEvents == Protocol.POINTER_EVENTS_BOX_ONLY) return null;
            for (int i = getChildCount() - 1; i >= 0; i--) {
                View child = getChildAt(i);
                if (child.getVisibility() != VISIBLE || effect != null && child == effect.shader) continue;
                float[] point = intoChild(this, child, e.getX(), e.getY());
                if (point == null) continue;
                float x = point[0], y = point[1];
                if (x >= 0 && y >= 0 && x < child.getWidth() && y < child.getHeight()) return null;
                Node c = byView.get(child);
                if (c == null) continue;
                boolean slop = c.slop != null && x >= -c.slop[3] * density && y >= -c.slop[0] * density
                        && x < child.getWidth() + c.slop[1] * density && y < child.getHeight() + c.slop[2] * density;
                // Overflow: Hidden clips the body, leaving getClipChildren false.
                boolean area = child instanceof ViewGroup && !((ViewGroup) child).getClipChildren() && !Box.clips(child)
                        || c.area != null && x >= c.area[0] * density && y >= c.area[1] * density
                        && x < (c.area[0] + c.area[2]) * density && y < (c.area[1] + c.area[3]) * density;
                if (!slop && !area) continue;
                if (child instanceof Group) ((Group) child).childrenOnly = !slop;
                int candidates = gestures.candidates();
                if (deliver(child, e)) return child;
                // A declined down has no up or cancel to clear this flag, and the gesture views it noted are not under the touch.
                if (child instanceof Group) ((Group) child).childrenOnly = false;
                gestures.forgetCandidates(candidates);
            }
            return null;
        }

        /** Dispatches e to child, in its coordinates. */
        private boolean deliver(View child, MotionEvent e) {
            MotionEvent local = MotionEvent.obtain(e);
            local.offsetLocation(getScrollX() - child.getLeft(), getScrollY() - child.getTop());
            Matrix inverse = new Matrix();
            if (!child.getMatrix().invert(inverse)) { local.recycle(); return false; }
            local.transform(inverse);
            boolean taken = child.dispatchTouchEvent(local);
            local.recycle();
            return taken;
        }

        @Override
        public void requestDisallowInterceptTouchEvent(boolean disallow) {
            // A descendant, such as a scroll view, took the touch first.
            if (disallow && !gestures.claiming) touchesTaken();
            if (disallow && node != null && !gestures.claiming) gestures.lost(node, false);
            super.requestDisallowInterceptTouchEvent(disallow);
        }

        @Override
        public boolean onInterceptTouchEvent(MotionEvent e) {
            // A box-only group takes its children's touches.
            if (node != null && node.pointerEvents == Protocol.POINTER_EVENTS_BOX_ONLY) return true;
            return gestures.owns(node) || super.onInterceptTouchEvent(e);
        }

        @Override
        public boolean onTouchEvent(MotionEvent e) {
            if (!ownTouch() && !gestures.owns(node)) return false;
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

        // The pinch: its node, whether it is in progress, its scale, velocity, and last center.
        private Node pinch;
        private boolean pinching, pinchTookPress;
        private float scale = 1, scaleVelocity, pinchX, pinchY;

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

        /** How many gesture views the touch going down has noted so far. */
        int candidates() {
            return candidates.size();
        }

        /** Forgets the gesture views noted after the first count, as a probe that declined the touch noted them. */
        void forgetCandidates(int count) {
            if (candidates.size() > count) candidates.subList(count, candidates.size()).clear();
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
            int[] root = new int[2], view = new int[2];
            getLocationInWindow(root);
            pan.view.getLocationInWindow(view);
            float rawX = downX + panX * density, rawY = downY + panY * density;
            float pageX = (rawX - root[0]) / density, pageY = (rawY - root[1]) / density;
            float positionX = (rawX - view[0]) / density, positionY = (rawY - view[1]) / density;
            gesture(pan, Protocol.EVENT_PAN, phase, panX, panY, vx, vy, 0, positionX, positionY, pageX, pageY);
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
            float[] f = focus(pinch, d);
            pinchX = f[0];
            pinchY = f[1];
            gesture(pinch, Protocol.EVENT_PINCH, Protocol.GESTURE_BEGAN, pinchX, pinchY, 0, 0, scale);
            return true;
        }

        @Override
        public boolean onScale(ScaleGestureDetector d) {
            if (!pinching) return true;
            float next = scale * d.getScaleFactor();
            long dt = d.getTimeDelta();
            if (dt > 0) scaleVelocity = (next - scale) * 1000f / dt;
            scale = next;
            float[] f = focus(pinch, d);
            pinchX = f[0];
            pinchY = f[1];
            gesture(pinch, Protocol.EVENT_PINCH, Protocol.GESTURE_CHANGED, pinchX, pinchY, scaleVelocity, 0, scale);
            return true;
        }

        @Override
        public void onScaleEnd(ScaleGestureDetector d) {
            if (!pinching) return;
            pinching = false;
            pinchTookPress = false;
            int phase = cancelling ? Protocol.GESTURE_CANCELLED : Protocol.GESTURE_ENDED;
            float[] f = cancelling ? new float[] {pinchX, pinchY} : focus(pinch, d);
            pinchX = f[0];
            pinchY = f[1];
            gesture(pinch, Protocol.EVENT_PINCH, phase, pinchX, pinchY, scaleVelocity, 0, scale);
        }

        /** Where a pinch is about, in n's untransformed points. */
        private float[] focus(Node n, ScaleGestureDetector d) {
            float[] p = fromView(NsgoView.this, n.view, d.getFocusX(), d.getFocusY());
            if (p != null) {
                return new float[] {p[0] / density, p[1] / density};
            }
            int[] root = new int[2], at = new int[2];
            getLocationInWindow(root);
            n.view.getLocationInWindow(at);
            return new float[] {(d.getFocusX() - at[0] + root[0]) / density, (d.getFocusY() - at[1] + root[1]) / density};
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
                    gesture(pinch, Protocol.EVENT_PINCH, Protocol.GESTURE_CANCELLED, pinchX, pinchY, scaleVelocity, 0, scale);
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

        /**
         * Reports a gesture of n's, after moving the values bound to it in
         * the frame it moves; a pan's start and end carry those values as
         * the event's size.
         */
        private void gesture(Node n, int kind, int phase, float x, float y, float vx, float vy, float value) {
			gesture(n, kind, phase, x, y, vx, vy, value, 0, 0, 0, 0);
		}

		private void gesture(Node n, int kind, int phase, float x, float y, float vx, float vy, float value,
				float positionX, float positionY, float pageX, float pageY) {
            float[] values = animator.gesture(kind, n.id, phase, x, y, value);
            if (values != null) applyAnimated();
            float w = values != null && values.length == 2 ? values[0] : 0;
            float h = values != null && values.length == 2 ? values[1] : 0;
            if (n.has(kind)) event(kind, n.id, 0, x, y, "", w, h, 0, phase, vx, vy, value, null,
					0, 0, false, "", false, pageX, pageY, positionX, positionY, 0);
        }
    }

    /**
     * Reports a wheel's turn or touchpad scroll over n, at e's place in n:
     * a notch moves the content about 100 points, three lines of 33, as
     * browsers scroll; a touchpad's scroll moves it by its axes' points and
     * ends with its release velocity after fingers lift so views glide on.
     */
    private void wheel(Node n, MotionEvent e) {
        boolean touchpad = e.isFromSource(InputDevice.SOURCE_TOUCHPAD);
        int[] root = new int[2];
        getLocationOnScreen(root);
        float pageX = (e.getRawX() - root[0]) / density, pageY = (e.getRawY() - root[1]) / density;
        wheelScroll.scroll(n.id, touchpad,
                e.getAxisValue(MotionEvent.AXIS_HSCROLL), e.getAxisValue(MotionEvent.AXIS_VSCROLL),
                e.getX() / density, e.getY() / density, pageX, pageY,
                modifiers(e.getMetaState()), e.getEventTime());
        ui.removeCallbacks(wheelEnd);
        if (touchpad) {
            ui.postDelayed(wheelEnd, WheelScroll.IDLE_MS);
        }
    }

    private void emitWheel(int nodeId, int phase, float x, float y, float pageX, float pageY,
            float dx, float dy, float vx, float vy, boolean touchpad, int mods) {
        Node n = nodes.get(nodeId);
        if (n == null || !n.has(Protocol.EVENT_WHEEL)) return;
        event(Protocol.EVENT_WHEEL, nodeId, 0, x, y, "", vx, vy, 0, phase,
                dx, dy, touchpad ? 1 : 0, null, 0, 0, false, "", false, pageX, pageY, mods);
    }

    /** Whether a drag of n translated by (dx, dy) points activates its pan (1), fails it (-1) or is undecided (0). */
    private static int decide(Node n, float dx, float dy) {
        if (past(dx, n.failX) || past(dy, n.failY)) return -1;
        if (n.activeX == 0 && n.activeY == 0) {
            return Math.abs(dx) >= Gestures.PAN_SLOP || Math.abs(dy) >= Gestures.PAN_SLOP ? 1 : 0;
        }
        return past(dx, n.activeX) || past(dy, n.activeY) ? 1 : 0;
    }

    /** What scroll node n's scroll view asks of the runtime. */
    private Scrolling.Host scrollHost(Node n) {
        return new Scrolling.Host() {
            @Override
            public void scrollEvent(int kind, float x, float y, float zoom) {
                if (kind == Protocol.EVENT_SCROLL_END) {
                    // At rest: the values that snap as a scroll ends settle.
                    animator.settle(n.id);
                    if (animator.running()) startTicking();
                }
                if (n.has(kind)) event(kind, n.id, n.applied, x, y, "", 0, 0, 0, 0, 0, 0, zoom, null);
            }

            @Override
            public void scrolled(float x, float y, float zoom) {
                // A scroll view the core insets below a large title collapses it, as on iOS.
                if (n.headerInset > 0) {
                    Node screen = n.up;
                    while (screen != null && screen.header == null) screen = screen.up;
                    if (screen != null) screen.header.follow(y + n.insetTop);
                }
                // Values following the offset move in the same frame, and
                // so do sticky cells.
                boolean moved = animator.scroll(n.id, x, y);
                if (animator.pulls(n.id) && n.scroller instanceof Scrolling.Scroller
                        && animator.pull(n.id, ((Scrolling.Scroller) n.scroller).pulled())) {
                    moved = true;
                }
                if (moved) applyAnimated();
                if (n.sticky != null) pinSticky(n);
                if (n.view instanceof ScrollEdges && ((ScrollEdges) n.view).hasEdges()) n.view.invalidate();
                if (!backdropAncestors.isEmpty()) {
                    // A backdrop filter inside or over the scroll view blurs what moved.
                    if (backdropAncestors.contains(n.view)) {
                        n.view.invalidate();
                        if (n.children != null && backdropAncestors.contains(n.children)) n.children.invalidate();
                    }
                    ViewParent p = n.view.getParent();
                    if (p instanceof ViewGroup && backdropAncestors.contains(p)) ((ViewGroup) p).invalidate();
                }
                scrollEvent(Protocol.EVENT_SCROLL, x, y, zoom);
            }

            @Override
            public void zoomChanged(float zoom) {
                followZoomScale(n, zoom);
            }

            @Override
            public boolean dismissKeyboard() {
                View focused = findFocus();
                if (!(focused instanceof EditText)) return false;
                blurInput(focused);
                return true;
            }

            @Override
            public boolean onInput(float rawX, float rawY) {
                for (int i = 0; i < nodes.size(); i++) {
                    Node o = nodes.valueAt(i);
                    if (o.field != null && under(o.fieldHost(), rawX, rawY)) return true;
                }
                return false;
            }

            @Override
            public boolean handlesTap(float rawX, float rawY) {
                for (int i = 0; i < nodes.size(); i++) {
                    Node o = nodes.valueAt(i);
                    if (o != n && (o.has(Protocol.EVENT_PRESS) || o.has(Protocol.EVENT_LONG_PRESS)
                            || o.has(Protocol.EVENT_PRESS_DOWN) || o.followsGestures()) && under(o.view, rawX, rawY)) {
                        return true;
                    }
                }
                return false;
            }
        };
    }

    /** Whether a shown view covers the raw screen point. */
    private static boolean under(View v, float rawX, float rawY) {
        if (!v.isShown()) return false;
        float[] p = fromScreen(v, rawX, rawY);
        return p != null && p[0] >= 0 && p[1] >= 0 && p[0] < v.getWidth() && p[1] < v.getHeight();
    }

    /** A point of the parent's content mapped through the child's native transform. */
    private static float[] intoChild(ViewGroup parent, View child, float x, float y) {
        float[] p = {x + parent.getScrollX() - child.getLeft(), y + parent.getScrollY() - child.getTop()};
        Matrix inverse = new Matrix();
        if (!child.getMatrix().invert(inverse)) return null;
        inverse.mapPoints(p);
        return p;
    }

    /** A raw screen point in v's viewport, with every ancestor's transform undone. */
    private static float[] fromScreen(View v, float x, float y) {
        if (v.getParent() instanceof ViewGroup) {
            ViewGroup parent = (ViewGroup) v.getParent();
            float[] p = fromScreen(parent, x, y);
            return p == null ? null : intoChild(parent, v, p[0], p[1]);
        }
        int[] at = new int[2];
        v.getLocationOnScreen(at);
        return new float[] {x - at[0], y - at[1]};
    }

    /** A point in ancestor's coordinates mapped into v's viewport, with every transform undone. */
    private static float[] fromView(View ancestor, View v, float x, float y) {
        if (v == ancestor) return new float[] {x, y};
        if (v.getParent() instanceof ViewGroup) {
            ViewGroup parent = (ViewGroup) v.getParent();
            float[] p = fromView(ancestor, parent, x, y);
            return p == null ? null : intoChild(parent, v, p[0], p[1]);
        }
        return null;
    }

    private static boolean past(float v, float limit) {
        return limit != 0 && Math.abs(v) >= Math.abs(limit);
    }

    /**
     * A scroll view's content. Scroll views leave its length along their
     * axis unbounded, so it takes the size ItemScrollContent gave it.
     */
    private final class Content extends FrameLayout {
        final Object backdropState = Backdrop.createState();
        private final Backdrop.Callbacks backdropCallbacks = new Backdrop.Callbacks() {
            @Override public void superDraw(Canvas canvas) { Content.super.draw(canvas); }
            @Override public boolean drawChild(Canvas canvas, View child, long drawingTime) {
                return Content.this.drawChild(canvas, child, drawingTime);
            }
            @Override public void superOnDrawForeground(Canvas canvas) { Content.super.onDrawForeground(canvas); }
        };

        Content(Context context) {
            super(context);
            setClipChildren(false);
            setClipToPadding(false);
        }

        @Override
        public void draw(Canvas canvas) {
            Backdrop.drawContainer(this, canvas, backdropHost, backdropCallbacks, backdropState);
        }

        @Override
        protected void onMeasure(int widthSpec, int heightSpec) {
            ViewGroup.LayoutParams lp = getLayoutParams();
            super.onMeasure(MeasureSpec.makeMeasureSpec(Math.max(lp.width, 0), MeasureSpec.EXACTLY),
                    MeasureSpec.makeMeasureSpec(Math.max(lp.height, 0), MeasureSpec.EXACTLY));
        }
    }

    /** A styled container whose outer decoration stays outside its content clip. */
    private static class DecoratedGroup extends FrameLayout {
        DecoratedGroup(Context context) {
            super(context);
        }

        @Override
        public void draw(Canvas canvas) {
            int save = Box.begin(canvas, this);
            super.draw(canvas);
            Box.end(canvas, this, save);
        }
    }

    /** A modal's layer: it takes the touches its children leave, so none reach the views it covers. */
    private final class Layer extends DecoratedGroup {
        final Object backdropState = Backdrop.createState();
        private final Backdrop.Callbacks backdropCallbacks = new Backdrop.Callbacks() {
            @Override public void superDraw(Canvas canvas) { Layer.super.draw(canvas); }
            @Override public boolean drawChild(Canvas canvas, View child, long drawingTime) {
                return Layer.this.drawChild(canvas, child, drawingTime);
            }
            @Override public void superOnDrawForeground(Canvas canvas) { Layer.super.onDrawForeground(canvas); }
        };

        Layer(Context context) {
            super(context);
            setClipChildren(false);
            setClipToPadding(false);
        }

        @Override
        public void draw(Canvas canvas) {
            Backdrop.drawContainer(this, canvas, backdropHost, backdropCallbacks, backdropState);
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
        /** The node's backdrop filters, null for none. */
        BackdropMath.Spec backdrop;
        /** A scroll view's top content inset, and the part of it a transparent or large system header covers, in points. */
        float insetTop, headerInset;
        /** The id of the text layout a text node shows. */
        int laid;
        InputField field;
        InputField.ScrollHost fieldScroll;

        /** The visible input box: the multiline ScrollHost or the single-line field. */
        View fieldHost() {
            return fieldScroll != null ? fieldScroll : field;
        }
        ImageNode image;
        CanvasNode canvas;
        ShaderNode shader;
        ShaderEffect effect;
        GpuView gpu;
        WebNode web;
        VideoNode video;
        /** The view's background, the look it shows and the transition moving it. */
        Box background;
        Look shown;
        /** Stepped raster scales for this node's transform, scroll zoom, and ancestor chain. */
        float rasterStep = 1f, zoomRasterStep = 1f, inheritedRasterStep = 1f;
        /** Whether it still shows the default style startDefault gave it, which the first style replaces. */
        boolean fresh;
        /** Whether the node is a selection area (driver.ViewProps.SelectionArea). */
        boolean selectionArea;
        /** The ripple's flags, color and radius, as ripple() takes them. */
        int rippleFlags, rippleColor;
        float rippleRadius;
        ValueAnimator animator;
        /** The keyframe animation running over the look, and what it runs. */
        Keyframes keyframes;
        ValueAnimator keyframer;
        /** The node the view went into, whose perspective its turns are seen in. */
        Node up;
        /** A modal's animation, -1 for other nodes, and whether it was presented. */
        int modal = -1;
        boolean presented;
        long events;
        int role, level;
        boolean disabled, selected, checked, mixed, focusable, noFocus, pressed, busy;
        boolean invalid, labelPress;
        int labelFor;
        int[] labelledBy = new int[0];
        String label = "", text = "", testID = "";
        /** Accessibility beyond the role, label and states, as core.ViewProps has it. */
        String hint = "", valueText = "";
        boolean expandable, expanded, hidden, range;
        int accessibleMode, liveRegion;
        float min, max, now;
        String[] actionNames = new String[0], actionLabels = new String[0];
        int[] actionIDs = new int[0];
        int nextCustomAction = CUSTOM_ACTION;
        /** The pan's activation offsets in points, as in core.PanActivation; 0 is unset. */
        float activeX, activeY, failX, failY;
        /** Where its touch went down, in it and its root view in points, with the modifier bits and clicks. */
        float pressX, pressY, pressPageX, pressPageY;
        int pressMods, pressClicks;
        /** core.PointerEvents, the hit slop (top, right, bottom, left) and areas in points, the long press delay in ms. */
        int pointerEvents;
        float[] slop, area;
        int delayLongPress;
        /** A touch is down; the long press it is timed for fired; where it went down, in pixels. */
        boolean touching, longFired, swallowed;
        float downX, downY;
        Runnable longTimer;
        /** The frame shown, in points (x, y, width, height), or null before the first. */
        float[] frame;
        /**
         * A scroll node's ItemScrollTo and ItemScrollBy applied, which its
         * reports carry, and its sticky cells (id, start, end); a sticky
         * cell's shift from its frame in points, across for a horizontal list.
         */
        long applied;
        float[] sticky;
        // stickyBound says a sticky cell's inset follows an animated value.
        boolean stickyBound;
        float shift;
        boolean shiftAcross;
        /** A layout animation's frame motion, preserving spring velocity across retargets. */
        LayoutTransition.Motion motion;
        /** A layout animation's appearance, and when it began. */
        LayoutTransition appear;
        long appearStart;
        /** The animated values bound to it, drawn over its style, or null. */
        Animator.Values values;
        /** An input's InputAccessoryView, 0 for none. */
        int accessory;
        /** A native view's component view, and whether one was asked for. */
        Embedded embedded;
        boolean embedTried;
        /** A native view's animated props. */
        Embedded.Bound bound;
        /** A screen's system header, the holder of a header view in one, and that screen. */
        Header header;
        Header.Holder holder;
        Node headerScreen;

        Node(int id, View view) {
            this.id = id;
            this.view = view;
        }

        boolean has(int kind) {
            return (events & (1L << kind)) != 0;
        }

        /** Whether the node offers the accessibility action name. */
        /**
         * A custom action keeps its ID when its label or position changes.
         * TalkBack can still send an ID from a menu it got before an update.
         */
        void setActions(String[] names, String[] labels) {
            if (Arrays.equals(actionNames, names)) {
                actionLabels = labels;
                return;
            }
            int[] ids = new int[names.length];
            boolean[] reused = new boolean[actionNames.length];
            for (int i = 0; i < names.length; i++) {
                if (isStandardAction(names[i])) continue;
                for (int old = 0; old < actionNames.length; old++) {
                    if (!reused[old] && actionNames[old].equals(names[i])) {
                        ids[i] = actionIDs[old];
                        reused[old] = true;
                        break;
                    }
                }
                if (ids[i] == 0) ids[i] = nextCustomAction++;
            }
            actionNames = names;
            actionLabels = labels;
            actionIDs = ids;
        }

        boolean offers(String name) {
            for (String a : actionNames) {
                if (a.equals(name)) return true;
            }
            return false;
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
