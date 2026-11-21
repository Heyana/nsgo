package android.graphics;

import java.util.ArrayList;
import java.util.List;

public final class Canvas {
    public final List<String> ops = new ArrayList<>();
    private float tx, ty;

    public Canvas() {}
    public Canvas(Bitmap bitmap) {}
    public void scale(float sx, float sy) {}
    public void drawBitmap(Bitmap bitmap, Rect src, RectF dst, Paint paint) { ops.add("bitmap"); }
    private final List<float[]> stack = new ArrayList<>();

    public int save() {
        stack.add(new float[] {tx, ty});
        return stack.size();
    }
    public int saveLayer(RectF bounds, Paint paint) { return save(); }
    public int saveLayer(float left, float top, float right, float bottom, Paint paint) { return save(); }
    public void restore() {
        float[] prev = stack.remove(stack.size() - 1);
        tx = prev[0];
        ty = prev[1];
    }
    public void restoreToCount(int count) {
        while (stack.size() >= count) restore();
    }
    public void translate(float dx, float dy) { tx += dx; ty += dy; }
    public void clipRect(float l, float t, float r, float b) {}
    public void drawColor(int color, PorterDuff.Mode mode) {}
    public void drawPath(Path path, Paint paint) { ops.add("path"); }
    public void drawRect(float l, float t, float r, float b, Paint paint) {
        ops.add("deco-rect:" + Math.round(l + tx) + ".." + Math.round(r + tx));
    }
    public void drawRect(RectF rect, Paint paint) {
        drawRect(rect.left, rect.top, rect.right, rect.bottom, paint);
    }
    public void drawLine(float x1, float y1, float x2, float y2, Paint paint) {
        ops.add("deco-line:" + Math.round(x1 + tx) + ".." + Math.round(x2 + tx));
    }
    public void drawRoundRect(RectF r, float rx, float ry, Paint paint) {
        ops.add("bg:" + Math.round(r.left + tx) + ".." + Math.round(r.right + tx));
    }
    public void recordGlyphs(String text, float x, float baselineY, boolean shadowed) {
        ops.add("glyphs:" + text + "@" + Math.round(x + tx) + "," + Math.round(baselineY + ty) + (shadowed ? "+shadow" : ""));
    }
}
