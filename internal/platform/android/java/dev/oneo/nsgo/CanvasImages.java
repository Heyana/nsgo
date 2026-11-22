package dev.oneo.nsgo;

import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.function.Consumer;

/**
 * One surface's bounded canvas cache, including failed entries, which loads under way and kept images exceed, as they are never evicted. The least recently drawn unkept entries go first, but never one the pass of drawing under way drew: a pass drawing more than the limit keeps them all, as evicting one would request it again at the next pass, forever.
 */
final class CanvasImages<T> {
    static final int LIMIT = 64;

    interface Requester {
        void request(String key, int generation, boolean cancel);
    }

    /** A frame holds a lease until its GL upload or canvas drawing finishes. */
    final class Lease implements AutoCloseable {
        final T value;
        private Entry entry;

        Lease(Entry entry) {
            this.entry = entry;
            value = entry.value;
            entry.refs++;
        }

        @Override
        public void close() {
            synchronized (CanvasImages.this) {
                if (entry == null) return;
                if (--entry.refs == 0 && entry.retired) dispose.accept(entry.value);
                entry = null;
            }
        }
    }

    private final class Entry {
        final int generation;
        long used;
        T value;
        boolean pending = true, retired;
        int refs;

        Entry(int generation) { this.generation = generation; }
    }

    private final LinkedHashMap<String, Entry> entries = new LinkedHashMap<>(LIMIT, .75f, true);
    private final Map<String, Integer> kept = new HashMap<>();
    private final Requester requester;
    private final Consumer<T> dispose;
    private final Consumer<Runnable> later;
    private int generation;
    private boolean closed;
    /** use counts acquisitions; pass is its value when the last pass of drawing ended, which passPosted says a posted endPass will end. */
    private long use, pass;
    private boolean passPosted;

    /** later runs a task after the pass of drawing under way, as a post to the UI thread's looper does. */
    CanvasImages(Requester requester, Consumer<T> dispose, Consumer<Runnable> later) {
        this.requester = requester;
        this.dispose = dispose;
        this.later = later;
    }

    private synchronized void endPass() {
        passPosted = false;
        pass = use;
    }

    synchronized void keep(String key) {
        if (closed || key == null || key.isEmpty()) return;
        kept.merge(key, 1, Integer::sum);
    }

    synchronized void release(String key) {
        if (closed || key == null || key.isEmpty()) return;
        Integer count = kept.get(key);
        if (count == null) return;
        if (count <= 1) kept.remove(key);
        else kept.put(key, count - 1);
        evict(0);
    }

    synchronized boolean isKept(String key) { return kept.containsKey(key); }

    private int unkeptSize() {
        int n = 0;
        for (String key : entries.keySet()) {
            if (!kept.containsKey(key)) n++;
        }
        return n;
    }

    private void evict(int reserve) {
        int unkept = unkeptSize();
        while (unkept + reserve > LIMIT) {
            // A kept image stays out of eviction; a load under way stays too: cancelling it would leave what waits for it, a canvas or a shader that draws only when its images arrive, without the image until it draws again.
            Map.Entry<String, Entry> oldest = null;
            for (Map.Entry<String, Entry> item : entries.entrySet()) {
                if (!kept.containsKey(item.getKey()) && !item.getValue().pending) {
                    oldest = item;
                    break;
                }
            }
            if (oldest == null || oldest.getValue().used > pass) break;
            entries.remove(oldest.getKey());
            retire(oldest.getKey(), oldest.getValue());
            unkept--;
        }
    }

    synchronized Lease acquire(String key) {
        if (closed) return null;
        use++;
        if (!passPosted) {
            passPosted = true;
            later.accept(this::endPass);
        }
        Entry entry = entries.get(key);
        if (entry == null) {
            evict(kept.containsKey(key) ? 0 : 1);
            entry = new Entry(++generation);
            entries.put(key, entry);
            requester.request(key, entry.generation, false);
        }
        entry.used = use;
        return entry.value == null ? null : new Lease(entry);
    }

    synchronized boolean wants(String key, int generation) {
        // Looking up an async response must not make it recently drawn.
        for (Map.Entry<String, Entry> item : entries.entrySet()) {
            Entry entry = item.getValue();
            if (item.getKey().equals(key)) return entry.generation == generation && entry.pending;
        }
        return false;
    }

    /** Takes ownership even of obsolete results, disposing them immediately. */
    synchronized boolean complete(String key, int generation, T value) {
        if (!wants(key, generation)) {
            if (value != null) dispose.accept(value);
            return false;
        }
        for (Map.Entry<String, Entry> item : entries.entrySet()) {
            if (!item.getKey().equals(key)) continue;
            Entry entry = item.getValue();
            entry.pending = false;
            entry.value = value;
            break;
        }
        return value != null;
    }

    synchronized boolean unavailable(String key, int generation) {
        if (!wants(key, generation)) return false;
        entries.remove(key);
        return true;
    }

    synchronized void close() {
        if (closed) return;
        closed = true;
        clear();
        kept.clear();
    }

    /** Drops native pictures on detach while keeping request generations unique. */
    synchronized void clear() {
        for (Map.Entry<String, Entry> item : entries.entrySet()) retire(item.getKey(), item.getValue());
        entries.clear();
    }

    synchronized int size() { return entries.size(); }

    /** The native test agent observes readiness without affecting eviction order. */
    synchronized boolean ready(String key) {
        for (Map.Entry<String, Entry> item : entries.entrySet()) {
            if (item.getKey().equals(key)) return item.getValue().value != null;
        }
        return false;
    }

    private void retire(String key, Entry entry) {
        entry.retired = true;
        if (entry.pending) requester.request(key, entry.generation, true);
        if (entry.value != null && entry.refs == 0) dispose.accept(entry.value);
    }
}
