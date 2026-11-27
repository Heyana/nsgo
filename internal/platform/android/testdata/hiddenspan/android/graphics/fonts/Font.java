package android.graphics.fonts;

import android.content.res.AssetManager;
import java.io.File;
import java.io.IOException;
import java.nio.ByteBuffer;

public final class Font {
    private Font() {}

    public static final class Builder {
        public Builder(AssetManager assets, String path) throws IOException { throw new IOException("no fonts on the JVM"); }
        public Builder(File file) throws IOException { throw new IOException("no fonts on the JVM"); }
        public Builder setWeight(int weight) { return this; }
        public Builder setSlant(int slant) { return this; }
        public Builder setFontVariationSettings(String variationSettings) { return this; }
        public Font build() throws IOException { throw new IOException("no fonts on the JVM"); }
    }

    public FontStyle getStyle() { return new FontStyle(400, FontStyle.FONT_SLANT_UPRIGHT); }
    public File getFile() { return null; }
    public int getTtcIndex() { return 0; }
    public ByteBuffer getBuffer() { return ByteBuffer.allocate(0); }
}
