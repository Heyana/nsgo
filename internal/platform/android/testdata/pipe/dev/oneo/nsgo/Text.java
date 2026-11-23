package dev.oneo.nsgo;

import android.text.StaticLayout;
import java.io.DataInputStream;
import java.io.IOException;
import java.util.List;

/** Stand-in for Android's typesetting. Pixel sizes expose the cache's chosen density. */
final class Text {
    static final class Style {
        final float size;

        Style(DataInputStream in, float density) throws IOException {
            Protocol.readString(in); // family
            Protocol.readString(in); // language
            size = in.readFloat() * density;
            in.skipNBytes(17); // weight, color, height, spacing, align, flags, variant after size
        }

        void readDecoration(DataInputStream in, float density) throws IOException {
            in.skipNBytes(13);
        }
    }

    static final class Span {
        final Style style;
        boolean hidden;

        Span(DataInputStream in, float density) throws IOException {
            in.skipNBytes(8); // offsets
            style = new Style(in, density);
            in.skipNBytes(18); // background
        }
    }

    static final class Attachment {
        Attachment(DataInputStream in, float density) throws IOException {
            in.skipNBytes(17);
        }
    }

    static final class Shadow {
        final int color;
        final float x;

        Shadow(DataInputStream in, float density) throws IOException {
            color = in.readInt();
            x = in.readFloat() * density;
            in.skipNBytes(8);
        }
    }

    static final class Prepared {
        final String text;
        final Style style;
        final int maxLines;
        final Shadow shadow;

        Prepared(String text, Style style, int maxLines, int ellipsize, Shadow shadow,
                List<Span> spans, List<Attachment> attachments) {
            this.text = text;
            this.style = style;
            this.maxLines = maxLines;
            this.shadow = shadow.color == 0 ? null : shadow;
        }

        float[] measure(float width) { return new float[]{Math.min(width, text.length() * style.size), style.size}; }
        StaticLayout layout(float width) { return new StaticLayout(text, width, style.size); }
        boolean hasAttachments() { return false; }
        int clipLines() { return maxLines; }
        Shadow shadow() { return shadow; }
    }

    static float[] attachments(StaticLayout layout) { return new float[0]; }

    static float[][] lines(StaticLayout layout) {
        return new float[][]{{0, layout.text.length(), layout.width}};
    }

    static byte[] carets(StaticLayout layout, float density) {
        return new byte[]{(byte) (layout.width / density)};
    }
}
