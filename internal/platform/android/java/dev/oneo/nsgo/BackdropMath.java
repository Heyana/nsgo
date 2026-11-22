package dev.oneo.nsgo;

import java.io.DataInputStream;
import java.io.IOException;

/** The decoding, color and geometry math Backdrop draws view backdrop filters with, free of Android classes so JVM tests check it against backdrop.go and internal/driver. */
final class BackdropMath {
    private BackdropMath() {}

    /** A view's backdrop filters: their combined color matrix (4×5, offsets from 0 to 1), null for none, and their widest blur in pixels. */
    static final class Spec {
        final float[] colorMatrix;
        final float blur;

        Spec(float[] colorMatrix, float blur) {
            this.colorMatrix = colorMatrix;
            this.blur = blur;
        }

        boolean active() {
            return blur > 0 || colorMatrix != null;
        }

        boolean sameAs(Spec o) {
            return o != null && blur == o.blur && java.util.Arrays.equals(colorMatrix, o.colorMatrix);
        }
    }

    /** Reads one view's filters as backdrop.go's writeBackdrop writes them, scaling the blur from points by density. */
    static Spec read(DataInputStream in, float density) throws IOException {
        float[] matrix = null;
        if (Protocol.flag(in.readUnsignedByte(), 0)) {
            matrix = new float[20];
            for (int i = 0; i < 20; i++) matrix[i] = in.readFloat();
        }
        float blur = Math.max(0f, in.readFloat() * density);
        return new Spec(matrix, blur);
    }

    /** Converts a driver.ColorMatrix, offsets from 0 to 1, into Android's ColorMatrix array, offsets from 0 to 255. */
    static float[] colorMatrix255(float[] matrix) {
        float[] m = matrix.clone();
        for (int i = 4; i < 20; i += 5) m[i] *= 255f;
        return m;
    }

    /** Expands the rectangle [left, top, right, bottom] by three times blurPx on each side, so the Gaussian blur samples the pixels around it, clamped to [clampLeft, clampTop, clampRight, clampBottom]. Returns {left, top, right, bottom}, or null when empty. */
    static int[] paddedBounds(float left, float top, float right, float bottom, float blurPx,
            float clampLeft, float clampTop, float clampRight, float clampBottom) {
        int pad = blurPx > 0f && Float.isFinite(blurPx) ? (int) Math.ceil(blurPx * 3f) : 0;
        int cLeft = (int) Math.floor(clampLeft);
        int cTop = (int) Math.floor(clampTop);
        int cRight = (int) Math.ceil(clampRight);
        int cBottom = (int) Math.ceil(clampBottom);
        int srcLeft = Math.max(cLeft, (int) Math.floor(left - pad));
        int srcTop = Math.max(cTop, (int) Math.floor(top - pad));
        int srcRight = Math.min(cRight, (int) Math.ceil(right + pad));
        int srcBottom = Math.min(cBottom, (int) Math.ceil(bottom + pad));
        if (srcRight <= srcLeft || srcBottom <= srcTop) return null;
        return new int[] {srcLeft, srcTop, srcRight, srcBottom};
    }
}
