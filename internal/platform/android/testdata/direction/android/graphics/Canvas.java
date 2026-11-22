package android.graphics;

public class Canvas {
    public float lastCircleX = Float.NaN;
    public float lastCircleY = Float.NaN;
    public float lastCircleR = Float.NaN;
    public void drawRoundRect(RectF rect, float rx, float ry, Paint paint) {}
    public void drawCircle(float cx, float cy, float radius, Paint paint) {
        lastCircleX = cx;
        lastCircleY = cy;
        lastCircleR = radius;
    }
}
