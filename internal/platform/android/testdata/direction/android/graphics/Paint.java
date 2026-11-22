package android.graphics;

public class Paint {
    public static final int ANTI_ALIAS_FLAG = 1;
    public enum Style { FILL, STROKE }
    public Paint(int flags) {}
    public void setStyle(Style style) {}
    public void setStrokeWidth(float width) {}
    public void setColor(int color) {}
    public int getColor() { return 0; }
}
