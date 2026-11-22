package android.view.inputmethod;

public class InputConnectionWrapper implements InputConnection {
    private final InputConnection target;

    public InputConnectionWrapper(InputConnection target, boolean mutable) {
        this.target = target;
    }

    @Override public boolean setComposingText(CharSequence text, int pos) { return target.setComposingText(text, pos); }
    @Override public boolean setComposingRegion(int start, int end) { return target.setComposingRegion(start, end); }
    @Override public boolean finishComposingText() { return target.finishComposingText(); }
    @Override public boolean commitText(CharSequence text, int pos) { return target.commitText(text, pos); }
    @Override public boolean deleteSurroundingText(int before, int after) { return target.deleteSurroundingText(before, after); }
    @Override public boolean endBatchEdit() { return target.endBatchEdit(); }
    @Override public void closeConnection() { target.closeConnection(); }
}
