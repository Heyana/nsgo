package android.graphics.drawable;

public class LayerDrawable extends Drawable {
    private final Drawable[] layers;

    public LayerDrawable(Drawable[] layers) { this.layers = layers; }
    public int getNumberOfLayers() { return layers.length; }
    public Drawable getDrawable(int index) { return layers[index]; }
}
