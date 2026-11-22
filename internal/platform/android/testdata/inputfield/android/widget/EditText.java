package android.widget;

import android.content.Context;
import android.graphics.drawable.Drawable;
import android.text.Editable;
import android.text.InputFilter;
import android.text.Layout;
import android.text.TextWatcher;
import android.text.method.KeyListener;
import android.view.KeyEvent;
import android.view.View;
import android.view.inputmethod.BaseInputConnection;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputConnection;
import java.util.ArrayList;
import java.util.List;

public class EditText extends View {
    public interface OnEditorActionListener {
        boolean onEditorAction(EditText v, int actionId, KeyEvent event);
    }

    private static final class SimpleEditable implements Editable {
        private final StringBuilder sb = new StringBuilder();
        private int compStart = -1, compEnd = -1;

        @Override public int length() { return sb.length(); }
        @Override public char charAt(int index) { return sb.charAt(index); }
        @Override public CharSequence subSequence(int start, int end) { return sb.subSequence(start, end); }
        @Override public String toString() { return sb.toString(); }
        @Override public int getSpanStart(Object tag) { return -1; }
        @Override public int getSpanEnd(Object tag) { return -1; }
        @Override public void setSpan(Object what, int start, int end, int flags) {}
        @Override public void removeSpan(Object what) {}
        @Override public Editable replace(int st, int en, CharSequence source) {
            sb.replace(st, en, source.toString());
            return this;
        }
        @Override public int composingStart() { return compStart; }
        @Override public int composingEnd() { return compEnd; }
        @Override public void setComposingSpan(int start, int end) { compStart = start; compEnd = end; }
        @Override public void clearComposingSpan() { compStart = -1; compEnd = -1; }
    }

    private final SimpleEditable text = new SimpleEditable();
    private final List<TextWatcher> watchers = new ArrayList<>();
    private int inputType;
    private int selStart = 0, selEnd = 0;
    public boolean notifyWatcherOnClearComposing;

    public EditText(Context context) { super(context); }

    public void setFallbackLineSpacing(boolean enabled) {}
    public void setSingleLine(boolean singleLine) {}
    public void setMaxLines(int maxLines) {}
    public void setInputType(int type) { this.inputType = type; }
    public int getInputType() { return inputType; }
    public void setImeOptions(int imeOptions) {}
    public void setFilters(InputFilter[] filters) {}
    public void setKeyListener(KeyListener listener) {}
    public void setHintTextColor(int color) {}
    public void setCursorVisible(boolean visible) {}
    public void setTextCursorDrawable(Drawable drawable) {}
    public void setHighlightColor(int color) {}
    public void setHint(CharSequence hint) {}
    public void setOnEditorActionListener(OnEditorActionListener l) {}
    public void addTextChangedListener(TextWatcher watcher) { watchers.add(watcher); }

    public Editable getText() { return text; }
    public int length() { return text.length(); }
    public int getSelectionStart() { return selStart; }
    public int getSelectionEnd() { return selEnd; }
    public Layout getLayout() { return null; }
    public int getTotalPaddingLeft() { return 0; }
    public int getTotalPaddingTop() { return 0; }
    public int getTotalPaddingBottom() { return 0; }

    public void setText(CharSequence s) {
        text.replace(0, text.length(), s == null ? "" : s);
        text.clearComposingSpan();
        selStart = text.length();
        selEnd = text.length();
        notifyAfterTextChanged();
    }

    public void setSelection(int index) {
        setSelection(index, index);
    }

    public void setSelection(int start, int end) {
        selStart = start;
        selEnd = end;
        onSelectionChanged(start, end);
    }

    protected void onSelectionChanged(int selStart, int selEnd) {}

    public void clearComposingText() {
        boolean had = text.composingStart() >= 0;
        text.clearComposingSpan();
        if (had && notifyWatcherOnClearComposing) {
            notifyAfterTextChanged();
        }
    }

    public void notifyAfterTextChanged() {
        for (TextWatcher w : watchers) {
            w.afterTextChanged(text);
        }
    }

    public boolean onKeyDown(int keyCode, KeyEvent event) { return false; }
    public boolean onKeyUp(int keyCode, KeyEvent event) { return false; }
    public boolean onTextContextMenuItem(int id) { return false; }

    public InputConnection onCreateInputConnection(EditorInfo outAttrs) {
        return new BaseInputConnection(this);
    }
}
