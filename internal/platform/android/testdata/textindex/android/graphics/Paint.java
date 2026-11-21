package android.graphics;

public class Paint {
    public static final int ANTI_ALIAS_FLAG = 1;

    public enum Style { FILL, STROKE }

    public Paint() {}
    public Paint(int flags) {}
    public void setColor(int color) {}
    public void setPathEffect(PathEffect effect) {}
    public void setStyle(Style style) {}
    public void setStrokeWidth(float width) {}
    public Xfermode setXfermode(Xfermode xfermode) { return xfermode; }
    public void setShadowLayer(float radius, float dx, float dy, int shadowColor) {}
    public void clearShadowLayer() {}
}
