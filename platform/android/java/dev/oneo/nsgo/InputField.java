package dev.oneo.nsgo;

import android.content.Context;
import android.graphics.drawable.ColorDrawable;
import android.os.Build;
import android.text.Editable;
import android.text.InputFilter;
import android.text.InputType;
import android.text.TextWatcher;
import android.view.KeyEvent;
import android.view.View;
import android.view.inputmethod.BaseInputConnection;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputConnection;
import android.view.inputmethod.InputConnectionWrapper;
import android.view.inputmethod.InputMethodManager;
import android.widget.EditText;
import java.io.DataInputStream;
import java.io.IOException;

/**
 * An input's EditText, as NSH's InputBox: it counts the user's edits,
 * answers maxLength (an InputFilter) and Return (SubmitBehavior) natively,
 * reports selections, keys and compositions, and holds a value the app
 * sets during a composition until it ends, since setting the text restarts
 * the input method, which commits the pre-edit text.
 */
final class InputField extends EditText {
    /** What the field tells the engine. */
    interface Listener {
        boolean has(int kind);

        void send(int kind, long count, int index, String text, boolean finished);

        void blur();
    }

    /** core.SubmitBehavior's codes, as InputConfig.Submit resolves them. */
    static final int SUBMIT = 1, BLUR_AND_SUBMIT = 2, NEWLINE = 3;

    private final Listener listener;
    /** Edits reported to the engine. */
    long count;
    /** The text as last reported or set. */
    private String known = "";
    private boolean multiline;
    private int submit = BLUR_AND_SUBMIT;
    private boolean applying, configured;
    private boolean inComposition;
    private String composed = "";
    private String pendingText;
    private long pendingCount;
    /** Inside the text watcher, where replacing the text would corrupt the IME's edit. */
    private boolean watching;
    private int selStart = -1, selEnd = -1;

    InputField(Context context, Listener listener) {
        super(context);
        this.listener = listener;
        setBackground(null);
        setPadding(0, 0, 0, 0);
        setSingleLine(true);
        setInputType(InputType.TYPE_CLASS_TEXT);
        setImeOptions(EditorInfo.IME_ACTION_DONE);
        addTextChangedListener(new TextWatcher() {
            @Override public void beforeTextChanged(CharSequence s, int a, int b, int c) {}
            @Override public void onTextChanged(CharSequence s, int a, int b, int c) {}
            @Override public void afterTextChanged(Editable s) {
                if (applying) return;
                // Reconfiguring sets the same text again: only real edits count.
                String text = s.toString();
                // A composition that starts or changes is reported before the
                // text holding it, one that ends after the text it committed.
                boolean composingNow = BaseInputConnection.getComposingSpanStart(s) >= 0;
                watching = true;
                try {
                    if (composingNow) syncComposition();
                    if (!text.equals(known)) {
                        known = text;
                        count++;
                        listener.send(Protocol.EVENT_CHANGE_TEXT, count, 0, text, false);
                    }
                    if (!composingNow) syncComposition();
                } finally {
                    watching = false;
                }
            }
        });
        setOnEditorActionListener((v, action, event) -> {
            // A hardware Enter breaks the line where Return does, and with
            // Shift or Alt in any multiline input.
            if (multiline && event != null
                    && (submit == NEWLINE || event.isShiftPressed() || event.isAltPressed())) return false;
            if (event == null) key("Enter");
            // A hardware Enter reports its down and its up: act once.
            if (event != null && event.getAction() != KeyEvent.ACTION_DOWN) return true;
            submitted();
            return true;
        });
    }

    @Override
    public boolean onKeyDown(int code, KeyEvent event) {
        // Hardware keys, and keys the keyboard sends as key events.
        if (code == KeyEvent.KEYCODE_DEL) key("Backspace");
        else if (code == KeyEvent.KEYCODE_ENTER) key("Enter");
        else if (event.getUnicodeChar() != 0) key(String.valueOf((char) event.getUnicodeChar()));
        return super.onKeyDown(code, event);
    }

    @Override
    public boolean onKeyUp(int code, KeyEvent event) {
        // The up of an Enter whose down submitted must not move the focus.
        if (code == KeyEvent.KEYCODE_ENTER && submit != NEWLINE) return true;
        return super.onKeyUp(code, event);
    }

    @Override
    public InputConnection onCreateInputConnection(EditorInfo info) {
        InputConnection connection = super.onCreateInputConnection(info);
        // TextView hides the action key of multiline inputs; one that submits
        // shows it, as React Native does.
        if (multiline && submit != NEWLINE) info.imeOptions &= ~EditorInfo.IME_FLAG_NO_ENTER_ACTION;
        return connection == null ? null : new KeyConnection(connection);
    }

