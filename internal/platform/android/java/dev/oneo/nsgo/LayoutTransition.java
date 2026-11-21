package dev.oneo.nsgo;

import android.animation.TimeInterpolator;
import java.io.DataInputStream;
import java.io.IOException;

/**
 * One phase of a batch's LayoutAnimation, or a frame's own transition,
 * sampled as driver.LayoutTransition.At (internal/driver/animated.go).
 * Durations are in nanoseconds. Springs run by their physics (response and
 * damping ratio) until settled, and Motion preserves frame position and
 * velocity across mid-flight retargets.
 */
final class LayoutTransition {
    // Curves, as core.LayoutCurve.
    static final int LINEAR = 0;
    static final int EASE_IN = 1;
    static final int EASE_OUT = 2;
    static final int EASE_IN_OUT = 3;
    static final int SPRING = 4;
    static final int KEYBOARD = 5;
    static final int BEZIER = 6;
    // What appearing and disappearing views animate, as core.LayoutProperty.
    static final int OPACITY = 0;
    static final int SCALE = 1;

    private static final double DEFAULT_RESPONSE_SEC = 0.5;
    private static final double MIN_RESPONSE_SEC = 0.001;
    private static final double MIN_DAMPING_RATIO = 0.05;
    private static final double MAX_SETTLE_SEC = 60.0;
    private static final double UNIT_SETTLE_EPS = 1e-3;
    private static final double FRAME_SETTLE_EPS = 0.05;
    private static final double CRIT_TOL = 1e-5;

    int curve, property;
    float damping;
    /** BEZIER's control points: x1, y1, x2, y2. */
    float[] bezier;
    long duration, delay;
    /** SPRING's natural period in nanoseconds, and damping ratio. */
    long response;
    double dampingRatio;
    /**
     * On the keyboard's curve, the IME's interpolator, and how far the
     * keyboard moved before the batch came, in nanoseconds.
     */
    private TimeInterpolator ime;
    private long ahead;

    /** The keyboard's motion as WindowInsetsAnimation reports it: nanoseconds. */
    static final class Keyboard {
        TimeInterpolator interpolator;
        long duration, start;
    }

    /**
     * Moves a phase on the keyboard's curve onto the keyboard's motion
     * while it runs at now: its interpolator and duration, and as far in
     * as the keyboard is.
     */
    void alignToKeyboard(Keyboard k, long now) {
        if (curve != KEYBOARD || k.duration <= 0 || now - k.start >= k.duration) return;
        ime = k.interpolator;
        duration = k.duration;
        delay = 0;
        ahead = now - k.start;
    }

    /** Reads a transition as protocol.go's writeLayoutTransition writes it: null for none. */
    static LayoutTransition read(DataInputStream in) throws IOException {
        if (in.readUnsignedByte() == 0) return null;
        LayoutTransition t = new LayoutTransition();
        t.curve = in.readUnsignedByte();
        t.damping = in.readFloat();
        t.bezier = new float[] {in.readFloat(), in.readFloat(), in.readFloat(), in.readFloat()};
        t.property = in.readUnsignedByte();
        t.duration = in.readLong();
        t.delay = in.readLong();
        t.response = in.readLong();
        t.dampingRatio = in.readDouble();
        return t;
    }

    /** Resolves (responseSec, zeta) matching driver.LayoutTransition.springParams. */
    double[] springParams() {
        double zeta = dampingRatio;
        if (zeta <= 0 && damping > 0) zeta = damping;
        if (zeta <= 0) zeta = 1.0;
        if (zeta < MIN_DAMPING_RATIO) zeta = MIN_DAMPING_RATIO;
        double respSec;
        if (response > 0) {
            respSec = response / 1e9;
        } else if (duration > 0) {
            double dur = duration / 1e9;
            double w0;
            if (zeta < 1.0 - CRIT_TOL) {
                w0 = (6.907755278982137 / zeta) / dur;
            } else if (zeta <= 1.0 + CRIT_TOL) {
                w0 = 9.23342 / dur;
            } else {
                double s = Math.sqrt(zeta * zeta - 1.0);
                double c1 = (zeta + s) / (2.0 * s);
                w0 = Math.log(c1 / UNIT_SETTLE_EPS) * (zeta + s) / dur;
            }
            respSec = 2.0 * Math.PI / w0;
        } else {
            respSec = DEFAULT_RESPONSE_SEC;
        }
        if (respSec < MIN_RESPONSE_SEC) respSec = MIN_RESPONSE_SEC;
        return new double[] {respSec, zeta};
    }

