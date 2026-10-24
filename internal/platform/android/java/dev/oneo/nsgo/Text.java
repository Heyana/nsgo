package dev.oneo.nsgo;

import android.graphics.Paint;
import android.graphics.Rect;
import android.graphics.Typeface;
import android.os.Build;
import android.text.Layout;
import android.text.SpannableString;
import android.text.Spanned;
import android.text.StaticLayout;
import android.text.TextPaint;
import android.text.TextUtils;
import android.text.style.LineHeightSpan;
import android.text.style.MetricAffectingSpan;
import android.text.style.ReplacementSpan;
import android.view.Gravity;
import java.io.DataInputStream;
import java.io.IOException;
import java.util.Arrays;
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
        /** How the decoration's lines are drawn and their ARGB color, 0 for the text's; read after the rest. */
        int decorationStyle, decorationColor;

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

        /** Reads the decoration's style and color, which PREPARE sends after the text's parts. */
        void readDecoration(DataInputStream in) throws IOException {
            decorationStyle = in.readUnsignedByte();
            decorationColor = argb(in.readInt());
        }

        /** Whether TextNode draws the decoration, which a paint draws only solid and in the text's color. */
        boolean drawsDecoration() {
            return (underline || lineThrough)
                    && (decorationStyle > Protocol.DECORATION_SOLID || (decorationColor >>> 24) != 0);
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
            if (drawsDecoration()) return flags;
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

        /** The decoration TextNode draws for this style, placed by its font's metrics. */
        Decoration decoration(TextPaint measure) {
            measure.setTypeface(typeface());
            measure.setTextSize(size);
            float thickness = size / 18, under = size / 9, strike = -size * 0.28f;
            if (Build.VERSION.SDK_INT >= 29) {
                thickness = measure.getUnderlineThickness();
                under = measure.getUnderlinePosition();
                strike = measure.getStrikeThruPosition();
            }
            int color = (decorationColor >>> 24) != 0 ? decorationColor : this.color;
            return new Decoration(underline, lineThrough, decorationStyle, color, under, strike, Math.max(thickness, 1));
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

    /** A style over the bytes from start to end of a text, and its background, as android.go writes it. */
    static final class Span {
        final int start, end;
        final Style style;
        /** The background's ARGB color, 0 for none, and its corner radius. */
        final int background;
        final float radius;

        Span(DataInputStream in, float density) throws IOException {
            start = in.readInt();
            end = in.readInt();
            style = new Style(in, density);
            background = argb(in.readInt());
            radius = in.readFloat() * density;
        }
    }

    /** A shadow behind the glyphs, as android.go writes it: its ARGB color, 0 for none, offset and radius in pixels. */
    static final class Shadow {
        final int color;
        final float dx, dy, radius;

        Shadow(DataInputStream in, float density) throws IOException {
            color = argb(in.readInt());
            dx = in.readFloat() * density;
            dy = in.readFloat() * density;
            radius = in.readFloat() * density;
        }
    }

    /**
     * A decoration TextNode draws, in a style or color a paint's flags
     * cannot: positions and thickness in pixels, below the baseline when
     * positive.
     */
    static final class Decoration {
        final boolean underline, strike;
        final int style, color;
        final float under, through, thickness;

        Decoration(boolean underline, boolean strike, int style, int color, float under, float through, float thickness) {
            this.underline = underline;
            this.strike = strike;
            this.style = style;
            this.color = color;
            this.under = under;
            this.through = through;
            this.thickness = thickness;
        }
    }

    /** An attachment: the byte offset of its placeholder, its size and alignment, as android.go writes it. */
    static final class Attachment {
        final int offset;
        final int width, height, align;

        Attachment(DataInputStream in, float density) throws IOException {
            offset = in.readInt();
            width = Math.round(in.readFloat() * density);
            height = Math.round(in.readFloat() * density);
            align = in.readUnsignedByte();
        }
    }

    /** Text prepared once, then measured at any width and laid out at its final size. */
    static final class Prepared {
        private final CharSequence text;
        private final TextPaint paint;
        private final Layout.Alignment align;
        private final int maxLines, ellipsize;
        private final boolean attached, justify;

        Prepared(String text, Style style, int maxLines, int ellipsize, Shadow shadow, List<Span> spans,
                List<Attachment> attachments) {
            boolean decorated = spans.isEmpty() && style.drawsDecoration();
            if (style.lineHeight > 0 || !spans.isEmpty() || !attachments.isEmpty() || decorated) {
                SpannableString spanned = new SpannableString(text);
                if (style.lineHeight > 0) {
                    spanned.setSpan(new LineHeight(Math.round(style.lineHeight)), 0, text.length(),
                            Spanned.SPAN_INCLUSIVE_INCLUSIVE);
                }
                Utf8 offsets = new Utf8(text);
                Paint.FontMetrics font = new Paint.FontMetrics();
                TextPaint measure = new TextPaint();
                if (decorated && !text.isEmpty()) {
                    spanned.setSpan(style.decoration(measure), 0, text.length(), Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
                }
                for (Span span : spans) {
                    int start = offsets.index(span.start), end = offsets.index(span.end);
                    if (start >= end) continue;
                    spanned.setSpan(new Run(span.style), start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
                    if (span.style.drawsDecoration()) {
                        spanned.setSpan(span.style.decoration(measure), start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
                    }
                    if ((span.background >>> 24) != 0) {
                        measure.setTypeface(span.style.typeface());
                        measure.setTextSize(span.style.size);
                        measure.getFontMetrics(font);
                        spanned.setSpan(new Background(span.background, span.radius, -font.ascent, font.descent),
                                start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
                    }
                }
                // Attachments come in ascending offsets.
                for (Attachment a : attachments) {
                    int at = offsets.index(a.offset);
                    if (at >= text.length()) continue;
                    spanned.setSpan(new Placeholder(a.width, a.height, a.align), at, at + 1,
                            Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
                }
                this.text = spanned;
            } else {
                this.text = text;
            }
            this.paint = style.paint();
            if ((shadow.color >>> 24) != 0) {
                // A zero radius would remove the shadow layer.
                paint.setShadowLayer(Math.max(0.01f, shadow.radius), shadow.dx, shadow.dy, shadow.color);
            }
            this.align = style.alignment();
            this.justify = style.align == Protocol.ALIGN_JUSTIFY;
            this.maxLines = maxLines;
            this.ellipsize = ellipsize;
            this.attached = !attachments.isEmpty();
        }

        /** The lines a clipped text shows, or 0 when an ellipsis ends them. */
        int clipLines() {
            return ellipsize == Protocol.ELLIPSIZE_CLIP ? maxLines : 0;
        }

        /** Whether the text has attachments, so its layouts answer where they went. */
        boolean hasAttachments() {
            return attached;
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
            if (justify) b.setJustificationMode(Layout.JUSTIFICATION_MODE_INTER_WORD);
            if (maxLines > 0) {
                b.setMaxLines(maxLines);
                switch (ellipsize) {
                    case Protocol.ELLIPSIZE_CLIP: break; // TextNode clips the lines past the limit
                    case Protocol.ELLIPSIZE_HEAD: b.setEllipsize(TextUtils.TruncateAt.START); break;
                    case Protocol.ELLIPSIZE_MIDDLE: b.setEllipsize(TextUtils.TruncateAt.MIDDLE); break;
                    default: b.setEllipsize(TextUtils.TruncateAt.END);
                }
            }
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

    /**
     * The lines of a final layout, for OnTextLayout: per line its UTF-8
     * start and end, left, top, width and height, ascent and descent, and
     * the cap and x heights of the font it starts in, in pixels.
     */
    static float[][] lines(Layout layout) {
        if (layout == null) return new float[0][];
        CharSequence text = layout.getText();
        int count = layout.getLineCount();
        float[][] out = new float[count][];
        Rect bounds = new Rect();
        for (int i = 0; i < count; i++) {
            int start = layout.getLineStart(i), end = layout.getLineEnd(i);
            TextPaint paint = new TextPaint(layout.getPaint());
            if (text instanceof Spanned) {
                for (MetricAffectingSpan s : ((Spanned) text).getSpans(start, Math.min(start + 1, text.length()),
                        MetricAffectingSpan.class)) {
                    s.updateMeasureState(paint);
                }
            }
            paint.getTextBounds("H", 0, 1, bounds);
            float cap = bounds.height();
            paint.getTextBounds("x", 0, 1, bounds);
            float x = bounds.height();
            float top = layout.getLineTop(i);
            out[i] = new float[] {utf8Length(text, start), utf8Length(text, end), layout.getLineLeft(i), top,
                    layout.getLineWidth(i), layout.getLineBottom(i) - top, -layout.getLineAscent(i),
                    layout.getLineDescent(i), cap, x};
        }
        return out;
    }

    /**
     * Where layout put the placeholders of its attachments, in text order:
     * the x and y of each one's top left in pixels, NaN for ones cut off by
     * the line limit or the ellipsis.
     */
    static float[] attachments(Layout layout) {
        CharSequence text = layout.getText();
        if (!(text instanceof Spanned)) return new float[0];
        Spanned spanned = (Spanned) text;
        Placeholder[] spans = spanned.getSpans(0, text.length(), Placeholder.class);
        Arrays.sort(spans, (a, b) -> spanned.getSpanStart(a) - spanned.getSpanStart(b));
        float[] out = new float[spans.length * 2];
        int lines = layout.getLineCount();
        for (int i = 0; i < spans.length; i++) {
            Placeholder p = spans[i];
            int at = spanned.getSpanStart(p);
            int line = layout.getLineForOffset(at);
            boolean shown = lines > 0 && at < layout.getLineEnd(lines - 1);
            if (shown && layout.getEllipsisCount(line) > 0) {
                shown = at < layout.getLineStart(line) + layout.getEllipsisStart(line);
            }
            if (!shown) {
                out[i * 2] = Float.NaN;
                out[i * 2 + 1] = Float.NaN;
                continue;
            }
            float x = layout.getPrimaryHorizontal(at);
            // Right to left, the placeholder reaches left of its offset.
            if (layout.isRtlCharAt(at)) x -= p.width;
            out[i * 2] = x;
            out[i * 2 + 1] = layout.getLineBaseline(line) - p.ascent;
        }
        return out;
    }

    /**
     * An attachment's placeholder: as wide as the attachment, its top
     * ascent above the baseline and its line reserving its extent, as
     * core.TextAttachment.Ascent and Extent say against the font around it.
     * Draws nothing: the attachment's view is a child of the text's.
     */
    static final class Placeholder extends ReplacementSpan {
        final int width, height, align;
        /** How far its top is above the baseline in pixels, below it when negative; set by getSize. */
        int ascent;

        Placeholder(int width, int height, int align) {
            this.width = width;
            this.height = height;
            this.align = align;
            this.ascent = height;
        }

        @Override
        public int getSize(Paint paint, CharSequence text, int start, int end, Paint.FontMetricsInt fm) {
            Paint.FontMetrics font = paint.getFontMetrics();
            switch (align) {
                case Protocol.VALIGN_TEXT_TOP:
                    ascent = Math.round(-font.ascent);
                    break;
                case Protocol.VALIGN_TEXT_BOTTOM:
                    ascent = Math.round(height - font.descent);
                    break;
                case Protocol.VALIGN_MIDDLE: {
                    Rect x = new Rect();
                    paint.getTextBounds("x", 0, 1, x);
                    ascent = Math.round((height - x.top) / 2f); // x.top is the x height, negative
                    break;
                }
                default:
                    ascent = height;
            }
            if (fm != null) {
                // The line grows where the attachment reaches past the font.
                Paint.FontMetricsInt own = paint.getFontMetricsInt();
                fm.ascent = Math.min(own.ascent, -Math.max(0, ascent));
                fm.top = Math.min(own.top, fm.ascent);
                fm.descent = Math.max(own.descent, height - ascent);
                fm.bottom = Math.max(own.bottom, fm.descent);
                fm.leading = own.leading;
            }
            return width;
        }

        @Override
        public void draw(android.graphics.Canvas canvas, CharSequence text, int start, int end, float x, int top,
                int y, int bottom, Paint paint) {}
    }

    /**
     * A span's background, which TextNode draws behind each line's part of
     * its characters, from its font's ascent to its descent (pixels).
     */
    static final class Background {
        final int color;
        final float radius, ascent, descent;

        Background(int color, float radius, float ascent, float descent) {
            this.color = color;
            this.radius = radius;
            this.ascent = ascent;
            this.descent = descent;
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
