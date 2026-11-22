package android.view.inputmethod;

public interface InputConnection {
    boolean setComposingText(CharSequence text, int newCursorPosition);
    boolean setComposingRegion(int start, int end);
    boolean finishComposingText();
    boolean commitText(CharSequence text, int newCursorPosition);
    boolean deleteSurroundingText(int beforeLength, int afterLength);
    boolean endBatchEdit();
    void closeConnection();
}
