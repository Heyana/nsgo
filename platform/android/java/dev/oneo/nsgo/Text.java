package dev.oneo.nsgo;

import android.graphics.Paint;
import android.graphics.Typeface;
import android.text.Layout;
import android.text.SpannableString;
import android.text.Spanned;
import android.text.StaticLayout;
import android.text.TextPaint;
import android.text.TextUtils;
import android.text.style.LineHeightSpan;
import android.text.style.MetricAffectingSpan;
import android.view.Gravity;
import java.io.DataInputStream;
import java.io.IOException;
import java.util.List;

/**
 * Text the engine prepares, measures and lays out on the reader thread, all
 * with StaticLayout; the UI thread only draws the final layouts. Lengths
 * here are pixels.
 */
final class Text {
    private Text() {}

    /** A resolved text style, as android.go's writeStyle writes it. */
    static final class Style {
        final String family;
        final float size;
        final int weight;
        final int color;
        final float lineHeight;
        final float letterSpacing;
        final int align;
        final boolean italic, underline, lineThrough;

        Style(DataInputStream in, float density) throws IOException {
            family = Protocol.readString(in);
            size = in.readFloat() * density;
            weight = in.readUnsignedShort();
            color = argb(in.readInt());
            lineHeight = in.readFloat() * density;
            letterSpacing = in.readFloat() * density;
            align = in.readUnsignedByte();
            int flags = in.readUnsignedByte();
            italic = Protocol.flag(flags, 0);
            underline = Protocol.flag(flags, 1);
            lineThrough = Protocol.flag(flags, 2);
        }

        Typeface typeface() {
            Typeface base = family.isEmpty() ? Typeface.DEFAULT : Typeface.create(family, Typeface.NORMAL);
            return Typeface.create(base, Math.max(1, Math.min(weight, 1000)), italic);
        }

        /** The letter spacing in ems, as Paint takes it. */
        float letterSpacingEm() {
            return size > 0 ? letterSpacing / size : 0;
        }

        /** The paint flags of the decoration, over the other bits of flags. */
        int paintFlags(int flags) {
            flags &= ~(Paint.UNDERLINE_TEXT_FLAG | Paint.STRIKE_THRU_TEXT_FLAG);
            if (underline) flags |= Paint.UNDERLINE_TEXT_FLAG;
            if (lineThrough) flags |= Paint.STRIKE_THRU_TEXT_FLAG;
            return flags;
        }

        TextPaint paint() {
            TextPaint paint = new TextPaint(Paint.ANTI_ALIAS_FLAG);
            paint.setTextSize(size);
            paint.setTypeface(typeface());
            paint.setColor(color);
            paint.setLetterSpacing(letterSpacingEm());
            paint.setFlags(paintFlags(paint.getFlags()));
            return paint;
        }

        Layout.Alignment alignment() {
            switch (align) {
                case Protocol.ALIGN_CENTER: return Layout.Alignment.ALIGN_CENTER;
                case Protocol.ALIGN_END: return Layout.Alignment.ALIGN_OPPOSITE;
                default: return Layout.Alignment.ALIGN_NORMAL;
            }
        }

        /** The horizontal gravity of a text field. */
        int gravity() {
            switch (align) {
                case Protocol.ALIGN_CENTER: return Gravity.CENTER_HORIZONTAL;
                case Protocol.ALIGN_END: return Gravity.END;
                default: return Gravity.START;
            }
        }
    }

    /** A style over the bytes from start to end of a text, as android.go writes it. */
    static final class Span {
        final int start, end;
        final Style style;

        Span(DataInputStream in, float density) throws IOException {
            start = in.readInt();
            end = in.readInt();
            style = new Style(in, density);
        }
    }

    /** Text prepared once, then measured at any width and laid out at its final size. */
    static final class Prepared {
        private final CharSequence text;
        private final TextPaint paint;
        private final Layout.Alignment align;
        private final int maxLines;

