package android.graphics;

public class Canvas {
    public int save() { return 0; }
    public void restore() {}
    public int saveLayer(RectF bounds, Paint paint) { return 0; }
    public int saveLayer(float left, float top, float right, float bottom, Paint paint) { return 0; }
    public void restoreToCount(int count) {}
    public void translate(float dx, float dy) {}
    public boolean clipRect(float left, float top, float right, float bottom) { return true; }
    public void drawPath(Path path, Paint paint) {}
    public void drawColor(int color, PorterDuff.Mode mode) {}
    public void drawRect(float left, float top, float right, float bottom, Paint paint) {}
    public void drawRect(RectF rect, Paint paint) {}
    public void drawLine(float x1, float y1, float x2, float y2, Paint paint) {}
    public void drawRoundRect(RectF rect, float rx, float ry, Paint paint) {}
}
