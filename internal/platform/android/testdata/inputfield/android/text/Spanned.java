package android.text;

public interface Spanned extends CharSequence {
    int SPAN_INCLUSIVE_INCLUSIVE = 18;
    int getSpanStart(Object tag);
    int getSpanEnd(Object tag);
}