        Prepared(String text, Style style, int maxLines, List<Span> spans) {
            if (style.lineHeight > 0 || !spans.isEmpty()) {
                SpannableString spanned = new SpannableString(text);
                if (style.lineHeight > 0) {
                    spanned.setSpan(new LineHeight(Math.round(style.lineHeight)), 0, text.length(),
                            Spanned.SPAN_INCLUSIVE_INCLUSIVE);
                }
                Utf8 offsets = new Utf8(text);
                for (Span span : spans) {
                    int start = offsets.index(span.start), end = offsets.index(span.end);
                    if (start < end) spanned.setSpan(new Run(span.style), start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
                }
                this.text = spanned;
            } else {
                this.text = text;
            }
            this.paint = style.paint();
            this.align = style.alignment();
            this.maxLines = maxLines;
        }

        /** The size of the text wrapped at maxWidth, or unwrapped when it is NaN. */
        float[] measure(float maxWidth) {
            StaticLayout layout = build(maxWidth, paint);
            int lines = layout.getLineCount();
            if (maxLines > 0) lines = Math.min(lines, maxLines);
            float width = 0;
            for (int i = 0; i < lines; i++) {
                width = Math.max(width, layout.getLineWidth(i));
            }
            if (text.length() == 0) width = 0;
            return new float[] {(float) Math.ceil(width), layout.getLineTop(lines)};
        }

        /** The final layout at width, with a paint of its own for the UI thread. */
        StaticLayout layout(float width) {
            return build(width, new TextPaint(paint));
        }

        private StaticLayout build(float width, TextPaint paint) {
            // The width came back from points, so allow for rounding below
            // a measured width before wrapping at it.
            int px = Float.isNaN(width)
                    ? (int) Math.ceil(Layout.getDesiredWidth(text, paint))
                    : (int) Math.ceil(width - 0.01f);
            StaticLayout.Builder b = StaticLayout.Builder.obtain(text, 0, text.length(), paint, Math.max(px, 1))
                    .setAlignment(align)
                    .setIncludePad(false);
            if (maxLines > 0) b.setMaxLines(maxLines).setEllipsize(TextUtils.TruncateAt.END);
            return b.build();
        }
    }

    /** Styles a run of text over the paint: font, size, letter spacing, decoration and color. */
    private static final class Run extends MetricAffectingSpan {
        private final Style style;
        private final Typeface typeface;

        Run(Style style) {
            this.style = style;
            typeface = style.typeface();
        }

        @Override
        public void updateMeasureState(TextPaint paint) {
            paint.setTextSize(style.size);
            paint.setTypeface(typeface);
            paint.setLetterSpacing(style.letterSpacingEm());
        }

        @Override
        public void updateDrawState(TextPaint paint) {
            updateMeasureState(paint);
            paint.setColor(style.color);
            paint.setFlags(style.paintFlags(paint.getFlags()));
        }
    }

    /** Converts ascending UTF-8 byte offsets into a string to UTF-16 indices, walking it once. */
    private static final class Utf8 {
        private final String s;
        private int index, bytes;

        Utf8(String s) {
            this.s = s;
        }

        int index(int offset) {
            if (offset < bytes) index = bytes = 0;
            while (index < s.length() && bytes < offset) {
                int c = s.codePointAt(index);
                bytes += c < 0x80 ? 1 : c < 0x800 ? 2 : c < 0x10000 ? 3 : 4;
                index += Character.charCount(c);
            }
            return index;
        }
    }

    /** The UTF-8 length of the first end chars of s. */
    static int utf8Length(CharSequence s, int end) {
        int n = 0;
        for (int i = 0; i < end; i++) {
            char c = s.charAt(i);
            if (c < 0x80) {
                n += 1;
            } else if (c < 0x800) {
                n += 2;
            } else if (Character.isHighSurrogate(c) && i + 1 < end && Character.isLowSurrogate(s.charAt(i + 1))) {
                n += 4;
                i++;
            } else {
                n += 3;
            }
        }
        return n;
    }

    /** Gives every line the same height, the glyphs centered in it as in CSS. */
    private static final class LineHeight implements LineHeightSpan {
        private final int height;

        LineHeight(int height) {
            this.height = height;
        }

        @Override
        public void chooseHeight(CharSequence text, int start, int end, int spanstartv, int lineHeight,
                Paint.FontMetricsInt fm) {
            int extra = height - (fm.descent - fm.ascent);
            fm.ascent -= extra / 2;
            fm.descent += extra - extra / 2;
            fm.top = fm.ascent;
            fm.bottom = fm.descent;
        }
    }

    static int argb(int rgba) {
        return (rgba >>> 8) | (rgba << 24);
    }
}
