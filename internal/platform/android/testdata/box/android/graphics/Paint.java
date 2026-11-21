package android.graphics;

public class Paint {
    public static final int ANTI_ALIAS_FLAG = 1;

    public enum Style { FILL, STROKE }
    public enum Cap { BUTT, ROUND }

    public Style style = Style.FILL;
    public int color;
    public float strokeWidth;
    public Cap cap = Cap.BUTT;
    public MaskFilter maskFilter;
    public PathEffect pathEffect;
    public Shader shader;

    public Paint() {}
    public Paint(int flags) {}

    public void setStyle(Style style) { this.style = style; }
    public void setColor(int color) { this.color = color; }
    public void setStrokeWidth(float width) { this.strokeWidth = width; }
    public void setStrokeCap(Cap cap) { this.cap = cap; }
    public void setMaskFilter(MaskFilter filter) { this.maskFilter = filter; }
    public void setPathEffect(PathEffect effect) { this.pathEffect = effect; }
    public void setShader(Shader shader) { this.shader = shader; }
    public void setColorFilter(ColorFilter filter) {}
    public void setBlendMode(BlendMode mode) {}
}
