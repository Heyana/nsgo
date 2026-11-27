package dev.oneo.nsgo;

import java.nio.BufferUnderflowException;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;

/**
 * Computes the area of a canvas that playing after instead of before changes,
 * in the canvas's points: a port of internal/driver/canvas_damage.go over the
 * encoded op stream that drawing.go writes and CanvasNode.patch mutates.
 *
 * Pure Java with no android.* imports so host JVM tests can verify parity with
 * driver.CanvasDamage directly.
 */
final class CanvasDamage {
    /** Extra points around a damaged box for anti-aliasing, matching apple.Damaged and web/windows. */
    static final double PAD_POINTS = 2.0;

    /** Result of scanning two encoded op streams. */
    static final class Result {
        static final Result FULL = new Result(false, false, 0, 0, 0, 0);
        static final Result EMPTY = new Result(true, true, 0, 0, 0, 0);

        final boolean bounded;
        final boolean empty;
        final double x0, y0, x1, y1;

        Result(boolean bounded, boolean empty, double x0, double y0, double x1, double y1) {
            this.bounded = bounded;
            this.empty = empty;
            this.x0 = x0;
            this.y0 = y0;
            this.x1 = x1;
            this.y1 = y1;
        }
    }

    /** Whole-pixel clip rectangle inside a backing bitmap, [left, right) x [top, bottom). */
    static final class PixelRect {
        final int left, top, right, bottom;

        PixelRect(int left, int top, int right, int bottom) {
            this.left = left;
            this.top = top;
            this.right = right;
            this.bottom = bottom;
        }

        int width() {
            return right - left;
        }

        int height() {
            return bottom - top;
        }
    }

    private CanvasDamage() {}

    /**
     * Converts a bounded, non-empty damage result (in drawing points) to whole
     * pixels in a width x height backing bitmap scaled by (sx, sy) pixels per
     * point, padded for anti-aliasing and clamped to the bitmap. Returns null
     * when no backing pixel is reached.
     */
    static PixelRect pixelClip(Result damage, float sx, float sy, int width, int height) {
        if (damage == null || !damage.bounded || damage.empty || width <= 0 || height <= 0
                || !(sx > 0f) || !(sy > 0f)) {
            return null;
        }
        double padX = Math.max(PAD_POINTS * sx, PAD_POINTS);
        double padY = Math.max(PAD_POINTS * sy, PAD_POINTS);
        int x0 = (int) Math.floor(damage.x0 * sx - padX);
        int y0 = (int) Math.floor(damage.y0 * sy - padY);
        int x1 = (int) Math.ceil(damage.x1 * sx + padX);
        int y1 = (int) Math.ceil(damage.y1 * sy + padY);
        if (x0 < 0) x0 = 0;
        if (y0 < 0) y0 = 0;
        if (x1 > width) x1 = width;
        if (y1 > height) y1 = height;
        if (x1 <= x0 || y1 <= y0) return null;
        return new PixelRect(x0, y0, x1, y1);
    }

    /**
     * Scans before and after (two snapshots of one encoded drawing differing
     * only in bound arguments) and returns the bounds in canvas points that
     * playing after instead of before can change, or Result.FULL when only a
     * full redraw is safe.
     */
    static Result scan(byte[] before, byte[] after) {
        if (before == null || after == null || before.length != after.length) {
            return Result.FULL;
        }
        Scanner s = new Scanner(before, after);
        try {
            return s.run();
        } catch (BufferUnderflowException | IllegalArgumentException | IndexOutOfBoundsException e) {
            return Result.FULL;
        }
    }

    /** Mutable 2D affine matrix: (x, y) -> (a*x + c*y + e, b*x + d*y + f). */
    private static final class Mat {
        double a, b, c, d, e, f;

        Mat() {
            a = 1;
            d = 1;
        }

        Mat(double a, double b, double c, double d, double e, double f) {
            this.a = a;
            this.b = b;
            this.c = c;
            this.d = d;
            this.e = e;
            this.f = f;
        }

        Mat copy() {
            return new Mat(a, b, c, d, e, f);
        }

        void set(double a, double b, double c, double d, double e, double f) {
            this.a = a;
            this.b = b;
            this.c = c;
            this.d = d;
            this.e = e;
            this.f = f;
        }

