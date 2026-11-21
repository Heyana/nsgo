package android.graphics.fonts;

public final class FontStyle {
    public static final int FONT_WEIGHT_MIN = 1;
    public static final int FONT_WEIGHT_MAX = 1000;
    public static final int FONT_SLANT_UPRIGHT = 0;
    public static final int FONT_SLANT_ITALIC = 1;
    private final int weight, slant;

    public FontStyle(int weight, int slant) {
        this.weight = weight;
        this.slant = slant;
    }

    public int getWeight() { return weight; }
    public int getSlant() { return slant; }
}
