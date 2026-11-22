package dev.oneo.nsgo;

import android.graphics.BitmapShader;
import android.graphics.Canvas;
import android.graphics.Matrix;
import android.graphics.Paint;
import android.graphics.Shader;
import android.os.Build;

import java.nio.ByteBuffer;
import java.util.Arrays;

/** Draws ordered atlas tiles with one shared bitmap shader per drawing. */
final class AtlasMesh {
    static final int SPRITE_BYTES = 9 * Float.BYTES;
    private static final int CHUNK = 512;
    private static final int VERTICES = 4;
    private static final int TRIANGLE_INDICES = 6;

    private AtlasMesh() {}

    // The matrix is used only to decide whether the recorded quads line
    // up with device pixels. Drawing still uses the canvas transform.
    @SuppressWarnings("deprecation")
    static boolean draw(Canvas canvas, CanvasNode.Picture picture, ByteBuffer source, int count, float alpha, boolean nearest) {
        if (Build.VERSION.SDK_INT < 29 || !canvas.isHardwareAccelerated()) return false;
        Matrix device = canvas.getMatrix();
        float[] m = new float[9];
        device.getValues(m);
        if (!device.isAffine() || m[Matrix.MSKEW_X] != 0 || m[Matrix.MSKEW_Y] != 0
                || m[Matrix.MSCALE_X] <= 0 || m[Matrix.MSCALE_Y] <= 0) return false;
        float scaleX = m[Matrix.MSCALE_X], scaleY = m[Matrix.MSCALE_Y];
        float offsetX = m[Matrix.MTRANS_X], offsetY = m[Matrix.MTRANS_Y];
        ByteBuffer sprites = source.duplicate();
        final float density = picture.scale;
        int bitmapWidth = picture.bitmap.getWidth(), bitmapHeight = picture.bitmap.getHeight();
        // Mesh edges are aliased. Keep drawBitmap for fractional or
        // rotated edges and for crops outside the bitmap.
        for (int i = 0; i < count; i++) {
            float x = sprites.getFloat(), y = sprites.getFloat(), w = sprites.getFloat(), h = sprites.getFloat();
            float c = sprites.getFloat(), s = sprites.getFloat();
            int left = Math.round(x * density), top = Math.round(y * density);
            int right = Math.round((x + w) * density), bottom = Math.round((y + h) * density);
            if (s != 0 || c <= 0 || left < 0 || top < 0 || right > bitmapWidth || bottom > bitmapHeight
                    || left >= right || top >= bottom) return false;
            float tx = sprites.getFloat(), ty = sprites.getFloat();
            sprites.getFloat(); // opacity
            if (!pixel(tx * scaleX + offsetX) || !pixel(ty * scaleY + offsetY)
                    || !pixel((tx + w * c) * scaleX + offsetX) || !pixel((ty + h * c) * scaleY + offsetY)) return false;
        }

        sprites.position(0);
        int capacity = Math.min(CHUNK, count);
        float[] positions = new float[2 * VERTICES * capacity];
        float[] texels = new float[positions.length];
        int[] colors = new int[VERTICES * capacity];
        short[] indices = indices(capacity);
        Paint paint = new Paint(nearest ? 0 : Paint.FILTER_BITMAP_FLAG);
        paint.setShader(new BitmapShader(picture.bitmap, Shader.TileMode.CLAMP, Shader.TileMode.CLAMP));

        for (int first = 0; first < count; first += capacity) {
            int n = Math.min(capacity, count - first);
            for (int i = 0; i < n; i++) {
                float sx = sprites.getFloat(), sy = sprites.getFloat(), w = sprites.getFloat(), h = sprites.getFloat();
                float c = sprites.getFloat();
                sprites.getFloat(); // axis-aligned
                float x = sprites.getFloat(), y = sprites.getFloat();
                int color = Math.round(sprites.getFloat() * alpha * 255) << 24 | 0x00ffffff;
                int l = Math.round(sx * density), t = Math.round(sy * density);
                int r = Math.round((sx + w) * density), b = Math.round((sy + h) * density);
                int start = VERTICES * i;
                putQuad(positions, 2 * start, x, y, x + w * c, y + h * c);
                putQuad(texels, 2 * start, l, t, r, b);
                Arrays.fill(colors, start, start + VERTICES, color);
            }
            canvas.drawVertices(Canvas.VertexMode.TRIANGLES, n * VERTICES * 2, positions, 0, texels, 0,
                    colors, 0, indices, 0, n * TRIANGLE_INDICES, paint);
        }
        return true;
    }

    private static boolean pixel(float coordinate) {
        return Float.isFinite(coordinate) && coordinate == Math.rint(coordinate);
    }

    private static void putQuad(float[] out, int i, float l, float t, float r, float b) {
        out[i] = l;
        out[i + 1] = t;
        out[i + 2] = r;
        out[i + 3] = t;
        out[i + 4] = l;
        out[i + 5] = b;
        out[i + 6] = r;
        out[i + 7] = b;
    }

    private static short[] indices(int count) {
        short[] out = new short[count * TRIANGLE_INDICES];
        int next = 0;
        for (int i = 0; i < count; i++) {
            int first = i * VERTICES;
            out[next++] = (short) first;
            out[next++] = (short) (first + 1);
            out[next++] = (short) (first + 2);
            out[next++] = (short) (first + 1);
            out[next++] = (short) (first + 3);
            out[next++] = (short) (first + 2);
        }
        return out;
    }
}