        /** Replaces this with this * n (applies n, then this). */
        void then(double na, double nb, double nc, double nd, double ne, double nf) {
            double ra = a * na + c * nb;
            double rb = b * na + d * nb;
            double rc = a * nc + c * nd;
            double rd = b * nc + d * nd;
            double re = a * ne + c * nf + e;
            double rf = b * ne + d * nf + f;
            a = ra;
            b = rb;
            c = rc;
            d = rd;
            e = re;
            f = rf;
        }

        double applyX(double x, double y) {
            return a * x + c * y + e;
        }

        double applyY(double x, double y) {
            return b * x + d * y + f;
        }

        double scale() {
            return Math.max(Math.hypot(a, b), Math.hypot(c, d));
        }
    }

    /** Axis-aligned box by its edges; empty when x0 > x1. */
    private static final class Box {
        double x0, y0, x1, y1;

        Box() {
            reset();
        }

        void reset() {
            x0 = Double.POSITIVE_INFINITY;
            y0 = Double.POSITIVE_INFINITY;
            x1 = Double.NEGATIVE_INFINITY;
            y1 = Double.NEGATIVE_INFINITY;
        }

        boolean empty() {
            return x0 > x1;
        }

        void add(double x, double y) {
            x0 = Math.min(x0, x);
            y0 = Math.min(y0, y);
            x1 = Math.max(x1, x);
            y1 = Math.max(y1, y);
        }

        void add(Mat m, double x, double y) {
            add(m.applyX(x, y), m.applyY(x, y));
        }

        void addRect(Mat m, double rx0, double ry0, double rx1, double ry1) {
            add(m, rx0, ry0);
            add(m, rx1, ry0);
            add(m, rx0, ry1);
            add(m, rx1, ry1);
        }

        void unionWith(double ox0, double oy0, double ox1, double oy1) {
            if (ox0 > ox1) return;
            x0 = Math.min(x0, ox0);
            y0 = Math.min(y0, oy0);
            x1 = Math.max(x1, ox1);
            y1 = Math.max(y1, oy1);
        }

        void unionWith(Box o) {
            unionWith(o.x0, o.y0, o.x1, o.y1);
        }

        void inflate(double by) {
            if (x0 > x1) return;
            x0 -= by;
            y0 -= by;
            x1 += by;
            y1 += by;
        }
    }

    private static final class State {
        Mat m = new Mat();
        double width = 1;
        double miter = 10;
        int cap;       // 0 butt, 1 round, 2 square
        int join;      // 0 miter, 1 round, 2 bevel
        double shadowX, shadowY, shadowR;
        boolean shadow;
        int composite; // 0 source-over
        boolean moved, painted, composed;

        State copy() {
            State s = new State();
            s.m = m.copy();
            s.width = width;
            s.miter = miter;
            s.cap = cap;
            s.join = join;
            s.shadowX = shadowX;
            s.shadowY = shadowY;
            s.shadowR = shadowR;
            s.shadow = shadow;
            s.composite = composite;
            s.moved = moved;
            s.painted = painted;
            s.composed = composed;
            return s;
        }

        double strokeReach() {
            double reach = width / 2.0 * m.scale();
            if (join == 0) {
                reach *= Math.max(miter, 1.0);
            } else if (cap == Protocol.CAP_SQUARE) {
                reach *= Math.sqrt(2.0);
            }
            return reach;
        }
    }

    private static final class Scanner {
        /** 9 floats per atlas sprite, matching AtlasMesh.SPRITE_BYTES and drawing.go's writeAtlas. */
        private static final int ATLAS_SPRITE_BYTES = 9 * 4;

        private final byte[] rawA, rawB;
        private final ByteBuffer a, b;
        private State cur0 = new State(), cur1 = new State();
        private final ArrayList<State> saved = new ArrayList<>();
        private final Box path0 = new Box(), path1 = new Box();
        private boolean pathMoved;
        private final Box total = new Box();
        private boolean empty = true;
        private final Box tmp0 = new Box(), tmp1 = new Box();

