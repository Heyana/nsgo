package android.graphics;

public class Paint {
    public static final int ANTI_ALIAS_FLAG = 1;
    public static final int FILTER_BITMAP_FLAG = 2;
    public static final int SUBPIXEL_TEXT_FLAG = 0x80;
    public static final int LINEAR_TEXT_FLAG = 0x40;

    private int flags;

    public int getFlags() { return flags; }
    public void setFlags(int flags) { this.flags = flags; }

    public enum Style {
        FILL, STROKE
    }

    public int color;
    public Style style = Style.FILL;
    public float strokeWidth;
    public PathEffect pathEffect;
    public Xfermode xfermode;
    public boolean hasShadow;
    public float shadowRadius, shadowDx, shadowDy;
    public int shadowColor;

    public Paint() {}

    public Paint(int flags) {}

    public void setColor(int color) {
        this.color = color;
    }

    public void setStyle(Style style) {
        this.style = style;
    }

    public void setStrokeWidth(float width) {
        this.strokeWidth = width;
    }

    public void setPathEffect(PathEffect effect) {
        this.pathEffect = effect;
    }

    public void setXfermode(Xfermode xfermode) {
        this.xfermode = xfermode;
    }

    public void setShadowLayer(float radius, float dx, float dy, int shadowColor) {
        this.hasShadow = true;
        this.shadowRadius = radius;
        this.shadowDx = dx;
        this.shadowDy = dy;
        this.shadowColor = shadowColor;
    }

    public void clearShadowLayer() {
        this.hasShadow = false;
        this.shadowRadius = 0;
        this.shadowDx = 0;
        this.shadowDy = 0;
        this.shadowColor = 0;
    }
}
