package android.os;

import java.util.ArrayDeque;
import java.util.Deque;

public class Handler {
    public static final Deque<Runnable> QUEUE = new ArrayDeque<>();

    public Handler(Looper looper) {}

    public boolean post(Runnable r) {
        QUEUE.addLast(r);
        return true;
    }

    public boolean postDelayed(Runnable r, long delayMillis) {
        QUEUE.addLast(r);
        return true;
    }

    public static void drain() {
        while (!QUEUE.isEmpty()) {
            QUEUE.removeFirst().run();
        }
    }
}