        Scanner(byte[] before, byte[] after) {
            this.rawA = before;
            this.rawB = after;
            this.a = ByteBuffer.wrap(before);
            this.b = ByteBuffer.wrap(after);
        }

        Result run() {
            while (a.hasRemaining() && b.hasRemaining()) {
                int kindA = a.get() & 0xff;
                int kindB = b.get() & 0xff;
                if (kindA != kindB) return Result.FULL;
                if (!step(kindB)) return Result.FULL;
            }
            if (a.hasRemaining() || b.hasRemaining()) return Result.FULL;
            if (empty) return Result.EMPTY;
            if (!finite(total.x0) || !finite(total.y0) || !finite(total.x1) || !finite(total.y1)) {
                return Result.FULL;
            }
            return new Result(true, false, total.x0, total.y0, total.x1, total.y1);
        }

        private boolean step(int kind) {
            switch (kind) {
                case Protocol.DRAW_SAVE:
                    saved.add(cur0.copy());
                    saved.add(cur1.copy());
                    return true;
                case Protocol.DRAW_RESTORE: {
                    int n = saved.size();
                    if (n >= 2) {
                        cur1 = saved.remove(n - 1);
                        cur0 = saved.remove(n - 2);
                    }
                    return true;
                }
                case Protocol.DRAW_TRANSLATE: {
                    float ax = a.getFloat(), ay = a.getFloat();
                    float bx = b.getFloat(), by = b.getFloat();
                    boolean changed = ax != bx || ay != by;
                    cur0.m.then(1, 0, 0, 1, ax, ay);
                    cur1.m.then(1, 0, 0, 1, bx, by);
                    cur0.moved |= changed;
                    cur1.moved |= changed;
                    return true;
                }
                case Protocol.DRAW_SCALE: {
                    float ax = a.getFloat(), ay = a.getFloat();
                    float bx = b.getFloat(), by = b.getFloat();
                    boolean changed = ax != bx || ay != by;
                    cur0.m.then(ax, 0, 0, ay, 0, 0);
                    cur1.m.then(bx, 0, 0, by, 0, 0);
                    cur0.moved |= changed;
                    cur1.moved |= changed;
                    return true;
                }
                case Protocol.DRAW_ROTATE: {
                    float aa = a.getFloat(), ba = b.getFloat();
                    boolean changed = aa != ba;
                    double sa = Math.sin(aa), ca = Math.cos(aa);
                    double sb = Math.sin(ba), cb = Math.cos(ba);
                    cur0.m.then(ca, sa, -sa, ca, 0, 0);
                    cur1.m.then(cb, sb, -sb, cb, 0, 0);
                    cur0.moved |= changed;
                    cur1.moved |= changed;
                    return true;
                }
                case Protocol.DRAW_TRANSFORM:
                case Protocol.DRAW_SET_TRANSFORM: {
                    float ax1 = a.getFloat(), ay1 = a.getFloat(), ax2 = a.getFloat(), ay2 = a.getFloat();
                    float ax = a.getFloat(), ay = a.getFloat();
                    float bx1 = b.getFloat(), by1 = b.getFloat(), bx2 = b.getFloat(), by2 = b.getFloat();
                    float bx = b.getFloat(), by = b.getFloat();
                    boolean changed = ax1 != bx1 || ay1 != by1 || ax2 != bx2 || ay2 != by2 || ax != bx || ay != by;
                    if (kind == Protocol.DRAW_SET_TRANSFORM) {
                        cur0.m.set(ax1, ay1, ax2, ay2, ax, ay);
                        cur1.m.set(bx1, by1, bx2, by2, bx, by);
                    } else {
                        cur0.m.then(ax1, ay1, ax2, ay2, ax, ay);
                        cur1.m.then(bx1, by1, bx2, by2, bx, by);
                    }
                    cur0.moved |= changed;
                    cur1.moved |= changed;
                    return true;
                }
                case Protocol.DRAW_BEGIN_PATH:
                    path0.reset();
                    path1.reset();
                    pathMoved = false;
                    return true;
                case Protocol.DRAW_MOVE_TO:
                case Protocol.DRAW_LINE_TO: {
                    float ax = a.getFloat(), ay = a.getFloat();
                    float bx = b.getFloat(), by = b.getFloat();
                    path0.add(cur0.m, ax, ay);
                    path1.add(cur1.m, bx, by);
                    pathMoved = pathMoved || ax != bx || ay != by || cur1.moved;
                    return true;
                }
                case Protocol.DRAW_QUAD_TO: {
                    float ax1 = a.getFloat(), ay1 = a.getFloat(), ax = a.getFloat(), ay = a.getFloat();
                    float bx1 = b.getFloat(), by1 = b.getFloat(), bx = b.getFloat(), by = b.getFloat();
                    path0.add(cur0.m, ax1, ay1);
                    path0.add(cur0.m, ax, ay);
                    path1.add(cur1.m, bx1, by1);
                    path1.add(cur1.m, bx, by);
                    pathMoved = pathMoved || ax1 != bx1 || ay1 != by1 || ax != bx || ay != by || cur1.moved;
                    return true;
                }
                case Protocol.DRAW_CUBIC_TO: {
                    float ax1 = a.getFloat(), ay1 = a.getFloat(), ax2 = a.getFloat(), ay2 = a.getFloat();
                    float ax = a.getFloat(), ay = a.getFloat();
                    float bx1 = b.getFloat(), by1 = b.getFloat(), bx2 = b.getFloat(), by2 = b.getFloat();
                    float bx = b.getFloat(), by = b.getFloat();
                    path0.add(cur0.m, ax1, ay1);
                    path0.add(cur0.m, ax2, ay2);
                    path0.add(cur0.m, ax, ay);
                    path1.add(cur1.m, bx1, by1);
                    path1.add(cur1.m, bx2, by2);
                    path1.add(cur1.m, bx, by);
                    pathMoved = pathMoved || ax1 != bx1 || ay1 != by1 || ax2 != bx2 || ay2 != by2
                            || ax != bx || ay != by || cur1.moved;
                    return true;
                }
                case Protocol.DRAW_ARC: {
                    float ax = a.getFloat(), ay = a.getFloat(), ar = a.getFloat(), aa0 = a.getFloat(), aa1 = a.getFloat();
                    byte accw = a.get();
                    float bx = b.getFloat(), by = b.getFloat(), br = b.getFloat(), ba0 = b.getFloat(), ba1 = b.getFloat();
                    byte bccw = b.get();
                    float ra = Math.abs(ar), rb = Math.abs(br);
                    path0.addRect(cur0.m, ax - ra, ay - ra, ax + ra, ay + ra);
                    path1.addRect(cur1.m, bx - rb, by - rb, bx + rb, by + rb);
                    boolean changed = ax != bx || ay != by || ar != br || aa0 != ba0 || aa1 != ba1 || accw != bccw;
                    pathMoved = pathMoved || changed || cur1.moved;
                    return true;
                }
                case Protocol.DRAW_RECT: {
                    float ax = a.getFloat(), ay = a.getFloat(), aw = a.getFloat(), ah = a.getFloat();
                    float bx = b.getFloat(), by = b.getFloat(), bw = b.getFloat(), bh = b.getFloat();
                    path0.addRect(cur0.m, ax, ay, ax + aw, ay + ah);
                    path1.addRect(cur1.m, bx, by, bx + bw, by + bh);
                    boolean changed = ax != bx || ay != by || aw != bw || ah != bh;
                    pathMoved = pathMoved || changed || cur1.moved;
                    return true;
                }
                case Protocol.DRAW_ROUND_RECT: {
                    float ax = a.getFloat(), ay = a.getFloat(), aw = a.getFloat(), ah = a.getFloat(), ar = a.getFloat();
                    float bx = b.getFloat(), by = b.getFloat(), bw = b.getFloat(), bh = b.getFloat(), br = b.getFloat();
                    path0.addRect(cur0.m, ax, ay, ax + aw, ay + ah);
                    path1.addRect(cur1.m, bx, by, bx + bw, by + bh);
                    boolean changed = ax != bx || ay != by || aw != bw || ah != bh || ar != br;
                    pathMoved = pathMoved || changed || cur1.moved;
                    return true;
                }
                case Protocol.DRAW_ELLIPSE: {
                    float ax = a.getFloat(), ay = a.getFloat(), arx = a.getFloat(), ary = a.getFloat();
                    float bx = b.getFloat(), by = b.getFloat(), brx = b.getFloat(), bry = b.getFloat();
                    float aRx = Math.abs(arx), aRy = Math.abs(ary);
                    float bRx = Math.abs(brx), bRy = Math.abs(bry);
                    path0.addRect(cur0.m, ax - aRx, ay - aRy, ax + aRx, ay + aRy);
                    path1.addRect(cur1.m, bx - bRx, by - bRy, bx + bRx, by + bRy);
                    boolean changed = ax != bx || ay != by || arx != brx || ary != bry;
                    pathMoved = pathMoved || changed || cur1.moved;
                    return true;
                }
                case Protocol.DRAW_CLOSE_PATH:
                    return true;
                case Protocol.DRAW_CLIP: {
                    byte ea = a.get(), eb = b.get();
                    return !(pathMoved || ea != eb);
                }
                case Protocol.DRAW_FILL_COLOR:
                case Protocol.DRAW_STROKE_COLOR: {
                    boolean changed = a.getInt() != b.getInt();
                    cur0.painted |= changed;
                    cur1.painted |= changed;
                    return true;
                }
                case Protocol.DRAW_ALPHA:
                case Protocol.DRAW_LINE_DASH_OFFSET: {
                    boolean changed = a.getFloat() != b.getFloat();
                    cur0.painted |= changed;
                    cur1.painted |= changed;
                    return true;
                }
                case Protocol.DRAW_FILL_GRADIENT:
                case Protocol.DRAW_STROKE_GRADIENT: {
                    int startA = a.position(), startB = b.position();
                    if (!skipGradient(a) || !skipGradient(b)) return false;
                    int endA = a.position(), endB = b.position();
                    boolean changed = (endA - startA != endB - startB)
                            || !bytesEqual(rawA, startA, rawB, startB, endA - startA);
                    cur0.painted |= changed;
                    cur1.painted |= changed;
                    return true;
                }
                case Protocol.DRAW_LINE_DASH: {
                    int startA = a.position(), startB = b.position();
                    int na = a.getInt(), nb = b.getInt();
                    if (na < 0 || nb < 0) return false;
                    skipBytes(a, na * 4L);
                    skipBytes(b, nb * 4L);
                    int endA = a.position(), endB = b.position();
                    boolean changed = (endA - startA != endB - startB)
                            || !bytesEqual(rawA, startA, rawB, startB, endA - startA);
                    cur0.painted |= changed;
                    cur1.painted |= changed;
                    return true;
                }
                case Protocol.DRAW_LINE_WIDTH: {
                    float aw = a.getFloat(), bw = b.getFloat();
                    boolean changed = aw != bw;
                    cur0.width = aw;
                    cur1.width = bw;
                    cur0.painted |= changed;
                    cur1.painted |= changed;
                    return true;
                }
                case Protocol.DRAW_LINE_CAP: {
                    int ac = a.get() & 0xff, bc = b.get() & 0xff;
                    boolean changed = ac != bc;
                    cur0.cap = ac;
                    cur1.cap = bc;
                    cur0.painted |= changed;
                    cur1.painted |= changed;
                    return true;
                }
                case Protocol.DRAW_LINE_JOIN: {
                    int aj = a.get() & 0xff, bj = b.get() & 0xff;
                    boolean changed = aj != bj;
                    cur0.join = aj;
                    cur1.join = bj;
                    cur0.painted |= changed;
                    cur1.painted |= changed;
                    return true;
                }
                case Protocol.DRAW_MITER_LIMIT: {
                    float am = a.getFloat(), bm = b.getFloat();
                    boolean changed = am != bm;
                    cur0.miter = am;
                    cur1.miter = bm;
                    cur0.painted |= changed;
                    cur1.painted |= changed;
                    return true;
                }
                case Protocol.DRAW_SHADOW: {
                    int ac = a.getInt();
                    float ax = a.getFloat(), ay = a.getFloat(), ar = a.getFloat();
                    int bc = b.getInt();
                    float bx = b.getFloat(), by = b.getFloat(), br = b.getFloat();
                    boolean changed = ac != bc || ax != bx || ay != by || ar != br;
                    cur0.shadow = (ac & 0xff) != 0;
                    cur0.shadowX = ax;
                    cur0.shadowY = ay;
                    cur0.shadowR = ar;
                    cur0.painted |= changed;
                    cur1.shadow = (bc & 0xff) != 0;
                    cur1.shadowX = bx;
                    cur1.shadowY = by;
                    cur1.shadowR = br;
                    cur1.painted |= changed;
                    return true;
                }
                case Protocol.DRAW_COMPOSITE: {
                    int ac = a.get() & 0xff, bc = b.get() & 0xff;
                    boolean changed = ac != bc;
                    cur0.composite = ac;
                    cur1.composite = bc;
                    cur0.composed |= changed;
                    cur1.composed |= changed;
                    return true;
                }
                case Protocol.DRAW_FILL: {
                    byte ea = a.get(), eb = b.get();
                    boolean changed = ea != eb;
                    if (!changed && !cur1.painted && !pathMoved && !cur1.moved && !cur1.composed) {
                        return true;
                    }
                    return paint(path0, path1);
                }
                case Protocol.DRAW_STROKE: {
                    if (!cur1.painted && !pathMoved && !cur1.moved && !cur1.composed) {
                        return true;
                    }
                    tmp0.x0 = path0.x0; tmp0.y0 = path0.y0; tmp0.x1 = path0.x1; tmp0.y1 = path0.y1;
                    tmp1.x0 = path1.x0; tmp1.y0 = path1.y0; tmp1.x1 = path1.x1; tmp1.y1 = path1.y1;
                    tmp0.inflate(cur0.strokeReach());
                    tmp1.inflate(cur1.strokeReach());
                    return paint(tmp0, tmp1);
                }
                case Protocol.DRAW_CLEAR: {
                    float ax = a.getFloat(), ay = a.getFloat(), aw = a.getFloat(), ah = a.getFloat();
                    float bx = b.getFloat(), by = b.getFloat(), bw = b.getFloat(), bh = b.getFloat();
                    boolean changed = ax != bx || ay != by || aw != bw || ah != bh;
                    if (!changed && !cur1.painted && !cur1.moved && !cur1.composed) {
                        return true;
                    }
                    tmp0.reset();
                    tmp0.addRect(cur0.m, ax, ay, ax + aw, ay + ah);
                    tmp1.reset();
                    tmp1.addRect(cur1.m, bx, by, bx + bw, by + bh);
                    return paint(tmp0, tmp1);
                }
                case Protocol.DRAW_FILL_TEXT:
                case Protocol.DRAW_STROKE_TEXT: {
                    int taLen = a.getInt(), taStart = a.position();
                    skipBytes(a, taLen);
                    float ax = a.getFloat(), ay = a.getFloat();
                    int faLen = a.getInt(), faStart = a.position();
                    skipBytes(a, faLen);
                    float aSize = a.getFloat();
                    short aWeight = a.getShort();
                    byte aItalic = a.get(), aVariant = a.get();

                    int tbLen = b.getInt(), tbStart = b.position();
                    skipBytes(b, tbLen);
                    float bx = b.getFloat(), by = b.getFloat();
                    int fbLen = b.getInt(), fbStart = b.position();
                    skipBytes(b, fbLen);
                    float bSize = b.getFloat();
                    short bWeight = b.getShort();
                    byte bItalic = b.get(), bVariant = b.get();

                    boolean changed = ax != bx || ay != by || aSize != bSize || aWeight != bWeight
                            || aItalic != bItalic || aVariant != bVariant
                            || taLen != tbLen || faLen != fbLen
                            || !bytesEqual(rawA, taStart, rawB, tbStart, taLen)
                            || !bytesEqual(rawA, faStart, rawB, fbStart, faLen);
                    if (!changed && !cur1.painted && !cur1.moved && !cur1.composed) {
                        return true;
                    }
                    boolean stroked = kind == Protocol.DRAW_STROKE_TEXT;
                    textBox(tmp0, cur0, rawA, taStart, taLen, ax, ay, aSize, stroked);
                    textBox(tmp1, cur1, rawB, tbStart, tbLen, bx, by, bSize, stroked);
                    return paint(tmp0, tmp1);
                }
                case Protocol.DRAW_IMAGE: {
                    int kaLen = a.getInt(), kaStart = a.position();
                    skipBytes(a, kaLen);
                    float aDens = a.getFloat();
                    float ax = a.getFloat(), ay = a.getFloat(), aw = a.getFloat(), ah = a.getFloat();
                    float asx = a.getFloat(), asy = a.getFloat(), asw = a.getFloat(), ash = a.getFloat();
                    byte aFlags = a.get();
                    boolean aTinted = (aFlags & 2) != 0;
                    int aTint = aTinted ? a.getInt() : 0;

                    int kbLen = b.getInt(), kbStart = b.position();
                    skipBytes(b, kbLen);
                    float bDens = b.getFloat();
                    float bx = b.getFloat(), by = b.getFloat(), bw = b.getFloat(), bh = b.getFloat();
                    float bsx = b.getFloat(), bsy = b.getFloat(), bsw = b.getFloat(), bsh = b.getFloat();
                    byte bFlags = b.get();
                    boolean bTinted = (bFlags & 2) != 0;
                    int bTint = bTinted ? b.getInt() : 0;

                    boolean changed = kaLen != kbLen || aDens != bDens
                            || ax != bx || ay != by || aw != bw || ah != bh
                            || asx != bsx || asy != bsy || asw != bsw || ash != bsh
                            || aFlags != bFlags || aTint != bTint
                            || !bytesEqual(rawA, kaStart, rawB, kbStart, kaLen);
                    if (!changed && !cur1.painted && !cur1.moved && !cur1.composed) {
                        return true;
                    }
                    // Natural-size image bounds depend on the decoded bitmap.
                    if (bw == 0f || bh == 0f || aw == 0f || ah == 0f) {
                        return false;
                    }
                    tmp0.reset();
                    tmp0.addRect(cur0.m, ax, ay, ax + aw, ay + ah);
                    tmp1.reset();
                    tmp1.addRect(cur1.m, bx, by, bx + bw, by + bh);
                    return paint(tmp0, tmp1);
                }
                case Protocol.DRAW_VERTICES: {
                    int kaLen = a.getInt(), kaStart = a.position();
                    skipBytes(a, kaLen);
                    byte aNear = a.get();
                    int aCount = a.getInt();
                    int aVertStart = a.position();
                    if (aCount < 0) return false;
                    skipBytes(a, aCount * 20L);

                    int kbLen = b.getInt(), kbStart = b.position();
                    skipBytes(b, kbLen);
                    byte bNear = b.get();
                    int bCount = b.getInt();
                    int bVertStart = b.position();
                    if (bCount < 0) return false;
                    skipBytes(b, bCount * 20L);

                    boolean changed = kaLen != kbLen || aNear != bNear || aCount != bCount
                            || !bytesEqual(rawA, kaStart, rawB, kbStart, kaLen)
                            || !bytesEqual(rawA, aVertStart, rawB, bVertStart, aCount * 20);
                    if (!changed && !cur1.painted && !cur1.moved && !cur1.composed) {
                        return true;
                    }
                    verticesBox(tmp0, cur0.m, rawA, aVertStart, aCount);
                    verticesBox(tmp1, cur1.m, rawB, bVertStart, bCount);
                    return paint(tmp0, tmp1);
                }
                case Protocol.DRAW_ATLAS: {
                    int kaLen = a.getInt(), kaStart = a.position();
                    skipBytes(a, kaLen);
                    byte aNear = a.get();
                    int aCount = a.getInt();
                    int aSpriteStart = a.position();
                    if (aCount < 0) return false;
                    skipBytes(a, aCount * (long) ATLAS_SPRITE_BYTES);

                    int kbLen = b.getInt(), kbStart = b.position();
                    skipBytes(b, kbLen);
                    byte bNear = b.get();
                    int bCount = b.getInt();
                    int bSpriteStart = b.position();
                    if (bCount < 0) return false;
                    skipBytes(b, bCount * (long) ATLAS_SPRITE_BYTES);

                    boolean changed = kaLen != kbLen || aNear != bNear || aCount != bCount
                            || !bytesEqual(rawA, kaStart, rawB, kbStart, kaLen)
                            || !bytesEqual(rawA, aSpriteStart, rawB, bSpriteStart, aCount * ATLAS_SPRITE_BYTES);
                    if (!changed && !cur1.painted && !cur1.moved && !cur1.composed) {
                        return true;
                    }
                    atlasBox(tmp0, cur0.m, rawA, aSpriteStart, aCount);
                    atlasBox(tmp1, cur1.m, rawB, bSpriteStart, bCount);
                    return paint(tmp0, tmp1);
                }
                default:
                    return false;
            }
        }

