package android.text;

import java.lang.reflect.Array;
import java.util.ArrayList;
import java.util.List;

public class SpannableString implements Spanned {
    static final class Entry {
        final Object span;
        final int start, end;

        Entry(Object span, int start, int end) {
            this.span = span;
            this.start = start;
            this.end = end;
        }
    }

    private final String text;
    final List<Entry> spans = new ArrayList<>();

    public SpannableString(CharSequence source) {
        this.text = source.toString();
    }

    public void setSpan(Object what, int start, int end, int flags) {
        spans.add(new Entry(what, start, end));
    }

    @Override
    @SuppressWarnings("unchecked")
    public <T> T[] getSpans(int start, int end, Class<T> type) {
        List<T> out = new ArrayList<>();
        for (Entry e : spans) {
            if (type.isInstance(e.span) && e.end > start && e.start < end) {
                out.add(type.cast(e.span));
            }
        }
        return out.toArray((T[]) Array.newInstance(type, out.size()));
    }

    @Override
    public int getSpanStart(Object tag) {
        for (Entry e : spans) if (e.span == tag) return e.start;
        return -1;
    }

    @Override
    public int getSpanEnd(Object tag) {
        for (Entry e : spans) if (e.span == tag) return e.end;
        return -1;
    }

    @Override public int length() { return text.length(); }
    @Override public char charAt(int index) { return text.charAt(index); }
    @Override public CharSequence subSequence(int start, int end) { return text.subSequence(start, end); }
    @Override public String toString() { return text; }
}
