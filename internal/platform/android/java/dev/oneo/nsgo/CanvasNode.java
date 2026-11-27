package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.Bitmap;
import android.graphics.BitmapShader;
import android.graphics.BlendMode;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.ColorMatrix;
import android.graphics.ColorMatrixColorFilter;
import android.graphics.DashPathEffect;
import android.graphics.LinearGradient;
import android.graphics.Matrix;
import android.graphics.Paint;
import android.graphics.Path;
import android.graphics.PorterDuff;
import android.graphics.PorterDuffXfermode;
import android.graphics.RadialGradient;
import android.graphics.Rect;
import android.graphics.RectF;
import android.graphics.Shader;
import android.graphics.SweepGradient;
import android.graphics.Typeface;
import android.os.Build;
import android.os.Bundle;
import android.view.MotionEvent;
import android.view.View;
import android.view.accessibility.AccessibilityEvent;
import android.view.accessibility.AccessibilityNodeInfo;
import android.view.accessibility.AccessibilityNodeProvider;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.util.ArrayDeque;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;

/**
 * A canvas node: it plays the latest drawing the engine recorded, as
 * android.go's writeDrawing writes it, in points over its background. The
 * ops play into a layer of their own, so clearing and the operators that
 * clear leave the background. Paths are built in the canvas's points, each
 * point through the transform current when it is added, as the web's
 * canvas builds them; strokes, text and images draw in the transform.
 * Arguments bound to animated nodes are patched into the ops each frame
 * they change. Named parts are virtual views for assistive technology.
 */
class CanvasNode extends View {
    /** An image canvases draw, decoded once, with its pixels a point. */
    static final class Picture {
        final Bitmap bitmap;
        final float scale;

        Picture(Bitmap bitmap, float scale) {
            this.bitmap = bitmap;
            this.scale = scale > 0 ? scale : 1;
        }
    }

    CanvasImages<Picture> pictures;
    private Map<String, Picture> pixels = Collections.emptyMap();

    /** A named part of the drawing, in points. */
    static final class Part {
        static final int ACTION_ACTIVATE = 1;
        static final int ACTION_INCREMENT = 2;
        static final int ACTION_DECREMENT = 4;
        static final int ACTION_SCROLL = 8;
        static final int ACTION_DISMISS = 16;

        final float x, y, w, h;
        final int role;
        final String id, label, hint, testID;
        final float value, min, max, step;
        final String valueText;
        final boolean selected, disabled, busy;
        final int expanded, checked, live, actions;

        Part(float x, float y, float w, float h, int role, String id, String label, String hint, String testID,
                float value, float min, float max, float step, String valueText,
                boolean selected, boolean disabled, boolean busy, int expanded, int checked, int live, int actions) {
            this.x = x;
            this.y = y;
            this.w = w;
            this.h = h;
            this.role = role;
            this.id = id != null ? id : "";
            this.label = label != null ? label : "";
            this.hint = hint != null ? hint : "";
            this.testID = testID != null ? testID : "";
            this.value = value;
            this.min = min;
            this.max = max;
            this.step = step;
            this.valueText = valueText != null ? valueText : "";
            this.selected = selected;
            this.disabled = disabled;
            this.busy = busy;
            this.expanded = expanded;
            this.checked = checked;
            this.live = live;
            this.actions = actions;
        }

        String nodeID() {
            return !id.isEmpty() ? id : testID;
        }

        boolean hasRange() {
            return max > min;
        }

        String liveText() {
            String val = valueText;
            if (val.isEmpty() && hasRange()) {
                val = Float.toString(value);
            }
            if (!label.isEmpty() && !val.isEmpty()) return label + ", " + val;
            if (!label.isEmpty()) return label;
            return val.trim();
        }
    }

    /** Presses at a point on the screen, on the view there that takes presses. */
    interface Presser {
        boolean pressAt(float rawX, float rawY);
    }

    /** Reports an assistive-technology action performed on a canvas node. */
    interface ActionListener {
        boolean onAction(String nodeID, String action, float value, boolean hasValue);
    }

    private byte[] ops;
    private int[] offsets = new int[0];
    private boolean[] colors = new boolean[0];
    private Part[] parts = new Part[0];
    Presser presser;
    ActionListener actionListener;
    String[] spoken;
    boolean handlesActions;
    private int hovered = -1;
    private int focusedPart = -1;
    /** Stepped raster scale (>1 when inside a scaled view or zooming scroll). */
    private float rasterScale = 1f;
    /** Canvas.ResolutionScale, in (0, 1], which multiplies rasterScale, and whether its pixels stretch smoothly. */
    private float resolution = 1f;
    private boolean smooth = true;
    private Bitmap backing;
    private boolean backingDirty = true;
    private boolean fullRedraw = true;
    /** Snapshot of ops as currently painted into backing, for partial redraw damage scans. */
    private byte[] drawnOps;
    /** Cached pixel clip from patch() for the next onDraw() at (pendingClipW, pendingClipH), or null. */
    private CanvasDamage.PixelRect pendingClip;
    private int pendingClipW, pendingClipH;
    private final Rect backingDst = new Rect();
    private final Paint backingPaint = new Paint(Paint.ANTI_ALIAS_FLAG | Paint.FILTER_BITMAP_FLAG);

    CanvasNode(Context context) {
        super(context);
    }

    float rasterScale() {
        return rasterScale;
    }

    private boolean usesBacking(int w, int h, float eff) {
        if (eff != 1f) return true;
        return offsets.length > 0
                && w <= RasterScale.MAX_SIDE
                && h <= RasterScale.MAX_SIDE
                && (long) w * h <= RasterScale.MAX_PIXELS;
    }

    void setRasterScale(float scale) {
        float next = scale > 1f ? scale : 1f;
        if (next == rasterScale) return;
        rasterScale = next;
        if (rasterScale <= 1f && resolution >= 1f && offsets.length == 0) releaseBacking();
        invalidate();
    }

    /** Sets Canvas.ResolutionScale, kept within (0, 1], and how its pixels stretch to the view. */
    void setResolution(float scale, boolean smooth) {
        float next = scale > 0f && scale < 1f ? scale : 1f;
        if (next == resolution && smooth == this.smooth) return;
        resolution = next;
        this.smooth = smooth;
        backingPaint.setFilterBitmap(smooth || next >= 1f);
        if (rasterScale <= 1f && resolution >= 1f && offsets.length == 0) releaseBacking();
        invalidate();
    }

    private void releaseBacking() {
        if (backing != null) {
            backing.recycle();
            backing = null;
        }
        backingDirty = true;
        fullRedraw = true;
        drawnOps = null;
        pendingClip = null;
    }

    private void snapshotDrawnOps() {
        if (ops == null) {
            drawnOps = null;
            return;
        }
        if (drawnOps == null || drawnOps.length != ops.length) {
            drawnOps = new byte[ops.length];
        }
        System.arraycopy(ops, 0, drawnOps, 0, ops.length);
    }

    @Override
    public void invalidate() {
        backingDirty = true;
        fullRedraw = true;
        pendingClip = null;
        super.invalidate();
    }

    @Override
    protected void onSizeChanged(int w, int h, int oldw, int oldh) {
        super.onSizeChanged(w, h, oldw, oldh);
        backingDirty = true;
        fullRedraw = true;
        drawnOps = null;
        pendingClip = null;
    }

