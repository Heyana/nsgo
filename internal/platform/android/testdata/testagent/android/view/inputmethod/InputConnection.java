package android.view.inputmethod;

public interface InputConnection {
    boolean beginBatchEdit();
    boolean endBatchEdit();
    boolean commitText(CharSequence text, int newCursorPosition);
    boolean setComposingText(CharSequence text, int newCursorPosition);
    boolean finishComposingText();
    boolean performEditorAction(int editorAction);
}
