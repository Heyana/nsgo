package dev.oneo.nsgo;

import android.content.res.AssetManager;
import android.graphics.Paint;
import android.graphics.Rect;
import android.graphics.Typeface;
import android.graphics.text.LineBreakConfig;
import android.graphics.fonts.Font;
import android.graphics.fonts.FontFamily;
import android.graphics.fonts.FontStyle;
import android.graphics.fonts.SystemFonts;
import android.os.Build;
import android.text.Layout;
import android.text.SpannableString;
import android.text.SpannableStringBuilder;
import android.text.Spanned;
import android.text.StaticLayout;
import android.text.TextDirectionHeuristic;
import android.text.TextDirectionHeuristics;
import android.text.TextPaint;
import android.text.TextUtils;
import android.text.style.LineHeightSpan;
import android.text.style.MetricAffectingSpan;
import android.text.style.ReplacementSpan;
import android.view.Gravity;
import java.io.DataInputStream;
import java.io.File;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;

/**
 * Text the engine prepares, measures and lays out on the reader thread, all
 * with StaticLayout; the UI thread only draws the final layouts. Lengths
 * here are pixels.
 */
final class Text {
    /** A face of the app's fonts. */
    private static final class Face {
        final int weight;
        final boolean italic;
        final Typeface typeface;
        /** An android.graphics.fonts.Font on API 29+, or null on API 28. */
        final Object font;

        Face(int weight, boolean italic, Typeface typeface, Object font) {
            this.weight = weight;
            this.italic = italic;
            this.typeface = typeface;
            this.font = font;
        }
    }

    /** The app's fonts by lowercase family name: those it starts with, then those it registers as it runs. */
    private static final Map<String, List<Face>> fonts = new ConcurrentHashMap<>();
    /** Cached FontFamily objects built for registered app families on API 29+. */
    private static final Map<String, Object> appFamilies = new ConcurrentHashMap<>();
    /** Bounded cache of resolved Typefaces keyed by family list, weight and slant. */
    private static final Map<String, Typeface> typefaceCache = new ConcurrentHashMap<>();
    private static final int MAX_TYPEFACE_CACHE = 256;

    /** Adds a face of the app's fonts under its family. */
    static void register(String family, int weight, boolean italic, Typeface typeface) {
        register(family, weight, italic, typeface, null);
    }

    /** Adds a face of the app's fonts under its family, with an optional API 29+ Font for fallback chains. */
    static void register(String family, int weight, boolean italic, Typeface typeface, Object font) {
        if (family == null || family.isEmpty()) return;
        String key = family.toLowerCase(Locale.ROOT);
        fonts.computeIfAbsent(key, k -> new CopyOnWriteArrayList<>())
                .add(new Face(weight, italic, typeface, font));
        appFamilies.remove(key);
        typefaceCache.clear();
    }

    /** Builds an API 29+ Font from an asset for fallback chains, or null on API 28 or failure. */
    static Object assetFont(AssetManager assets, String path, int weight, boolean italic) {
        return Build.VERSION.SDK_INT >= 29 ? Api29.fontFromAsset(assets, path, weight, italic) : null;
    }

    /** Builds an API 29+ Font from a file for fallback chains, or null on API 28 or failure. */
    static Object fileFont(File file, int weight, boolean italic) {
        return Build.VERSION.SDK_INT >= 29 ? Api29.fontFromFile(file, weight, italic) : null;
    }

    /**
     * Resolves a CSS fontFamily list at weight and slant. On API 29+, builds a Typeface.CustomFallbackBuilder chain so glyphs missing from earlier families fall back through the remaining families in order; on API 28, selects the first available family in the list.
     */
    static Typeface typeface(String familyList, int weight, boolean italic) {
        weight = Math.max(1, Math.min(weight, 1000));
        String raw = familyList == null ? "" : familyList;
        String cacheKey = raw + '\0' + weight + '\0' + (italic ? '1' : '0');
        Typeface cached = typefaceCache.get(cacheKey);
        if (cached != null) return cached;

        List<FontFallback.Entry> entries = FontFallback.parse(raw);
        Typeface resolved;
        if (entries.isEmpty()) {
            resolved = Typeface.create(Typeface.DEFAULT, weight, italic);
        } else if (entries.size() == 1 && entries.get(0).generic) {
            resolved = Typeface.create(genericBase(entries.get(0).name), weight, italic);
        } else if (Build.VERSION.SDK_INT >= 29) {
            resolved = Api29.resolve(entries, weight, italic);
        } else {
            resolved = resolveLegacy(entries, weight, italic);
        }
        if (typefaceCache.size() >= MAX_TYPEFACE_CACHE) typefaceCache.clear();
        typefaceCache.put(cacheKey, resolved);
        return resolved;
    }

