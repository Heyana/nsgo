package dev.oneo.nsgo;

import android.app.Activity;
import android.os.SystemClock;
import android.view.InputDevice;
import android.view.KeyCharacterMap;
import android.view.KeyEvent;
import java.util.LinkedHashMap;

/** A test keyboard's held native keys, separate from editing shortcuts. */
final class HeldKeyAgent {
    private final LinkedHashMap<Integer, Integer> held = new LinkedHashMap<>();

    int modifiers() {
        int meta = 0;
        for (int code : held.keySet()) {
            switch (code) {
                case KeyEvent.KEYCODE_SHIFT_LEFT: meta |= KeyEvent.META_SHIFT_ON | KeyEvent.META_SHIFT_LEFT_ON; break;
                case KeyEvent.KEYCODE_CTRL_LEFT: meta |= KeyEvent.META_CTRL_ON | KeyEvent.META_CTRL_LEFT_ON; break;
                case KeyEvent.KEYCODE_ALT_LEFT: meta |= KeyEvent.META_ALT_ON | KeyEvent.META_ALT_LEFT_ON; break;
                case KeyEvent.KEYCODE_META_LEFT: meta |= KeyEvent.META_META_ON | KeyEvent.META_META_LEFT_ON; break;
                default: break;
            }
        }
        return meta;
    }

    /** Presses or releases one key as a hardware keyboard's, through the activity, within a scripted step of gate. */
    void key(Activity activity, TestInputGate gate, String name, boolean down) {
        int code = TestAgent.keyCode(name.toLowerCase());
        Integer previous = held.get(code);
        if (!down && previous == null) throw new IllegalArgumentException("key is not held: " + name);
        int repeat = down && previous != null ? previous + 1 : 0;
        if (down) held.put(code, repeat);
        else held.remove(code);
        long now = SystemClock.uptimeMillis();
        KeyEvent e = new KeyEvent(now, now, down ? KeyEvent.ACTION_DOWN : KeyEvent.ACTION_UP,
                code, repeat, modifiers(), KeyCharacterMap.VIRTUAL_KEYBOARD, 0);
        e.setSource(InputDevice.SOURCE_KEYBOARD);
        gate.enterScripted();
        try {
            activity.dispatchKeyEvent(e);
        } finally {
            gate.leaveScripted();
        }
    }
}
