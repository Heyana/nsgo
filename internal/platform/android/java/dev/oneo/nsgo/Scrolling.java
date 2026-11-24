package dev.oneo.nsgo;

import android.content.Context;
import android.content.res.Configuration;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.RectF;
import android.os.SystemClock;
import android.view.Gravity;
import android.view.MotionEvent;
import android.view.ScaleGestureDetector;
import android.view.View;
import android.view.ViewConfiguration;
import android.view.ViewGroup;
import android.widget.FrameLayout;
import android.widget.HorizontalScrollView;
import android.widget.OverScroller;
import android.widget.ProgressBar;
import android.widget.ScrollView;

import java.io.DataInputStream;
import java.io.IOException;
import java.util.function.BiConsumer;

/**
 * Scroll views, as NSH's ScrollBox scrolls: Android's ScrollView and
 * HorizontalScrollView, whose flings glide with OverScroller's spline
 * (its friction scaled by core.ScrollBehavior's deceleration) or end at
 * the snap point ScrollPhysics.snapTarget
 * picks; drag and momentum events; the content inset as padding;
 * keyboard dismissal on drags and taps; pull to refresh with a spinner,
 * or the app's own indicator, above the content; and ZoomScroll, which a
 * zoomable view switches to: it scales the content around its top left and
 * scrolls both ways itself.
 */
final class Scrolling {
    private Scrolling() {}

    /** How far past the top a pull refreshes, and the spinner's room, in points. */
    static final float REFRESH_PULL = 64, REFRESH_SPACE = 56;
    /** Speed threshold (5x this, in points per millisecond) below which a release does not glide. */
    private static final float STOP = 0.01f;
    /** Default DecelerationRate (core.DecelerationNormal), velocity kept per millisecond. */
    private static final float NORMAL = 0.998f;
    /** Duration of animated scrolls, in milliseconds. */
    private static final long ANIMATION = 300;
    /** Phone scroll handle geometry in points (mirrors internal/driver/scrollhandle.go). */
    private static final float HANDLE_INSET = 3f;
    private static final float HANDLE_MARGIN = 4f;
    private static final float HANDLE_THICKNESS = 6f;
    private static final float HANDLE_ACTIVE_THICKNESS = 10f;
    private static final float HANDLE_MIN_LENGTH = 44f;
    private static final float HANDLE_GRAB_THICKNESS = 32f;
    private static final float HANDLE_GRAB_SLOP = 8f;

    /** A scroll view's configuration (core.ScrollConfig), lengths in points. */
    static final class Config {
        boolean horizontal, hideIndicator, paging, oneStep, noScroll, noBounces, refreshable, refreshing;
        boolean noBouncesZoom, noPinchZoom, centerContent;
        float interval, rate = NORMAL;
        int alignment, dismiss, taps;
        float[] offsets = new float[0];
        float insetTop, insetRight, insetBottom, insetLeft;
        /** The part of insetTop a transparent or large system header covers. */
        float headerInset;
        float minZoom = 1, maxZoom = 1, zoom = 1;
        /** The height of the app's own refresh indicator; 0 shows the spinner. */
        float refreshIndicator;
        /** Drawn scroll edge effects for top, right, bottom, left; null entries draw nothing. */
        final Edge[] edges = new Edge[4];

        /** One side's drawn scroll edge effect, colors in ARGB. */
        static final class Edge {
            int effect;
            float extent;
            int tint, line;
        }

        boolean snaps() {
            return paging || interval > 0 || offsets.length > 0;
        }

        boolean zooms() {
            return maxZoom > minZoom;
        }

        /** How far past the top a released pull refreshes, in points. */
        float refreshPull() {
            return refreshIndicator > 0 ? refreshIndicator : REFRESH_PULL;
        }

        /** The room kept above the content while it refreshes, in points. */
        float refreshSpace() {
            return refreshIndicator > 0 ? refreshIndicator : REFRESH_SPACE;
        }

        /** Reads ItemScrollConfig as internal/platform/android/android.go writes it. */
        static Config read(DataInputStream in) throws IOException {
            Config c = new Config();
            int flags = in.readUnsignedByte();
            c.horizontal = Protocol.flag(flags, 0);
            c.hideIndicator = Protocol.flag(flags, 1);
            flags = in.readUnsignedByte();
            c.paging = Protocol.flag(flags, 0);
            c.oneStep = Protocol.flag(flags, 1);
            c.noScroll = Protocol.flag(flags, 2);
            c.noBounces = Protocol.flag(flags, 3);
            c.refreshable = Protocol.flag(flags, 4);
            c.refreshing = Protocol.flag(flags, 5);
            c.noBouncesZoom = Protocol.flag(flags, 6);
            c.noPinchZoom = Protocol.flag(flags, 7);
            c.centerContent = Protocol.flag(in.readUnsignedByte(), 0);
            c.interval = in.readFloat();
            c.alignment = in.readUnsignedByte();
            c.offsets = new float[in.readInt()];
            for (int i = 0; i < c.offsets.length; i++) c.offsets[i] = in.readFloat();
            java.util.Arrays.sort(c.offsets);
            c.rate = in.readFloat();
            c.insetTop = in.readFloat();
            c.insetRight = in.readFloat();
            c.insetBottom = in.readFloat();
            c.insetLeft = in.readFloat();
            c.dismiss = in.readUnsignedByte();
            c.taps = in.readUnsignedByte();
            c.minZoom = in.readFloat();
            c.maxZoom = in.readFloat();
            c.zoom = in.readFloat();
            c.refreshIndicator = in.readFloat();
            c.headerInset = in.readFloat();
            for (int i = 0; i < 4; i++) {
                int effect = in.readUnsignedByte();
                if (effect != 0) {
                    Edge e = new Edge();
                    e.effect = effect;
                    e.extent = in.readFloat();
                    int tint = in.readInt();
                    e.tint = (tint >>> 8) | (tint << 24);
                    int line = in.readInt();
                    e.line = (line >>> 8) | (line << 24);
                    c.edges[i] = e;
                }
            }
            return c;
        }
    }

