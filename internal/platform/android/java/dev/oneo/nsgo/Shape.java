package dev.oneo.nsgo;

import android.graphics.Path;

/**
 * Outlines of rounded rectangles, as driver.RoundedRect and EllipticRect
 * draw them, so Android's corners match the other platforms': circular or
 * continuous, easing into the edges over (1 + SMOOTHING) times their radius
 * where the box has room.
 */
final class Shape {
    static final double SMOOTHING = 0.6;

    private Shape() {}

    /**
     * Adds the box at x, y of size w, h with corners (top left, top right,
     * bottom right, bottom left), fitted as CSS fits them, to path.
     */
    static void roundedRect(Path path, float x, float y, float w, float h, float[] corners, boolean continuous) {
        float[] r = fit(corners, w, h);
        float[][] radii = new float[4][];
        for (int i = 0; i < 4; i++) radii[i] = new float[] {r[i], r[i]};
        ellipticRect(path, x, y, w, h, radii, continuous);
    }

    /** The radii scaled down, all by one factor, until adjacent ones fit along a w by h box. */
    static float[] fit(float[] corners, float w, float h) {
        float[] r = new float[4];
        for (int i = 0; i < 4; i++) r[i] = Math.max(corners[i], 0);
        float f = 1;
        float[][] sides = {{w, r[0] + r[1]}, {h, r[1] + r[2]}, {w, r[2] + r[3]}, {h, r[3] + r[0]}};
        for (float[] s : sides) {
            if (s[1] > 0 && s[0] >= 0 && s[1] * f > s[0]) f = s[0] / s[1];
        }
        for (int i = 0; i < 4; i++) r[i] *= f;
        return r;
    }

    /** The radii less d, never below 0, as CSS shrinks inner corners and grows spread ones. */
    static float[] inset(float[] corners, float d) {
        float[] r = new float[4];
        for (int i = 0; i < 4; i++) r[i] = corners[i] > 0 ? Math.max(corners[i] - d, 0) : 0;
        return r;
    }

    /**
     * Adds the box with each corner's radius along its horizontal and
     * vertical edge (radii[i][0], radii[i][1]), already fitted, to path.
     */
    static void ellipticRect(Path path, float x, float y, float w, float h, float[][] radii, boolean continuous) {
        float[] tl = radii[0], tr = radii[1], br = radii[2], bl = radii[3];
        float x1 = x + w, y1 = y + h;
        // Each corner: its point, the directions it is entered and left
        // along, its radius along each and how far it may reach.
        double[][] specs = {
            {x1, y, 1, 0, 0, 1, tr[0], tr[1], limit(w, tr[0], tl[0]), limit(h, tr[1], br[1])},
            {x1, y1, 0, 1, -1, 0, br[1], br[0], limit(h, br[1], tr[1]), limit(w, br[0], bl[0])},
            {x, y1, -1, 0, 0, -1, bl[0], bl[1], limit(w, bl[0], br[0]), limit(h, bl[1], tl[1])},
            {x, y, 0, -1, 1, 0, tl[1], tl[0], limit(h, tl[1], bl[1]), limit(w, tl[0], tr[0])},
        };
        double smoothing = continuous ? SMOOTHING : 0;
        for (int i = 0; i < 4; i++) {
            double[] s = specs[i];
            double ru = s[6], rv = s[7];
            double r = Math.max(ru, rv);
            if (r <= 0) {
                if (i == 0) path.moveTo((float) s[0], (float) s[1]);
                else path.lineTo((float) s[0], (float) s[1]);
                continue;
            }
            double su = ru / r, sv = rv / r;
            double budget = r;
            if (su > 0 && sv > 0) budget = Math.max(Math.min(s[8] / su, s[9] / sv), r);
            double sm = Math.max(Math.min(smoothing, budget / r - 1), 0);
            double p = Math.min((1 + sm) * r, budget);
            double arc = Math.PI / 2 * (1 - sm);
            double arcLen = Math.sin(arc / 2) * r * Math.sqrt(2);
            double alpha = (Math.PI / 2 - arc) / 2;
            double p3p4 = r * Math.tan(alpha / 2);
            double beta = Math.PI / 4 * sm;
            double c = p3p4 * Math.cos(beta);
            double d = c * Math.tan(beta);
            double b = Math.max((p - arcLen - c - d) / 3, 0);
            double a = 2 * b;
            Corner k = new Corner(s, su, sv);
            if (i == 0) path.moveTo(k.x(-p, 0), k.y(-p, 0));
            else path.lineTo(k.x(-p, 0), k.y(-p, 0));
            boolean transitions = a + b + c > 1e-9 * r;
            if (transitions) {
                path.cubicTo(k.x(-p + a, 0), k.y(-p + a, 0), k.x(-p + a + b, 0), k.y(-p + a + b, 0),
                        k.x(-p + a + b + c, d), k.y(-p + a + b + c, d));
            }
            // The arc, symmetric about the corner's diagonal, as one cubic;
            // its circle is centered at (-r, r) in the corner's frame.
            double hk = 4.0 / 3.0 * Math.tan(arc / 4) * r;
            double start = -Math.PI / 4 - arc / 2, end = -Math.PI / 4 + arc / 2;
            double p0x = -r + r * Math.cos(start), p0y = r + r * Math.sin(start);
            double p3x = -r + r * Math.cos(end), p3y = r + r * Math.sin(end);
            double t0x = -Math.sin(start), t0y = Math.cos(start), t3x = -Math.sin(end), t3y = Math.cos(end);
            path.cubicTo(k.x(p0x + hk * t0x, p0y + hk * t0y), k.y(p0x + hk * t0x, p0y + hk * t0y),
                    k.x(p3x - hk * t3x, p3y - hk * t3y), k.y(p3x - hk * t3x, p3y - hk * t3y), k.x(p3x, p3y), k.y(p3x, p3y));
            if (transitions) {
                path.cubicTo(k.x(0, p - a - b), k.y(0, p - a - b), k.x(0, p - a), k.y(0, p - a), k.x(0, p), k.y(0, p));
            }
        }
        path.close();
    }

    private static double limit(double length, double r, double other) {
        return r + other > 0 ? length * r / (r + other) : 0;
    }

    /** A corner's frame: along u from the corner, along v from it, each scaled to its own radius. */
    private static final class Corner {
        final double[] s;
        final double su, sv;

        Corner(double[] s, double su, double sv) {
            this.s = s;
            this.su = su;
            this.sv = sv;
        }

        float x(double a, double b) {
            return (float) (s[0] + s[2] * a * su + s[4] * b * sv);
        }

        float y(double a, double b) {
            return (float) (s[1] + s[3] * a * su + s[5] * b * sv);
        }
    }
}
