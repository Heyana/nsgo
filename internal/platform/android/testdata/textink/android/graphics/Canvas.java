package android.graphics;

import java.util.ArrayList;
import java.util.List;

public class Canvas {
    public Canvas() {}
    public Canvas(Bitmap bitmap) {}
    public void scale(float sx, float sy) {}
    public void drawBitmap(Bitmap bitmap, Rect src, RectF dst, Paint paint) {}
    public final List<RectF> clips = new ArrayList<>();
    public final List<RectF> saveLayers = new ArrayList<>();
    public final List<RectF> drawnRects = new ArrayList<>();
    public final List<RectF> drawnPaths = new ArrayList<>();
    public final List<Boolean> drawnShadows = new ArrayList<>();
    public int saveCount;
    public float tx, ty;

    public int save() {
        return saveCount++;
    }

    public int saveLayer(RectF bounds, Paint paint) {
        saveLayers.add(bounds == null ? null : new RectF(bounds.left, bounds.top, bounds.right, bounds.bottom));
        return saveCount++;
    }

    public int saveLayer(float left, float top, float right, float bottom, Paint paint) {
        saveLayers.add(new RectF(left, top, right, bottom));
        return saveCount++;
    }

    public void restore() {
        saveCount--;
        if (saveCount == 0) {
            tx = 0;
            ty = 0;
        }
    }

    public void restoreToCount(int count) {
        saveCount = count;
    }

    public void translate(float dx, float dy) {
        tx += dx;
        ty += dy;
    }

    public boolean clipRect(float left, float top, float right, float bottom) {
        clips.add(new RectF(left + tx, top + ty, right + tx, bottom + ty));
        return true;
    }

    public void drawRect(float left, float top, float right, float bottom, Paint paint) {
        drawnRects.add(new RectF(left + tx, top + ty, right + tx, bottom + ty));
        drawnShadows.add(paint != null && paint.hasShadow);
    }

    public void drawRect(RectF rect, Paint paint) {
        drawRect(rect.left, rect.top, rect.right, rect.bottom, paint);
    }

    public void drawRoundRect(RectF rect, float rx, float ry, Paint paint) {
        drawRect(rect, paint);
    }

    public void drawLine(float startX, float startY, float stopX, float stopY, Paint paint) {
        float half = paint != null ? paint.strokeWidth / 2 : 0;
        drawnRects.add(new RectF(Math.min(startX, stopX) + tx, Math.min(startY, stopY) - half + ty,
                Math.max(startX, stopX) + tx, Math.max(startY, stopY) + half + ty));
        drawnShadows.add(paint != null && paint.hasShadow);
    }

    public void drawPath(Path path, Paint paint) {
        if (path.maxX >= path.minX) {
            drawnPaths.add(new RectF(path.minX + tx, path.minY + ty, path.maxX + tx, path.maxY + ty));
            drawnShadows.add(paint != null && paint.hasShadow);
        }
    }

    public void drawColor(int color, PorterDuff.Mode mode) {}
}
