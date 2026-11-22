package android.text;

public class SpannableString implements Spannable {
    private final String text;
    public SpannableString(CharSequence source) { this.text = source.toString(); }
    @Override public int length() { return text.length(); }
    @Override public char charAt(int index) { return text.charAt(index); }
    @Override public CharSequence subSequence(int start, int end) { return text.subSequence(start, end); }
    @Override public String toString() { return text; }
    @Override public int getSpanStart(Object tag) { return -1; }
    @Override public int getSpanEnd(Object tag) { return -1; }
    @Override public void setSpan(Object what, int start, int end, int flags) {}
    @Override public void removeSpan(Object what) {}
}
