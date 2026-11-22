package android.text;

public interface Editable extends Spannable {
    Editable replace(int st, int en, CharSequence source);
    int composingStart();
    int composingEnd();
    void setComposingSpan(int start, int end);
    void clearComposingSpan();
}