    private static Typeface genericBase(String name) {
        if (name == null || name.isEmpty() || FontFallback.SANS_SERIF.equals(name)) return Typeface.DEFAULT;
        if (FontFallback.SERIF.equals(name)) return Typeface.SERIF;
        if (FontFallback.MONOSPACE.equals(name)) return Typeface.MONOSPACE;
        return Typeface.create(name, Typeface.NORMAL);
    }

    private static Typeface legacyAppFace(String name, int weight, boolean italic) {
        List<Face> faces = fonts.get(name.toLowerCase(Locale.ROOT));
        if (faces == null || faces.isEmpty()) return null;
        Face best = null;
        for (Face f : faces) {
            if (best == null) {
                best = f;
                continue;
            }
            boolean slant = f.italic == italic, bestSlant = best.italic == italic;
            if (slant != bestSlant) {
                if (slant) best = f;
            } else if (Math.abs(f.weight - weight) < Math.abs(best.weight - weight)) {
                best = f;
            }
        }
        if (best.weight == weight && best.italic == italic) return best.typeface;
        return Typeface.create(best.typeface, weight, italic);
    }

    private static boolean isSystemAlias(String name) {
        if (name == null || name.isEmpty()) return false;
        Typeface tf = Typeface.create(name, Typeface.NORMAL);
        return tf != null && !tf.equals(Typeface.DEFAULT);
    }

    /** API 28 fallback: picks the first registered, generic or installed family in preference order. */
    private static Typeface resolveLegacy(List<FontFallback.Entry> entries, int weight, boolean italic) {
        for (FontFallback.Entry e : entries) {
            if (e.generic) {
                return Typeface.create(genericBase(e.name), weight, italic);
            }
            Typeface app = legacyAppFace(e.name, weight, italic);
            if (app != null) return app;
            if (isSystemAlias(e.name)) {
                return Typeface.create(Typeface.create(e.name, Typeface.NORMAL), weight, italic);
            }
        }
        return Typeface.create(Typeface.DEFAULT, weight, italic);
    }

    /** Isolated API 29+ CustomFallbackBuilder and SystemFonts integration so API 28 never verifies these classes. */
    private static final class Api29 {
        private static volatile Map<String, FontFamily> systemIndex;

        static Object fontFromAsset(AssetManager assets, String path, int weight, boolean italic) {
            try {
                return new Font.Builder(assets, path)
                        .setWeight(clampWeight(weight))
                        .setSlant(italic ? FontStyle.FONT_SLANT_ITALIC : FontStyle.FONT_SLANT_UPRIGHT)
                        .build();
            } catch (IOException | RuntimeException e) {
                return null;
            }
        }

        static Object fontFromFile(File file, int weight, boolean italic) {
            try {
                return new Font.Builder(file)
                        .setWeight(clampWeight(weight))
                        .setSlant(italic ? FontStyle.FONT_SLANT_ITALIC : FontStyle.FONT_SLANT_UPRIGHT)
                        .build();
            } catch (IOException | RuntimeException e) {
                return null;
            }
        }

        private static int clampWeight(int weight) {
            return Math.max(FontStyle.FONT_WEIGHT_MIN, Math.min(weight, FontStyle.FONT_WEIGHT_MAX));
        }

        static Typeface resolve(List<FontFallback.Entry> entries, int weight, boolean italic) {
            FontFallback.Lookup<FontFamily> lookup = new FontFallback.Lookup<FontFamily>() {
                @Override
                public FontFamily appFamily(String name) {
                    return Api29.appFamily(name);
                }

                @Override
                public FontFamily systemFamily(String name) {
                    return systemFamilies().get(name.toLowerCase(Locale.ROOT));
                }

                @Override
                public FontFamily genericFamily(String genericName) {
                    Map<String, FontFamily> sys = systemFamilies();
                    for (String candidate : FontFallback.genericCandidates(genericName)) {
                        FontFamily fam = sys.get(candidate.toLowerCase(Locale.ROOT));
                        if (fam != null) return fam;
                    }
                    return null;
                }

                @Override
                public boolean isSystemAlias(String name) {
                    return Text.isSystemAlias(name);
                }
            };
            int maxCustom = Typeface.CustomFallbackBuilder.getMaxCustomFallbackCount();
            FontFallback.Plan<FontFamily> plan = FontFallback.plan(entries, maxCustom, lookup);
            if (plan.custom.isEmpty()) {
                // Handle registered app fonts that lacked an API 29 Font object, or single generic/alias families.
                for (FontFallback.Entry e : entries) {
                    if (e.generic) break;
                    Typeface app = legacyAppFace(e.name, weight, italic);
                    if (app != null) return app;
                }
                return Typeface.create(genericBase(plan.systemFallback), weight, italic);
            }
            Typeface.CustomFallbackBuilder builder = new Typeface.CustomFallbackBuilder(plan.custom.get(0));
            for (int i = 1; i < plan.custom.size(); i++) {
                builder.addCustomFallback(plan.custom.get(i));
            }
            int slant = italic ? FontStyle.FONT_SLANT_ITALIC : FontStyle.FONT_SLANT_UPRIGHT;
            builder.setStyle(new FontStyle(clampWeight(weight), slant));
            builder.setSystemFallback(plan.systemFallback);
            return builder.build();
        }

