package android.graphics;

import java.util.ArrayList;
import java.util.List;

public class Canvas {
    public static final class Draw {
        public final String kind;
        public final Paint.Style style;
        public final int color;
        public final float strokeWidth;
        public final boolean hasShader, hasMaskFilter, hasPathEffect;
        public final float minX, minY, maxX, maxY;

        Draw(String kind, Paint.Style style, int color, float strokeWidth,
                boolean hasShader, boolean hasMaskFilter, boolean hasPathEffect,
                float minX, float minY, float maxX, float maxY) {
            this.kind = kind;
            this.style = style;
            this.color = color;
            this.strokeWidth = strokeWidth;
            this.hasShader = hasShader;
            this.hasMaskFilter = hasMaskFilter;
            this.hasPathEffect = hasPathEffect;
            this.minX = minX;
            this.minY = minY;
            this.maxX = maxX;
            this.maxY = maxY;
        }
    }

    public final List<Draw> draws = new ArrayList<>();
    private int depth;

    public int save() { return depth++; }
    public void restoreToCount(int count) { depth = count; }
    public void translate(float dx, float dy) {}

    public boolean clipOutPath(Path path) {
        draws.add(new Draw("clipOut", null, 0, 0, false, false, false,
                path.minX, path.minY, path.maxX, path.maxY));
        return true;
    }

    public boolean clipPath(Path path) {
        draws.add(new Draw("clip", null, 0, 0, false, false, false,
                path.minX, path.minY, path.maxX, path.maxY));
        return true;
    }

    public boolean clipRect(float left, float top, float right, float bottom) {
        draws.add(new Draw("clip", null, 0, 0, false, false, false, left, top, right, bottom));
        return true;
    }

    public void drawRect(float left, float top, float right, float bottom, Paint paint) {
        draws.add(new Draw("rect", paint.style, paint.color, paint.strokeWidth,
                paint.shader != null, paint.maskFilter != null, paint.pathEffect != null,
                left, top, right, bottom));
    }

    public void drawPath(Path path, Paint paint) {
        draws.add(new Draw("path", paint.style, paint.color, paint.strokeWidth,
                paint.shader != null, paint.maskFilter != null, paint.pathEffect != null,
                path.minX, path.minY, path.maxX, path.maxY));
    }
}
