package dev.oneo.nsgo;

import android.graphics.Bitmap;

/**
 * A stack blur, a fast approximation of a Gaussian, for image nodes on
 * Android before 12, which has no RenderEffect to blur a view with. It
 * follows NSH's Blur.java.
 */
final class Blur {
    private Blur() {}

    /** bitmap blurred by radius pixels of the stack: a new, mutable bitmap. */
    static Bitmap blurred(Bitmap bitmap, int radius) {
        radius = Math.min(254, radius);
        int w = bitmap.getWidth(), h = bitmap.getHeight();
        if (radius < 1 || w == 0 || h == 0) return bitmap;
        Bitmap out = bitmap.copy(Bitmap.Config.ARGB_8888, true);
        int[] px = new int[w * h];
        out.getPixels(px, 0, w, 0, 0, w, h);
        // Averaged unpremultiplied, transparent pixels would darken the
        // colors around them.
        for (int i = 0; i < px.length; i++) px[i] = premultiplied(px[i]);
        int[] stack = new int[(2 * radius + 1) * 4];
        for (int y = 0; y < h; y++) line(px, y * w, 1, w, radius, stack);
        for (int x = 0; x < w; x++) line(px, x, w, h, radius, stack);
        for (int i = 0; i < px.length; i++) px[i] = unpremultiplied(px[i]);
        out.setPixels(px, 0, w, 0, 0, w, h);
        return out;
    }

    /**
     * The stack radius whose blur looks like RenderEffect's of radius
     * pixels: its sigma is 0.577 radius + 0.5, and a stack of radius r
     * one of (r + 1) / √6.
     */
    static int stackRadius(float radius) {
        return Math.round((float) Math.sqrt(6) * (0.57735f * radius + 0.5f) - 1);
    }

    private static int premultiplied(int argb) {
        int a = argb >>> 24;
        if (a == 255) return argb;
        int r = (argb >> 16 & 0xff) * a / 255, g = (argb >> 8 & 0xff) * a / 255, b = (argb & 0xff) * a / 255;
        return a << 24 | r << 16 | g << 8 | b;
    }

    private static int unpremultiplied(int argb) {
        int a = argb >>> 24;
        if (a == 255) return argb;
        if (a == 0) return 0;
        int r = Math.min(255, (argb >> 16 & 0xff) * 255 / a), g = Math.min(255, (argb >> 8 & 0xff) * 255 / a);
        int b = Math.min(255, (argb & 0xff) * 255 / a);
        return a << 24 | r << 16 | g << 8 | b;
    }

    private static int channel(int argb, int c) {
        return (argb >>> (24 - c * 8)) & 0xff;
    }

    /** One pass in place over count pixels from start, step apart. */
    private static void line(int[] px, int start, int step, int count, int radius, int[] stack) {
        int div = 2 * radius + 1, last = count - 1;
        long[] sum = new long[4], in = new long[4], out = new long[4];
        for (int i = 0; i <= radius; i++) {
            int p = px[start];
            for (int c = 0; c < 4; c++) {
                int v = channel(p, c);
                stack[i * 4 + c] = v;
                sum[c] += (long) v * (i + 1);
                out[c] += v;
            }
        }
        for (int i = 1; i <= radius; i++) {
            int p = px[start + Math.min(i, last) * step];
            for (int c = 0; c < 4; c++) {
                int v = channel(p, c);
                stack[(i + radius) * 4 + c] = v;
                sum[c] += (long) v * (radius + 1 - i);
                in[c] += v;
            }
        }
        long weight = (long) (radius + 1) * (radius + 1);
        int sp = radius;
        for (int x = 0; x < count; x++) {
            int value = 0;
            for (int c = 0; c < 4; c++) {
                value |= (int) (sum[c] / weight) << (24 - c * 8);
                sum[c] -= out[c];
            }
            px[start + x * step] = value;
            int slot = (sp + div - radius) % div;
            int incoming = px[start + Math.min(x + radius + 1, last) * step];
            for (int c = 0; c < 4; c++) {
                int v = channel(incoming, c);
                out[c] -= stack[slot * 4 + c];
                stack[slot * 4 + c] = v;
                in[c] += v;
                sum[c] += in[c];
            }
            sp = (sp + 1) % div;
            for (int c = 0; c < 4; c++) {
                out[c] += stack[sp * 4 + c];
                in[c] -= stack[sp * 4 + c];
            }
        }
    }
}