    @Override
    protected void onSelectionChanged(int start, int end) {
        super.onSelectionChanged(start, end);
        if (listener == null || applying || !listener.has(Protocol.EVENT_SELECTION_CHANGE)) return;
        if (start == selStart && end == selEnd) return;
        selStart = start;
        selEnd = end;
        listener.send(Protocol.EVENT_SELECTION_CHANGE, Math.max(start, end), Math.min(start, end), "", false);
    }

    @Override
    protected void onFocusChanged(boolean focused, int direction, android.graphics.Rect previous) {
        super.onFocusChanged(focused, direction, previous);
        if (!focused) {
            // A composition does not outlive the focus.
            clearComposingText();
            syncComposition();
        }
    }

    /**
     * Reads what ItemInput appends after the fields NsgoView reads, and
     * applies it with the keyboard type, mask and editability.
     */
    void configure(DataInputStream in, int keyboard, boolean masked, boolean readonly, boolean disabled,
            float density) throws IOException {
        boolean multiline = Protocol.flag(in.readUnsignedByte(), 0);
        int maxLength = in.readInt();
        int submit = in.readUnsignedByte();
        int returnKey = in.readUnsignedByte();
        int capitalize = in.readUnsignedByte();
        int flags = in.readUnsignedByte();
        boolean noAutoCorrect = Protocol.flag(flags, 0), hideCaret = Protocol.flag(flags, 1);
        String autofill = Protocol.readString(in);
        int placeholderColor = in.readInt(), selectionColor = in.readInt();

        boolean restart = configured && multiline && this.submit != submit;
        this.multiline = multiline;
        this.submit = submit;
        int type = NsgoView.inputType(keyboard, masked && !multiline);
        if ((type & InputType.TYPE_MASK_CLASS) == InputType.TYPE_CLASS_TEXT) {
            // core.AutoCapitalize: sentences, none, words, characters.
            switch (capitalize) {
                case 0: type |= InputType.TYPE_TEXT_FLAG_CAP_SENTENCES; break;
                case 2: type |= InputType.TYPE_TEXT_FLAG_CAP_WORDS; break;
                case 3: type |= InputType.TYPE_TEXT_FLAG_CAP_CHARACTERS; break;
                default: break;
            }
            type |= noAutoCorrect ? InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS : InputType.TYPE_TEXT_FLAG_AUTO_CORRECT;
            if (multiline) type |= InputType.TYPE_TEXT_FLAG_MULTI_LINE;
        }
        int selection = getSelectionEnd();
        // Before the input type: setSingleLine replaces a password's
        // transformation.
        applying = true;
        setSingleLine(!multiline);
        if (multiline) setMaxLines(Integer.MAX_VALUE);
        // Setting the input type restarts the keyboard, so only on change.
        if (getInputType() != type) setInputType(type);
        if (selection >= 0 && selection <= length()) setSelection(selection);
        applying = false;
        int ime;
        switch (returnKey) {
            case 1: ime = EditorInfo.IME_ACTION_DONE; break;
            case 2: ime = EditorInfo.IME_ACTION_GO; break;
            case 3: ime = EditorInfo.IME_ACTION_NEXT; break;
            case 4: ime = EditorInfo.IME_ACTION_SEARCH; break;
            case 5: ime = EditorInfo.IME_ACTION_SEND; break;
            default: ime = multiline && submit == NEWLINE ? EditorInfo.IME_ACTION_NONE : EditorInfo.IME_ACTION_DONE;
        }
        setImeOptions(ime);
        setFilters(maxLength > 0 ? new InputFilter[] {new InputFilter.LengthFilter(maxLength)} : new InputFilter[0]);
        // Without a key listener the field takes no input but still selects.
        if (readonly) setKeyListener(null);
        setEnabled(!disabled);
        if (placeholderColor != 0) setHintTextColor(Text.argb(placeholderColor));
        setCursorVisible(!hideCaret);
        if (selectionColor != 0) {
            int c = Text.argb(selectionColor);
            if (Build.VERSION.SDK_INT >= 29) {
                setTextCursorDrawable(new ColorDrawable(c) {
                    @Override
                    public int getIntrinsicWidth() {
                        return Math.round(2 * density);
                    }
                });
            }
            setHighlightColor((c & 0x00ffffff) | 0x4d000000);
        }
        if (Build.VERSION.SDK_INT >= 26) {
            if (autofill == null || autofill.equals("-")) {
                setImportantForAutofill(View.IMPORTANT_FOR_AUTOFILL_AUTO);
            } else if (autofill.isEmpty()) {
                setImportantForAutofill(View.IMPORTANT_FOR_AUTOFILL_NO);
            } else {
                setImportantForAutofill(View.IMPORTANT_FOR_AUTOFILL_YES);
                setAutofillHints(autofill);
            }
        }
        configured = true;
        // The keyboard reads the action key when the input connects.
        if (restart && hasFocus()) {
            InputMethodManager imm = (InputMethodManager) getContext().getSystemService(Context.INPUT_METHOD_SERVICE);
            if (imm != null) imm.restartInput(this);
        }
    }