    /** How long (ms) an eased move of distance starting at velocity takes, as NSH's snap_duration. */
    static long snapDuration(float distance, float velocity) {
        if (Math.abs(distance) < 0.5f) return 0;
        if (velocity * distance > 0) return (long) Math.min(Math.max(3 * Math.abs(distance) / Math.abs(velocity), 180), 600);
        return ANIMATION;
    }

    /** The rubber band of iOS: how far content pulled over points past an edge of a dim-point viewport moves. */
    static float rubber(float over, float dim) {
        dim = Math.max(dim, 1);
        return (1 - 1 / (over * 0.55f / dim + 1)) * dim;
    }

    /** What a scroll view asks of its runtime. */
    interface Host {
        /** Reports kind with the offset (points, UIKit's contentOffset) and zoom, if the node listens. */
        void scrollEvent(int kind, float x, float y, float zoom);

        /** The offset moved: values bound to it follow, and EventScroll goes out. */
        void scrolled(float x, float y, float zoom);

        /** Takes keyboard focus away, hiding the keyboard; false when nothing had it. */
        boolean dismissKeyboard();

        /** Whether a tap at the raw screen point lands on a text field. */
        boolean onInput(float rawX, float rawY);

        /** Whether a tap at the raw screen point lands on a view that takes presses. */
        boolean handlesTap(float rawX, float rawY);

        /** The scroll view's zoom scale changed; texts and canvases inside may need sharper pixels. */
        default void zoomChanged(float zoom) {}
    }

    /** A tap while the keyboard shows dismisses it, as KeyboardShouldPersistTaps says. */
    private static void tapped(Config config, Host host, MotionEvent e) {
        float x = e.getRawX(), y = e.getRawY();
        if (config.taps == 1 || host.onInput(x, y)) return; // always
        if (config.taps == 2 && host.handlesTap(x, y)) return; // handled
        host.dismissKeyboard();
    }

    /** A scroll view of the runtime: VScroll, HScroll or ZoomScroll. */
    interface Scroller {
        void configure(Config c);

        /** Scrolls to the offset in points (UIKit's contentOffset), clamped. */
        void scrollToPoints(float x, float y, boolean animated);

        /**
         * Moves the offset by points, carrying a glide or snap in flight
         * along: the core keeps the visible content in place as rows above
         * it change size.
         */
        void scrollByPoints(float dx, float dy);

        float offsetX();

        float offsetY();

        /** How far the view is pulled past its top, its refresh room included, in points. */
        float pulled();

        /** Releases any sibling overlay views attached to the outer frame before replacing the scroller. */
        default void detach() {}

        /** Sets the backdrop source the scroll view's frame passes down while it draws, null for none, and pass, which hands its content the source forwarded through the scroll offset, or null. */
        void setInheritedBackdrop(Object source, BiConsumer<View, Object> pass);
    }

    /** The backdrop source a scroll view's frame passes down while it draws, from Android 12. */
    static final class InheritedBackdrop {
        private Object source;
        private BiConsumer<View, Object> pass;

        void set(Object source, BiConsumer<View, Object> pass) {
            this.source = source;
            this.pass = pass;
        }

        /** Runs dispatch, the scroller's own child drawing, with the source passed to its content. */
        void dispatch(ViewGroup scroller, Runnable dispatch) {
            if (source == null || pass == null || scroller.getChildCount() == 0) {
                dispatch.run();
                return;
            }
            View child = scroller.getChildAt(0);
            pass.accept(child, source);
            try {
                dispatch.run();
            } finally {
                pass.accept(child, null);
            }
        }
    }

    /**
     * What VScroll and HScroll share: the drag and its events, the glide
     * or snap after a fling, the pull to refresh and the keyboard.
     */
    static final class Driver {
        final FrameLayout view;
        final boolean horizontal;
        final Host host;
        final float density;
        final View content;
        final ViewGroup frame;
        Config config = new Config();
        private final int slop;
        private final OverScroller flinger;
        // The touch: where it went down, and whether it dragged.
        private float downX, downY, lastY;
        private boolean touching, dragging, flung;
        // The glide or eased move running, with its momentum events.
        private Runnable motion;
        private boolean momentum;
        // What ItemScrollBy moved the motion running by, in pixels.
        private int carry;
        // The pull to refresh, in pixels, and its spinner.
        private float pull, pullTarget;
        private boolean pulling, refreshing;
        private ProgressBar spinner;
        // Touch-draggable scroll handle overlay on frame, and layout listener keeping it synced.
        private HandleView handle;
        private final View.OnLayoutChangeListener layoutListener;

        Driver(FrameLayout view, boolean horizontal, Host host, float density, View content, ViewGroup frame) {
            this.view = view;
            this.horizontal = horizontal;
            this.host = host;
            this.density = density;
            this.content = content;
            this.frame = frame;
            slop = ViewConfiguration.get(view.getContext()).getScaledTouchSlop();
            flinger = new OverScroller(view.getContext());
            layoutListener = (v, l, t, r, b, ol, ot, or, ob) -> updateHandle();
            view.addOnLayoutChangeListener(layoutListener);
            content.addOnLayoutChangeListener(layoutListener);
        }

        int pos() {
            return horizontal ? view.getScrollX() : view.getScrollY();
        }

        /** The offset's range in pixels: the content and its padding (the inset) less the viewport. */
        int range() {
            if (horizontal) return Math.max(content.getWidth() + view.getPaddingLeft() + view.getPaddingRight() - view.getWidth(), 0);
            return Math.max(content.getHeight() + view.getPaddingTop() + view.getPaddingBottom() - view.getHeight(), 0);
        }

        float offsetX() {
            return view.getScrollX() / density - config.insetLeft;
        }

        float offsetY() {
            return (view.getScrollY() - pull) / density - config.insetTop;
        }

        float pulled() {
            return horizontal ? 0 : Math.max(pull - view.getScrollY(), 0) / density;
        }

        /** What the keyboard covers of the view, added to its bottom inset, in pixels. */
        private int keyboardPad;

        boolean phoneHandleEnabled() {
            int uiType = view.getResources().getConfiguration().uiMode & Configuration.UI_MODE_TYPE_MASK;
            return !config.hideIndicator && !config.noScroll && uiType != Configuration.UI_MODE_TYPE_TELEVISION;
        }

