package dev.oneo.nsgo;

import android.content.Context;
import android.view.KeyEvent;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputConnection;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

/**
 * Exercises InputField's real Java implementation on the host JVM to verify
 * that ending a composition on blur, disable or read-only reports
 * EVENT_COMPOSITION_CHANGE(finished=true) before View's focus listener reports
 * EVENT_BLUR, keeps composed text, and applies or drops held values cleanly.
 */
public final class InputFieldCheck {
    private static final class Fixture {
        final List<String> events = new ArrayList<>();
        final InputField field;
        final InputConnection ic;

        Fixture(boolean notifyWatcherOnClear) {
            field = new InputField(new Context(), new InputField.Listener() {
                @Override public boolean has(int kind) {
                    return kind == Protocol.EVENT_COMPOSITION_CHANGE
                            || kind == Protocol.EVENT_CHANGE_TEXT
                            || kind == Protocol.EVENT_FOCUS
                            || kind == Protocol.EVENT_BLUR;
                }

                @Override public void send(int kind, long count, int index, String text, boolean finished) {
                    if (kind == Protocol.EVENT_CHANGE_TEXT) {
                        events.add("change:" + text);
                    } else if (kind == Protocol.EVENT_COMPOSITION_CHANGE) {
                        events.add(finished ? "comp:end" : "comp:" + text);
                    }
                }

                @Override public void blur() {
                    field.clearFocus();
                }

                @Override public boolean keyBinding(KeyEvent event) { return false; }
                @Override public void caret(float x, float y, float w, float h) {}
                @Override public void paste(String text, String data) {}
            });
            field.notifyWatcherOnClearComposing = notifyWatcherOnClear;
            field.setOnFocusChangeListener((v, focused) -> events.add(focused ? "focus" : "blur"));
            field.requestFocus();
            ic = field.onCreateInputConnection(new EditorInfo());
        }

        void configure(boolean readonly, boolean disabled) throws IOException {
            ByteArrayOutputStream buf = new ByteArrayOutputStream();
            DataOutputStream out = new DataOutputStream(buf);
            out.writeByte(0); // single-line
            out.writeInt(0);  // maxLength
            out.writeByte(InputField.BLUR_AND_SUBMIT);
            out.writeByte(0); // returnKey
            out.writeByte(0); // capitalize
            out.writeByte(0); // flags
            out.writeInt(0);  // autofill length
            out.writeInt(0);  // placeholderColor
            out.writeInt(0);  // selectionColor
            out.writeByte(0); // pasteReport
            out.writeFloat(0);
            out.writeFloat(0);
            out.writeFloat(0);
            out.writeFloat(0);
            field.configure(new DataInputStream(new ByteArrayInputStream(buf.toByteArray())),
                    0, false, readonly, disabled, 1f);
        }
    }

    private static void expect(String name, List<String> got, String... want) {
        List<String> expected = Arrays.asList(want);
        if (!got.equals(expected)) {
            throw new AssertionError(name + ": events = " + got + ", want " + expected);
        }
    }

    private static void expectText(String name, InputField field, String want) {
        String got = field.getText().toString();
        if (!got.equals(want)) {
            throw new AssertionError(name + ": text = \"" + got + "\", want \"" + want + "\"");
        }
    }

    public static void main(String[] args) throws Exception {
        // 1. Blur while composing ends composition before View's focus listener reports blur,
        // both when clearComposingText does not notify TextWatchers and when it does.
        for (boolean notify : new boolean[] {false, true}) {
            Fixture f = new Fixture(notify);
            f.ic.setComposingText("ni", 1);
            f.field.clearFocus();
            f.field.flushPosted();
            expect("blur-notify=" + notify, f.events, "focus", "comp:ni", "change:ni", "comp:end", "blur");
            expectText("blur-notify=" + notify, f.field, "ni");
        }

        // 2. Unreported text edit present at blur reports change before composition end and blur.
        {
            Fixture f = new Fixture(false);
            f.ic.setComposingText("ni", 1);
            f.field.getText().replace(0, 2, "ni!");
            f.field.clearFocus();
            f.field.flushPosted();
            expect("unsynced-blur", f.events, "focus", "comp:ni", "change:ni", "change:ni!", "comp:end", "blur");
            expectText("unsynced-blur", f.field, "ni!");
        }

        // 3. Stale setValue held during composition is dropped on blur; fresh setValue is applied.
        {
            Fixture f = new Fixture(false);
            f.ic.setComposingText("ni", 1);
            f.field.setValue("stale", 0);
            f.field.clearFocus();
            f.field.flushPosted();
            expect("held-stale", f.events, "focus", "comp:ni", "change:ni", "comp:end", "blur");
            expectText("held-stale", f.field, "ni");
        }
        {
            Fixture f = new Fixture(true);
            f.ic.setComposingText("ni", 1);
            f.field.setValue("fresh", f.field.count);
            f.field.clearFocus();
            f.field.flushPosted();
            expect("held-fresh", f.events, "focus", "comp:ni", "change:ni", "comp:end", "blur");
            expectText("held-fresh", f.field, "fresh");
        }

        // 4. Blur without an active composition only reports blur.
        {
            Fixture f = new Fixture(false);
            f.field.clearFocus();
            expect("no-comp-blur", f.events, "focus", "blur");
        }

        // 5. Turning disabled while composing ends composition before blur with no duplicate end.
        {
            Fixture f = new Fixture(false);
            f.ic.setComposingText("ni", 1);
            f.configure(false, true);
            f.field.flushPosted();
            expect("disable-while-composing", f.events, "focus", "comp:ni", "change:ni", "comp:end", "blur");
            expectText("disable-while-composing", f.field, "ni");
        }

        // 6. Turning read-only while composing ends composition without blurring, and later blur emits only blur.
        {
            Fixture f = new Fixture(false);
            f.ic.setComposingText("ni", 1);
            f.configure(true, false);
            f.field.flushPosted();
            expect("readonly-while-composing", f.events, "focus", "comp:ni", "change:ni", "comp:end");
            f.field.clearFocus();
            expect("readonly-then-blur", f.events, "focus", "comp:ni", "change:ni", "comp:end", "blur");
            expectText("readonly-then-blur", f.field, "ni");
        }
    }
}