    boolean multiline() {
        return multiline;
    }

    private void submitted() {
        listener.send(Protocol.EVENT_SUBMIT_EDITING, count, 0, getText().toString(), false);
        if (submit == BLUR_AND_SUBMIT) listener.blur();
    }

    private void key(String key) {
        if (listener.has(Protocol.EVENT_KEY_PRESS)) listener.send(Protocol.EVENT_KEY_PRESS, 0, 0, key, false);
    }

    /**
     * Reports a change of the composition and applies the value the app set
     * during it once it ends. Read from the composing span after every edit:
     * keyboards start, change and end compositions in many ways.
     */
    private void syncComposition() {
        Editable text = getText();
        int start = text == null ? -1 : BaseInputConnection.getComposingSpanStart(text);
        int end = text == null ? -1 : BaseInputConnection.getComposingSpanEnd(text);
        boolean now = start >= 0 && end >= 0 && start != end;
        String composedNow = now ? text.subSequence(Math.min(start, end), Math.max(start, end)).toString() : "";
        if (now == inComposition && composedNow.equals(composed)) return;
        inComposition = now;
        composed = composedNow;
        if (listener.has(Protocol.EVENT_COMPOSITION_CHANGE)) {
            listener.send(Protocol.EVENT_COMPOSITION_CHANGE, 0, 0, composedNow, !now);
        }
        if (!now && pendingText != null) {
            // Not from inside the text watcher: the edit that ended the
            // composition is still in progress there.
            if (watching) post(this::applyPending);
            else applyPending();
        }
    }

    private void applyPending() {
        if (inComposition || pendingText == null) return;
        String text = pendingText;
        pendingText = null;
        setValue(text, pendingCount);
    }

    /** ItemInputValue: the app's text unless the user typed since it saw count. */
    void setValue(String text, long count) {
        if (inComposition) {
            pendingText = text;
            pendingCount = count;
            return;
        }
        pendingText = null;
        if (count < this.count || text.contentEquals(getText())) return;
        applying = true;
        known = text;
        int selection = getSelectionEnd();
        boolean atEnd = selection >= length();
        setText(text);
        setSelection(atEnd ? text.length() : Math.min(Math.max(selection, 0), text.length()));
        applying = false;
    }

    /** ItemSelection: selects start to end, clamped to the text. */
    void select(int start, int end) {
        int length = length();
        int s = Math.min(Math.max(start, 0), length);
        applying = true;
        setSelection(s, Math.min(Math.max(s, end), length));
        applying = false;
        selStart = getSelectionStart();
        selEnd = getSelectionEnd();
    }

    /** Reports the keys of soft keyboards, which edit through the input connection. */
    private final class KeyConnection extends InputConnectionWrapper {
        private CharSequence composing;

        KeyConnection(InputConnection target) {
            super(target, true);
        }

        @Override
        public boolean setComposingText(CharSequence text, int position) {
            int before = composing == null ? 0 : composing.length();
            if (text.length() > before) key(text.subSequence(text.length() - 1, text.length()).toString());
            else if (text.length() < before) key("Backspace");
            composing = text;
            boolean done = super.setComposingText(text, position);
            syncComposition();
            return done;
        }

        @Override
        public boolean setComposingRegion(int start, int end) {
            boolean done = super.setComposingRegion(start, end);
            syncComposition();
            return done;
        }

        @Override
        public boolean finishComposingText() {
            composing = null;
            boolean done = super.finishComposingText();
            syncComposition();
            return done;
        }

        @Override
        public boolean commitText(CharSequence text, int position) {
            String added = text.toString();
            if (composing != null && added.startsWith(composing.toString())) added = added.substring(composing.length());
            composing = null;
            if (added.equals("\n")) key("Enter");
            else if (!added.isEmpty()) key(added);
            // Keyboards that type Return as a line break: an input that
            // submits takes none.
            if (multiline && submit != NEWLINE && text.toString().equals("\n")) {
                submitted();
                return true;
            }
            boolean done = super.commitText(text, position);
            syncComposition();
            return done;
        }

        @Override
        public boolean deleteSurroundingText(int before, int after) {
            if (before > 0 || after > 0) key("Backspace");
            boolean done = super.deleteSurroundingText(before, after);
            syncComposition();
            return done;
        }

        @Override
        public boolean endBatchEdit() {
            boolean more = super.endBatchEdit();
            syncComposition();
            return more;
        }

        @Override
        public void closeConnection() {
            super.closeConnection();
            syncComposition();
        }
    }
}
