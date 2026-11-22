package android.os;
import java.util.ArrayDeque;
public class Handler {
    private static final ArrayDeque<Runnable> queue = new ArrayDeque<>();
    public Handler(Looper loop) {}
    public boolean post(Runnable fn) { queue.add(fn); return true; }
    public static void drain() { while (!queue.isEmpty()) queue.remove().run(); }
}