        private boolean paint(Box b0, Box b1) {
            if (!paintOne(cur0, b0)) return false;
            return paintOne(cur1, b1);
        }

        private boolean paintOne(State s, Box b) {
            if (s.composite != 0) return false;
            double x0 = b.x0, y0 = b.y0, x1 = b.x1, y1 = b.y1;
            if (x0 > x1) return true;
            if (s.shadow) {
                double reach = 2.0 * s.shadowR;
                x0 = Math.min(x0, x0 + s.shadowX - reach);
                y0 = Math.min(y0, y0 + s.shadowY - reach);
                x1 = Math.max(x1, x1 + s.shadowX + reach);
                y1 = Math.max(y1, y1 + s.shadowY + reach);
            }
            if (!finite(x0) || !finite(y0) || !finite(x1) || !finite(y1)) {
                return false;
            }
            total.unionWith(x0, y0, x1, y1);
            empty = false;
            return true;
        }

        private static void textBox(Box out, State s, byte[] raw, int start, int len,
                float x, float y, float size, boolean stroked) {
            out.reset();
            float em = size > 0f ? size : 10f;
            String text = new String(raw, start, len, StandardCharsets.UTF_8);
            float n = Math.max(text.codePointCount(0, text.length()), 1);
            out.addRect(s.m, x - 1.5f * em, y - 1.5f * em, x + 1.5f * em * n, y + 1.5f * em);
            if (stroked) {
                out.inflate(s.strokeReach());
            }
        }

