package dev.oneo.nsgo;

import android.content.Context;
import android.view.MotionEvent;
import android.view.View;
import android.widget.FrameLayout;

/** Keyboard tap policies must run even when a pressable child owns the touch. */
public final class ScrollKeyboardCheck {
    static final class Host implements Scrolling.Host {
        int dismissals;
        boolean input, handled;
        public void scrollEvent(int kind, float x, float y, float zoom) {}
        public void scrolled(float x, float y, float zoom) {}
        public boolean dismissKeyboard() { dismissals++; return true; }
        public boolean onInput(float x, float y) { return input; }
        public boolean handlesTap(float x, float y) { return handled; }
    }

    static final class Child extends View {
        int presses;
        final boolean consumes;
        Child(Context ctx, boolean consumes) { super(ctx); this.consumes = consumes; }
        public boolean onTouchEvent(MotionEvent e) {
            if (!consumes) return false;
            if (e.getActionMasked() == MotionEvent.ACTION_UP) presses++;
            return true;
        }
    }

    static void check(boolean ok, String message) {
        if (!ok) throw new AssertionError(message);
    }

    static void touch(View scroll, int end) {
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_DOWN, 20, 20, 100));
        scroll.dispatchTouchEvent(new MotionEvent(end, 20, 20, 120));
    }

    static void policy(String kind, boolean consumes, int taps, boolean input, boolean handled, int end, int expected) {
        Context ctx = new Context();
        Host host = new Host();
        host.input = input;
        host.handled = handled;
        Child child = new Child(ctx, consumes);
        child.setSize(1000, 2000);
        FrameLayout frame = new FrameLayout(ctx);
        FrameLayout scroll;
        if (kind.equals("vertical")) scroll = new Scrolling.VScroll(ctx, host, 1, child, frame);
        else if (kind.equals("horizontal")) scroll = new Scrolling.HScroll(ctx, host, 1, child, frame);
        else scroll = new Scrolling.ZoomScroll(ctx, host, 1, child);
        scroll.setSize(300, 400);
        scroll.addView(child);
        frame.addView(scroll);
        Scrolling.Config config = new Scrolling.Config();
        config.minZoom = config.zoom = 1;
        config.maxZoom = 3;
        config.taps = taps;
        ((Scrolling.Scroller) scroll).configure(config);
        touch(scroll, end);
        String label = kind + " consuming=" + consumes + " taps=" + taps + " input=" + input + " handled=" + handled + " end=" + end;
        check(host.dismissals == expected, label + ": dismissals=" + host.dismissals + ", expected=" + expected);
        check(child.presses == (consumes && end == MotionEvent.ACTION_UP ? 1 : 0), label + ": child press lost");
        // CANCEL must clear the old touch; an orphaned UP cannot dismiss again.
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_UP, 20, 20, 140));
        check(host.dismissals == expected, label + ": duplicate dismissal");
    }

    public static void main(String[] args) {
        for (String kind : new String[]{"vertical", "horizontal", "zoom"}) {
            for (boolean consumes : new boolean[]{true, false}) {
                policy(kind, consumes, 0, false, true, MotionEvent.ACTION_UP, 1); // never, even on a handled child
                policy(kind, consumes, 1, false, false, MotionEvent.ACTION_UP, 0); // always
                policy(kind, consumes, 2, false, true, MotionEvent.ACTION_UP, 0); // handled row
                policy(kind, consumes, 2, false, false, MotionEvent.ACTION_UP, 1); // blank content
                policy(kind, consumes, 0, true, true, MotionEvent.ACTION_UP, 0); // input keeps its own keyboard
                policy(kind, consumes, 0, false, true, MotionEvent.ACTION_CANCEL, 0);
            }
        }
    }
}