        private static FontFamily appFamily(String name) {
            String key = name.toLowerCase(Locale.ROOT);
            Object cached = appFamilies.get(key);
            if (cached instanceof FontFamily) return (FontFamily) cached;
            List<Face> faces = fonts.get(key);
            if (faces == null || faces.isEmpty()) return null;
            Map<Long, Font> unique = new LinkedHashMap<>();
            for (Face f : faces) {
                if (!(f.font instanceof Font)) continue;
                Font font = (Font) f.font;
                FontStyle st = font.getStyle();
                long styleKey = (((long) st.getWeight()) << 32) | (st.getSlant() & 0xffffffffL);
                unique.put(styleKey, font);
            }
            if (unique.isEmpty()) return null;
            FontFamily built = buildFamily(new ArrayList<>(unique.values()));
            if (built != null) appFamilies.put(key, built);
            return built;
        }

        private static Map<String, FontFamily> systemFamilies() {
            Map<String, FontFamily> existing = systemIndex;
            if (existing != null) return existing;
            synchronized (Api29.class) {
                if (systemIndex != null) return systemIndex;
                Map<String, Map<Long, Font>> grouped = new HashMap<>();
                Set<Font> available = SystemFonts.getAvailableFonts();
                if (available != null) {
                    for (Font font : available) {
                        List<String> names = Collections.emptyList();
                        if (Build.VERSION.SDK_INT >= 31) {
                            try {
                                names = FontFallback.readFamilyNames(Api31.buffer(font), font.getTtcIndex());
                            } catch (RuntimeException ignored) {
                                names = Collections.emptyList();
                            }
                        }
                        if (names.isEmpty() && font.getFile() != null) {
                            names = FontFallback.readFamilyNames(font.getFile(), font.getTtcIndex());
                        }
                        if (names.isEmpty()) continue;
                        FontStyle st = font.getStyle();
                        long styleKey = (((long) st.getWeight()) << 32) | (st.getSlant() & 0xffffffffL);
                        for (String name : names) {
                            String key = name.toLowerCase(Locale.ROOT);
                            Map<Long, Font> byStyle = grouped.computeIfAbsent(key, k -> new LinkedHashMap<>());
                            byStyle.putIfAbsent(styleKey, font);
                        }
                    }
                }
                Map<String, FontFamily> built = new HashMap<>(grouped.size());
                for (Map.Entry<String, Map<Long, Font>> entry : grouped.entrySet()) {
                    FontFamily fam = buildFamily(new ArrayList<>(entry.getValue().values()));
                    if (fam != null) built.put(entry.getKey(), fam);
                }
                systemIndex = built;
                return built;
            }
        }

        private static FontFamily buildFamily(List<Font> list) {
            if (list.isEmpty()) return null;
            FontFamily.Builder builder = null;
            for (Font f : list) {
                try {
                    if (builder == null) builder = new FontFamily.Builder(f);
                    else builder.addFont(f);
                } catch (IllegalArgumentException ignored) {
                    // Duplicate style inside a family: skip.
                }
            }
            return builder == null ? null : builder.build();
        }
    }

    /** Isolated API 31 Font.getBuffer call so API 29-30 never soft-verify it. */
    private static final class Api31 {
        static ByteBuffer buffer(Font font) {
            return font.getBuffer();
        }
    }

    private Text() {}

    /** The OpenType tags of core.FontVariant's bits, from its second. */
    private static final String[] FEATURES = {"smcp", "onum", "lnum", "tnum", "pnum"};

    /** The font feature settings of core.FontVariant's bits, as Paint takes them, or null for none. */
    static String features(int variant) {
        StringBuilder b = null;
        for (int i = 0; i < FEATURES.length; i++) {
            if ((variant & (2 << i)) == 0) continue;
            if (b == null) b = new StringBuilder();
            else b.append(", ");
            b.append('\'').append(FEATURES[i]).append('\'');
        }
        return b == null ? null : b.toString();
    }

    /** Reports whether language is Korean (primary subtag "ko"), which breaks lines between words. */
    static boolean isKorean(String language) {
        if (language == null || language.length() < 2) return false;
        char a = language.charAt(0), b = language.charAt(1);
        if ((a != 'k' && a != 'K') || (b != 'o' && b != 'O')) return false;
        return language.length() == 2 || language.charAt(2) == '-' || language.charAt(2) == '_';
    }

