package android.graphics;

import java.io.OutputStream;

public class Bitmap {
    public enum Config { ARGB_8888 }
    public enum CompressFormat { PNG }
    public static Bitmap createBitmap(int w, int h, Config c) { return new Bitmap(); }
    public boolean compress(CompressFormat fmt, int q, OutputStream out) { return true; }
    public void recycle() {}
}
