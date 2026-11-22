package android.graphics.drawable;

import android.graphics.Canvas;
import android.graphics.Rect;

public class GradientDrawable extends Drawable {
    private Rect bounds = new Rect(0, 0, 0, 0);

    public Rect getBounds() { return bounds; }
    public void setBounds(int left, int top, int right, int bottom) {
        bounds = new Rect(left, top, right, bottom);
    }
    public void setColor(int argb) {}
    public void setCornerRadius(float radius) {}
    public void setCornerRadii(float[] radii) {}
    public void invalidateSelf() {}
    public void draw(Canvas canvas) {}
}
