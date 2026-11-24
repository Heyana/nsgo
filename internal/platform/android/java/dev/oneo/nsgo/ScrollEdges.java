package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.BlendMode;
import android.graphics.Canvas;
import android.graphics.LinearGradient;
import android.graphics.Paint;
import android.graphics.RecordingCanvas;
import android.graphics.RenderEffect;
import android.graphics.RenderNode;
import android.graphics.Shader;
import android.os.Build;
import android.view.View;
import android.widget.FrameLayout;

import java.util.function.Consumer;

/**
 * Outer frame of a scroll node: holds the scroll view (VScroll, HScroll or
 * ZoomScroll) and draws its scroll edge effects over the scrolled content.
 *
 * From Android 12 (API 31) with hardware acceleration, the content is recorded
 * into a RenderNode once per frame and each active side blurs its band through
 * RenderEffect.createBlurEffect: progressive multi-level masked blurs for a
 * soft edge, or a uniform blur for a hard edge, under the smoothstep or flat
 * tint and hairline. Before Android 12 (or on a software canvas), it falls back
 * to the unblurred translucent gradient scrim and hairline.
 */
final class ScrollEdges extends FrameLayout {
    private final float density;
    private final Scrolling.Config.Edge[] edges = new Scrolling.Config.Edge[4];
    private final Paint paint = new Paint(Paint.ANTI_ALIAS_FLAG);
    private final int[] rect = new int[4];
    private final float[] line = new float[4];
    private final int[] stopColors = new int[ScrollEdgeMath.SOFT_TINT_STEPS + 1];
    private final float[] stopPositions = new float[ScrollEdgeMath.SOFT_TINT_STEPS + 1];
    private final TintCache[] tintCaches = new TintCache[4];
    private boolean hasEdges;
    /** Lazily instantiated only on API 31+ so older ART runtimes never load RenderNode/RenderEffect. */
    private Hardware hardware;
    /** What NsgoView answers about backdrop filters, null until it sets it, and the state Backdrop keeps for this view, null before Android 12. */
    Backdrop.Host backdropHost;
    final Object backdropState = Backdrop.createState();
    private final Backdrop.Callbacks backdropCallbacks = new Backdrop.Callbacks() {
        @Override public void superDraw(Canvas canvas) { ScrollEdges.super.draw(canvas); }
        @Override public boolean drawChild(Canvas canvas, View child, long drawingTime) {
            return ScrollEdges.this.drawChild(canvas, child, drawingTime);
        }
        @Override public void superOnDrawForeground(Canvas canvas) { ScrollEdges.super.onDrawForeground(canvas); }
    };

    ScrollEdges(Context context, float density) {
        super(context);
        this.density = density;
        paint.setStyle(Paint.Style.FILL);
    }

    @Override
    public void draw(Canvas canvas) {
        // Box keeps the outer shadows and outline outside the content clip.
        int save = Box.begin(canvas, this);
        drawContent(canvas);
        Box.end(canvas, this, save);
    }

    private void drawContent(Canvas canvas) {
        if (backdropHost == null || !backdropHost.hasBackdropDescendant(this)) {
            super.draw(canvas);
            return;
        }
        if (!hasEdges) {
            Backdrop.drawContainer(this, canvas, backdropHost, backdropCallbacks, backdropState);
            return;
        }
        // The edges record the scroller in dispatchDraw, so it draws there, and a backdrop filter inside it blurs what this view and its ancestors drew.
        View scroller = null;
        for (int i = 0; i < getChildCount() && scroller == null; i++) {
            if (backdropHost.hasBackdropDescendant(getChildAt(i))) scroller = getChildAt(i);
        }
        if (scroller != null) backdropHost.setInherited(scroller, Backdrop.forwardContainerBackground(this, backdropState, scroller));
        try {
            super.draw(canvas);
        } finally {
            if (scroller != null) backdropHost.setInherited(scroller, null);
        }
    }

    boolean hasEdges() {
        return hasEdges;
    }

    /** Updates the four scroll edge effects from ItemScrollConfig. */
    void setEdges(Scrolling.Config.Edge[] next) {
        boolean changed = false;
        boolean any = false;
        for (int i = 0; i < 4; i++) {
            Scrolling.Config.Edge e = next != null ? next[i] : null;
            if (e != null && ScrollEdgeMath.band(e.effect, e.extent) <= 0f) e = null;
            if (!sameEdge(edges[i], e)) {
                edges[i] = e;
                changed = true;
                if (e == null) tintCaches[i] = null;
            }
            if (edges[i] != null) any = true;
        }
        if (!changed) return;
        hasEdges = any;
        if (hardware != null) hardware.sync(edges, hasEdges);
        invalidate();
    }