    @Override
    protected void onDetachedFromWindow() {
        super.onDetachedFromWindow();
        releaseBacking();
    }

    /** Retains exactly the frames the new drawing uses. A new version can reuse a bitmap of its size. */
    void setPixels(String[] keys, Map<String, PixelFrame> frames) {
        if (keys.length == 0) {
            pixels = Collections.emptyMap();
            return;
        }
        Map<String, Picture> next = new HashMap<>();
        for (String key : keys) next.put(key, pixels.get(key));
        Map<Long, ArrayDeque<Bitmap>> spare = new HashMap<>();
        for (Map.Entry<String, Picture> entry : pixels.entrySet()) {
            Picture picture = entry.getValue();
            if (picture == null || next.containsKey(entry.getKey())) continue;
            Bitmap bitmap = picture.bitmap;
            long size = PixelFrame.size(bitmap.getWidth(), bitmap.getHeight());
            spare.computeIfAbsent(size, ignored -> new ArrayDeque<>()).add(bitmap);
        }
        for (Map.Entry<String, PixelFrame> entry : frames.entrySet()) {
            String key = entry.getKey();
            if (!next.containsKey(key)) continue;
            PixelFrame frame = entry.getValue();
            ArrayDeque<Bitmap> candidates = spare.get(frame.size());
            Bitmap bitmap = frame.copyInto(candidates == null ? null : candidates.poll());
            if (bitmap != null) next.put(key, new Picture(bitmap, 1));
        }
        pixels = next;
    }

    void setOps(byte[] ops, int[] offsets, boolean[] colors, Part[] parts) {
        this.ops = ops;
        this.offsets = offsets;
        this.colors = colors;
        drawnOps = null;
        if (ops == null || ops.length == 0 || (rasterScale <= 1f && resolution >= 1f && offsets.length == 0)) {
            releaseBacking();
        }
        Part[] prev = this.parts;
        boolean changed = parts.length != prev.length;
        this.parts = parts;
        if (focusedPart >= parts.length) focusedPart = -1;
        if (changed) {
            setImportantForAccessibility(parts.length > 0 ? IMPORTANT_FOR_ACCESSIBILITY_YES
                    : IMPORTANT_FOR_ACCESSIBILITY_AUTO);
        }
        announceLiveParts(prev, parts);
        if (parts.length > 0) sendAccessibilityEvent(AccessibilityEvent.TYPE_WINDOW_CONTENT_CHANGED);
        invalidate();
    }

    /** Writes the values of the bound nodes into the ops and redraws only the damaged area when safe. */
    void patch(double[] nums, int[] cols) {
        if (ops == null) return;
        ByteBuffer b = ByteBuffer.wrap(ops);
        boolean mutated = false;
        for (int i = 0; i < offsets.length && i < nums.length; i++) {
            int at = offsets[i];
            if (at < 0 || at + 4 > ops.length) continue;
            if (colors[i]) {
                int next = cols[i];
                if (b.getInt(at) != next) {
                    b.putInt(at, next);
                    mutated = true;
                }
            } else {
                float next = (float) nums[i];
                if (Float.floatToRawIntBits(b.getFloat(at)) != Float.floatToRawIntBits(next)) {
                    b.putFloat(at, next);
                    mutated = true;
                }
            }
        }
        if (!mutated && !backingDirty) return;
        if (fullRedraw || backing == null || drawnOps == null || drawnOps.length != ops.length) {
            backingDirty = true;
            pendingClip = null;
            super.invalidate();
            return;
        }
        if (backingDirty) {
            // Another patch arrived before onDraw consumed the previous one:
            // let onDraw scan once from drawnOps to the latest ops.
            pendingClip = null;
            return;
        }
        int w = getWidth(), h = getHeight();
        if (w <= 0 || h <= 0) {
            backingDirty = true;
            fullRedraw = true;
            pendingClip = null;
            super.invalidate();
            return;
        }
        CanvasDamage.Result dmg = CanvasDamage.scan(drawnOps, ops);
        if (!dmg.bounded) {
            backingDirty = true;
            fullRedraw = true;
            pendingClip = null;
            super.invalidate();
            return;
        }
        if (dmg.empty) {
            snapshotDrawnOps();
            return;
        }
        float density = getResources().getDisplayMetrics().density;
        float eff = RasterScale.canvasScale(w, h, rasterScale, resolution);
        int bw = eff != 1f ? RasterScale.scaledPixels(w, eff) : w;
        int bh = eff != 1f ? RasterScale.scaledPixels(h, eff) : h;
        float sx = density * ((float) bw / w);
        float sy = density * ((float) bh / h);
        CanvasDamage.PixelRect clip = CanvasDamage.pixelClip(dmg, sx, sy, bw, bh);
        if (clip == null) {
            snapshotDrawnOps();
            return;
        }
        pendingClip = clip;
        pendingClipW = bw;
        pendingClipH = bh;
        backingDirty = true;
        super.invalidate();
    }

    @Override
    protected void onDraw(Canvas canvas) {
        if (ops == null || ops.length == 0) return;
        int w = getWidth(), h = getHeight();
        if (w <= 0 || h <= 0) return;
        float density = getResources().getDisplayMetrics().density;
        float eff = RasterScale.canvasScale(w, h, rasterScale, resolution);
        if (usesBacking(w, h, eff)) {
            int bw = eff != 1f ? RasterScale.scaledPixels(w, eff) : w;
            int bh = eff != 1f ? RasterScale.scaledPixels(h, eff) : h;
            if (backing == null || backing.getWidth() != bw || backing.getHeight() != bh) {
                releaseBacking();
                backing = Bitmap.createBitmap(bw, bh, Bitmap.Config.ARGB_8888);
            }
            if (backingDirty) {
                updateBacking(w, h, bw, bh, density);
            }
            backingDst.set(0, 0, w, h);
            canvas.drawBitmap(backing, null, backingDst, backingPaint);
            return;
        }
        releaseBacking();
        int layer = canvas.saveLayer(0, 0, w, h, null);
        canvas.scale(density, density);
        new Player(canvas, w / density, h / density, pictures, pixels).play(ByteBuffer.wrap(ops));
        canvas.restoreToCount(layer);
    }

    private void updateBacking(int w, int h, int bw, int bh, float density) {
        float sx = density * ((float) bw / w);
        float sy = density * ((float) bh / h);
        CanvasDamage.PixelRect clip = null;
        if (!fullRedraw && drawnOps != null && drawnOps.length == ops.length) {
            if (pendingClip != null && pendingClipW == bw && pendingClipH == bh) {
                clip = pendingClip;
            } else {
                CanvasDamage.Result dmg = CanvasDamage.scan(drawnOps, ops);
                if (dmg.bounded) {
                    if (dmg.empty) {
                        snapshotDrawnOps();
                        backingDirty = false;
                        pendingClip = null;
                        return;
                    }
                    clip = CanvasDamage.pixelClip(dmg, sx, sy, bw, bh);
                    if (clip == null) {
                        snapshotDrawnOps();
                        backingDirty = false;
                        pendingClip = null;
                        return;
                    }
                }
            }
        }
        Canvas bmCanvas = new Canvas(backing);
        if (clip != null) {
            int save = bmCanvas.save();
            bmCanvas.clipRect(clip.left, clip.top, clip.right, clip.bottom);
            bmCanvas.drawColor(Color.TRANSPARENT, PorterDuff.Mode.CLEAR);
            bmCanvas.scale(sx, sy);
            new Player(bmCanvas, w / density, h / density, pictures, pixels).play(ByteBuffer.wrap(ops));
            bmCanvas.restoreToCount(save);
        } else {
            backing.eraseColor(Color.TRANSPARENT);
            bmCanvas.scale(sx, sy);
            new Player(bmCanvas, w / density, h / density, pictures, pixels).play(ByteBuffer.wrap(ops));
        }
        snapshotDrawnOps();
        backingDirty = false;
        fullRedraw = false;
        pendingClip = null;
    }