    /** Appends the BCP 47 Unicode lw-keepall extension for Korean tags without an explicit lw keyword. */
    static String keepAllLanguage(String language) {
        if (!isKorean(language)) return language;
        String tag = language.replace('_', '-');
        String[] parts = tag.split("-");
        int uAt = -1, xAt = -1;
        for (int i = 1; i < parts.length; i++) {
            if (parts[i].length() != 1) continue;
            char c = parts[i].charAt(0);
            if ((c == 'u' || c == 'U') && uAt < 0) uAt = i;
            if (c == 'x' || c == 'X') {
                xAt = i;
                break;
            }
        }
        if (uAt >= 0) {
            int end = parts.length;
            for (int i = uAt + 1; i < parts.length; i++) {
                if (parts[i].length() == 1) {
                    end = i;
                    break;
                }
                if (parts[i].equalsIgnoreCase("lw")) return tag;
            }
            return insertSubtags(parts, end, "lw", "keepall");
        }
        if (xAt >= 0) return insertSubtags(parts, xAt, "u", "lw", "keepall");
        return tag + "-u-lw-keepall";
    }

    private static String insertSubtags(String[] parts, int at, String... extra) {
        StringBuilder b = new StringBuilder();
        for (int i = 0; i < at; i++) {
            if (i > 0) b.append('-');
            b.append(parts[i]);
        }
        for (String s : extra) b.append('-').append(s);
        for (int i = at; i < parts.length; i++) b.append('-').append(parts[i]);
        return b.toString();
    }

    /** A resolved text style, as android.go's writeStyle writes it. */
    static final class Style {
        final String family;
        /** The text's language, which picks Han glyph forms, fallback fonts and line breaks, or null for the system's. */
        final Locale locale;
        /** Whether the language breaks lines between words (Korean keep-all). */
        final boolean keepAll;
        final float size;
        final int weight;
        final int color;
        final float lineHeight;
        final float letterSpacing;
        final int align;
        final boolean italic, underline, lineThrough;
        /** The paragraphs' base direction is right to left. */
        final boolean rtl;
        /** The OpenType features of core.FontVariant's bits, as Paint takes them, or null. */
        final String features;
        /** How the decoration's lines are drawn and their ARGB color, 0 for the text's; read after the rest. */
        int decorationStyle, decorationColor;
        /** The lines' thickness in pixels, 0 for the font's, and an underline's top below the baseline, NaN for the font's. */
        float thickness, underlineOffset = Float.NaN;

        Style(DataInputStream in, float density) throws IOException {
            family = Protocol.readString(in);
            String language = Protocol.readString(in);
            keepAll = isKorean(language);
            locale = language.isEmpty() ? null : Locale.forLanguageTag(keepAllLanguage(language));
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
            rtl = Protocol.flag(flags, 3);
            features = features(in.readUnsignedByte());
        }

        /** Reads how the decoration is drawn, which PREPARE sends after the text's parts. */
        void readDecoration(DataInputStream in, float density) throws IOException {
            decorationStyle = in.readUnsignedByte();
            decorationColor = argb(in.readInt());
            thickness = in.readFloat() * density;
            underlineOffset = in.readFloat() * density;
        }

        /**
         * Whether TextNode draws the decoration, which a paint draws only
         * solid, in the text's color and where the font puts it.
         */
        boolean drawsDecoration() {
            return (underline || lineThrough)
                    && (decorationStyle > Protocol.DECORATION_SOLID || (decorationColor >>> 24) != 0 || thickness > 0
                            || underline && !Float.isNaN(underlineOffset));
        }

