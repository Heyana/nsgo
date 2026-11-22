package dev.oneo.nsgo;
import java.io.*;
import java.nio.charset.StandardCharsets;
import java.util.*;
import java.util.concurrent.*;
/** A controllable engine pipe; the recorder still uses its production sender. */
final class NsgoEngine {
    private final File directory;
    final List<String> reports = Collections.synchronizedList(new ArrayList<>());
    volatile boolean alive = true, blocked;
    final CountDownLatch writing = new CountDownLatch(1), writable = new CountDownLatch(1), drained = new CountDownLatch(1);
    final CountDownLatch secondWriting = new CountDownLatch(1), secondWritable = new CountDownLatch(1);
    NsgoEngine(File directory) { this.directory = directory; directory.mkdirs(); }
    File recordings() { return directory; }
    boolean isAlive() { return alive; }
    void recordingEnded() { drained.countDown(); }
    void write(int kind, byte[] payload) {
        if (blocked) {
            boolean second = new String(payload, StandardCharsets.UTF_8).contains("\"id\":2,");
            (second ? secondWriting : writing).countDown();
            try { (second ? secondWritable : writable).await(); } catch (InterruptedException e) { Thread.currentThread().interrupt(); return; }
        }
        if (alive) reports.add(new String(payload, StandardCharsets.UTF_8));
    }
    String report(String type) {
        synchronized (reports) { for (String value : reports) if (value.contains("\"type\":\"" + type + "\"")) return value; }
        return null;
    }
}