    /** Replays a static drawing, also used by the tab bar's SVG icons. */
    static void play(Canvas canvas, float width, float height, byte[] ops) {
        new Player(canvas, width, height, null, Collections.emptyMap()).play(ByteBuffer.wrap(ops));
    }

    // Assistive technology sees the parts as virtual views.

    @Override
    public AccessibilityNodeProvider getAccessibilityNodeProvider() {
        if (parts.length == 0) return super.getAccessibilityNodeProvider();
        return provider;
    }

    @Override
    public boolean dispatchHoverEvent(MotionEvent e) {
        if (parts.length == 0) return super.dispatchHoverEvent(e);
        float density = getResources().getDisplayMetrics().density;
        int at = -1;
        if (e.getAction() != MotionEvent.ACTION_HOVER_EXIT) {
            float x = e.getX() / density, y = e.getY() / density;
            for (int i = parts.length - 1; i >= 0; i--) {
                Part p = parts[i];
                if (x >= p.x && y >= p.y && x < p.x + p.w && y < p.y + p.h) {
                    at = i;
                    break;
                }
            }
        }
        if (at != hovered) {
            if (at >= 0) sendPartEvent(at, AccessibilityEvent.TYPE_VIEW_HOVER_ENTER);
            if (hovered >= 0) sendPartEvent(hovered, AccessibilityEvent.TYPE_VIEW_HOVER_EXIT);
            hovered = at;
        }
        return at >= 0 || super.dispatchHoverEvent(e);
    }

    private void sendPartEvent(int part, int type) {
        AccessibilityEvent e = AccessibilityEvent.obtain(type);
        e.setPackageName(getContext().getPackageName());
        e.setSource(this, part);
        if (getParent() != null) getParent().requestSendAccessibilityEvent(this, e);
    }

    /** Moves TalkBack accessibility focus to the named canvas part, if present. */
    void focusPart(String nodeID) {
        if (nodeID == null || nodeID.isEmpty()) return;
        for (int i = 0; i < parts.length; i++) {
            if (nodeID.equals(parts[i].nodeID())) {
                focusedPart = i;
                sendPartEvent(i, AccessibilityEvent.TYPE_VIEW_ACCESSIBILITY_FOCUSED);
                return;
            }
        }
    }

    private void announceLiveParts(Part[] prev, Part[] next) {
        if (prev == null || prev.length == 0 || next == null || next.length == 0) return;
        Map<String, String> byID = new HashMap<>();
        for (Part p : prev) {
            String key = p.nodeID();
            if (!key.isEmpty()) byID.put(key, p.liveText());
        }
        for (int i = 0; i < next.length; i++) {
            Part p = next[i];
            if (p.live == 0) continue;
            String text = p.liveText();
            if (text.isEmpty()) continue;
            String old = null;
            String key = p.nodeID();
            if (!key.isEmpty()) {
                old = byID.get(key);
            } else if (i < prev.length && prev[i].nodeID().isEmpty()) {
                old = prev[i].liveText();
            }
            if (old != null && !old.equals(text)) {
                sendPartEvent(i, AccessibilityEvent.TYPE_WINDOW_CONTENT_CHANGED);
                announceForAccessibility(text);
            }
        }
    }

