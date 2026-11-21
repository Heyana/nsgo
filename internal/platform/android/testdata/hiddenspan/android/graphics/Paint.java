package android.graphics;

import java.util.Locale;

public class Paint {
    public static final int ANTI_ALIAS_FLAG = 1;
    public static final int UNDERLINE_TEXT_FLAG = 8;
    public static final int STRIKE_THRU_TEXT_FLAG = 16;
    public static final int FILTER_BITMAP_FLAG = 2;
    public static final int SUBPIXEL_TEXT_FLAG = 0x80;
    public static final int LINEAR_TEXT_FLAG = 0x40;

    public enum Style { FILL, STROKE }

    public static class FontMetrics {
        public float ascent = -12, descent = 4, top = -12, bottom = 4, leading = 0;
    }

    public static class FontMetricsInt {
        public int ascent = -12, descent = 4, top = -12, bottom = 4, leading = 0;
    }

    private float textSize = 16;
    private int color = 0xff000000;
    private int flags;
    private Typeface typeface = Typeface.DEFAULT;
    private float letterSpacing;
    private String fontFeatureSettings;
    private Locale locale = Locale.ROOT;
    private float shadowRadius, shadowDx, shadowDy;
    private int shadowColor;
    private Style style = Style.FILL;
    private float strokeWidth;

    public Paint() {}
    public Paint(int flags) { this.flags = flags; }
    public Paint(Paint p) {
        this.textSize = p.textSize;
        this.color = p.color;
        this.flags = p.flags;
        this.typeface = p.typeface;
        this.letterSpacing = p.letterSpacing;
        this.fontFeatureSettings = p.fontFeatureSettings;
        this.locale = p.locale;
        this.shadowRadius = p.shadowRadius;
        this.shadowDx = p.shadowDx;
        this.shadowDy = p.shadowDy;
        this.shadowColor = p.shadowColor;
        this.style = p.style;
        this.strokeWidth = p.strokeWidth;
    }

    public void setTextSize(float s) { textSize = s; }
    public float getTextSize() { return textSize; }
    public void setTypeface(Typeface t) { typeface = t; }
    public void setColor(int c) { color = c; }
    public int getColor() { return color; }
    public void setFlags(int f) { flags = f; }
    public int getFlags() { return flags; }
    public void setLetterSpacing(float ls) { letterSpacing = ls; }
    public void setFontFeatureSettings(String f) { fontFeatureSettings = f; }
    public void setTextLocale(Locale l) { locale = l; }
    public void setShadowLayer(float r, float dx, float dy, int c) { shadowRadius = r; shadowDx = dx; shadowDy = dy; shadowColor = c; }
    public void clearShadowLayer() { shadowRadius = 0; shadowDx = 0; shadowDy = 0; shadowColor = 0; }
    public boolean hasShadow() { return (shadowColor >>> 24) != 0 && shadowRadius > 0; }
    public void setStyle(Style s) { style = s; }
    public void setStrokeWidth(float w) { strokeWidth = w; }
    public void setPathEffect(PathEffect e) {}
    public void setXfermode(Xfermode x) {}
    public float getUnderlineThickness() { return Math.max(1f, textSize / 16f); }
    public float getUnderlinePosition() { return textSize / 8f; }
    public float getStrikeThruPosition() { return -textSize * 0.3f; }
    public FontMetrics getFontMetrics() { return new FontMetrics(); }
    public float getFontMetrics(FontMetrics fm) {
        fm.ascent = -textSize * 0.8f;
        fm.descent = textSize * 0.2f;
        fm.top = fm.ascent;
        fm.bottom = fm.descent;
        return fm.descent - fm.ascent;
    }
    public FontMetricsInt getFontMetricsInt() {
        FontMetricsInt fm = new FontMetricsInt();
        getFontMetricsInt(fm);
        return fm;
    }
    public int getFontMetricsInt(FontMetricsInt fm) {
        fm.ascent = -Math.round(textSize * 0.8f);
        fm.descent = Math.round(textSize * 0.2f);
        fm.top = fm.ascent;
        fm.bottom = fm.descent;
        return fm.descent - fm.ascent;
    }
    public void getTextBounds(String text, int start, int end, Rect bounds) {
        bounds.set(0, -Math.round(textSize * 0.6f), (end - start) * 10, 0);
    }
    public float measureText(CharSequence text, int start, int end) {
        return (end - start) * 10f;
    }
}