    /**
     * Evaluates a 1D damped harmonic oscillator at t >= 0 (seconds) from
     * displacement y0 = from - to and velocity v0 (units/sec). Writes {y, v}
     * into out and returns whether settled within eps.
     */
    static boolean evalSpring1D(double y0, double v0, double w0, double zeta, double t, double eps, double[] out) {
        if (t <= 0) {
            if (Math.abs(y0) <= eps && Math.abs(v0) / w0 <= eps) {
                out[0] = 0;
                out[1] = 0;
                return true;
            }
            out[0] = y0;
            out[1] = v0;
            return false;
        }
        if (t >= MAX_SETTLE_SEC) {
            out[0] = 0;
            out[1] = 0;
            return true;
        }
        double y, v;
        if (zeta < 1.0 - CRIT_TOL) {
            double alpha = zeta * w0;
            double wd = w0 * Math.sqrt(1.0 - zeta * zeta);
            double env = Math.exp(-alpha * t);
            double cos = Math.cos(wd * t), sin = Math.sin(wd * t);
            double b = (v0 + alpha * y0) / wd;
            y = env * (y0 * cos + b * sin);
            v = env * (v0 * cos - (alpha * v0 + w0 * w0 * y0) / wd * sin);
        } else if (zeta <= 1.0 + CRIT_TOL) {
            double env = Math.exp(-w0 * t);
            double b = v0 + w0 * y0;
            y = env * (y0 + b * t);
            v = env * (v0 - w0 * b * t);
        } else {
            double s = Math.sqrt(zeta * zeta - 1.0);
            double gamma = w0 * s;
            double r2 = -w0 * (zeta + s);
            double r1 = -w0 / (zeta + s);
            double c1 = (v0 - r2 * y0) / (2.0 * gamma);
            double c2 = y0 - c1;
            double e1 = Math.exp(r1 * t), e2 = Math.exp(r2 * t);
            y = c1 * e1 + c2 * e2;
            v = c1 * r1 * e1 + c2 * r2 * e2;
        }
        if (Math.abs(y) <= eps && Math.abs(v) / w0 <= eps) {
            out[0] = 0;
            out[1] = 0;
            return true;
        }
        out[0] = y;
        out[1] = v;
        return false;
    }

    /** The progress at elapsed nanoseconds since the change; a spring may overshoot 1. */
    double at(long elapsed) {
        elapsed += ahead;
        if (curve == SPRING) {
            if (elapsed <= delay) return 0;
            double[] params = springParams();
            double w0 = 2.0 * Math.PI / params[0];
            double[] out = new double[2];
            if (evalSpring1D(-1.0, 0.0, w0, params[1], (elapsed - delay) / 1e9, UNIT_SETTLE_EPS, out)) {
                return 1.0;
            }
            return 1.0 + out[0];
        }
        double x = 1;
        if (duration > 0) x = (double) (elapsed - delay) / (double) duration;
        if (x <= 0) return 0;
        if (x >= 1) return 1;
        switch (curve) {
            case EASE_IN: return Animator.bezier(0.42f, 0, 1, 1, x);
            case EASE_OUT: return Animator.bezier(0, 0, 0.58f, 1, x);
            case EASE_IN_OUT: return Animator.bezier(0.42f, 0, 0.58f, 1, x);
            case BEZIER: return Animator.bezier(bezier[0], bezier[1], bezier[2], bezier[3], x);
            // The IME's own interpolator, else InsetsController's.
            case KEYBOARD: return ime != null ? ime.getInterpolation((float) x) : Animator.bezier(0.2f, 0, 0, 1, x);
            default: return x;
        }
    }

    /** Whether the transition is done at elapsed nanoseconds. */
    boolean done(long elapsed) {
        elapsed += ahead;
        if (curve == SPRING) {
            if (elapsed <= delay) return false;
            double[] params = springParams();
            double w0 = 2.0 * Math.PI / params[0];
            double[] out = new double[2];
            return evalSpring1D(-1.0, 0.0, w0, params[1], (elapsed - delay) / 1e9, UNIT_SETTLE_EPS, out);
        }
        return duration <= 0 || elapsed - delay >= duration;
    }