        Typeface typeface() {
            return Text.typeface(family, weight, italic);
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

        void applyMetrics(TextPaint paint, Typeface typeface) {
            paint.setTextSize(size);
            paint.setTypeface(typeface);
            paint.setLetterSpacing(letterSpacingEm());
            paint.setFontFeatureSettings(features);
            if (locale != null) paint.setTextLocale(locale);
        }

        TextPaint paint() {
            TextPaint paint = new TextPaint(Paint.ANTI_ALIAS_FLAG);
            applyMetrics(paint, typeface());
            paint.setColor(color);
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
            if (this.thickness > 0) thickness = this.thickness;
            thickness = Math.max(thickness, 1);
            // The app's offset is the line's top; Decoration's is its middle.
            if (!Float.isNaN(underlineOffset)) under = underlineOffset + thickness / 2;
            int color = (decorationColor >>> 24) != 0 ? decorationColor : this.color;
            return new Decoration(underline, lineThrough, decorationStyle, color, under, strike, thickness);
        }

        /** The horizontal gravity of a text field. */
        int gravity() {
            switch (align) {
                case Protocol.ALIGN_CENTER: return Gravity.CENTER_HORIZONTAL;
                case Protocol.ALIGN_END: return rtl ? Gravity.LEFT : Gravity.RIGHT;
                default: return rtl ? Gravity.RIGHT : Gravity.LEFT;
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
        final BackgroundInset top, bottom;
        /** Whether the span is hidden in place, which TEXT_PREPARE sends after the decorations, one byte per span; false from a Go side that predates it. */
        boolean hidden;

        Span(DataInputStream in, float density) throws IOException {
            start = in.readInt();
            end = in.readInt();
            style = new Style(in, density);
            background = argb(in.readInt());
            radius = in.readFloat() * density;
            top = new BackgroundInset(in, density);
            bottom = new BackgroundInset(in, density);
        }
    }

    private static final class BackgroundInset {
        final boolean relative;
        final float value;

        BackgroundInset(DataInputStream in, float density) throws IOException {
            relative = in.readUnsignedByte() != 0;
            float amount = in.readFloat();
            value = relative ? amount : amount * density;
        }

        float resolve(float height) {
            if (!(value > 0)) return 0;
            return Math.min(relative ? value * height : value, height);
        }
    }

    /** A shadow behind the glyphs, as android.go writes it: its ARGB color, 0 for none, offset and radius in pixels. */
    static final class Shadow {
        final int color;
        final float dx, dy, radius;

        Shadow(int color, float dx, float dy, float radius) {
            this.color = color;
            this.dx = dx;
            this.dy = dy;
            this.radius = radius;
        }

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

    /** An attachment: the byte offset of its placeholder, its size, alignment and descent, as android.go writes it. */
    static final class Attachment {
        final int offset;
        final int width, height, align, descent;

        Attachment(DataInputStream in, float density) throws IOException {
            offset = in.readInt();
            width = Math.round(in.readFloat() * density);
            height = Math.round(in.readFloat() * density);
            align = in.readUnsignedByte();
            descent = Math.round(in.readFloat() * density);
        }
    }

    /** Text prepared once, then measured at any width and laid out at its final size. */
    static final class Prepared {
        private final CharSequence text;
        private final TextPaint paint;
        private final Layout.Alignment align;
        private final int maxLines, ellipsize;
        private final boolean attached, justify, keepAll;
        private final TextDirectionHeuristic direction;
        private final Shadow shadow;

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
                    if (span.hidden) {
                        spanned.setSpan(new Hidden(span.style), start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
                        continue;
                    }
                    spanned.setSpan(new Run(span.style), start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
                    if (span.style.drawsDecoration()) {
                        spanned.setSpan(span.style.decoration(measure), start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
                    }
                    if ((span.background >>> 24) != 0) {
                        measure.setTypeface(span.style.typeface());
                        measure.setTextSize(span.style.size);
                        measure.getFontMetrics(font);
                        float height = font.descent - font.ascent;
                        float top = font.ascent + span.top.resolve(height);
                        float bottom = font.descent - span.bottom.resolve(height);
                        if (bottom > top) {
                            spanned.setSpan(new Background(span.background, span.radius, top, bottom),
                                    start, end, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
                        }
                    }
                }
                // Attachments come in ascending offsets.
                for (Attachment a : attachments) {
                    int at = offsets.index(a.offset);
                    if (at >= text.length()) continue;
                    spanned.setSpan(new Placeholder(a.width, a.height, a.align, a.descent), at, at + 1,
                            Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
                }
                this.text = spanned;
            } else {
                this.text = text;
            }
            this.paint = style.paint();
            if (shadow != null && (shadow.color >>> 24) != 0) {
                // A zero radius would remove the shadow layer.
                paint.setShadowLayer(Math.max(0.01f, shadow.radius), shadow.dx, shadow.dy, shadow.color);
                this.shadow = shadow;
            } else {
                this.shadow = null;
            }
            this.align = style.alignment();
            this.justify = style.align == Protocol.ALIGN_JUSTIFY;
            this.keepAll = style.keepAll;
            this.direction = style.rtl ? TextDirectionHeuristics.RTL : TextDirectionHeuristics.LTR;
            this.maxLines = maxLines;
            this.ellipsize = ellipsize;
            this.attached = !attachments.isEmpty();
        }

        /** The shadow behind the glyphs and drawn decorations, or null for none. */
        Shadow shadow() {
            return shadow;
        }

        /** The lines a clipped text shows, or 0 when an ellipsis ends them. */
        int clipLines() {
            return ellipsize == Protocol.ELLIPSIZE_CLIP ? maxLines : 0;
        }

        /** Whether the text has attachments, so its layouts answer where they went. */
        boolean hasAttachments() {
            return attached;
        }

        private int cachedWidthPx = -1;
        private StaticLayout cachedLayout;

        /** The size of the text wrapped at maxWidth, or unwrapped when it is NaN. */
        float[] measure(float maxWidth) {
            int px = widthPx(maxWidth);
            StaticLayout layout = cachedLayout != null && cachedWidthPx == px
                    ? cachedLayout : buildPx(px, new TextPaint(paint));
            cachedWidthPx = px;
            cachedLayout = layout;
            int lines = layout.getLineCount();
            if (maxLines > 0) lines = Math.min(lines, maxLines);
            float width = 0;
            for (int i = 0; i < lines; i++) {
                width = Math.max(width, layout.getLineWidth(i));
            }
            if (text.length() == 0) width = 0;
            return new float[] {(float) Math.ceil(width), layout.getLineTop(lines)};
        }

        /** The final layout at width, reusing measure's StaticLayout when the pixel width matches. */
        StaticLayout layout(float width) {
            int px = widthPx(width);
            if (cachedLayout != null && cachedWidthPx == px) {
                StaticLayout reused = cachedLayout;
                cachedLayout = null;
                cachedWidthPx = -1;
                return reused;
            }
            return buildPx(px, new TextPaint(paint));
        }

        private int widthPx(float width) {
            // The width came back from points, so allow for rounding below
            // a measured width before wrapping at it.
            int px = Float.isNaN(width)
                    ? (int) Math.ceil(Layout.getDesiredWidth(text, paint))
                    : (int) Math.ceil(width - 0.01f);
            return Math.max(px, 1);
        }

        private StaticLayout buildPx(int px, TextPaint paint) {
            StaticLayout.Builder b = StaticLayout.Builder.obtain(text, 0, text.length(), paint, px)
                    .setAlignment(align)
                    .setTextDirection(direction)
                    .setIncludePad(false);
            if (justify) b.setJustificationMode(Layout.JUSTIFICATION_MODE_INTER_WORD);
            if (keepAll && Build.VERSION.SDK_INT >= 33) {
                b.setLineBreakConfig(new LineBreakConfig.Builder()
                        .setLineBreakWordStyle(LineBreakConfig.LINE_BREAK_WORD_STYLE_PHRASE)
                        .build());
            }
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
            style.applyMetrics(paint, typeface);
        }

        @Override
        public void updateDrawState(TextPaint paint) {
            updateMeasureState(paint);
            paint.setColor(style.color);
            paint.setFlags(style.paintFlags(paint.getFlags()));
        }
    }

    /**
     * Marks a hidden span in the laid-out text: measured in its own font metrics so wrapping, alignment and hit testing match visible text, skipped as TextNode draws (drawLines), and left out of copies.
     */
    static final class Hidden extends MetricAffectingSpan {
        final Style style;
        final Typeface typeface;

        Hidden(Style style) {
            this.style = style;
            this.typeface = style.typeface();
        }

        @Override
        public void updateMeasureState(TextPaint paint) {
            style.applyMetrics(paint, typeface);
        }

        @Override
        public void updateDrawState(TextPaint paint) {
            updateMeasureState(paint);
        }
    }

    /**
     * A ReplacementSpan that reserves a hidden span's advance and font metrics on one laid-out line and draws nothing — skipping glyphs, color emoji, decoration lines and shadows alike.
     */
    static final class BlankSpan extends ReplacementSpan {
        private final Style style;
        private final Typeface typeface;

        BlankSpan(Style style, Typeface typeface) {
            this.style = style;
            this.typeface = typeface;
        }

        @Override
        public int getSize(Paint paint, CharSequence text, int start, int end, Paint.FontMetricsInt fm) {
            TextPaint work = new TextPaint(paint);
            style.applyMetrics(work, typeface);
            if (fm != null) work.getFontMetricsInt(fm);
            return Math.round(work.measureText(text, start, end));
        }

        @Override
        public void draw(android.graphics.Canvas canvas, CharSequence text, int start, int end, float x, int top,
                int y, int bottom, Paint paint) {}
    }

    /** One line of a layout containing hidden spans, drawn at (dx, dy) with hidden runs skipped. */
    static final class DrawnLine {
        final StaticLayout layout;
        final float dx, dy;

        DrawnLine(StaticLayout layout, float dx, float dy) {
            this.layout = layout;
            this.dx = dx;
            this.dy = dy;
        }
    }

    /** Reports whether layout contains any hidden spans. */
    static boolean hasHidden(Layout layout) {
        if (layout == null) return false;
        CharSequence text = layout.getText();
        return text instanceof Spanned && ((Spanned) text).getSpans(0, text.length(), Hidden.class).length > 0;
    }

    /**
     * Builds per-line draw layouts for a StaticLayout containing hidden spans, replacing each hidden slice on each line with a BlankSpan so glyphs, color emoji, decorations and shadows are skipped while keeping exact line breaks, alignment and advances. Returns null when layout has no hidden spans.
     */
    static DrawnLine[] drawLines(StaticLayout layout, int clipLines) {
        if (!hasHidden(layout)) return null;
        CharSequence text = layout.getText();
        Spanned spanned = (Spanned) text;
        int lines = layout.getLineCount();
        if (clipLines > 0) lines = Math.min(lines, clipLines);
        List<DrawnLine> out = new ArrayList<>(lines);
        TextPaint paint = layout.getPaint();
        for (int i = 0; i < lines; i++) {
            int lineStart = layout.getLineStart(i), lineEnd = layout.getLineEnd(i);
            while (lineEnd > lineStart) {
                char c = text.charAt(lineEnd - 1);
                if (c != '\n' && c != '\r') break;
                lineEnd--;
            }
            if (lineEnd <= lineStart) continue;
            int ellCount = layout.getEllipsisCount(i);
            int ellStart = ellCount > 0 ? lineStart + layout.getEllipsisStart(i) : -1;
            int ellEnd = ellCount > 0 ? Math.min(lineEnd, ellStart + ellCount) : -1;
            SpannableStringBuilder lineText = new SpannableStringBuilder();
            boolean anyVisible = false;
            if (ellCount > 0 && ellStart >= lineStart && ellStart <= lineEnd) {
                if (appendLineSlice(lineText, text, spanned, lineStart, ellStart)) anyVisible = true;
                lineText.append('\u2026');
                anyVisible = true;
                if (appendLineSlice(lineText, text, spanned, ellEnd, lineEnd)) anyVisible = true;
            } else {
                anyVisible = appendLineSlice(lineText, text, spanned, lineStart, lineEnd);
            }
            if (!anyVisible || lineText.length() == 0) continue;
            int width = Math.max(layout.getWidth(), (int) Math.ceil(Layout.getDesiredWidth(lineText, paint)) + 16);
            TextDirectionHeuristic dir = layout.getParagraphDirection(i) < 0
                    ? TextDirectionHeuristics.RTL : TextDirectionHeuristics.LTR;
            StaticLayout lineLayout = StaticLayout.Builder.obtain(lineText, 0, lineText.length(), paint, Math.max(width, 1))
                    .setAlignment(Layout.Alignment.ALIGN_NORMAL)
                    .setTextDirection(dir)
                    .setIncludePad(false)
                    .setMaxLines(1)
                    .build();
            float dx = layout.getLineLeft(i) - lineLayout.getLineLeft(0);
            float dy = layout.getLineBaseline(i) - lineLayout.getLineBaseline(0);
            out.add(new DrawnLine(lineLayout, dx, dy));
        }
        return out.toArray(new DrawnLine[0]);
    }

    /**
     * Appends text[from..to) into dst, copying Run and Placeholder spans and replacing Hidden spans (outside Placeholders) with BlankSpans. Returns true if any character in the slice is visible.
     */
    private static boolean appendLineSlice(SpannableStringBuilder dst, CharSequence text, Spanned spanned,
            int from, int to) {
        if (from >= to) return false;
        int base = dst.length();
        dst.append(text, from, to);
        for (Run run : spanned.getSpans(from, to, Run.class)) {
            int s = Math.max(from, spanned.getSpanStart(run)), e = Math.min(to, spanned.getSpanEnd(run));
            if (e > s) dst.setSpan(run, base + s - from, base + e - from, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
        }
        Placeholder[] placeholders = spanned.getSpans(from, to, Placeholder.class);
        Arrays.sort(placeholders, (a, b) -> spanned.getSpanStart(a) - spanned.getSpanStart(b));
        for (Placeholder p : placeholders) {
            int s = Math.max(from, spanned.getSpanStart(p)), e = Math.min(to, spanned.getSpanEnd(p));
            if (e > s) dst.setSpan(p, base + s - from, base + e - from, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
        }
        Hidden[] hidden = spanned.getSpans(from, to, Hidden.class);
        Arrays.sort(hidden, (a, b) -> spanned.getSpanStart(a) - spanned.getSpanStart(b));
        int covered = 0;
        int prevEnd = from;
        for (Hidden h : hidden) {
            int hs = Math.max(from, spanned.getSpanStart(h)), he = Math.min(to, spanned.getSpanEnd(h));
            if (he <= hs) continue;
            int at = hs;
            for (Placeholder p : placeholders) {
                int ps = Math.max(from, spanned.getSpanStart(p)), pe = Math.min(to, spanned.getSpanEnd(p));
                if (pe <= at || ps >= he) continue;
                if (ps > at) {
                    dst.setSpan(new BlankSpan(h.style, h.typeface), base + at - from, base + ps - from,
                            Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
                }
                at = Math.max(at, pe);
            }
            if (at < he) {
                dst.setSpan(new BlankSpan(h.style, h.typeface), base + at - from, base + he - from,
                        Spanned.SPAN_EXCLUSIVE_EXCLUSIVE);
            }
            if (hs > prevEnd) prevEnd = hs;
            if (he > prevEnd) {
                covered += he - prevEnd;
                prevEnd = he;
            }
        }
        return covered < to - from;
    }

    /**
     * Where a final layout's characters are, as internal/platform/android
     * readCarets reads it: per line its UTF-16 range, box (left, top, width,
     * height) and direction, then its grapheme clusters' ranges, left and
     * right edges and directions, in logical order, without its break or
     * what an ellipsis replaced. Lengths in units of {@code density}.
     */
    static byte[] carets(Layout l, float density) {
        java.io.ByteArrayOutputStream bytes = new java.io.ByteArrayOutputStream();
        java.io.DataOutputStream d = new java.io.DataOutputStream(bytes);
        try {
            int count = l == null ? 0 : l.getLineCount();
            d.writeInt(count);
            if (count == 0) return bytes.toByteArray();
            CharSequence text = l.getText();
            java.text.BreakIterator chars = java.text.BreakIterator.getCharacterInstance();
            chars.setText(text.toString());
            for (int line = 0; line < count; line++) {
                int start = l.getLineStart(line), end = l.getLineEnd(line);
                float top = l.getLineTop(line), bottom = l.getLineBottom(line);
                d.writeInt(start);
                d.writeInt(end);
                d.writeFloat(l.getLineLeft(line) / density);
                d.writeFloat(top / density);
                d.writeFloat(l.getLineWidth(line) / density);
                d.writeFloat((bottom - top) / density);
                d.writeByte(l.getParagraphDirection(line) == Layout.DIR_RIGHT_TO_LEFT ? 1 : 0);
                int stop = TextNode.breakStart(l, line);
                int ellipsis = l.getEllipsisCount(line) > 0 ? start + l.getEllipsisStart(line) : -1;
                int resume = ellipsis >= 0 ? ellipsis + l.getEllipsisCount(line) : -1;
                java.util.List<float[]> clusters = new java.util.ArrayList<>();
                for (int i = start; i < stop; ) {
                    int j = chars.following(i);
                    if (j == java.text.BreakIterator.DONE || j > stop) j = stop;
                    if (ellipsis < 0 || i < ellipsis || i >= resume) {
                        boolean rtl = l.isRtlCharAt(i);
                        float[] before = edges(l, line, i), after = edges(l, line, j);
                        float[] box = narrowest(rtl ? after : before, rtl ? before : after);
                        if (box == null) box = narrowest(concat(before, after), concat(before, after));
                        if (box != null) clusters.add(new float[] {i, j, box[0], box[1], rtl ? 1 : 0});
                    }
                    i = j;
                }
                d.writeInt(clusters.size());
                for (float[] c : clusters) {
                    d.writeInt((int) c[0]);
                    d.writeInt((int) c[1]);
                    d.writeFloat(c[2] / density);
                    d.writeFloat(c[3] / density);
                    d.writeByte((int) c[4]);
                }
            }
        } catch (java.io.IOException impossible) {
            throw new AssertionError(impossible);
        }
        return bytes.toByteArray();
    }

    /**
     * The x of the end of a line that wrapped: the far edge of the line in
     * its paragraph's direction, as far from where it starts as it is wide.
     */
    private static float wrappedEndX(Layout l, int line) {
        float width = l.getLineWidth(line);
        float start = l.getPrimaryHorizontal(l.getLineStart(line));
        return l.getParagraphDirection(line) == Layout.DIR_RIGHT_TO_LEFT ? start - width : start + width;
    }

    /**
     * The carets of offset on a line: Android's primary and secondary ones,
     * which differ where the direction changes. A wrapped line's end is one
     * of its own: getOffsetForHorizontal and the carets stop before it.
     */
    private static float[] edges(Layout l, int line, int offset) {
        if (offset == l.getLineEnd(line) && line < l.getLineCount() - 1) return new float[] {wrappedEndX(l, line)};
        float primary = l.getPrimaryHorizontal(offset), secondary = l.getSecondaryHorizontal(offset);
        return primary == secondary ? new float[] {primary} : new float[] {primary, secondary};
    }

    /** The narrowest box from one of lefts to one of rights, or null when none has width. */
    private static float[] narrowest(float[] lefts, float[] rights) {
        float[] best = null;
        for (float lo : lefts) {
            for (float hi : rights) {
                if (hi > lo && (best == null || hi - lo < best[1] - best[0])) best = new float[] {lo, hi};
            }
        }
        return best;
    }

    private static float[] concat(float[] a, float[] b) {
        float[] out = new float[a.length + b.length];
        System.arraycopy(a, 0, out, 0, a.length);
        System.arraycopy(b, 0, out, a.length, b.length);
        return out;
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
            out[i] = new float[] {TextNode.utf8Length(text, start), TextNode.utf8Length(text, end), layout.getLineLeft(i), top,
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
        /** descent is how far below the baseline it reaches aligned to it, as a formula's baseline puts it. */
        final int width, height, align, descent;
        /** How far its top is above the baseline in pixels, below it when negative; set by getSize. */
        int ascent;

        Placeholder(int width, int height, int align, int descent) {
            this.width = width;
            this.height = height;
            this.align = align;
            this.descent = descent;
            this.ascent = height - descent;
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
                    ascent = height - descent;
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
     * A span's background, which TextNode draws behind each line's part of its characters, between its font's ascent and descent (pixels).
     */
    static final class Background {
        final int color;
        final float radius, top, bottom;

        Background(int color, float radius, float top, float bottom) {
            this.color = color;
            this.radius = Math.max(0, Math.min(radius, (bottom - top) / 2));
            this.top = top;
            this.bottom = bottom;
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

    /** Gives every line the same height, the glyphs centered in it as in CSS. */
    static final class LineHeight implements LineHeightSpan {
        final int height;

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
