package android.graphics;

public final class RenderEffect {
    public static RenderEffect createColorFilterEffect(ColorFilter filter) { return new RenderEffect(); }
    public static RenderEffect createColorFilterEffect(ColorFilter filter, RenderEffect input) { return new RenderEffect(); }
    public static RenderEffect createBlurEffect(float rx, float ry, Shader.TileMode mode) { return new RenderEffect(); }
    public static RenderEffect createBlurEffect(float rx, float ry, RenderEffect input, Shader.TileMode mode) { return new RenderEffect(); }
    public static RenderEffect createChainEffect(RenderEffect outer, RenderEffect inner) { return new RenderEffect(); }
    public static RenderEffect createOffsetEffect(float x, float y) { return new RenderEffect(); }
    public static RenderEffect createOffsetEffect(float x, float y, RenderEffect input) { return new RenderEffect(); }
    public static RenderEffect createBlendModeEffect(RenderEffect dst, RenderEffect src, BlendMode mode) { return new RenderEffect(); }
}