    private static boolean sameEdge(Scrolling.Config.Edge a, Scrolling.Config.Edge b) {
        if (a == b) return true;
        if (a == null || b == null) return false;
        return a.effect == b.effect && a.extent == b.extent && a.tint == b.tint && a.line == b.line;
    }

    @Override
    public void onDescendantInvalidated(View child, View target) {
        super.onDescendantInvalidated(child, target);
        if (hasEdges && child != this) invalidate();
    }

    @Override
    protected void onDetachedFromWindow() {
        super.onDetachedFromWindow();
        if (hardware != null) hardware.discard();
    }

    /** Keeps the node's own outer shadows and outline outside its Overflow clip, as NsgoView's other styled views do. */
    private void drawChildren(Canvas canvas) {
        super.dispatchDraw(canvas);
    }

    @Override
    protected void dispatchDraw(Canvas canvas) {
        int save = Box.children(canvas, this);
        drawEdges(canvas);
        if (save >= 0) canvas.restoreToCount(save);
    }

    /** Draws the children and, when the scroll stretches, its edges. */
    private void drawEdges(Canvas canvas) {
        if (!hasEdges) {
            super.dispatchDraw(canvas);
            return;
        }
        int w = getWidth(), h = getHeight();
        if (w <= 0 || h <= 0) {
            super.dispatchDraw(canvas);
            return;
        }
        if (Build.VERSION.SDK_INT >= 31 && canvas.isHardwareAccelerated()) {
            if (hardware == null) hardware = new Hardware(density);
            hardware.record(canvas, w, h, this::drawChildren);
            for (int side = 0; side < 4; side++) {
                Scrolling.Config.Edge e = edges[side];
                if (e == null) continue;
                float bandPts = ScrollEdgeMath.band(e.effect, e.extent);
                int bandPx = Math.round(bandPts * density);
                if (bandPx <= 0) continue;
                ScrollEdgeMath.bandRect(side, w, h, bandPx, rect);
                int bL = rect[0], bT = rect[1], bR = rect[2], bB = rect[3];
                if (bR <= bL || bB <= bT) continue;
                hardware.drawSide(canvas, side, e, w, h, bL, bT, bR, bB);
                drawSideOverlay(canvas, side, e, bandPts, bL, bT, bR, bB,
                        ScrollEdgeMath.EDGE_TINT, ScrollEdgeMath.EDGE_TINT);
            }
            return;
        }
        super.dispatchDraw(canvas);
        for (int side = 0; side < 4; side++) {
            Scrolling.Config.Edge e = edges[side];
            if (e == null) continue;
            float bandPts = ScrollEdgeMath.band(e.effect, e.extent);
            int bandPx = Math.round(bandPts * density);
            if (bandPx <= 0) continue;
            ScrollEdgeMath.bandRect(side, w, h, bandPx, rect);
            int bL = rect[0], bT = rect[1], bR = rect[2], bB = rect[3];
            if (bR <= bL || bB <= bT) continue;
            drawSideOverlay(canvas, side, e, bandPts, bL, bT, bR, bB,
                    ScrollEdgeMath.UNBLURRED_SOFT_TINT, ScrollEdgeMath.UNBLURRED_HARD_TINT);
        }
    }

    private void drawSideOverlay(Canvas canvas, int side, Scrolling.Config.Edge e, float bandPts,
            int bL, int bT, int bR, int bB, float softMost, float hardMost) {
        if (e.effect == ScrollEdgeMath.SOFT) {
            if ((e.tint >>> 24) == 0) return;
            Shader shader = softTintShader(side, e.extent, bandPts, e.tint, softMost, bL, bT, bR, bB);
            paint.setShader(shader);
            paint.setColor(0xff000000);
            canvas.drawRect(bL, bT, bR, bB, paint);
            paint.setShader(null);
            return;
        }
        if (e.effect == ScrollEdgeMath.HARD) {
            paint.setShader(null);
            if ((e.tint >>> 24) != 0) {
                paint.setColor(ScrollEdgeMath.withAlpha(e.tint, hardMost));
                canvas.drawRect(bL, bT, bR, bB, paint);
            }
            if ((e.line >>> 24) != 0) {
                float linePx = ScrollEdgeMath.HAIRLINE_WIDTH * density;
                ScrollEdgeMath.hairlineRect(side, bL, bT, bR, bB, linePx, line);
                paint.setColor(e.line);
                canvas.drawRect(line[0], line[1], line[2], line[3], paint);
            }
        }
    }

