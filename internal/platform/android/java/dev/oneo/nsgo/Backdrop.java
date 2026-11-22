package dev.oneo.nsgo;

import android.graphics.Canvas;
import android.graphics.ColorMatrix;
import android.graphics.ColorMatrixColorFilter;
import android.graphics.Matrix;
import android.graphics.Path;
import android.graphics.RecordingCanvas;
import android.graphics.Rect;
import android.graphics.RectF;
import android.graphics.RenderEffect;
import android.graphics.RenderNode;
import android.graphics.Shader;
import android.graphics.drawable.Drawable;
import android.os.Build;
import android.view.View;
import android.view.ViewGroup;
import java.util.ArrayList;

/**
 * Draws views' BackdropFilters from Android 12 (API 31) through hardware RenderNodes and RenderEffects: each container holding a backdrop-filtered child or descendant records what it drew before that child into a RenderNode, runs the child's blur and color matrix RenderEffect over a padded region of it, and draws the result clipped to the child's rounded box before the child itself.
 *
 * Every RenderNode and RenderEffect reference lives in Api31, so earlier versions never resolve those classes and draw the view's own background alone.
 */
final class Backdrop {
    private Backdrop() {}

    /** Runtime queries a container uses during backdrop-aware drawing. */
    interface Host {
        /** The child's own backdrop filters, null for none. */
        BackdropMath.Spec backdrop(View child);

        Box backdropBox(View child);

        boolean hasBackdropDescendant(View child);

        void setInherited(View child, Object source);
    }

    /** Hooks back into the ViewGroup's own super drawing methods. */
    interface Callbacks {
        void superDraw(Canvas canvas);

        boolean drawChild(Canvas canvas, View child, long drawingTime);

        void superOnDrawForeground(Canvas canvas);
    }

    /** Allocates opaque per-container backdrop state on API 31+, null on older APIs. */
    static Object createState() {
        return Build.VERSION.SDK_INT >= 31 ? new Api31.State() : null;
    }

    /** Stores a transient inherited backdrop source on state (API 31+ only). */
    static void setInherited(Object state, Object source) {
        if (Build.VERSION.SDK_INT >= 31 && state != null) {
            Api31.setInherited(state, source);
        }
    }

    /** Composes an inherited backdrop source through a pass-through child view, such as a scroll view forwarding into its Content, so coordinates stay aligned across scroll offsets and transforms. */
    static Object forward(Object source, View child) {
        if (Build.VERSION.SDK_INT >= 31 && source != null && child != null) {
            return Api31.forward(source, child);
        }
        return null;
    }

    /** Composes a container's inherited backdrop source and its own background into a source forwarded to child: ScrollEdges uses it while its edges own its draw pass and a descendant inside the scroller blurs what is behind it. */
    static Object forwardContainerBackground(ViewGroup container, Object state, View child) {
        if (Build.VERSION.SDK_INT >= 31 && state != null && child != null) {
            return Api31.forwardContainerBackground(container, (Api31.State) state, child);
        }
        return null;
    }

    /** Releases pooled RenderNodes held by state. */
    static void release(Object state) {
        if (Build.VERSION.SDK_INT >= 31 && state != null) {
            Api31.release(state);
        }
    }

    /** Draws container with its children's backdrop filters on hardware canvases from API 31; before it, on software canvases and when no child or descendant has a backdrop filter, it is cb.superDraw. */
    static void drawContainer(ViewGroup container, Canvas canvas, Host host, Callbacks cb, Object state) {
        if (Build.VERSION.SDK_INT < 31 || state == null || !canvas.isHardwareAccelerated()
                || host == null || !host.hasBackdropDescendant(container)) {
            cb.superDraw(canvas);
            return;
        }
        Api31.drawContainer(container, canvas, host, cb, (Api31.State) state);
    }

    /** API 31+ RenderNode / RenderEffect implementation, never loaded on API < 31. */
    private static final class Api31 {
        private Api31() {}

        /** What an ancestor drew before a descendant container, in node's coordinate space. */
        static final class Source {
            final RenderNode node;
            /** Maps node's coordinate space into the receiving container's content space. */
            final Matrix toLocal;
            /** Valid drawn bounds of node, in node's coordinate space. */
            final RectF bounds;

            Source(RenderNode node, Matrix toLocal, RectF bounds) {
                this.node = node;
                this.toLocal = toLocal;
                this.bounds = bounds;
            }
        }

        static final class State {
            boolean drawing;
            Source inherited;
            final ArrayList<RenderNode> segments = new ArrayList<>();
            final ArrayList<RenderNode> blurs = new ArrayList<>();
            final Path tempPath = new Path();
            final Rect tempBounds = new Rect();
            final RectF tempRect = new RectF();

