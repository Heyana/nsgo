package android.graphics;

public class Paint {
    public static final int ANTI_ALIAS_FLAG = 1;
    public static final int FILTER_BITMAP_FLAG = 2;
    public static final int SUBPIXEL_TEXT_FLAG = 0x80;
    public static final int LINEAR_TEXT_FLAG = 0x40;

    private int flags;

    public int getFlags() { return flags; }
    public void setFlags(int flags) { this.flags = flags; }

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
