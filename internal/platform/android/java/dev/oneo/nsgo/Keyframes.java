package dev.oneo.nsgo;

import android.view.animation.PathInterpolator;
import java.io.DataInputStream;
import java.io.IOException;
import java.util.Arrays;

/**
 * A keyframe animation's steps, as driver.KeyframeAnimation.Cycle gives
 * them: android.go sends each step's place in the cycle, the opacity,
 * scale, turn and drop (in heights) it shows, and the easing to the next.
 */
final class Keyframes {
    long duration, delay;
    int iterations;
    /** Each step: offset, opacity, scale, rotate, translateY, then its easing's x1, y1, x2, y2. */
    float[][] steps = new float[0][];
    private PathInterpolator[] easings = new PathInterpolator[0];

    static Keyframes read(DataInputStream in) throws IOException {
        Keyframes k = new Keyframes();
        k.duration = in.readInt() & 0xffffffffL;
        k.delay = in.readInt() & 0xffffffffL;
        k.iterations = in.readInt();
        k.steps = new float[in.readUnsignedByte()][9];
        k.easings = new PathInterpolator[k.steps.length];
        for (int i = 0; i < k.steps.length; i++) {
            for (int j = 0; j < 9; j++) k.steps[i][j] = in.readFloat();
            float[] s = k.steps[i];
            boolean linear = s[5] == 0 && s[6] == 0 && s[7] == 0 && s[8] == 0;
            k.easings[i] = linear ? null : new PathInterpolator(s[5], s[6], s[7], s[8]);
        }
        return k;
    }

    /** The look l shows at fraction x of a cycle on a view height pixels tall, over its own. */
    Look apply(Look l, float x, int height) {
        for (int i = 1; i < steps.length; i++) {
            float[] a = steps[i - 1], b = steps[i];
            if (x > b[0] && i < steps.length - 1) continue;
            float f = b[0] > a[0] ? Math.max(0, Math.min(1, (x - a[0]) / (b[0] - a[0]))) : 1;
            if (easings[i - 1] != null) f = easings[i - 1].getInterpolation(f);
            Look out = l.copy();
            out.opacity *= a[1] + (b[1] - a[1]) * f;
            float scale = a[2] + (b[2] - a[2]) * f;
            out.scaleX *= scale;
            out.scaleY *= scale;
            out.rotation += a[3] + (b[3] - a[3]) * f;
            out.translateY += (a[4] + (b[4] - a[4]) * f) * height;
            return out;
        }
        return l;
    }

    @Override
    public boolean equals(Object o) {
        if (!(o instanceof Keyframes)) return false;
        Keyframes k = (Keyframes) o;
        return duration == k.duration && delay == k.delay && iterations == k.iterations
                && Arrays.deepEquals(steps, k.steps);
    }

    @Override
    public int hashCode() {
        return Arrays.deepHashCode(steps);
    }
}