            RenderNode segment(int index, int w, int h) {
                while (segments.size() <= index) {
                    RenderNode n = new RenderNode("nsgo-backdrop-seg");
                    n.setClipToBounds(false);
                    segments.add(n);
                }
                RenderNode n = segments.get(index);
                n.setPosition(0, 0, w, h);
                n.setClipToBounds(false);
                return n;
            }

            RenderNode blur(int index, int w, int h) {
                while (blurs.size() <= index) {
                    RenderNode n = new RenderNode("nsgo-backdrop-blur");
                    n.setClipToBounds(true);
                    blurs.add(n);
                }
                RenderNode n = blurs.get(index);
                n.setPosition(0, 0, w, h);
                n.setClipToBounds(true);
                return n;
            }

            void trim(int usedSegs, int usedBlurs) {
                for (int i = segments.size() - 1; i >= usedSegs; i--) {
                    segments.remove(i).discardDisplayList();
                }
                for (int i = blurs.size() - 1; i >= usedBlurs; i--) {
                    blurs.remove(i).discardDisplayList();
                }
            }

            void release() {
                inherited = null;
                trim(0, 0);
            }
        }

        static void setInherited(Object state, Object source) {
            ((State) state).inherited = (Source) source;
        }

        static Object forward(Object sourceObj, View child) {
            Source source = (Source) sourceObj;
            Matrix childToParent = new Matrix(child.getMatrix());
            childToParent.postTranslate(child.getLeft(), child.getTop());
            Matrix parentToChild = new Matrix();
            if (!childToParent.invert(parentToChild)) return null;
            int sx = child.getScrollX(), sy = child.getScrollY();
            if ((sx | sy) != 0) parentToChild.postTranslate(sx, sy);
            Matrix nextToLocal = new Matrix(source.toLocal);
            nextToLocal.postConcat(parentToChild);
            return new Source(source.node, nextToLocal, source.bounds);
        }

        static Object forwardContainerBackground(ViewGroup container, State state, View child) {
            Source inherited = state.inherited;
            Drawable bg = container.getBackground();
            if (bg == null) {
                state.trim(0, 0);
                return inherited != null ? forward(inherited, child) : null;
            }
            int w = container.getWidth(), h = container.getHeight();
            if (w <= 0 || h <= 0) {
                return inherited != null ? forward(inherited, child) : null;
            }
            RectF accumBounds = new RectF(0, 0, w, h);
            if (inherited != null) {
                RectF mapped = new RectF();
                inherited.toLocal.mapRect(mapped, inherited.bounds);
                if (container.getClipToOutline() || container.getClipChildren()) {
                    if (!mapped.intersect(0, 0, w, h)) mapped.set(0, 0, w, h);
                } else {
                    mapped.union(0, 0, w, h);
                }
                accumBounds.set(mapped);
            }
            RenderNode seg = state.segment(0, w, h);
            RecordingCanvas rc = seg.beginRecording();
            if (inherited != null) {
                rc.save();
                rc.concat(inherited.toLocal);
                rc.drawRenderNode(inherited.node);
                rc.restore();
            }
            drawBackground(container, rc);
            seg.endRecording();
            state.trim(1, 0);
            return forward(new Source(seg, new Matrix(), accumBounds), child);
        }

        static void release(Object state) {
            ((State) state).release();
        }

        /** Builds a backdrop filter's RenderEffect: a Gaussian blur with clamped edges, chained with a ColorMatrixColorFilter for color filters. */
        static RenderEffect createEffect(float blurPx, float[] colorMatrix) {
            RenderEffect effect = null;
            if (blurPx > 0f && Float.isFinite(blurPx)) {
                effect = RenderEffect.createBlurEffect(blurPx, blurPx, Shader.TileMode.CLAMP);
            }
            if (colorMatrix != null) {
                ColorMatrixColorFilter cf = new ColorMatrixColorFilter(
                        new ColorMatrix(BackdropMath.colorMatrix255(colorMatrix)));
                effect = effect == null
                        ? RenderEffect.createColorFilterEffect(cf)
                        : RenderEffect.createColorFilterEffect(cf, effect);
            }
            return effect;
        }

