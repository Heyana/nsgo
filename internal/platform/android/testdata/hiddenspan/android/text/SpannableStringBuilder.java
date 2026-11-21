package android.text;

import java.lang.reflect.Array;
import java.util.ArrayList;
import java.util.List;

public final class SpannableStringBuilder implements Spanned {
    private final StringBuilder sb = new StringBuilder();
    private final List<SpannableString.Entry> spans = new ArrayList<>();

    public SpannableStringBuilder append(char c) { sb.append(c); return this; }
    public SpannableStringBuilder append(CharSequence text, int start, int end) { sb.append(text, start, end); return this; }

    public void setSpan(Object what, int start, int end, int flags) {
        spans.add(new SpannableString.Entry(what, start, end));
    }

    @Override
    @SuppressWarnings("unchecked")
    public <T> T[] getSpans(int start, int end, Class<T> type) {
        List<T> out = new ArrayList<>();
        for (SpannableString.Entry e : spans) {
            if (type.isInstance(e.span) && e.end > start && e.start < end) {
                out.add(type.cast(e.span));
            }
        }
        return out.toArray((T[]) Array.newInstance(type, out.size()));
    }

    @Override
    public int getSpanStart(Object tag) {
        for (SpannableString.Entry e : spans) if (e.span == tag) return e.start;
        return -1;
    }

    @Override
    public int getSpanEnd(Object tag) {
        for (SpannableString.Entry e : spans) if (e.span == tag) return e.end;
        return -1;
    }

    @Override public int length() { return sb.length(); }
    @Override public char charAt(int index) { return sb.charAt(index); }
    @Override public CharSequence subSequence(int start, int end) { return sb.subSequence(start, end); }
    @Override public String toString() { return sb.toString(); }
}