    private Shader softTintShader(int side, float extent, float bandPts, int tint, float most,
            int bL, int bT, int bR, int bB) {
        TintCache tc = tintCaches[side];
        if (tc != null && tc.extent == extent && tc.band == bandPts && tc.tint == tint && tc.most == most
                && tc.l == bL && tc.t == bT && tc.r == bR && tc.b == bB) {
            return tc.shader;
        }
        ScrollEdgeMath.edgeLine(side, bL, bT, bR, bB, line);
        if (line[0] == line[2] && line[1] == line[3]) line[3] = line[1] + 1f;
        ScrollEdgeMath.softTintStops(extent, bandPts, tint, most, stopColors, stopPositions);
        Shader shader = new LinearGradient(line[0], line[1], line[2], line[3],
                stopColors, stopPositions, Shader.TileMode.CLAMP);
        if (tc == null) tc = tintCaches[side] = new TintCache();
        tc.extent = extent;
        tc.band = bandPts;
        tc.tint = tint;
        tc.most = most;
        tc.l = bL;
        tc.t = bT;
        tc.r = bR;
        tc.b = bB;
        tc.shader = shader;
        return shader;
    }

    private static final class TintCache {
        float extent, band, most;
        int tint, l, t, r, b;
        Shader shader;
    }

    /**
     * API 31+ GPU-accelerated RenderNode + RenderEffect blur implementation,
     * isolated into its own class so API 28-30 never resolve RenderNode or
     * RenderEffect.
     */
    private static final class Hardware {
        private final float density;
        private RenderNode contentNode;
        private final SideBlur[] sides = new SideBlur[4];
        private final int[] cap = new int[4];
        private final float[] maskLine = new float[4];

        Hardware(float density) {
            this.density = density;
        }

        void sync(Scrolling.Config.Edge[] edges, boolean hasEdges) {
            for (int i = 0; i < 4; i++) {
                if (edges[i] == null && sides[i] != null) {
                    sides[i].discard();
                    sides[i] = null;
                }
            }
            if (!hasEdges && contentNode != null) {
                contentNode.discardDisplayList();
                contentNode = null;
            }
        }

        void discard() {
            for (int i = 0; i < 4; i++) {
                if (sides[i] != null) {
                    sides[i].discard();
                    sides[i] = null;
                }
            }
            if (contentNode != null) {
                contentNode.discardDisplayList();
                contentNode = null;
            }
        }

        void record(Canvas canvas, int w, int h, Consumer<Canvas> drawChildren) {
            if (contentNode == null) {
                contentNode = new RenderNode("nsgo-scroll-content");
            }
            contentNode.setPosition(0, 0, w, h);
            contentNode.setClipToBounds(true);
            RecordingCanvas rec = contentNode.beginRecording(w, h);
            drawChildren.accept(rec);
            contentNode.endRecording();
            canvas.drawRenderNode(contentNode);
        }

        void drawSide(Canvas canvas, int side, Scrolling.Config.Edge e, int w, int h,
                int bL, int bT, int bR, int bB) {
            int effectiveBandPx = (side == ScrollEdgeMath.SIDE_TOP || side == ScrollEdgeMath.SIDE_BOTTOM)
                    ? (bB - bT) : (bR - bL);
            if (effectiveBandPx <= 0) return;

            SideBlur sb = sides[side];
            if (sb == null || sb.effect != e.effect || sb.w != w || sb.h != h || sb.bandPx != effectiveBandPx) {
                if (sb != null) sb.discard();
                sb = buildSide(side, e.effect, w, h, effectiveBandPx);
                sides[side] = sb;
            }
            if (sb == null || sb.nodes.length == 0) return;

            int save = canvas.save();
            canvas.clipRect(bL, bT, bR, bB);
            for (int i = 0; i < sb.nodes.length; i++) {
                RenderNode node = sb.nodes[i];
                if (node == null) continue;
                int capL = sb.capL[i], capT = sb.capT[i], capW = sb.capW[i], capH = sb.capH[i];
                RecordingCanvas lrec = node.beginRecording(capW, capH);
                if ((e.tint >>> 24) != 0) lrec.drawColor(e.tint);
                lrec.translate(-capL, -capT);
                lrec.drawRenderNode(contentNode);
                node.endRecording();
                canvas.drawRenderNode(node);
            }
            canvas.restoreToCount(save);
        }

