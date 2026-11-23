package dev.oneo.nsgo;

import java.io.Closeable;
import java.io.IOException;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * Open document handles for a surface's picked folders, keyed by a handle ID
 * handed back to Go. Each open handle keeps its provider's descriptor alive
 * across reads until Go closes it or the surface releases, with reference
 * counting so a concurrent close waits until active reads finish.
 */
final class FolderHandles {
    /** Reads at most n bytes from off on one opened document, and releases its descriptor on close. */
    interface Document extends Closeable {
        byte[] read(long off, int n) throws IOException;
    }

    /** Opens a document of a picked tree. */
    interface Opener {
        Document open(String tree, String document) throws IOException;
    }

    private static final class Entry {
        final Document doc;
        int refs;
        boolean closing;

        Entry(Document doc) {
            this.doc = doc;
        }
    }

    private final Opener opener;
    private final Map<Integer, Entry> open = new HashMap<>();
    private int next = 1;
    private boolean closed;

    FolderHandles(Opener opener) {
        this.opener = opener;
    }

    int open(String tree, String document) throws IOException {
        synchronized (this) {
            if (closed) throw new IOException("folder handles closed");
        }
        Document doc = opener.open(tree, document);
        if (doc == null) throw new IOException("no document handle");
        int id;
        synchronized (this) {
            if (closed) {
                closeQuietly(doc);
                throw new IOException("folder handles closed");
            }
            do {
                id = next++;
                if (id == 0) id = next++;
            } while (open.containsKey(id));
            open.put(id, new Entry(doc));
        }
        return id;
    }

    byte[] read(int id, long off, int n) throws IOException {
        if (off < 0 || n < 0) throw new IOException("invalid read range");
        if (n == 0) {
            synchronized (this) {
                if (!open.containsKey(id)) throw new IOException("document handle " + id + " is closed");
            }
            return new byte[0];
        }
        Entry entry;
        synchronized (this) {
            entry = open.get(id);
            if (entry == null) throw new IOException("document handle " + id + " is closed");
            entry.refs++;
        }
        try {
            return entry.doc.read(off, n);
        } finally {
            boolean dispose;
            synchronized (this) {
                entry.refs--;
                dispose = entry.closing && entry.refs == 0;
            }
            if (dispose) closeQuietly(entry.doc);
        }
    }

    void close(int id) {
        Entry entry;
        boolean dispose;
        synchronized (this) {
            entry = open.remove(id);
            if (entry == null) return;
            entry.closing = true;
            dispose = entry.refs == 0;
        }
        if (dispose) closeQuietly(entry.doc);
    }

    void closeAll() {
        List<Document> toClose = new ArrayList<>();
        synchronized (this) {
            closed = true;
            for (Entry entry : open.values()) {
                entry.closing = true;
                if (entry.refs == 0) toClose.add(entry.doc);
            }
            open.clear();
        }
        for (Document doc : toClose) {
            closeQuietly(doc);
        }
    }

    private static void closeQuietly(Closeable c) {
        try {
            c.close();
        } catch (IOException ignored) {
        }
    }
}