    /** A frame (x, y, width, height) from a to b at progress p, never of negative size, as core.LerpBounds. */
    static float[] lerp(float[] a, float[] b, double p) {
        float[] f = new float[4];
        for (int i = 0; i < 4; i++) f[i] = a[i] + (float) ((b[i] - a[i]) * p);
        f[2] = Math.max(f[2], 0);
        f[3] = Math.max(f[3], 0);
        return f;
    }

    /**
     * Tracks a view's frame moving under a LayoutTransition, preserving
     * current position and velocity across mid-flight spring retargets, as
     * driver.LayoutMotion.
     */
    static final class Motion {
        LayoutTransition tr;
        long start;
        final double[] from = new double[4];
        final double[] vel0 = new double[4];
        final double[] to = new double[4];
        final double[] pos = new double[4];
        final double[] vel = new double[4];
        final float[] shown = new float[4];
        final float[] target = new float[4];
        boolean done;

        static Motion start(float[] fromFrame, float[] toFrame, LayoutTransition tr, long now) {
            Motion m = new Motion();
            m.tr = tr;
            m.start = now;
            boolean same = true;
            for (int i = 0; i < 4; i++) {
                m.from[i] = fromFrame[i];
                m.pos[i] = fromFrame[i];
                m.to[i] = toFrame[i];
                m.shown[i] = fromFrame[i];
                m.target[i] = toFrame[i];
                if (fromFrame[i] != toFrame[i]) same = false;
            }
            m.done = same;
            return m;
        }

        void retarget(float[] toFrame, LayoutTransition next, long now) {
            if (!done) step(now);
            for (int i = 0; i < 4; i++) {
                from[i] = pos[i];
                vel0[i] = next.curve == SPRING ? vel[i] : 0;
                if (next.curve != SPRING) vel[i] = 0;
                to[i] = toFrame[i];
                target[i] = toFrame[i];
            }
            tr = next;
            start = now;
            writeShown(from);
            done = false;
            if (next.curve == SPRING) {
                double[] params = next.springParams();
                double w0 = 2.0 * Math.PI / params[0];
                boolean settled = true;
                for (int i = 0; i < 4; i++) {
                    if (Math.abs(from[i] - to[i]) > FRAME_SETTLE_EPS || Math.abs(vel0[i]) / w0 > FRAME_SETTLE_EPS) {
                        settled = false;
                        break;
                    }
                }
                if (settled) {
                    finish();
                }
            } else {
                boolean same = true;
                for (int i = 0; i < 4; i++) {
                    if (from[i] != to[i]) same = false;
                }
                if (same) finish();
            }
        }

        boolean step(long now) {
            if (done) return true;
            long elapsed = now - start;
            if (tr.curve == SPRING) {
                if (elapsed <= tr.delay) {
                    System.arraycopy(from, 0, pos, 0, 4);
                    System.arraycopy(vel0, 0, vel, 0, 4);
                    writeShown(from);
                    return false;
                }
                double t = (elapsed - tr.delay) / 1e9;
                double[] params = tr.springParams();
                double w0 = 2.0 * Math.PI / params[0];
                double[] out = new double[2];
                boolean allSettled = true;
                for (int i = 0; i < 4; i++) {
                    boolean ok = evalSpring1D(from[i] - to[i], vel0[i], w0, params[1], t, FRAME_SETTLE_EPS, out);
                    pos[i] = to[i] + out[0];
                    vel[i] = out[1];
                    if (!ok) allSettled = false;
                }
                if (allSettled) {
                    finish();
                    return true;
                }
                writeShown(pos);
                return false;
            }
            for (int i = 0; i < 4; i++) vel[i] = 0;
            if (tr.done(elapsed)) {
                finish();
                return true;
            }
            double p = tr.at(elapsed);
            for (int i = 0; i < 4; i++) pos[i] = from[i] + (to[i] - from[i]) * p;
            writeShown(pos);
            return false;
        }

        private void finish() {
            System.arraycopy(to, 0, pos, 0, 4);
            for (int i = 0; i < 4; i++) vel[i] = 0;
            writeShown(to);
            done = true;
        }

        private void writeShown(double[] v) {
            shown[0] = (float) v[0];
            shown[1] = (float) v[1];
            shown[2] = Math.max((float) v[2], 0f);
            shown[3] = Math.max((float) v[3], 0f);
        }
    }
}