        private SideBlur buildSide(int side, int effect, int w, int h, int bandPx) {
            if (effect == ScrollEdgeMath.SOFT) {
                ScrollEdgeMath.Level[] levels = ScrollEdgeMath.softLevels();
                SideBlur sb = new SideBlur(effect, w, h, bandPx, levels.length);
                for (int i = 0; i < levels.length; i++) {
                    ScrollEdgeMath.Level l = levels[i];
                    float r = l.total * density;
                    if (r <= 0f) continue;
                    int margin = ScrollEdgeMath.blurMarginPx(r);
                    ScrollEdgeMath.captureRect(side, w, h, bandPx, margin, cap);
                    int capL = cap[0], capT = cap[1], capW = cap[2] - cap[0], capH = cap[3] - cap[1];
                    if (capW <= 0 || capH <= 0) continue;
                    ScrollEdgeMath.levelMaskLine(side, capW, capH, bandPx, l.full, l.gone, maskLine);
                    if (maskLine[0] == maskLine[2] && maskLine[1] == maskLine[3]) maskLine[3] = maskLine[1] + 1f;
                    LinearGradient mask = new LinearGradient(
                            maskLine[0], maskLine[1], maskLine[2], maskLine[3],
                            0xff000000, 0x00000000, Shader.TileMode.CLAMP);
                    RenderEffect blur = RenderEffect.createBlurEffect(r, r, Shader.TileMode.CLAMP);
                    RenderEffect maskEffect = RenderEffect.createShaderEffect(mask);
                    RenderEffect masked = RenderEffect.createBlendModeEffect(blur, maskEffect, BlendMode.DST_IN);
                    RenderNode node = new RenderNode("nsgo-scroll-edge-soft-" + side + "-" + i);
                    node.setPosition(capL, capT, capL + capW, capT + capH);
                    node.setClipToBounds(true);
                    node.setRenderEffect(masked);
                    sb.nodes[i] = node;
                    sb.capL[i] = capL;
                    sb.capT[i] = capT;
                    sb.capW[i] = capW;
                    sb.capH[i] = capH;
                }
                return sb;
            }
            if (effect == ScrollEdgeMath.HARD) {
                float r = ScrollEdgeMath.HARD_BLUR * density;
                if (r <= 0f) return null;
                int margin = ScrollEdgeMath.blurMarginPx(r);
                ScrollEdgeMath.captureRect(side, w, h, bandPx, margin, cap);
                int capL = cap[0], capT = cap[1], capW = cap[2] - cap[0], capH = cap[3] - cap[1];
                if (capW <= 0 || capH <= 0) return null;
                SideBlur sb = new SideBlur(effect, w, h, bandPx, 1);
                RenderNode node = new RenderNode("nsgo-scroll-edge-hard-" + side);
                node.setPosition(capL, capT, capL + capW, capT + capH);
                node.setClipToBounds(true);
                node.setRenderEffect(RenderEffect.createBlurEffect(r, r, Shader.TileMode.CLAMP));
                sb.nodes[0] = node;
                sb.capL[0] = capL;
                sb.capT[0] = capT;
                sb.capW[0] = capW;
                sb.capH[0] = capH;
                return sb;
            }
            return null;
        }

        private static final class SideBlur {
            final int effect, w, h, bandPx;
            final RenderNode[] nodes;
            final int[] capL, capT, capW, capH;

            SideBlur(int effect, int w, int h, int bandPx, int count) {
                this.effect = effect;
                this.w = w;
                this.h = h;
                this.bandPx = bandPx;
                this.nodes = new RenderNode[count];
                this.capL = new int[count];
                this.capT = new int[count];
                this.capW = new int[count];
                this.capH = new int[count];
            }

            void discard() {
                for (int i = 0; i < nodes.length; i++) {
                    if (nodes[i] != null) {
                        nodes[i].discardDisplayList();
                        nodes[i] = null;
                    }
                }
            }
        }
    }
}
