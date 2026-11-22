package android.view.inputmethod;

import android.text.Editable;
import android.text.Spannable;
import android.widget.EditText;

public class BaseInputConnection implements InputConnection {
    private final EditText target;

    public BaseInputConnection(EditText target) {
        this.target = target;
    }

    public static int getComposingSpanStart(Spannable text) {
        return text instanceof Editable ? ((Editable) text).composingStart() : -1;
    }

    public static int getComposingSpanEnd(Spannable text) {
        return text instanceof Editable ? ((Editable) text).composingEnd() : -1;
    }

    @Override
    public boolean setComposingText(CharSequence text, int newCursorPosition) {
        Editable e = target.getText();
        int start = e.composingStart();
        int end = e.composingEnd();
        if (start < 0 || end < 0) {
            start = target.getSelectionStart();
            end = target.getSelectionEnd();
            if (start < 0) start = e.length();
            if (end < 0) end = e.length();
        }
        int a = Math.min(start, end), b = Math.max(start, end);
        e.replace(a, b, text);
        if (text.length() > 0) {
            e.setComposingSpan(a, a + text.length());
        } else {
            e.clearComposingSpan();
        }
        target.notifyAfterTextChanged();
        return true;
    }

    @Override
    public boolean setComposingRegion(int start, int end) {
        target.getText().setComposingSpan(start, end);
        return true;
    }

    @Override
    public boolean finishComposingText() {
        target.clearComposingText();
        return true;
    }

    @Override
    public boolean commitText(CharSequence text, int newCursorPosition) {
        Editable e = target.getText();
        int start = e.composingStart();
        int end = e.composingEnd();
        if (start < 0 || end < 0) {
            start = e.length();
            end = e.length();
        }
        int a = Math.min(start, end), b = Math.max(start, end);
        e.replace(a, b, text);
        e.clearComposingSpan();
        target.notifyAfterTextChanged();
        return true;
    }

    @Override
    public boolean deleteSurroundingText(int beforeLength, int afterLength) {
        return true;
    }

    @Override
    public boolean endBatchEdit() {
        return false;
    }

    @Override
    public void closeConnection() {}
}
