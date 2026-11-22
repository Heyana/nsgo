package dev.oneo.nsgo;

import java.io.DataInputStream;
import java.io.FileInputStream;

/** Run by backdrop_test.go: decodes Go-written backdrop filters with BackdropMath and prints them, floats as their bits, with the padded blur bounds of each case that follows, for Go to compare against internal/driver. */
public final class BackdropCheck {
    public static void main(String[] args) throws Exception {
        try (DataInputStream in = new DataInputStream(new FileInputStream(args[0]))) {
            for (int i = 0, count = in.readInt(); i < count; i++) {
                float density = in.readFloat();
                BackdropMath.Spec spec = BackdropMath.read(in, density);
                StringBuilder sb = new StringBuilder("spec ").append(spec.active()).append(' ')
                        .append(Integer.toHexString(Float.floatToIntBits(spec.blur))).append(' ').append(spec.colorMatrix != null);
                if (spec.colorMatrix != null) {
                    for (float v : BackdropMath.colorMatrix255(spec.colorMatrix)) sb.append(' ').append(Integer.toHexString(Float.floatToIntBits(v)));
                }
                System.out.println(sb);
            }
            for (int i = 0, count = in.readInt(); i < count; i++) {
                float[] f = new float[9];
                for (int k = 0; k < 9; k++) f[k] = in.readFloat();
                int[] r = BackdropMath.paddedBounds(f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8]);
                System.out.println(r == null ? "pad null" : "pad " + r[0] + " " + r[1] + " " + r[2] + " " + r[3]);
            }
            if (in.available() != 0) throw new IllegalStateException(in.available() + " bytes left");
        }
    }
}
