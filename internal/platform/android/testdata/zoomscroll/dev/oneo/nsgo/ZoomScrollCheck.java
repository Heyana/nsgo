package dev.oneo.nsgo;

import android.content.Context;
import android.view.MotionEvent;
import android.view.View;
import android.widget.FrameLayout;
import java.util.ArrayList;
import java.util.List;

/**
 * Exercises Scrolling.ZoomScroll's braking touches under Android's ViewGroup
 * touch dispatch rules: a finger landing on a fling is intercepted so the
 * child under it never sees down or press, while dragging the same finger
 * past slop pans smoothly.
 */
public final class ZoomScrollCheck {
    static void check(boolean ok, String message) {
        if (!ok) throw new AssertionError(message);
    }

    static final class ChildButton extends View {
        int downs, ups, cancels;

        ChildButton(Context context) {
            super(context);
        }

        @Override
        public boolean onTouchEvent(MotionEvent e) {
            switch (e.getActionMasked()) {
                case MotionEvent.ACTION_DOWN:
                    downs++;
                    return true;
                case MotionEvent.ACTION_UP:
                    ups++;
                    return true;
                case MotionEvent.ACTION_CANCEL:
                    cancels++;
                    return true;
                default:
                    return true;
            }
        }
    }

    static final class Recorder implements Scrolling.Host {
        final List<Integer> events = new ArrayList<>();

        @Override
        public void scrollEvent(int kind, float x, float y, float zoom) {
            events.add(kind);
        }

        @Override
        public void scrolled(float x, float y, float zoom) {}

        @Override
        public boolean dismissKeyboard() {
            return false;
        }

        @Override
        public boolean onInput(float rawX, float rawY) {
            return false;
        }

        @Override
        public boolean handlesTap(float rawX, float rawY) {
            return false;
        }
    }

