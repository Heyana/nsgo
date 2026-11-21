package android.graphics;

public final class Rect {
    public int left, top, right, bottom;
    public void set(int l, int t, int r, int b) { left = l; top = t; right = r; bottom = b; }
    public int height() { return bottom - top; }
}
