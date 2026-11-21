package android.graphics;

public class Bitmap {
    public enum Config { ARGB_8888 }

    private final int width, height;

    private Bitmap(int width, int height) {
        this.width = width;
        this.height = height;
    }

    public static Bitmap createBitmap(int width, int height, Config config) { return new Bitmap(width, height); }
    public int getWidth() { return width; }
    public int getHeight() { return height; }
    public void eraseColor(int color) {}
    public void recycle() {}
}