        private static void verticesBox(Box out, Mat m, byte[] raw, int start, int count) {
            out.reset();
            if (count == 0) return;
            ByteBuffer v = ByteBuffer.wrap(raw, start, count * 20);
            for (int i = 0; i < count; i++) {
                float px = v.getFloat(), py = v.getFloat();
                v.position(v.position() + 12); // tx, ty, color
                out.add(m, px, py);
            }
        }

        private static void atlasBox(Box out, Mat m, byte[] raw, int start, int count) {
            out.reset();
            if (count == 0) return;
            ByteBuffer sp = ByteBuffer.wrap(raw, start, count * ATLAS_SPRITE_BYTES);
            Mat local = new Mat();
            for (int i = 0; i < count; i++) {
                sp.getFloat(); // rx
                sp.getFloat(); // ry
                double w = sp.getFloat(), h = sp.getFloat();
                double sc = sp.getFloat(), ss = sp.getFloat(), tx = sp.getFloat(), ty = sp.getFloat();
                sp.getFloat(); // alpha
                local.set(m.a, m.b, m.c, m.d, m.e, m.f);
                local.then(sc, ss, -ss, sc, tx, ty);
                out.add(local, 0, 0);
                out.add(local, w, 0);
                out.add(local, 0, h);
                out.add(local, w, h);
            }
        }

        private static boolean skipGradient(ByteBuffer buf) {
            // 1 byte kind + 7 floats + 6 floats + 1 byte spread = 54 bytes before stop count.
            skipBytes(buf, 54);
            int n = buf.getInt();
            if (n < 0) return false;
            skipBytes(buf, n * 8L);
            return true;
        }

        private static void skipBytes(ByteBuffer buf, long count) {
            if (count < 0 || count > buf.remaining()) throw new BufferUnderflowException();
            buf.position(buf.position() + (int) count);
        }

        private static boolean bytesEqual(byte[] a, int aOff, byte[] b, int bOff, int len) {
            for (int i = 0; i < len; i++) {
                if (a[aOff + i] != b[bOff + i]) return false;
            }
            return true;
        }

        private static boolean finite(double v) {
            return !Double.isNaN(v) && !Double.isInfinite(v);
        }
    }
}
