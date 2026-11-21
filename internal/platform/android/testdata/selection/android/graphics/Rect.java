package android.graphics;

public final class Rect {
    public int left, top, right, bottom;

    public Rect(int left, int top, int right, int bottom) {
        set(left, top, right, bottom);
    }

    public void set(int left, int top, int right, int bottom) {
        this.left = left;
        this.top = top;
        this.right = right;
        this.bottom = bottom;
    }

    public boolean contains(int x, int y) {
        return left <= x && x < right && top <= y && y < bottom;
    }
}
