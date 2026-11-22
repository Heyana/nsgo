package android.app;

import android.content.Context;
import android.view.KeyEvent;
import android.view.MotionEvent;
import android.view.View;
import android.view.Window;

public class Activity extends Context {
    public View currentFocus;
    public Window window = new Window();
    public java.util.function.Consumer<MotionEvent> onTouch;

    public boolean dispatchTouchEvent(MotionEvent e) {
        if (onTouch != null) onTouch.accept(e);
        return true;
    }

    public boolean dispatchKeyEvent(KeyEvent e) { return true; }
    public boolean dispatchKeyShortcutEvent(KeyEvent e) { return false; }
    public View getCurrentFocus() { return currentFocus; }
    public Window getWindow() { return window; }
}
