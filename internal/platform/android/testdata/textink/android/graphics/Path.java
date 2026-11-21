package android.graphics;

public class Path {
    public float minX = Float.POSITIVE_INFINITY, minY = Float.POSITIVE_INFINITY;
    public float maxX = Float.NEGATIVE_INFINITY, maxY = Float.NEGATIVE_INFINITY;

    public void rewind() {
        minX = Float.POSITIVE_INFINITY;
        minY = Float.POSITIVE_INFINITY;
        maxX = Float.NEGATIVE_INFINITY;
        maxY = Float.NEGATIVE_INFINITY;
    }

    public void moveTo(float x, float y) {
        grow(x, y);
    }

    public void quadTo(float x1, float y1, float x2, float y2) {
        grow(x1, y1);
        grow(x2, y2);
    }

    private void grow(float x, float y) {
        if (x < minX) minX = x;
        if (y < minY) minY = y;
        if (x > maxX) maxX = x;
        if (y > maxY) maxY = y;
    }
}
