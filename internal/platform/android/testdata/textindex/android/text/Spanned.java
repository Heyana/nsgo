package android.text;

public interface Spanned extends CharSequence {
    <T> T[] getSpans(int start, int end, Class<T> type);
    int getSpanStart(Object tag);
    int getSpanEnd(Object tag);
}
