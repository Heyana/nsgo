package android.graphics;

public class Path {
    public enum FillType { WINDING, EVEN_ODD }
    public enum Direction { CW }

    public boolean empty = true;
    public float minX = Float.POSITIVE_INFINITY, minY = Float.POSITIVE_INFINITY;
    public float maxX = Float.NEGATIVE_INFINITY, maxY = Float.NEGATIVE_INFINITY;
    public FillType fillType = FillType.WINDING;

    public void reset() {
        empty = true;
        minX = Float.POSITIVE_INFINITY;
        minY = Float.POSITIVE_INFINITY;
        maxX = Float.NEGATIVE_INFINITY;
        maxY = Float.NEGATIVE_INFINITY;
    }

    public void setFillType(FillType ft) { fillType = ft; }

    private void add(float x, float y) {
        empty = false;
        if (x < minX) minX = x;
        if (y < minY) minY = y;
        if (x > maxX) maxX = x;
        if (y > maxY) maxY = y;
    }

    public void moveTo(float x, float y) { add(x, y); }
    public void lineTo(float x, float y) { add(x, y); }

    public void cubicTo(float x1, float y1, float x2, float y2, float x3, float y3) {
        add(x1, y1);
        add(x2, y2);
        add(x3, y3);
    }

    public void addRect(float left, float top, float right, float bottom, Direction dir) {
        add(left, top);
        add(right, bottom);
    }

    public void close() {}
}
