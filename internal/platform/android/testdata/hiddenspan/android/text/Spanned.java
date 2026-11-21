package android.text;

public interface Spanned extends CharSequence {
    int SPAN_INCLUSIVE_INCLUSIVE = 18;
    int SPAN_EXCLUSIVE_EXCLUSIVE = 33;

    <T> T[] getSpans(int start, int end, Class<T> type);
    int getSpanStart(Object tag);
    int getSpanEnd(Object tag);
}