    static void fling(Scrolling.ZoomScroll scroll, long t0) {
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_DOWN, 100, 300, t0));
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_MOVE, 100, 270, t0 + 16));
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_MOVE, 100, 200, t0 + 32));
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_MOVE, 100, 120, t0 + 48));
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_UP, 100, 120, t0 + 50));
    }

    static int count(List<Integer> list, int kind) {
        int n = 0;
        for (int v : list) if (v == kind) n++;
        return n;
    }

    public static void main(String[] args) {
        Context ctx = new Context();
        FrameLayout parent = new FrameLayout(ctx);
        Recorder host = new Recorder();
        ChildButton content = new ChildButton(ctx);
        content.setSize(1000, 2000);
        Scrolling.ZoomScroll scroll = new Scrolling.ZoomScroll(ctx, host, 1f, content);
        scroll.setSize(300, 400);
        Scrolling.Config cfg = new Scrolling.Config();
        cfg.minZoom = 1f;
        cfg.maxZoom = 3f;
        cfg.zoom = 1f;
        scroll.configure(cfg);
        scroll.addView(content);
        parent.addView(scroll);

        check(scroll.canScrollVertically(1) && !scroll.canScrollVertically(-1), "initial vertical scroll ability");
        check(scroll.canScrollHorizontally(1) && !scroll.canScrollHorizontally(-1), "initial horizontal scroll ability");

        // 1. Tap at rest reaches the child normally.
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_DOWN, 100, 100, 100));
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_UP, 100, 100, 120));
        check(content.downs == 1 && content.ups == 1 && content.cancels == 0,
                "tap at rest did not press child: downs=" + content.downs + " ups=" + content.ups);

        // 2. Start a fling and let it step once so it is actively moving.
        fling(scroll, 200);
        check(count(host.events, Protocol.EVENT_MOMENTUM_START) == 1, "fling did not start momentum");
        check(count(host.events, Protocol.EVENT_MOMENTUM_END) == 0, "momentum ended prematurely");
        int beforeStep = scroll.getScrollY();
        scroll.computeScroll();
        check(scroll.getScrollY() > beforeStep, "computeScroll did not advance fling");

        // 3. Braking touch stops the fling and presses nothing when it lifts.
        int downsBefore = content.downs, upsBefore = content.ups;
        host.events.clear();
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_DOWN, 100, 150, 400));
        int brakedY = scroll.getScrollY();
        scroll.computeScroll();
        check(scroll.getScrollY() == brakedY, "scroll moved after braking touch");
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_UP, 100, 150, 450));
        check(content.downs == downsBefore && content.ups == upsBefore,
                "braking touch reached child: downs=" + (content.downs - downsBefore) + " ups=" + (content.ups - upsBefore));
        check(count(host.events, Protocol.EVENT_MOMENTUM_END) == 1,
                "braking touch momentum end count=" + count(host.events, Protocol.EVENT_MOMENTUM_END));
        check(count(host.events, Protocol.EVENT_SCROLL_DRAG_START) == 0
                && count(host.events, Protocol.EVENT_SCROLL_DRAG_END) == 0
                && count(host.events, Protocol.EVENT_SCROLL_END) == 0,
                "braking tap emitted drag or scroll end events: " + host.events);

        // 4. Subsequent tap at rest reaches the child again.
        downsBefore = content.downs;
        upsBefore = content.ups;
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_DOWN, 100, 150, 500));
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_UP, 100, 150, 520));
        check(content.downs == downsBefore + 1 && content.ups == upsBefore + 1,
                "tap after brake did not press child");

        // 5. Braking touch that drags past slop pans smoothly without pressing child.
        host.events.clear();
        fling(scroll, 600);
        check(count(host.events, Protocol.EVENT_MOMENTUM_START) == 1, "second fling did not start momentum");
        scroll.computeScroll();
        downsBefore = content.downs;
        upsBefore = content.ups;
        host.events.clear();
        int yAtBrake = scroll.getScrollY();
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_DOWN, 100, 200, 800));
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_MOVE, 100, 196, 816));
        check(count(host.events, Protocol.EVENT_SCROLL_DRAG_START) == 0, "sub-slop move started drag");
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_MOVE, 100, 150, 832));
        check(count(host.events, Protocol.EVENT_SCROLL_DRAG_START) == 1, "drag past slop after brake did not start");
        check(scroll.getScrollY() > yAtBrake, "drag past slop after brake did not pan");
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_UP, 100, 150, 900));
        check(count(host.events, Protocol.EVENT_SCROLL_DRAG_END) == 1, "drag after brake did not end");
        check(content.downs == downsBefore && content.ups == upsBefore, "braking drag reached child");

        // 6. Braking touch when flinger finished its last step but momentum has not yet been flushed by computeScroll.
        // Settle any fling from step 5 first.
        for (int i = 0; i < 12; i++) scroll.computeScroll();
        host.events.clear();
        scroll.scrollToPoints(0, 100, false);
        fling(scroll, 1000);
        check(count(host.events, Protocol.EVENT_MOMENTUM_START) == 1, "third fling did not start momentum");
        // Step until flinger reports finished, stopping before the final empty computeScroll.
        for (int i = 0; i < 10; i++) scroll.computeScroll();
        check(count(host.events, Protocol.EVENT_MOMENTUM_END) == 0, "momentum should still be pending");
        downsBefore = content.downs;
        upsBefore = content.ups;
        host.events.clear();
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_DOWN, 100, 150, 1300));
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_UP, 100, 150, 1320));
        scroll.computeScroll();
        check(content.downs == downsBefore && content.ups == upsBefore, "pending-momentum brake reached child");
        check(count(host.events, Protocol.EVENT_MOMENTUM_END) == 1, "pending-momentum brake did not end momentum once");
        check(count(host.events, Protocol.EVENT_SCROLL_END) == 0, "pending-momentum brake emitted scroll end");

        // 7. Lifting one of two fingers leaves the content where it is: the
        // one that stays pans on from its own place, not from the middle of
        // the two that included the finger going up.
        for (int i = 0; i < 12; i++) scroll.computeScroll();
        scroll.scrollToPoints(100, 300, false);
        int xBefore = scroll.getScrollX(), yBefore = scroll.getScrollY();
        long t = 1400;
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_DOWN, 100, 300, t));
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_POINTER_DOWN, 1,
                new float[]{100, 200}, new float[]{300, 300}, t + 16));
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_MOVE, 0,
                new float[]{100, 200}, new float[]{260, 260}, t + 32));
        int xPinched = scroll.getScrollX(), yPinched = scroll.getScrollY();
        check(yPinched == yBefore + 40 && xPinched == xBefore, "two fingers did not pan by their middle: "
                + (xPinched - xBefore) + "," + (yPinched - yBefore));
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_POINTER_UP, 1,
                new float[]{100, 200}, new float[]{260, 260}, t + 48));
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_MOVE, 100, 260, t + 64));
        check(scroll.getScrollX() == xPinched && scroll.getScrollY() == yPinched,
                "lifting a finger moved the content by " + (scroll.getScrollX() - xPinched) + "," + (scroll.getScrollY() - yPinched));
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_MOVE, 100, 250, t + 80));
        check(scroll.getScrollY() == yPinched + 10, "the finger that stayed did not pan on");
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_UP, 100, 250, t + 96));

        // 8. NoScroll leaves touches alone.
        cfg.noScroll = true;
        scroll.configure(cfg);
        downsBefore = content.downs;
        upsBefore = content.ups;
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_DOWN, 100, 150, 1500));
        scroll.dispatchTouchEvent(new MotionEvent(MotionEvent.ACTION_UP, 100, 150, 1520));
        check(content.downs == downsBefore + 1 && content.ups == upsBefore + 1, "noScroll blocked child press");
    }
}
