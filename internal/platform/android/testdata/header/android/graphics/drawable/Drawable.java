package android.graphics.drawable;

import android.graphics.Canvas;
import android.graphics.ColorFilter;
import android.graphics.Rect;

public abstract class Drawable {
    public void setAutoMirrored(boolean mirrored) {}
    public boolean isAutoMirrored() { return false; }
    public int getLayoutDirection() { return 0; }
    public void invalidateSelf() {}
    public Rect getBounds() { return new Rect(); }
    public int getIntrinsicWidth() { return -1; }
    public int getIntrinsicHeight() { return -1; }
    public abstract void draw(Canvas canvas);
    public abstract void setAlpha(int alpha);
    public abstract void setColorFilter(ColorFilter filter);
    public abstract int getOpacity();
}