        static void drawContainer(ViewGroup container, Canvas canvas, Host host, Callbacks cb, State state) {
            if (state.drawing) {
                // Re-entrancy guard: prevent any infinite recursion.
                cb.superDraw(canvas);
                return;
            }
            int w = container.getWidth(), h = container.getHeight();
            int count = container.getChildCount();
            if (w <= 0 || h <= 0 || count == 0) {
                cb.superDraw(canvas);
                return;
            }
            int[] order = childOrder(container, count);
            int lastNeed = -1;
            for (int oi = 0; oi < count; oi++) {
                View child = container.getChildAt(order[oi]);
                if (child.getVisibility() == View.VISIBLE
                        && (host.backdrop(child) != null || host.hasBackdropDescendant(child))) {
                    lastNeed = oi;
                }
            }
            if (lastNeed < 0) {
                cb.superDraw(canvas);
                return;
            }

            state.drawing = true;
            try {
                RectF accumBounds = new RectF(0, 0, w, h);
                Source inherited = state.inherited;
                if (inherited != null) {
                    RectF mapped = new RectF();
                    inherited.toLocal.mapRect(mapped, inherited.bounds);
                    if (container.getClipToOutline() || container.getClipChildren()) {
                        if (!mapped.intersect(0, 0, w, h)) mapped.set(0, 0, w, h);
                    } else {
                        mapped.union(0, 0, w, h);
                    }
                    accumBounds.set(mapped);
                }

                int segIdx = 0, blurIdx = 0;
                long drawingTime = container.getDrawingTime();
                RenderNode accumulated = null;
                RenderNode openSeg = state.segment(segIdx++, w, h);
                RecordingCanvas rc = openSeg.beginRecording();
                drawBackground(container, rc);
                rc.enableZ();

                for (int oi = 0; oi < count; oi++) {
                    View child = container.getChildAt(order[oi]);
                    if (child.getVisibility() != View.VISIBLE) continue;
                    BackdropMath.Spec spec = host.backdrop(child);
                    boolean needsBlur = spec != null && spec.active();
                    boolean needsDesc = host.hasBackdropDescendant(child);

                    if (!needsBlur && !needsDesc) {
                        if (oi < lastNeed) {
                            cb.drawChild(rc, child, drawingTime);
                        } else {
                            canvas.enableZ();
                            cb.drawChild(canvas, child, drawingTime);
                            canvas.disableZ();
                        }
                        continue;
                    }

                    // Flush the open segment before this backdrop child or ancestor.
                    rc.disableZ();
                    openSeg.endRecording();
                    canvas.drawRenderNode(openSeg);
                    if (accumulated == null && inherited == null) {
                        accumulated = openSeg;
                    } else {
                        RenderNode combined = state.segment(segIdx++, w, h);
                        RecordingCanvas cc = combined.beginRecording();
                        if (accumulated != null) {
                            cc.drawRenderNode(accumulated);
                        } else {
                            cc.save();
                            cc.concat(inherited.toLocal);
                            cc.drawRenderNode(inherited.node);
                            cc.restore();
                        }
                        cc.drawRenderNode(openSeg);
                        combined.endRecording();
                        accumulated = combined;
                    }
                    inherited = null;

                    // Render this child's backdrop filter over accumulated.
                    RenderNode childBlur = null;
                    Matrix childToParent = null;
                    Matrix parentToChild = null;
                    int blurLeft = 0, blurTop = 0;
                    if (needsBlur && child.getAlpha() > 0f && child.getWidth() > 0 && child.getHeight() > 0) {
                        childToParent = new Matrix(child.getMatrix());
                        childToParent.postTranslate(child.getLeft(), child.getTop());
                        parentToChild = new Matrix();
                        if (childToParent.invert(parentToChild)) {
                            state.tempRect.set(0, 0, child.getWidth(), child.getHeight());
                            childToParent.mapRect(state.tempRect);
                            int[] src = BackdropMath.paddedBounds(
                                    state.tempRect.left, state.tempRect.top,
                                    state.tempRect.right, state.tempRect.bottom,
                                    spec.blur,
                                    accumBounds.left, accumBounds.top,
                                    accumBounds.right, accumBounds.bottom);
                            if (src != null) {
                                blurLeft = src[0];
                                blurTop = src[1];
                                int bw = src[2] - src[0], bh = src[3] - src[1];
                                RenderEffect effect = createEffect(spec.blur, spec.colorMatrix);
                                if (effect != null) {
                                    childBlur = state.blur(blurIdx++, bw, bh);
                                    childBlur.setAlpha(child.getAlpha());
                                    childBlur.setRenderEffect(effect);
                                    RecordingCanvas bc = childBlur.beginRecording();
                                    bc.translate(-blurLeft, -blurTop);
                                    bc.drawRenderNode(accumulated);
                                    childBlur.endRecording();
                                }
                            }
                        }
                    }

                    // If child holds nested backdrop descendants, pass accumulated down.
                    if (needsDesc) {
                        if (childToParent == null) {
                            childToParent = new Matrix(child.getMatrix());
                            childToParent.postTranslate(child.getLeft(), child.getTop());
                            parentToChild = new Matrix();
                            if (!childToParent.invert(parentToChild)) parentToChild = null;
                        }
                        if (parentToChild != null) {
                            Matrix toChildContent = new Matrix(parentToChild);
                            int csx = child.getScrollX(), csy = child.getScrollY();
                            if ((csx | csy) != 0) toChildContent.postTranslate(csx, csy);
                            RenderNode forChild = accumulated;
                            if (childBlur != null) {
                                RenderNode withBlur = state.segment(segIdx++, w, h);
                                RecordingCanvas wbc = withBlur.beginRecording();
                                wbc.drawRenderNode(accumulated);
                                drawClippedBlur(wbc, child, host.backdropBox(child),
                                        childToParent, parentToChild, childBlur, blurLeft, blurTop,
                                        state.tempPath, state.tempBounds);
                                withBlur.endRecording();
                                forChild = withBlur;
                            }
                            host.setInherited(child, new Source(forChild, toChildContent, new RectF(accumBounds)));
                        }
                    }

                    // Draw child's backdrop + child; if more backdrop children follow, keep recording.
                    if (oi < lastNeed) {
                        RenderNode childSeg = state.segment(segIdx++, w, h);
                        RecordingCanvas cs = childSeg.beginRecording();
                        if (childBlur != null) {
                            drawClippedBlur(cs, child, host.backdropBox(child),
                                    childToParent, parentToChild, childBlur, blurLeft, blurTop,
                                    state.tempPath, state.tempBounds);
                        }
                        cs.enableZ();
                        try {
                            cb.drawChild(cs, child, drawingTime);
                        } finally {
                            if (needsDesc) host.setInherited(child, null);
                        }
                        cs.disableZ();
                        childSeg.endRecording();
                        canvas.drawRenderNode(childSeg);

                        RenderNode combined = state.segment(segIdx++, w, h);
                        RecordingCanvas cc = combined.beginRecording();
                        cc.drawRenderNode(accumulated);
                        cc.drawRenderNode(childSeg);
                        combined.endRecording();
                        accumulated = combined;

                        openSeg = state.segment(segIdx++, w, h);
                        rc = openSeg.beginRecording();
                        rc.enableZ();
                    } else {
                        if (childBlur != null) {
                            drawClippedBlur(canvas, child, host.backdropBox(child),
                                    childToParent, parentToChild, childBlur, blurLeft, blurTop,
                                    state.tempPath, state.tempBounds);
                        }
                        canvas.enableZ();
                        try {
                            cb.drawChild(canvas, child, drawingTime);
                        } finally {
                            if (needsDesc) host.setInherited(child, null);
                        }
                        canvas.disableZ();
                        openSeg = null;
                        rc = null;
                    }
                }

                if (openSeg != null) {
                    rc.disableZ();
                    openSeg.endRecording();
                    canvas.drawRenderNode(openSeg);
                }
                state.trim(segIdx, blurIdx);
                cb.superOnDrawForeground(canvas);
            } finally {
                state.drawing = false;
            }
        }