    private final AccessibilityNodeProvider provider = new AccessibilityNodeProvider() {
        @Override
        public AccessibilityNodeInfo createAccessibilityNodeInfo(int id) {
            float density = getResources().getDisplayMetrics().density;
            if (id == View.NO_ID) {
                AccessibilityNodeInfo info = AccessibilityNodeInfo.obtain(CanvasNode.this);
                onInitializeAccessibilityNodeInfo(info);
                for (int i = 0; i < parts.length; i++) info.addChild(CanvasNode.this, i);
                return info;
            }
            if (id < 0 || id >= parts.length) return null;
            Part p = parts[id];
            AccessibilityNodeInfo info = AccessibilityNodeInfo.obtain(CanvasNode.this, id);
            info.setParent(CanvasNode.this);
            info.setPackageName(getContext().getPackageName());
            info.setEnabled(!p.disabled);
            info.setVisibleToUser(true);
            info.setContentDescription(p.label.isEmpty() ? null : p.label);
            info.setSelected(p.selected);
            String resID = !p.testID.isEmpty() ? p.testID : p.id;
            if (!resID.isEmpty()) info.setViewIdResourceName(resID);
            if (!p.hint.isEmpty() && Build.VERSION.SDK_INT >= 26) info.setHintText(p.hint);
            Accessibility.Description d = Accessibility.describe(
                    p.role, false, null, p.checked == 2, p.checked == 3, false, p.busy,
                    p.expanded != 0, p.expanded == 2, 0, p.valueText, spoken);
            if (d.className != null) info.setClassName(d.className);
            if (d.heading && Build.VERSION.SDK_INT >= 28) info.setHeading(true);
            if (d.roleDescription != null) {
                info.getExtras().putCharSequence("AccessibilityNodeInfo.roleDescription", d.roleDescription);
            }
            if (d.checkable || p.checked != 0) {
                info.setCheckable(true);
                info.setChecked(p.checked == 2);
            }
            if (p.hasRange()) {
                info.setRangeInfo(AccessibilityNodeInfo.RangeInfo.obtain(
                        AccessibilityNodeInfo.RangeInfo.RANGE_TYPE_FLOAT, p.min, p.max, p.value));
            }
            if (d.stateDescription != null && Build.VERSION.SDK_INT >= 30) {
                info.setStateDescription(d.stateDescription);
            }
            if (p.live == 1) info.setLiveRegion(ACCESSIBILITY_LIVE_REGION_POLITE);
            else if (p.live == 2) info.setLiveRegion(ACCESSIBILITY_LIVE_REGION_ASSERTIVE);
            if (focusedPart == id) {
                info.setAccessibilityFocused(true);
                info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_CLEAR_ACCESSIBILITY_FOCUS);
            } else {
                info.setAccessibilityFocused(false);
                info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_ACCESSIBILITY_FOCUS);
            }
            if (!p.disabled) {
                if ((p.actions & Part.ACTION_ACTIVATE) != 0 || (!handlesActions && presser != null)) {
                    info.setClickable(true);
                    info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_CLICK);
                }
                if ((p.actions & (Part.ACTION_INCREMENT | Part.ACTION_SCROLL)) != 0) {
                    info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_SCROLL_FORWARD);
                }
                if ((p.actions & (Part.ACTION_DECREMENT | Part.ACTION_SCROLL)) != 0) {
                    info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_SCROLL_BACKWARD);
                }
                if (p.hasRange() && (p.actions & (Part.ACTION_INCREMENT | Part.ACTION_DECREMENT)) != 0
                        && Build.VERSION.SDK_INT >= 24) {
                    info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_SET_PROGRESS);
                }
                if ((p.actions & Part.ACTION_DISMISS) != 0) {
                    info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_DISMISS);
                }
            }
            Rect parent = new Rect((int) (p.x * density), (int) (p.y * density), (int) ((p.x + p.w) * density),
                    (int) ((p.y + p.h) * density));
            info.setBoundsInParent(parent);
            int[] at = new int[2];
            getLocationOnScreen(at);
            parent.offset(at[0], at[1]);
            info.setBoundsInScreen(parent);
            return info;
        }

        @Override
        public boolean performAction(int id, int action, Bundle arguments) {
            if (id == View.NO_ID) return performAccessibilityAction(action, arguments);
            if (id < 0 || id >= parts.length) return false;
            if (action == AccessibilityNodeInfo.ACTION_ACCESSIBILITY_FOCUS) {
                focusedPart = id;
                sendPartEvent(id, AccessibilityEvent.TYPE_VIEW_ACCESSIBILITY_FOCUSED);
                return true;
            }
            if (action == AccessibilityNodeInfo.ACTION_CLEAR_ACCESSIBILITY_FOCUS) {
                if (focusedPart == id) {
                    focusedPart = -1;
                    sendPartEvent(id, AccessibilityEvent.TYPE_VIEW_ACCESSIBILITY_FOCUS_CLEARED);
                    return true;
                }
                return false;
            }
            Part p = parts[id];
            if (p.disabled) return false;
            if (action == AccessibilityNodeInfo.ACTION_CLICK) {
                if (handlesActions && (p.actions & Part.ACTION_ACTIVATE) != 0 && actionListener != null) {
                    return actionListener.onAction(p.nodeID(), "activate", 0f, false);
                }
                if (presser == null) return false;
                float density = getResources().getDisplayMetrics().density;
                int[] at = new int[2];
                getLocationOnScreen(at);
                return presser.pressAt(at[0] + (p.x + p.w / 2) * density, at[1] + (p.y + p.h / 2) * density);
            }
            if (!handlesActions || actionListener == null) return false;
            if (action == AccessibilityNodeInfo.ACTION_SCROLL_FORWARD) {
                if ((p.actions & Part.ACTION_INCREMENT) != 0) {
                    return actionListener.onAction(p.nodeID(), "increment", 0f, false);
                }
                if ((p.actions & Part.ACTION_SCROLL) != 0) {
                    return actionListener.onAction(p.nodeID(), "scroll", 0f, false);
                }
                return false;
            }
            if (action == AccessibilityNodeInfo.ACTION_SCROLL_BACKWARD) {
                if ((p.actions & Part.ACTION_DECREMENT) != 0) {
                    return actionListener.onAction(p.nodeID(), "decrement", 0f, false);
                }
                if ((p.actions & Part.ACTION_SCROLL) != 0) {
                    return actionListener.onAction(p.nodeID(), "scroll", 0f, false);
                }
                return false;
            }
            if (action == AccessibilityNodeInfo.ACTION_DISMISS) {
                if ((p.actions & Part.ACTION_DISMISS) != 0) {
                    return actionListener.onAction(p.nodeID(), "dismiss", 0f, false);
                }
                return false;
            }
            if (Build.VERSION.SDK_INT >= 24
                    && action == AccessibilityNodeInfo.AccessibilityAction.ACTION_SET_PROGRESS.getId()
                    && arguments != null && arguments.containsKey(AccessibilityNodeInfo.ACTION_ARGUMENT_PROGRESS_VALUE)) {
                float target = arguments.getFloat(AccessibilityNodeInfo.ACTION_ARGUMENT_PROGRESS_VALUE, p.value);
                if (target > p.value && (p.actions & Part.ACTION_INCREMENT) != 0) {
                    return actionListener.onAction(p.nodeID(), "increment", target, true);
                }
                if (target < p.value && (p.actions & Part.ACTION_DECREMENT) != 0) {
                    return actionListener.onAction(p.nodeID(), "decrement", target, true);
                }
                return false;
            }
            return false;
        }
    };

    /** A gradient as android.go's writeGradient writes it, made a shader in the space it paints in. */
    private static final class Gradient {
        int kind; // 0 linear, 1 radial, 2 conic
        float x1, y1, r1, x2, y2, r2, angle;
        final float[] m = new float[6];
        int spread;
        float[] offsets;
        int[] colors;

        static Gradient read(ByteBuffer b) {
            Gradient g = new Gradient();
            g.kind = b.get();
            g.x1 = b.getFloat();
            g.y1 = b.getFloat();
            g.r1 = b.getFloat();
            g.x2 = b.getFloat();
            g.y2 = b.getFloat();
            g.r2 = b.getFloat();
            g.angle = b.getFloat();
            for (int i = 0; i < 6; i++) g.m[i] = b.getFloat();
            g.spread = b.get();
            int n = b.getInt();
            g.offsets = new float[Math.max(n, 2)];
            g.colors = new int[Math.max(n, 2)];
            for (int i = 0; i < n; i++) {
                g.offsets[i] = b.getFloat();
                g.colors[i] = Text.argb(b.getInt());
            }
            if (n == 0) return null;
            if (n == 1) {
                g.offsets[1] = 1;
                g.colors[1] = g.colors[0];
            }
            return g;
        }

        /**
         * The shader, its gradient space mapped through the gradient's own
         * transform, then to; null for one Android cannot paint. Before API
         * 31 a radial gradient's focal circle is its center.
         */
        Shader shader(Matrix to) {
            Shader.TileMode tile = spread == Protocol.SPREAD_REFLECT ? Shader.TileMode.MIRROR
                    : spread == Protocol.SPREAD_REPEAT ? Shader.TileMode.REPEAT : Shader.TileMode.CLAMP;
            Shader shader;
            try {
                if (kind == 2) {
                    shader = new SweepGradient(x1, y1, colors, offsets);
                } else if (kind == 0) {
                    shader = new LinearGradient(x1, y1, x2, y2, colors, offsets, tile);
                } else if (Build.VERSION.SDK_INT >= 31) {
                    long[] packed = new long[colors.length];
                    for (int i = 0; i < colors.length; i++) packed[i] = Color.pack(colors[i]);
                    shader = new RadialGradient(x1, y1, Math.max(r1, 0), x2, y2, r2, packed, offsets, tile);
                } else {
                    if (r2 <= 0) return null;
                    // The focal circle's radius moves the stops outward.
                    float inner = Math.max(0, Math.min(r1 / r2, 1));
                    float[] stops = new float[offsets.length];
                    for (int i = 0; i < stops.length; i++) stops[i] = inner + offsets[i] * (1 - inner);
                    shader = new RadialGradient(x2, y2, r2, colors, stops, tile);
                }
            } catch (IllegalArgumentException e) {
                return null;
            }
            Matrix local = new Matrix(to);
            boolean identity = true;
            for (float v : m) identity &= v == 0;
            if (!identity) {
                Matrix own = new Matrix();
                own.setValues(new float[] {m[0], m[2], m[4], m[1], m[3], m[5], 0, 0, 1});
                local.preConcat(own);
            }
            // A sweep starts along the x axis: turn it to its angle.
            if (kind == 2) local.preRotate((float) Math.toDegrees(angle), x1, y1);
            shader.setLocalMatrix(local);
            return shader;
        }
    }

    /** The settings Save keeps and Restore brings back, besides the canvas's clip. */
    private static final class State {
        final Matrix ctm = new Matrix();
        int fill = Color.BLACK, stroke = Color.BLACK;
        float lineWidth = 1, alpha = 1, miter = 10, dashOffset;
        float[] dash;
        Paint.Cap cap = Paint.Cap.BUTT;
        Paint.Join join = Paint.Join.MITER;
        /** The gradients filling and stroking in place of the colors, or null. */
        Gradient fillGradient, strokeGradient;
        int composite;
        int shadowColor;
        float shadowX, shadowY, shadowBlur;

        State copy() {
            State s = new State();
            s.ctm.set(ctm);
            s.fill = fill;
            s.stroke = stroke;
            s.lineWidth = lineWidth;
            s.alpha = alpha;
            s.miter = miter;
            s.dash = dash;
            s.dashOffset = dashOffset;
            s.cap = cap;
            s.join = join;
            s.fillGradient = fillGradient;
            s.strokeGradient = strokeGradient;
            s.composite = composite;
            s.shadowColor = shadowColor;
            s.shadowX = shadowX;
            s.shadowY = shadowY;
            s.shadowBlur = shadowBlur;
            return s;
        }
    }

    // core.CompositeOp values.
    private static final int SOURCE_OVER = 0, SOURCE_IN = 1, SOURCE_OUT = 2, SOURCE_ATOP = 3, DESTINATION_OVER = 4,
            DESTINATION_IN = 5, DESTINATION_OUT = 6, DESTINATION_ATOP = 7, LIGHTER = 8, COPY = 9, XOR = 10,
            MULTIPLY = 11;

    private static final PorterDuff.Mode[] PORTER_DUFF = {PorterDuff.Mode.SRC_OVER, PorterDuff.Mode.SRC_IN,
            PorterDuff.Mode.SRC_OUT, PorterDuff.Mode.SRC_ATOP, PorterDuff.Mode.DST_OVER, PorterDuff.Mode.DST_IN,
            PorterDuff.Mode.DST_OUT, PorterDuff.Mode.DST_ATOP, PorterDuff.Mode.ADD, PorterDuff.Mode.SRC,
            PorterDuff.Mode.XOR, PorterDuff.Mode.MULTIPLY, PorterDuff.Mode.SCREEN, PorterDuff.Mode.OVERLAY,
            PorterDuff.Mode.DARKEN, PorterDuff.Mode.LIGHTEN};

    private static final class Player {
        private final Canvas canvas;
        private final float width, height;
        private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
        private final ArrayDeque<State> saved = new ArrayDeque<>();
        private final float[] pt = new float[6];
        private Path path = new Path();
        private boolean has;
        private float curX, curY, startX, startY; // in the canvas's points
        private State state = new State();

        private final CanvasImages<Picture> pictures;

        private final Map<String, Picture> pixels;

        Player(Canvas canvas, float width, float height, CanvasImages<Picture> pictures, Map<String, Picture> pixels) {
            this.pictures = pictures;
            this.pixels = pixels;
            this.canvas = canvas;
            this.width = width;
            this.height = height;
        }

        void play(ByteBuffer b) {
            while (b.hasRemaining()) {
                switch (b.get()) {
                    case Protocol.DRAW_SAVE:
                        canvas.save();
                        saved.push(state.copy());
                        break;
                    case Protocol.DRAW_RESTORE:
                        if (saved.isEmpty()) break;
                        canvas.restore();
                        state = saved.pop();
                        break;
                    case Protocol.DRAW_TRANSLATE:
                        state.ctm.preTranslate(b.getFloat(), b.getFloat());
                        break;
                    case Protocol.DRAW_SCALE:
                        state.ctm.preScale(b.getFloat(), b.getFloat());
                        break;
                    case Protocol.DRAW_ROTATE:
                        state.ctm.preRotate((float) Math.toDegrees(b.getFloat()));
                        break;
                    case Protocol.DRAW_TRANSFORM:
                        state.ctm.preConcat(matrix(b));
                        break;
                    case Protocol.DRAW_SET_TRANSFORM:
                        state.ctm.set(matrix(b));
                        break;
                    case Protocol.DRAW_BEGIN_PATH:
                        path = new Path();
                        has = false;
                        break;
                    case Protocol.DRAW_MOVE_TO:
                        moveTo(b.getFloat(), b.getFloat());
                        break;
                    case Protocol.DRAW_LINE_TO:
                        lineTo(b.getFloat(), b.getFloat());
                        break;
                    case Protocol.DRAW_QUAD_TO: {
                        float cx = b.getFloat(), cy = b.getFloat(), x = b.getFloat(), y = b.getFloat();
                        if (!has) moveTo(cx, cy);
                        map(cx, cy, x, y);
                        path.quadTo(pt[0], pt[1], pt[2], pt[3]);
                        curX = pt[2];
                        curY = pt[3];
                        break;
                    }
                    case Protocol.DRAW_CUBIC_TO: {
                        float x1 = b.getFloat(), y1 = b.getFloat(), x2 = b.getFloat(), y2 = b.getFloat();
                        float x = b.getFloat(), y = b.getFloat();
                        if (!has) moveTo(x1, y1);
                        cubic(x1, y1, x2, y2, x, y);
                        break;
                    }
                    case Protocol.DRAW_ARC:
                        arc(b.getFloat(), b.getFloat(), b.getFloat(), b.getFloat(), b.getFloat(), b.get() != 0);
                        break;
                    case Protocol.DRAW_RECT: {
                        float x = b.getFloat(), y = b.getFloat(), w = b.getFloat(), h = b.getFloat();
                        moveTo(x, y);
                        lineTo(x + w, y);
                        lineTo(x + w, y + h);
                        lineTo(x, y + h);
                        close();
                        moveTo(x, y);
                        break;
                    }
                    case Protocol.DRAW_ROUND_RECT: {
                        float x = b.getFloat(), y = b.getFloat(), w = b.getFloat(), h = b.getFloat(), r = b.getFloat();
                        r = Math.max(0, Math.min(r, Math.min(Math.abs(w), Math.abs(h)) / 2));
                        moveTo(x + r, y);
                        lineTo(x + w - r, y);
                        ellipseArc(x + w - r, y + r, r, r, -Math.PI / 2, Math.PI / 2);
                        lineTo(x + w, y + h - r);
                        ellipseArc(x + w - r, y + h - r, r, r, 0, Math.PI / 2);
                        lineTo(x + r, y + h);
                        ellipseArc(x + r, y + h - r, r, r, Math.PI / 2, Math.PI / 2);
                        lineTo(x, y + r);
                        ellipseArc(x + r, y + r, r, r, Math.PI, Math.PI / 2);
                        close();
                        break;
                    }
                    case Protocol.DRAW_ELLIPSE: {
                        float x = b.getFloat(), y = b.getFloat(), rx = b.getFloat(), ry = b.getFloat();
                        moveTo(x + rx, y);
                        ellipseArc(x, y, rx, ry, 0, 2 * Math.PI);
                        close();
                        break;
                    }
                    case Protocol.DRAW_CLOSE_PATH:
                        close();
                        break;
                    case Protocol.DRAW_FILL:
                        path.setFillType(b.get() != 0 ? Path.FillType.EVEN_ODD : Path.FillType.WINDING);
                        fill();
                        break;
                    case Protocol.DRAW_STROKE:
                        stroke();
                        break;
                    case Protocol.DRAW_CLIP:
                        path.setFillType(b.get() != 0 ? Path.FillType.EVEN_ODD : Path.FillType.WINDING);
                        canvas.clipPath(path);
                        break;
                    case Protocol.DRAW_FILL_COLOR:
                        state.fill = Text.argb(b.getInt());
                        state.fillGradient = null;
                        break;
                    case Protocol.DRAW_STROKE_COLOR:
                        state.stroke = Text.argb(b.getInt());
                        state.strokeGradient = null;
                        break;
                    case Protocol.DRAW_FILL_GRADIENT: {
                        // An invalid gradient keeps the color before it.
                        Gradient g = Gradient.read(b);
                        if (g != null) state.fillGradient = g;
                        break;
                    }
                    case Protocol.DRAW_STROKE_GRADIENT: {
                        Gradient g = Gradient.read(b);
                        if (g != null) state.strokeGradient = g;
                        break;
                    }
                    case Protocol.DRAW_LINE_WIDTH:
                        state.lineWidth = b.getFloat();
                        break;
                    case Protocol.DRAW_LINE_CAP: {
                        int cap = b.get();
                        state.cap = cap == Protocol.CAP_ROUND ? Paint.Cap.ROUND
                                : cap == Protocol.CAP_SQUARE ? Paint.Cap.SQUARE : Paint.Cap.BUTT;
                        break;
                    }
                    case Protocol.DRAW_LINE_JOIN: {
                        int join = b.get();
                        state.join = join == Protocol.JOIN_ROUND ? Paint.Join.ROUND
                                : join == Protocol.JOIN_BEVEL ? Paint.Join.BEVEL : Paint.Join.MITER;
                        break;
                    }
                    case Protocol.DRAW_MITER_LIMIT:
                        state.miter = b.getFloat();
                        break;
                    case Protocol.DRAW_LINE_DASH: {
                        int n = b.getInt();
                        float[] dash = new float[n];
                        for (int i = 0; i < n; i++) dash[i] = b.getFloat();
                        boolean any = false;
                        for (float d : dash) any |= d > 0;
                        state.dash = n >= 2 && any ? dash : null;
                        break;
                    }
                    case Protocol.DRAW_LINE_DASH_OFFSET:
                        state.dashOffset = b.getFloat();
                        break;
                    case Protocol.DRAW_ALPHA:
                        state.alpha = Math.max(0, Math.min(1, b.getFloat()));
                        break;
                    case Protocol.DRAW_COMPOSITE:
                        state.composite = b.get();
                        break;
                    case Protocol.DRAW_SHADOW:
                        state.shadowColor = Text.argb(b.getInt());
                        state.shadowX = b.getFloat();
                        state.shadowY = b.getFloat();
                        state.shadowBlur = Math.max(0, b.getFloat());
                        break;
                    case Protocol.DRAW_FILL_TEXT:
                        text(b, false);
                        break;
                    case Protocol.DRAW_STROKE_TEXT:
                        text(b, true);
                        break;
                    case Protocol.DRAW_IMAGE:
                        image(b);
                        break;
                    case Protocol.DRAW_VERTICES:
                        vertices(b);
                        break;
                    case Protocol.DRAW_ATLAS:
                        atlas(b);
                        break;
                    case Protocol.DRAW_CLEAR: {
                        float x = b.getFloat(), y = b.getFloat(), w = b.getFloat(), h = b.getFloat();
                        canvas.save();
                        canvas.concat(state.ctm);
                        canvas.clipRect(x, y, x + w, y + h);
                        canvas.drawColor(Color.TRANSPARENT, PorterDuff.Mode.CLEAR);
                        canvas.restore();
                        break;
                    }
                    default:
                        return; // an op this runtime does not know: its fields are unknown too
                }
            }
        }

        private static Matrix matrix(ByteBuffer b) {
            float a = b.getFloat(), bb = b.getFloat(), c = b.getFloat(), d = b.getFloat();
            float e = b.getFloat(), f = b.getFloat();
            Matrix m = new Matrix();
            m.setValues(new float[] {a, c, e, bb, d, f, 0, 0, 1});
            return m;
        }

        /** Maps up to three points through the transform into pt. */
        private void map(float... xy) {
            System.arraycopy(xy, 0, pt, 0, xy.length);
            state.ctm.mapPoints(pt, 0, pt, 0, xy.length / 2);
        }

        private void moveTo(float x, float y) {
            map(x, y);
            path.moveTo(pt[0], pt[1]);
            curX = startX = pt[0];
            curY = startY = pt[1];
            has = true;
        }

        private void lineTo(float x, float y) {
            if (!has) {
                moveTo(x, y);
                return;
            }
            map(x, y);
            path.lineTo(pt[0], pt[1]);
            curX = pt[0];
            curY = pt[1];
        }

        private void cubic(float x1, float y1, float x2, float y2, float x, float y) {
            map(x1, y1, x2, y2, x, y);
            path.cubicTo(pt[0], pt[1], pt[2], pt[3], pt[4], pt[5]);
            curX = pt[4];
            curY = pt[5];
        }

        private void close() {
            if (!has) return;
            path.close();
            curX = startX;
            curY = startY;
        }

        /** Adds cubic curves along an ellipse around (cx, cy) from angle a0 by sweep, from where the path is. */
        private void ellipseArc(double cx, double cy, double rx, double ry, double a0, double sweep) {
            int n = Math.max((int) Math.ceil(Math.abs(sweep) / (Math.PI / 2) - 1e-6), 1);
            double step = sweep / n, k = 4.0 / 3 * Math.tan(step / 4), a = a0;
            for (int i = 0; i < n; i++) {
                double s0 = Math.sin(a), c0 = Math.cos(a), s1 = Math.sin(a + step), c1 = Math.cos(a + step);
                cubic((float) (cx + rx * (c0 - k * s0)), (float) (cy + ry * (s0 + k * c0)),
                        (float) (cx + rx * (c1 + k * s1)), (float) (cy + ry * (s1 - k * c1)),
                        (float) (cx + rx * c1), (float) (cy + ry * s1));
                a += step;
            }
        }

        /**
         * Adds an arc as the web's canvas does: a line to its start, then
         * clockwise from a0 to a1 unless ccw, a full circle at most.
         */
        private void arc(float x, float y, float r, float a0, float a1, boolean ccw) {
            r = Math.max(r, 0);
            double tau = 2 * Math.PI;
            double d = ccw ? a0 - a1 : a1 - a0;
            double sweep = d >= tau ? tau : ((d % tau) + tau) % tau;
            if (ccw) sweep = -sweep;
            float sx = (float) (x + r * Math.cos(a0)), sy = (float) (y + r * Math.sin(a0));
            if (has) {
                lineTo(sx, sy);
            } else {
                moveTo(sx, sy);
            }
            if (sweep != 0) ellipseArc(x, y, r, r, a0, sweep);
        }

        /** Sets the paint to the color, or the gradient over it in to's space, under the global alpha. */
        private boolean paintWith(int color, Gradient g, Matrix to) {
            paint.reset();
            paint.setAntiAlias(true);
            if (g != null) {
                Shader s = g.shader(to);
                if (s == null) return false;
                // A shader takes only the paint's alpha.
                paint.setColor(fade(Color.BLACK));
                paint.setShader(s);
            } else {
                paint.setColor(fade(color));
            }
            return true;
        }

        /**
         * Draws with the paint as draw does, in the shadow when there is
         * one: its offset and blur in the canvas's points, whatever the
         * transform inTransform says the drawing is under. An operator
         * that clears outside the shape draws into a layer of the whole
         * canvas, which it mixes by.
         */
        private void draw(Runnable draw, boolean inTransform) {
            if (Color.alpha(state.shadowColor) != 0 && (state.shadowX != 0 || state.shadowY != 0 || state.shadowBlur != 0)) {
                float dx = state.shadowX, dy = state.shadowY, blur = state.shadowBlur;
                if (inTransform) {
                    Matrix inv = new Matrix();
                    if (state.ctm.invert(inv)) {
                        float[] v = {dx, dy};
                        inv.mapVectors(v);
                        dx = v[0];
                        dy = v[1];
                        blur = inv.mapRadius(blur);
                    }
                }
                // Skia's sigma is radius * 0.57735 + 0.5; the canvas's is
                // half the blur. A radius of 0 removes the shadow.
                float radius = Math.max((blur / 2 - 0.5f) / 0.57735f, 0.01f);
                paint.setShadowLayer(radius, dx, dy, state.shadowColor);
            }
            int c = state.composite;
            if (c == SOURCE_OVER) {
                draw.run();
                return;
            }
            Paint mix = new Paint();
            if (Build.VERSION.SDK_INT >= 29) {
                mix.setBlendMode(blendMode(c));
            } else if (c < PORTER_DUFF.length) {
                mix.setXfermode(new PorterDuffXfermode(PORTER_DUFF[c]));
            }
            boolean unbounded = c == SOURCE_IN || c == SOURCE_OUT || c == DESTINATION_IN || c == DESTINATION_ATOP
                    || c == COPY;
            if (unbounded) {
                int layer = canvas.saveLayer(null, mix);
                draw.run();
                canvas.restoreToCount(layer);
                return;
            }
            if (Build.VERSION.SDK_INT >= 29) {
                paint.setBlendMode(blendMode(c));
            } else if (c < PORTER_DUFF.length) {
                paint.setXfermode(new PorterDuffXfermode(PORTER_DUFF[c]));
            }
            draw.run();
        }

        private static BlendMode blendMode(int c) {
            switch (c) {
                case SOURCE_IN: return BlendMode.SRC_IN;
                case SOURCE_OUT: return BlendMode.SRC_OUT;
                case SOURCE_ATOP: return BlendMode.SRC_ATOP;
                case DESTINATION_OVER: return BlendMode.DST_OVER;
                case DESTINATION_IN: return BlendMode.DST_IN;
                case DESTINATION_OUT: return BlendMode.DST_OUT;
                case DESTINATION_ATOP: return BlendMode.DST_ATOP;
                case LIGHTER: return BlendMode.PLUS;
                case COPY: return BlendMode.SRC;
                case XOR: return BlendMode.XOR;
                case MULTIPLY: return BlendMode.MULTIPLY;
                case MULTIPLY + 1: return BlendMode.SCREEN;
                case MULTIPLY + 2: return BlendMode.OVERLAY;
                case MULTIPLY + 3: return BlendMode.DARKEN;
                case MULTIPLY + 4: return BlendMode.LIGHTEN;
                case MULTIPLY + 5: return BlendMode.COLOR_DODGE;
                case MULTIPLY + 6: return BlendMode.COLOR_BURN;
                case MULTIPLY + 7: return BlendMode.HARD_LIGHT;
                case MULTIPLY + 8: return BlendMode.SOFT_LIGHT;
                case MULTIPLY + 9: return BlendMode.DIFFERENCE;
                case MULTIPLY + 10: return BlendMode.EXCLUSION;
                case MULTIPLY + 11: return BlendMode.HUE;
                case MULTIPLY + 12: return BlendMode.SATURATION;
                case MULTIPLY + 13: return BlendMode.COLOR;
                case MULTIPLY + 14: return BlendMode.LUMINOSITY;
                default: return BlendMode.SRC_OVER;
            }
        }

        /** Whether source-over painting at zero alpha cannot change any pixel (its shadow included). */
        private boolean transparent() {
            return state.composite == SOURCE_OVER && state.alpha <= 0f;
        }

        /** Whether painting with color or gradient cannot change any pixel. */
        private boolean unseen(int color, Gradient g) {
            if (transparent()) return true;
            if (state.composite != SOURCE_OVER) return false;
            if (Color.alpha(state.shadowColor) != 0
                    && (state.shadowX != 0 || state.shadowY != 0 || state.shadowBlur != 0)) {
                return false;
            }
            if (g == null) return Color.alpha(color) == 0;
            for (int c : g.colors) {
                if (Color.alpha(c) != 0) return false;
            }
            return true;
        }

        /** Fills the path, built in the canvas's points, in which the gradient is mapped by the transform. */
        private void fill() {
            if (unseen(state.fill, state.fillGradient)) return;
            if (!paintWith(state.fill, state.fillGradient, state.ctm)) return;
            paint.setStyle(Paint.Style.FILL);
            draw(() -> canvas.drawPath(path, paint), false);
        }

        /** Strokes the path in the transform, as the web's canvas does: its width in the drawing's units. */
        private void stroke() {
            if (unseen(state.stroke, state.strokeGradient)) return;
            Matrix inv = new Matrix();
            if (!state.ctm.invert(inv)) return;
            if (!paintWith(state.stroke, state.strokeGradient, new Matrix())) return;
            strokeStyle();
            Path local = new Path();
            path.transform(inv, local);
            canvas.save();
            canvas.concat(state.ctm);
            draw(() -> canvas.drawPath(local, paint), true);
            canvas.restore();
        }

        private void strokeStyle() {
            paint.setStyle(Paint.Style.STROKE);
            paint.setStrokeWidth(state.lineWidth);
            paint.setStrokeCap(state.cap);
            paint.setStrokeJoin(state.join);
            paint.setStrokeMiter(state.miter);
            if (state.dash != null) paint.setPathEffect(new DashPathEffect(state.dash, state.dashOffset));
        }

        private void text(ByteBuffer b, boolean stroke) {
            String text = string(b);
            float x = b.getFloat(), y = b.getFloat();
            String family = string(b);
            float size = b.getFloat();
            int weight = b.getShort() & 0xffff;
            int flags = b.get() & 0xff;
            boolean italic = (flags & 1) != 0;
            boolean hasAxes = (flags & 2) != 0;
            boolean hasFeatures = (flags & 4) != 0;
            int variant = b.get() & 0xff;
            String variations = hasAxes ? string(b) : null;
            if (variations != null && variations.isEmpty()) variations = null;
            String features = hasFeatures ? string(b) : Text.features(variant);
            if (features != null && features.isEmpty()) features = null;
            if (unseen(stroke ? state.stroke : state.fill, stroke ? state.strokeGradient : state.fillGradient)) {
                return;
            }
            if (stroke) {
                if (!paintWith(state.stroke, state.strokeGradient, new Matrix())) return;
                strokeStyle();
            } else {
                if (!paintWith(state.fill, state.fillGradient, new Matrix())) return;
                paint.setStyle(Paint.Style.FILL);
            }
            paint.setTypeface(Text.typeface(family, weight, italic, variations));
            paint.setTextSize(size);
            paint.setFlags(paint.getFlags() | Paint.LINEAR_TEXT_FLAG | Paint.SUBPIXEL_TEXT_FLAG);
            paint.setFontFeatureSettings(features);
            if (Build.VERSION.SDK_INT >= 26) paint.setFontVariationSettings(variations);
            canvas.save();
            canvas.concat(state.ctm);
            draw(() -> canvas.drawText(text, x, y, paint), true);
            canvas.restore();
        }

        private void image(ByteBuffer b) {
            String key = string(b);
            b.getFloat(); // the density its name gives, which the picture carries
            float x = b.getFloat(), y = b.getFloat(), w = b.getFloat(), h = b.getFloat();
            float sx = b.getFloat(), sy = b.getFloat(), sw = b.getFloat(), sh = b.getFloat();
            byte flags = b.get();
            boolean nearest = (flags & 1) != 0, tinted = (flags & 2) != 0;
            int tint = tinted ? Text.argb(b.getInt()) : 0;
            if (transparent()) return;
            if (pixels.containsKey(key)) {
                Picture picture = pixels.get(key);
                if (picture != null) image(picture, x, y, w, h, sx, sy, sw, sh, nearest, tinted, tint);
            } else if (pictures != null && !key.isEmpty()) {
                try (CanvasImages<Picture>.Lease lease = pictures.acquire(key)) {
                    if (lease != null) image(lease.value, x, y, w, h, sx, sy, sw, sh, nearest, tinted, tint);
                }
            }
        }

        private void image(Picture p, float x, float y, float w, float h, float sx, float sy, float sw, float sh,
                boolean nearest, boolean tinted, int tint) {
            if (w == 0 && h == 0) {
                w = p.bitmap.getWidth() / p.scale;
                h = p.bitmap.getHeight() / p.scale;
            }
            Rect from = sw == 0 && sh == 0 ? null : new Rect(Math.round(sx * p.scale), Math.round(sy * p.scale),
                    Math.round((sx + sw) * p.scale), Math.round((sy + sh) * p.scale));
            paint.reset();
            paint.setAntiAlias(true);
            paint.setFilterBitmap(!nearest);
            paint.setAlpha(Math.round(255 * state.alpha));
            if (tinted) {
                ColorMatrix matrix = new ColorMatrix();
                matrix.setScale(Color.red(tint) / 255f, Color.green(tint) / 255f, Color.blue(tint) / 255f,
                        Color.alpha(tint) / 255f);
                paint.setColorFilter(new ColorMatrixColorFilter(matrix));
            }
            RectF dst = new RectF(x, y, x + w, y + h);
            canvas.save();
            canvas.concat(state.ctm);
            draw(() -> canvas.drawBitmap(p.bitmap, from, dst, paint), true);
            canvas.restore();
        }

        private void vertices(ByteBuffer b) {
            String key = string(b);
            boolean nearest = b.get() != 0;
            int count = b.getInt();
            int end = b.position() + count * 20;
            if (transparent()) {
                b.position(end);
                return;
            }
            if (pixels.containsKey(key)) {
                Picture picture = pixels.get(key);
                if (picture != null) vertices(b, count, picture, nearest);
            } else if (pictures != null && !key.isEmpty()) {
                try (CanvasImages<Picture>.Lease lease = pictures.acquire(key)) {
                    if (lease != null) vertices(b, count, lease.value, nearest);
                }
            }
            b.position(end);
        }

        private void vertices(ByteBuffer b, int count, Picture picture, boolean nearest) {
            float[] verts = new float[count * 2], tex = new float[count * 2];
            int[] colors = new int[count];
            for (int i = 0; i < count; i++) {
                verts[i * 2] = b.getFloat();
                verts[i * 2 + 1] = b.getFloat();
                tex[i * 2] = b.getFloat() * picture.scale;
                tex[i * 2 + 1] = b.getFloat() * picture.scale;
                int color = b.getInt();
                colors[i] = color == 0 ? Color.WHITE : Text.argb(color);
            }
            paint.reset();
            paint.setFilterBitmap(!nearest);
            paint.setAlpha(Math.round(255 * state.alpha));
            paint.setShader(new BitmapShader(picture.bitmap, Shader.TileMode.CLAMP, Shader.TileMode.CLAMP));
            canvas.save();
            canvas.concat(state.ctm);
            draw(() -> canvas.drawVertices(Canvas.VertexMode.TRIANGLES, verts.length, verts, 0,
                    tex, 0, colors, 0, null, 0, 0, paint), true);
            canvas.restore();
        }

        private void atlas(ByteBuffer ops) {
            String key = string(ops);
            boolean nearest = ops.get() != 0;
            int count = ops.getInt();
            ByteBuffer sprites = ops.slice();
            ops.position(ops.position() + count * AtlasMesh.SPRITE_BYTES);
            if (count == 0 || transparent()) return;
            if (pixels.containsKey(key)) {
                Picture picture = pixels.get(key);
                if (picture != null) atlas(picture, sprites, count, nearest);
            } else if (pictures != null && !key.isEmpty()) {
                try (CanvasImages<Picture>.Lease lease = pictures.acquire(key)) {
                    if (lease != null) atlas(lease.value, sprites, count, nearest);
                }
            }
        }

        private void atlas(Picture p, ByteBuffer sprites, int count, boolean nearest) {
            canvas.save();
            canvas.concat(state.ctm);
            boolean simple = state.composite == SOURCE_OVER && Color.alpha(state.shadowColor) == 0;
            if (simple && AtlasMesh.draw(canvas, p, sprites, count, state.alpha, nearest)) {
                canvas.restore();
                return;
            }
            Rect from = new Rect();
            RectF to = new RectF();
            paint.reset();
            paint.setAntiAlias(true);
            paint.setFilterBitmap(!nearest);
            for (int i = 0; i < count; i++) {
                float x = sprites.getFloat(), y = sprites.getFloat(), w = sprites.getFloat(), h = sprites.getFloat();
                float c = sprites.getFloat(), s = sprites.getFloat(), tx = sprites.getFloat(), ty = sprites.getFloat();
                float alpha = sprites.getFloat() * state.alpha;
                from.set(Math.round(x * p.scale), Math.round(y * p.scale),
                        Math.round((x + w) * p.scale), Math.round((y + h) * p.scale));
                paint.setAlpha(Math.round(255 * alpha));
                if (s == 0 && c > 0) {
                    to.set(tx, ty, tx + w * c, ty + h * c);
                    if (simple) {
                        canvas.drawBitmap(p.bitmap, from, to, paint);
                    } else {
                        draw(() -> canvas.drawBitmap(p.bitmap, from, to, paint), true);
                    }
                    continue;
                }
                to.set(0, 0, w, h);
                Matrix local = new Matrix();
                local.setValues(new float[]{c, -s, tx, s, c, ty, 0, 0, 1});
                canvas.save();
                canvas.concat(local);
                Matrix original = new Matrix(state.ctm);
                state.ctm.preConcat(local);
                draw(() -> canvas.drawBitmap(p.bitmap, from, to, paint), true);
                state.ctm.set(original);
                canvas.restore();
            }
            canvas.restore();
        }

        /** The color with the global alpha applied. */
        private int fade(int color) {
            int alpha = Math.round(Color.alpha(color) * state.alpha);
            return (color & 0x00ffffff) | (alpha << 24);
        }

        private static String string(ByteBuffer b) {
            byte[] bytes = new byte[b.getInt()];
            b.get(bytes);
            return new String(bytes, StandardCharsets.UTF_8);
        }
    }
}