        void configure(Config c) {
            config = c;
            pad();
            view.setClipToPadding(false);
            view.setOverScrollMode(c.noBounces ? View.OVER_SCROLL_NEVER : View.OVER_SCROLL_IF_CONTENT_SCROLLS);
            boolean phoneHandle = phoneHandleEnabled();
            view.setVerticalScrollBarEnabled(!c.hideIndicator && !horizontal && !phoneHandle);
            view.setHorizontalScrollBarEnabled(!c.hideIndicator && horizontal && !phoneHandle);
            setRefreshing(c.refreshing && !horizontal);
            updateHandle();
        }

        void updateHandle() {
            boolean show = phoneHandleEnabled() && range() > 0;
            if (!show) {
                if (handle != null) {
                    handle.dragging = false;
                    handle.setVisibility(View.GONE);
                }
                return;
            }
            if (handle == null) {
                handle = new HandleView(view.getContext(), this);
                frame.addView(handle, new FrameLayout.LayoutParams(
                        ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
                handle.bringToFront();
            }
            handle.setVisibility(View.VISIBLE);
            handle.invalidate();
        }

        void detach() {
            view.removeOnLayoutChangeListener(layoutListener);
            content.removeOnLayoutChangeListener(layoutListener);
            if (handle != null) {
                frame.removeView(handle);
                handle = null;
            }
            showSpinner(false);
        }

        private void pad() {
            Config c = config;
            view.setPadding(Math.round(c.insetLeft * density), Math.round(c.insetTop * density),
                    Math.round(c.insetRight * density), Math.round(c.insetBottom * density) + keyboardPad);
        }

        /**
         * Keeps field clear of the keyboard whose top is at keyboardTop in
         * the window: what it covers of the view adds to the bottom inset,
         * and the view scrolls the field above it. A null field gives the
         * inset back.
         */
        void keepClear(View field, int keyboardTop) {
            int[] at = new int[2];
            int covered = 0;
            if (field != null) {
                view.getLocationInWindow(at);
                covered = Math.max(0, at[1] + view.getHeight() - keyboardTop);
            }
            if (covered != keyboardPad) {
                keyboardPad = covered;
                pad();
            }
            if (field == null || horizontal) return;
            field.getLocationInWindow(at);
            int margin = Math.round(8 * density);
            int over = at[1] + field.getHeight() + margin - keyboardTop;
            if (over > 0) {
                stop();
                // The new inset takes a layout pass to widen the range.
                view.post(() -> ease(Math.min(pos() + over, range()), ANIMATION, false));
            }
        }

        void scrollTo(float x, float y, boolean animated) {
            stop();
            int target = Math.round((horizontal ? x + config.insetLeft : y + config.insetTop) * density);
            target = Math.min(Math.max(target, 0), range());
            if (animated) ease(target, ANIMATION, false);
            else move(target);
        }

        void move(int px) {
            if (horizontal) view.scrollTo(px, 0);
            else view.scrollTo(0, px);
        }

        /** ItemScrollBy: a motion in flight carries the shift along; a drag goes on from where it is. */
        void scrollBy(float dx, float dy) {
            int d = Math.round((horizontal ? dx : dy) * density);
            if (d == 0) return;
            if (motion != null) carry += d;
            if (handle != null && handle.dragging) handle.shift(d);
            move(pos() + d);
        }

        void event(int kind) {
            host.scrollEvent(kind, offsetX(), offsetY(), 1);
        }

        /** Stops a glide or an eased move, which ends its momentum. */
        void stop() {
            flinger.forceFinished(true);
            if (motion != null) {
                view.removeCallbacks(motion);
                motion = null;
            }
            if (momentum) {
                momentum = false;
                event(Protocol.EVENT_MOMENTUM_END);
            }
        }

        /** Whether a glide or an eased move is running, which a finger landing brakes. */
        boolean moving() {
            return motion != null || momentum;
        }

        /** Whether the view takes the touch: a vertical refreshable view pulled down at its top. */
        boolean intercept(MotionEvent e) {
            if (e.getActionMasked() == MotionEvent.ACTION_DOWN) {
                downX = e.getX();
                downY = lastY = e.getY();
                return false;
            }
            return e.getActionMasked() == MotionEvent.ACTION_MOVE && pullable() && e.getY() - downY > slop
                    && Math.abs(e.getY() - downY) > Math.abs(e.getX() - downX);
        }

        private boolean pullable() {
            return !horizontal && config.refreshable && !refreshing && !config.noScroll && view.getScrollY() <= 0;
        }

        /**
         * Follows the view's touches before ScrollView sees them; true
         * when the pull to refresh takes this one.
         */
        boolean before(MotionEvent e) {
            switch (e.getActionMasked()) {
                case MotionEvent.ACTION_DOWN:
                    stop();
                    touching = true;
                    dragging = flung = false;
                    downX = e.getX();
                    downY = lastY = e.getY();
                    return false;
                case MotionEvent.ACTION_MOVE: {
                    float dy = e.getY() - lastY;
                    lastY = e.getY();
                    if (pulling || pullable() && dy > 0 && e.getY() - downY > slop) {
                        if (!pulling) {
                            pulling = true;
                            dragStart();
                        }
                        pull = Math.max(rubber(Math.max(e.getY() - downY - slop, 0) / density, view.getHeight() / density) * density, 0);
                        content.setTranslationY(pull);
                        showSpinner(pull > 0);
                        host.scrolled(offsetX(), offsetY(), 1);
                        if (pull <= 0 && dy < 0) pulling = false; // back to scrolling
                        return pulling;
                    }
                    return false;
                }
                case MotionEvent.ACTION_UP:
                case MotionEvent.ACTION_CANCEL:
                    if (!pulling) return false;
                    pulling = touching = false;
                    dragging = false;
                    event(Protocol.EVENT_SCROLL_DRAG_END);
                    if (pull >= config.refreshPull() * density && e.getActionMasked() == MotionEvent.ACTION_UP) {
                        // Shown right away; the next config says whether the app keeps it.
                        event(Protocol.EVENT_REFRESH);
                        refreshing = false;
                        setRefreshing(true);
                    } else {
                        settlePull(0);
                    }
                    return true;
            }
            return false;
        }

        /** Follows the view's touches after ScrollView handled them. */
        void after(MotionEvent e) {
            switch (e.getActionMasked()) {
                case MotionEvent.ACTION_MOVE:
                    if (!dragging && touching && Math.hypot(e.getX() - downX, e.getY() - downY) > slop) dragStart();
                    break;
                case MotionEvent.ACTION_UP:
                case MotionEvent.ACTION_CANCEL:
                    boolean tap = touching && !dragging && e.getActionMasked() == MotionEvent.ACTION_UP;
                    touching = false;
                    if (dragging) {
                        dragging = false;
                        if (!flung) {
                            event(Protocol.EVENT_SCROLL_DRAG_END);
                            settle(0);
                            if (!momentum) event(Protocol.EVENT_SCROLL_END);
                        }
                    }
                    if (tap) tapped(config, host, e);
                    break;
            }
        }

        private void dragStart() {
            dragging = true;
            event(Protocol.EVENT_SCROLL_DRAG_START);
            if (config.dismiss != 0) host.dismissKeyboard();
        }

        /** ScrollView flings at velocity, in pixels per second as the offset grows. */
        void fling(int velocity) {
            flung = true;
            if (dragging) {
                dragging = false;
                event(Protocol.EVENT_SCROLL_DRAG_END);
            }
            settle(velocity / 1000f / density);
            if (!momentum) event(Protocol.EVENT_SCROLL_END);
        }

        /** The user let go at v points per millisecond: a snap, a glide or nothing. */
        void settle(float v) {
            float pos = pos() / density;
            float max = range() / density;
            if (config.snaps()) {
                float lead = horizontal ? config.insetLeft : config.insetTop;
                float viewport = (horizontal ? view.getWidth() : view.getHeight()) / density;
                float length = max + viewport - lead; // the content and its trailing inset
                float to = ScrollPhysics.snapTarget(config.interval, config.alignment, config.offsets, config.paging, config.oneStep,
                        pos - lead, ScrollPhysics.projected(pos, v, config.rate) - lead, v, viewport, length);
                if (!Float.isNaN(to)) {
                    int target = Math.min(Math.max(Math.round((to + lead) * density), 0), range());
                    long duration = snapDuration(target / density - pos, v);
                    if (duration > 0) {
                        momentum = true;
                        event(Protocol.EVENT_MOMENTUM_START);
                        ease(target, duration, true);
                    }
                    return;
                }
            }
            if (Math.abs(v) > STOP * 5 && (v > 0 && pos < max || v < 0 && pos > 0)) glide(v);
        }

        private void glide(float velocity) {
            momentum = true;
            event(Protocol.EVENT_MOMENTUM_START);
            carry = 0;
            float friction = ViewConfiguration.getScrollFriction();
            if (config.rate > 0f && config.rate < 1f && config.rate != NORMAL) {
                friction *= (float) Math.pow(Math.log(config.rate) / Math.log(NORMAL), 1.358);
            }
            flinger.setFriction(friction);
            flinger.forceFinished(true);
            final int start = pos();
            final int vpx = Math.round(velocity * density * 1000f);
            flinger.fling(0, 0, 0, vpx, 0, 0, Integer.MIN_VALUE, Integer.MAX_VALUE);
            motion = new Runnable() {
                @Override
                public void run() {
                    if (motion != this) return;
                    boolean active = flinger.computeScrollOffset();
                    int max = range();
                    int at = Math.min(Math.max(start + flinger.getCurrY() + carry, 0), max);
                    move(at);
                    boolean done = !active || flinger.isFinished() || (vpx > 0 ? at >= max : at <= 0);
                    if (done) {
                        flinger.forceFinished(true);
                        finish();
                    } else {
                        view.postOnAnimation(this);
                    }
                }
            };
            view.postOnAnimation(motion);
        }

        private void ease(int target, long duration, boolean isMomentum) {
            final int from = pos();
            final long start = SystemClock.uptimeMillis();
            carry = 0;
            motion = new Runnable() {
                @Override
                public void run() {
                    if (motion != this) return;
                    float t = Math.min((SystemClock.uptimeMillis() - start) / (float) duration, 1);
                    float k = 1 - (1 - t) * (1 - t) * (1 - t);
                    move(Math.round(from + carry + (target - from) * k));
                    if (t >= 1) finish();
                    else view.postOnAnimation(this);
                }
            };
            view.postOnAnimation(motion);
        }

        /** A glide, a snap or an eased scroll ran to its end: the view is at rest. */
        private void finish() {
            motion = null;
            if (momentum) {
                momentum = false;
                event(Protocol.EVENT_MOMENTUM_END);
            }
            event(Protocol.EVENT_SCROLL_END);
        }

        /** Shows the spinner, or the app's indicator, and its room above the content, or takes them away. */
        void setRefreshing(boolean on) {
            float room = config.refreshSpace() * density;
            if (on == refreshing && !(on && !pulling && pullTarget != room)) return;
            refreshing = on;
            if (pulling) return;
            settlePull(on ? room : 0);
        }

        private void settlePull(float to) {
            pullTarget = to;
            showSpinner(to > 0 || pull > 0);
            content.animate().translationY(to).setDuration(ANIMATION).setUpdateListener(a -> {
                pull = content.getTranslationY();
                host.scrolled(offsetX(), offsetY(), 1);
            }).withEndAction(() -> {
                pull = to;
                if (to == 0) showSpinner(false);
                host.scrolled(offsetX(), offsetY(), 1);
            }).start();
        }

        private void showSpinner(boolean on) {
            on = on && config.refreshIndicator <= 0;
            if (on && spinner == null) {
                spinner = new ProgressBar(view.getContext());
                spinner.setIndeterminate(true);
                int side = Math.round(28 * density);
                FrameLayout.LayoutParams lp = new FrameLayout.LayoutParams(side, side, Gravity.TOP | Gravity.CENTER_HORIZONTAL);
                lp.topMargin = Math.round((config.refreshSpace() * density - side) / 2 + config.insetTop * density);
                frame.addView(spinner, lp);
            } else if (!on && spinner != null) {
                frame.removeView(spinner);
                spinner = null;
            }
        }
    }

    /**
     * Touch-draggable scroll handle drawn over a vertical or horizontal
     * scroll view's viewport on Android phones and tablets, mirroring
     * internal/driver/scrollhandle.go.
     */
    static final class HandleView extends View {
        private final Driver d;
        private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
        private final RectF visual = new RectF();
        private final RectF grab = new RectF();
        boolean dragging;
        private float dragFrom, dragStart, dragTravel, dragSpan;

        HandleView(Context context, Driver d) {
            super(context);
            this.d = d;
            setWillNotDraw(false);
        }

        void shift(int deltaPx) {
            dragStart = Math.min(Math.max(dragStart + deltaPx, 0f), dragSpan);
        }

        /** Computes the visual and touch-grab rectangles in pixels; returns travel (> 0) or 0 when hidden. */
        private float layoutRects(boolean active) {
            int w = d.view.getWidth();
            int h = d.view.getHeight();
            int span = d.range();
            float viewport = d.horizontal ? w : h;
            float margin = HANDLE_MARGIN * d.density;
            if (span <= 0 || viewport <= 2f * margin + d.density) return 0f;
            float total = viewport + span;
            float track = viewport - 2f * margin;
            float minLen = Math.min(HANDLE_MIN_LENGTH * d.density, track / 2f);
            float length = Math.min(Math.max(track * viewport / total, minLen), track);
            float travel = Math.max(track - length, 0f);
            if (travel <= 0f) return 0f;
            float t = Math.min(Math.max(d.pos() / (float) span, 0f), 1f);
            float start = margin + travel * t;
            float thickness = (active ? HANDLE_ACTIVE_THICKNESS : HANDLE_THICKNESS) * d.density;
            float inset = HANDLE_INSET * d.density;
            float grabThickness = HANDLE_GRAB_THICKNESS * d.density;
            float grabSlop = HANDLE_GRAB_SLOP * d.density;
            if (d.horizontal) {
                float top = Math.max(h - inset - thickness, 0f);
                visual.set(start, top, start + length, Math.min(top + thickness, h));
                float gx0 = Math.max(start - grabSlop, 0f);
                float gx1 = Math.min(start + length + grabSlop, w);
                grab.set(gx0, Math.max(h - grabThickness, 0f), gx1, h);
            } else {
                float left = Math.max(w - inset - thickness, 0f);
                visual.set(left, start, Math.min(left + thickness, w), start + length);
                float gy0 = Math.max(start - grabSlop, 0f);
                float gy1 = Math.min(start + length + grabSlop, h);
                grab.set(Math.max(w - grabThickness, 0f), gy0, w, gy1);
            }
            return travel;
        }

        @Override
        public boolean onTouchEvent(MotionEvent e) {
            int action = e.getActionMasked();
            if (!dragging) {
                if (action != MotionEvent.ACTION_DOWN || !d.phoneHandleEnabled()) return false;
                float travel = layoutRects(false);
                if (travel <= 0f || !grab.contains(e.getX(), e.getY())) return false;
                int span = d.range();
                if (span <= 0) return false;
                d.stop();
                dragging = true;
                dragFrom = d.horizontal ? e.getX() : e.getY();
                dragStart = d.pos();
                dragTravel = travel;
                dragSpan = span;
                if (getParent() != null) getParent().requestDisallowInterceptTouchEvent(true);
                d.event(Protocol.EVENT_SCROLL_DRAG_START);
                invalidate();
                return true;
            }
            switch (action) {
                case MotionEvent.ACTION_MOVE: {
                    float at = d.horizontal ? e.getX() : e.getY();
                    float to = Math.min(Math.max(dragStart + (at - dragFrom) * dragSpan / dragTravel, 0f), dragSpan);
                    d.move(Math.round(to));
                    invalidate();
                    return true;
                }
                case MotionEvent.ACTION_UP:
                case MotionEvent.ACTION_CANCEL:
                    dragging = false;
                    invalidate();
                    d.event(Protocol.EVENT_SCROLL_DRAG_END);
                    d.settle(0);
                    if (!d.momentum) d.event(Protocol.EVENT_SCROLL_END);
                    return true;
            }
            return true;
        }

        @Override
        protected void onDraw(Canvas canvas) {
            if (!d.phoneHandleEnabled() || layoutRects(dragging) <= 0f) return;
            boolean dark = (getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES;
            int alpha = dragging ? 0xCC : 0x73;
            paint.setColor((alpha << 24) | (dark ? 0x00FFFFFF : 0x00000000));
            float r = Math.min(visual.width(), visual.height()) / 2f;
            canvas.drawRoundRect(visual, r, r, paint);
        }
    }

    /** A vertical scroll view. */
    static final class VScroll extends ScrollView implements Scroller {
        final Driver d;
        private final InheritedBackdrop backdrop = new InheritedBackdrop();

        VScroll(Context context, Host host, float density, View content, ViewGroup frame) {
            super(context);
            d = new Driver(this, false, host, density, content, frame);
        }

        @Override
        public void setInheritedBackdrop(Object source, BiConsumer<View, Object> pass) {
            backdrop.set(source, pass);
        }

        @Override
        protected void dispatchDraw(Canvas canvas) {
            backdrop.dispatch(this, () -> super.dispatchDraw(canvas));
        }

        @Override
        public boolean dispatchTouchEvent(MotionEvent e) {
            // A consuming child can keep DOWN out of onTouchEvent, so stop our own glide and remember the drag's origin before dispatching it. A touch that brakes a glide or an eased move is the view's, as on platform lists: the child under the finger never sees it, so lifting presses nothing and moving drags the view. ScrollView's own onTouchEvent takes that DOWN, as before() has followed it already; with no touch target, ViewGroup hands the view the rest of the touch. A view that does not scroll leaves the touch alone, as its onTouchEvent does.
            if (e.getActionMasked() == MotionEvent.ACTION_DOWN && !d.config.noScroll) {
                boolean braking = d.moving();
                d.before(e);
                if (braking) return super.onTouchEvent(e);
            }
            boolean handled = super.dispatchTouchEvent(e);
            // A child may consume UP, bypassing onTouchEvent. after() clears touching, so the view-owned path can safely have called it already.
            if (e.getActionMasked() == MotionEvent.ACTION_UP || e.getActionMasked() == MotionEvent.ACTION_CANCEL) d.after(e);
            return handled;
        }

        @Override
        public boolean onInterceptTouchEvent(MotionEvent e) {
            if (d.config.noScroll) return false;
            boolean pull = d.intercept(e);
            return super.onInterceptTouchEvent(e) || pull;
        }

        @Override
        public boolean onTouchEvent(MotionEvent e) {
            if (d.config.noScroll) return false;
            if (d.before(e)) return true;
            boolean r = super.onTouchEvent(e);
            d.after(e);
            return r;
        }

        @Override
        public void fling(int velocityY) {
            d.fling(velocityY);
        }

        @Override
        public void configure(Config c) {
            d.configure(c);
        }

        @Override
        public void scrollToPoints(float x, float y, boolean animated) {
            d.scrollTo(x, y, animated);
        }

        @Override
        public void scrollByPoints(float dx, float dy) {
            d.scrollBy(dx, dy);
        }

        @Override
        public float offsetX() {
            return d.offsetX();
        }

        @Override
        public float offsetY() {
            return d.offsetY();
        }

        @Override
        protected void onScrollChanged(int l, int t, int oldl, int oldt) {
            super.onScrollChanged(l, t, oldl, oldt);
            d.updateHandle();
        }

        @Override
        public void detach() {
            d.detach();
        }

        @Override
        public float pulled() {
            return d.pulled();
        }
    }

    /** A horizontal scroll view. */
    static final class HScroll extends HorizontalScrollView implements Scroller {
        final Driver d;
        private final InheritedBackdrop backdrop = new InheritedBackdrop();

        HScroll(Context context, Host host, float density, View content, ViewGroup frame) {
            super(context);
            d = new Driver(this, true, host, density, content, frame);
        }

        @Override
        public void setInheritedBackdrop(Object source, BiConsumer<View, Object> pass) {
            backdrop.set(source, pass);
        }

        @Override
        protected void dispatchDraw(Canvas canvas) {
            backdrop.dispatch(this, () -> super.dispatchDraw(canvas));
        }

        @Override
        public boolean dispatchTouchEvent(MotionEvent e) {
            if (e.getActionMasked() == MotionEvent.ACTION_DOWN && !d.config.noScroll) {
                boolean braking = d.moving();
                d.before(e);
                if (braking) return super.onTouchEvent(e);
            }
            boolean handled = super.dispatchTouchEvent(e);
            // A child may consume UP, bypassing onTouchEvent. after() clears touching, so the view-owned path can safely have called it already.
            if (e.getActionMasked() == MotionEvent.ACTION_UP || e.getActionMasked() == MotionEvent.ACTION_CANCEL) d.after(e);
            return handled;
        }

        @Override
        public boolean onInterceptTouchEvent(MotionEvent e) {
            if (d.config.noScroll) return false;
            d.intercept(e);
            return super.onInterceptTouchEvent(e);
        }

        @Override
        public boolean onTouchEvent(MotionEvent e) {
            if (d.config.noScroll) return false;
            d.before(e);
            boolean r = super.onTouchEvent(e);
            d.after(e);
            return r;
        }

        @Override
        public void fling(int velocityX) {
            d.fling(velocityX);
        }

        @Override
        public void configure(Config c) {
            d.configure(c);
        }

        @Override
        public void scrollToPoints(float x, float y, boolean animated) {
            d.scrollTo(x, y, animated);
        }

        @Override
        public void scrollByPoints(float dx, float dy) {
            d.scrollBy(dx, dy);
        }

        @Override
        public float offsetX() {
            return d.offsetX();
        }

        @Override
        public float offsetY() {
            return d.offsetY();
        }

        @Override
        protected void onScrollChanged(int l, int t, int oldl, int oldt) {
            super.onScrollChanged(l, t, oldl, oldt);
            d.updateHandle();
        }

        @Override
        public void detach() {
            d.detach();
        }

        @Override
        public float pulled() {
            return 0;
        }
    }

    /**
     * A zoomable scroll view: Android has none, so it scales its content
     * around its top left and scrolls both ways itself, pinched around the
     * fingers' focal point, panned with any number of fingers and flung
     * with an OverScroller. Pinches past the limits spring back unless
     * NoBouncesZoom.
     */
    static final class ZoomScroll extends FrameLayout implements Scroller {
        private final Host host;
        private final float density;
        private final View content;
        private final ScaleGestureDetector scale;
        private final OverScroller flinger;
        private final android.view.VelocityTracker[] tracker = {null};
        private final int slop;
        private Config config = new Config();
        private float zoom = 1;
        private float notifiedZoom = 1;
        private float lastX, lastY, downX, downY;
        private boolean dragging, pinching, momentum, tapping;
        private final InheritedBackdrop backdrop = new InheritedBackdrop();

        ZoomScroll(Context context, Host host, float density, View content) {
            super(context);
            this.host = host;
            this.density = density;
            this.content = content;
            content.setPivotX(0);
            content.setPivotY(0);
            flinger = new OverScroller(context);
            slop = ViewConfiguration.get(context).getScaledTouchSlop();
            scale = new ScaleGestureDetector(context, new ScaleGestureDetector.SimpleOnScaleGestureListener() {
                @Override
                public boolean onScaleBegin(ScaleGestureDetector g) {
                    if (config.noPinchZoom) return false;
                    pinching = true;
                    if (!dragging) {
                        dragging = true;
                        event(Protocol.EVENT_SCROLL_DRAG_START);
                    }
                    return true;
                }

                @Override
                public boolean onScale(ScaleGestureDetector g) {
                    zoomAround(limit(zoom * g.getScaleFactor(), true), g.getFocusX(), g.getFocusY());
                    return true;
                }
            });
            setClipChildren(true);
        }

        private void event(int kind) {
            host.scrollEvent(kind, offsetX(), offsetY(), zoom);
        }

        private float limit(float z, boolean rubber) {
            float lo = config.minZoom, hi = config.maxZoom;
            if (z > hi) return rubber && !config.noBouncesZoom ? hi * (float) Math.pow(z / hi, 0.3) : hi;
            if (z < lo) return rubber && !config.noBouncesZoom ? lo * (float) Math.pow(z / lo, 0.3) : lo;
            return z;
        }

        /** Where the content starts: inside the inset, centered while smaller than the viewport. */
        private float originX() {
            float in = config.insetLeft * density;
            if (!config.centerContent) return in;
            float free = getWidth() - in - config.insetRight * density - content.getWidth() * zoom;
            return in + Math.max(free / 2, 0);
        }

        private float originY() {
            float in = config.insetTop * density;
            if (!config.centerContent) return in;
            float free = getHeight() - in - config.insetBottom * density - content.getHeight() * zoom;
            return in + Math.max(free / 2, 0);
        }

        private int maxX() {
            return (int) Math.max(content.getWidth() * zoom + (config.insetLeft + config.insetRight) * density - getWidth(), 0);
        }

        private int maxY() {
            return (int) Math.max(content.getHeight() * zoom + (config.insetTop + config.insetBottom) * density - getHeight(), 0);
        }

        private void place() {
            content.setScaleX(zoom);
            content.setScaleY(zoom);
            content.setTranslationX(originX());
            content.setTranslationY(originY());
            if (zoom != notifiedZoom) {
                notifiedZoom = zoom;
                host.zoomChanged(zoom);
            }
        }

        private void moveTo(int x, int y) {
            scrollTo(Math.min(Math.max(x, 0), maxX()), Math.min(Math.max(y, 0), maxY()));
        }

        @Override
        protected void onScrollChanged(int l, int t, int oldl, int oldt) {
            super.onScrollChanged(l, t, oldl, oldt);
            host.scrolled(offsetX(), offsetY(), zoom);
        }

        @Override
        protected void onLayout(boolean changed, int l, int t, int r, int b) {
            super.onLayout(changed, l, t, r, b);
            place();
            moveTo(getScrollX(), getScrollY());
        }

        /** Zooms to z keeping the content under the focal point fx, fy (in the view) in place. */
        private void zoomAround(float z, float fx, float fy) {
            float cx = (getScrollX() + fx - originX()) / zoom, cy = (getScrollY() + fy - originY()) / zoom;
            zoom = z;
            place();
            moveTo(Math.round(cx * zoom + originX() - fx), Math.round(cy * zoom + originY() - fy));
            host.scrolled(offsetX(), offsetY(), zoom);
        }

        void zoomTo(float z, boolean hasCenter, float x, float y, boolean animated) {
            final float target = limit(z, false);
            final float fx = getWidth() / 2f, fy = getHeight() / 2f;
            final float cx = hasCenter ? x * density : (getScrollX() + fx - originX()) / zoom;
            final float cy = hasCenter ? y * density : (getScrollY() + fy - originY()) / zoom;
            if (!animated) {
                zoomCentered(target, cx, cy);
                return;
            }
            final float z0 = zoom;
            final float x0 = (getScrollX() + fx - originX()) / zoom, y0 = (getScrollY() + fy - originY()) / zoom;
            final long start = SystemClock.uptimeMillis();
            postOnAnimation(new Runnable() {
                @Override
                public void run() {
                    float t = Math.min((SystemClock.uptimeMillis() - start) / 250f, 1);
                    float k = 1 - (1 - t) * (1 - t) * (1 - t);
                    zoomCentered(z0 + (target - z0) * k, x0 + (cx - x0) * k, y0 + (cy - y0) * k);
                    if (t < 1) postOnAnimation(this);
                }
            });
        }

        /** Zooms to z with content point cx, cy (pixels, unzoomed) in the middle. */
        private void zoomCentered(float z, float cx, float cy) {
            zoom = z;
            place();
            moveTo(Math.round(cx * zoom + originX() - getWidth() / 2f), Math.round(cy * zoom + originY() - getHeight() / 2f));
            host.scrolled(offsetX(), offsetY(), zoom);
        }

        /**
         * Content that pans keeps the drag from a scroll view around it,
         * which takes what is left at its edges.
         */
        private void nest() {
            if (maxX() > 0 || maxY() > 0) startNestedScroll(SCROLL_AXIS_HORIZONTAL | SCROLL_AXIS_VERTICAL);
        }

        @Override
        public boolean canScrollHorizontally(int direction) {
            if (config.noScroll) return false;
            return direction > 0 ? getScrollX() < maxX() : getScrollX() > 0;
        }

        @Override
        public boolean canScrollVertically(int direction) {
            if (config.noScroll) return false;
            return direction > 0 ? getScrollY() < maxY() : getScrollY() > 0;
        }

        @Override
        public boolean dispatchTouchEvent(MotionEvent e) {
            int action = e.getActionMasked();
            if (action == MotionEvent.ACTION_DOWN) {
                tapping = !config.noScroll && flinger.isFinished() && !momentum;
                downX = e.getX();
                downY = e.getY();
            } else if (action == MotionEvent.ACTION_MOVE) {
                if (Math.hypot(e.getX() - downX, e.getY() - downY) > slop) tapping = false;
            } else if (action == MotionEvent.ACTION_POINTER_DOWN || action == MotionEvent.ACTION_CANCEL) {
                tapping = false;
            }
            boolean handled = super.dispatchTouchEvent(e);
            // The child can own the entire tap; panning and pinching are not taps.
            if (action == MotionEvent.ACTION_UP) {
                boolean tap = tapping;
                tapping = false;
                if (tap) tapped(config, host, e);
            }
            return handled;
        }

        @Override
        public boolean onInterceptTouchEvent(MotionEvent e) {
            if (config.noScroll) return false;
            switch (e.getActionMasked()) {
                case MotionEvent.ACTION_DOWN:
                    downX = lastX = e.getX();
                    downY = lastY = e.getY();
                    // A touch that brakes a fling is the view's, as in VScroll and HScroll: the child under the finger never sees it, so lifting presses nothing and moving pans the view.
                    if (!flinger.isFinished() || momentum) {
                        flinger.forceFinished(true);
                        endMomentum();
                        return true;
                    }
                    scale.onTouchEvent(e);
                    nest();
                    return false;
                case MotionEvent.ACTION_POINTER_DOWN:
                    // The view takes the touch at this event, which onTouchEvent
                    // then never sees: the pan goes on from the fingers' middle.
                    lastX = middleX(e, -1);
                    lastY = middleY(e, -1);
                    return true;
                case MotionEvent.ACTION_MOVE:
                    return Math.hypot(e.getX() - downX, e.getY() - downY) > slop;
            }
            return false;
        }

        /** The middle of e's fingers but the one at index skip (-1 for none). */
        private static float middleX(MotionEvent e, int skip) {
            float sum = 0;
            int n = 0;
            for (int i = 0; i < e.getPointerCount(); i++) {
                if (i != skip) {
                    sum += e.getX(i);
                    n++;
                }
            }
            return sum / n;
        }

        private static float middleY(MotionEvent e, int skip) {
            float sum = 0;
            int n = 0;
            for (int i = 0; i < e.getPointerCount(); i++) {
                if (i != skip) {
                    sum += e.getY(i);
                    n++;
                }
            }
            return sum / n;
        }

        @Override
        public boolean onTouchEvent(MotionEvent e) {
            if (config.noScroll) return false;
            scale.onTouchEvent(e);
            if (tracker[0] == null) tracker[0] = android.view.VelocityTracker.obtain();
            tracker[0].addMovement(e);
            // The fingers' middle pans while they pinch. A finger going up is
            // still in its event: the middle of those that stay is where the
            // next move goes on from, or the content jumps by half the
            // fingers' spread as one lifts.
            int up = e.getActionMasked() == MotionEvent.ACTION_POINTER_UP ? e.getActionIndex() : -1;
            float x = middleX(e, up), y = middleY(e, up);
            switch (e.getActionMasked()) {
                case MotionEvent.ACTION_DOWN:
                    downX = lastX = x;
                    downY = lastY = y;
                    nest(); // View's dispatch stopped the one onInterceptTouchEvent started
                    return true;
                case MotionEvent.ACTION_POINTER_DOWN:
                case MotionEvent.ACTION_POINTER_UP:
                    lastX = x;
                    lastY = y;
                    return true;
                case MotionEvent.ACTION_MOVE:
                    if (!dragging && Math.hypot(x - downX, y - downY) > slop) {
                        dragging = true;
                        event(Protocol.EVENT_SCROLL_DRAG_START);
                        if (config.dismiss != 0) host.dismissKeyboard();
                        if (getParent() != null) getParent().requestDisallowInterceptTouchEvent(true);
                    }
                    if (dragging) {
                        int fromX = getScrollX(), fromY = getScrollY();
                        int dx = Math.round(-(x - lastX)), dy = Math.round(-(y - lastY));
                        moveTo(fromX + dx, fromY + dy);
                        int movedX = getScrollX() - fromX, movedY = getScrollY() - fromY;
                        dispatchNestedScroll(movedX, movedY, dx - movedX, dy - movedY, null);
                    }
                    lastX = x;
                    lastY = y;
                    return true;
                case MotionEvent.ACTION_UP:
                case MotionEvent.ACTION_CANCEL:
                    stopNestedScroll();
                    tracker[0].computeCurrentVelocity(1000);
                    float vx = tracker[0].getXVelocity(), vy = tracker[0].getYVelocity();
                    tracker[0].recycle();
                    tracker[0] = null;
                    if (pinching) {
                        pinching = false;
                        float settled = limit(zoom, false);
                        if (settled != zoom) zoomTo(settled, false, 0, 0, true);
                    }
                    if (dragging) {
                        dragging = false;
                        event(Protocol.EVENT_SCROLL_DRAG_END);
                        flinger.fling(getScrollX(), getScrollY(), -Math.round(vx), -Math.round(vy), 0, maxX(), 0, maxY());
                        if (!flinger.isFinished()) {
                            momentum = true;
                            event(Protocol.EVENT_MOMENTUM_START);
                            postInvalidateOnAnimation();
                        } else {
                            event(Protocol.EVENT_SCROLL_END);
                        }
                    }
                    return true;
            }
            return true;
        }

        @Override
        public void computeScroll() {
            if (flinger.computeScrollOffset()) {
                moveTo(flinger.getCurrX(), flinger.getCurrY());
                postInvalidateOnAnimation();
            } else if (momentum) {
                // The fling ran to its end: at rest.
                endMomentum();
                event(Protocol.EVENT_SCROLL_END);
            }
        }

        private void endMomentum() {
            if (momentum) {
                momentum = false;
                event(Protocol.EVENT_MOMENTUM_END);
            }
        }

        @Override
        public void configure(Config c) {
            float old = config.zoom;
            boolean first = !config.zooms();
            config = c;
            setVerticalScrollBarEnabled(!c.hideIndicator);
            setHorizontalScrollBarEnabled(!c.hideIndicator);
            if (first || c.zoom != old) {
                if (first) zoom = limit(c.zoom, false);
                else zoomTo(c.zoom, false, 0, 0, true);
            } else {
                zoom = limit(zoom, false);
            }
            place();
            moveTo(getScrollX(), getScrollY());
        }

        @Override
        public void scrollToPoints(float x, float y, boolean animated) {
            moveTo(Math.round((x + config.insetLeft) * density), Math.round((y + config.insetTop) * density));
        }

        @Override
        public void scrollByPoints(float dx, float dy) {
            moveTo(getScrollX() + Math.round(dx * density), getScrollY() + Math.round(dy * density));
        }

        @Override
        public float offsetX() {
            return getScrollX() / density - config.insetLeft;
        }

        @Override
        public float offsetY() {
            return getScrollY() / density - config.insetTop;
        }

        @Override
        public float pulled() {
            return 0;
        }

        @Override
        public void setInheritedBackdrop(Object source, BiConsumer<View, Object> pass) {
            backdrop.set(source, pass);
        }

        @Override
        protected void dispatchDraw(Canvas canvas) {
            backdrop.dispatch(this, () -> super.dispatchDraw(canvas));
        }
    }
}