        private static void drawBackground(ViewGroup container, Canvas canvas) {
            Drawable bg = container.getBackground();
            if (bg == null) return;
            Rect b = bg.getBounds();
            if (b.isEmpty()) bg.setBounds(0, 0, container.getWidth(), container.getHeight());
            int sx = container.getScrollX(), sy = container.getScrollY();
            if ((sx | sy) != 0) canvas.translate(sx, sy);
            bg.draw(canvas);
            if ((sx | sy) != 0) canvas.translate(-sx, -sy);
        }

        private static void drawClippedBlur(Canvas canvas, View child, Box box,
                Matrix childToParent, Matrix parentToChild, RenderNode blurNode,
                int blurLeft, int blurTop, Path tempPath, Rect tempBounds) {
            int save = canvas.save();
            canvas.concat(childToParent);
            if (box != null) {
                tempBounds.set(0, 0, child.getWidth(), child.getHeight());
                canvas.clipPath(box.shape(tempBounds, tempPath));
            } else {
                canvas.clipRect(0, 0, child.getWidth(), child.getHeight());
            }
            canvas.concat(parentToChild);
            canvas.translate(blurLeft, blurTop);
            canvas.drawRenderNode(blurNode);
            canvas.restoreToCount(save);
        }

        /** Stable child order by ascending Z, matching ViewGroup's enableZ() ordering. */
        private static int[] childOrder(ViewGroup container, int count) {
            int[] order = new int[count];
            boolean hasZ = false;
            for (int i = 0; i < count; i++) {
                order[i] = i;
                if (container.getChildAt(i).getZ() != 0f) hasZ = true;
            }
            if (!hasZ) return order;
            // Stable insertion sort (count is small).
            for (int i = 1; i < count; i++) {
                int idx = order[i];
                float z = container.getChildAt(idx).getZ();
                int j = i - 1;
                while (j >= 0 && container.getChildAt(order[j]).getZ() > z) {
                    order[j + 1] = order[j];
                    j--;
                }
                order[j + 1] = idx;
            }
            return order;
        }
    }
}
