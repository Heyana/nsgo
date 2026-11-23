package dev.oneo.nsgo;

import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * Exercises FolderHandles on the JVM: reusing one opened document across
 * multiple reads, deferring close while a read is in flight, rejecting closed
 * handles and closing all handles on teardown.
 */
public final class FolderHandlesCheck {
    public static void main(String[] args) throws Exception {
        byte[] content = "0123456789abcdef".getBytes(StandardCharsets.UTF_8);
        AtomicInteger opens = new AtomicInteger();
        AtomicInteger closes = new AtomicInteger();

        FolderHandles handles = new FolderHandles((tree, doc) -> {
            opens.incrementAndGet();
            return new FolderHandles.Document() {
                private volatile boolean closed;

                @Override
                public byte[] read(long off, int n) throws IOException {
                    if (closed) throw new IOException("already closed");
                    if (off >= content.length) return new byte[0];
                    int start = (int) off;
                    int end = Math.min(content.length, start + n);
                    return Arrays.copyOfRange(content, start, end);
                }

                @Override
                public void close() {
                    closed = true;
                    closes.incrementAndGet();
                }
            };
        });

        int h = handles.open("content://tree", "doc-1");
        if (h == 0 || opens.get() != 1) throw new AssertionError("open failed: h=" + h + " opens=" + opens.get());

        byte[] a = handles.read(h, 0, 4);
        byte[] b = handles.read(h, 10, 6);
        byte[] empty = handles.read(h, 0, 0);
        if (!Arrays.equals(a, "0123".getBytes(StandardCharsets.UTF_8))) throw new AssertionError("first read mismatch");
        if (!Arrays.equals(b, "abcdef".getBytes(StandardCharsets.UTF_8))) throw new AssertionError("second read mismatch");
        if (empty.length != 0) throw new AssertionError("empty read returned bytes");
        if (opens.get() != 1 || closes.get() != 0) throw new AssertionError("handle was not reused");

        handles.close(h);
        handles.close(h);
        if (closes.get() != 1) throw new AssertionError("close count " + closes.get());

        try {
            handles.read(h, 0, 1);
            throw new AssertionError("read after close succeeded");
        } catch (IOException expected) {
        }

        // Concurrent close waits until an active read finishes before closing the document.
        CountDownLatch inRead = new CountDownLatch(1);
        CountDownLatch releaseRead = new CountDownLatch(1);
        AtomicInteger blockingCloses = new AtomicInteger();
        FolderHandles concurrent = new FolderHandles((tree, doc) -> new FolderHandles.Document() {
            @Override
            public byte[] read(long off, int n) throws IOException {
                inRead.countDown();
                try {
                    if (!releaseRead.await(5, TimeUnit.SECONDS)) throw new IOException("timeout");
                } catch (InterruptedException e) {
                    throw new IOException("interrupted", e);
                }
                if (blockingCloses.get() != 0) throw new IOException("closed while reading");
                return new byte[]{42};
            }

            @Override
            public void close() {
                blockingCloses.incrementAndGet();
            }
        });

        int ch = concurrent.open("content://tree", "doc-blocking");
        Throwable[] err = new Throwable[1];
        byte[][] got = new byte[1][];
        Thread reader = new Thread(() -> {
            try {
                got[0] = concurrent.read(ch, 0, 1);
            } catch (Throwable t) {
                err[0] = t;
            }
        });
        reader.start();
        if (!inRead.await(5, TimeUnit.SECONDS)) throw new AssertionError("reader never entered read");
        concurrent.close(ch);
        if (blockingCloses.get() != 0) throw new AssertionError("document closed before in-flight read finished");
        releaseRead.countDown();
        reader.join(5000);
        if (err[0] != null) throw new AssertionError("concurrent read failed", err[0]);
        if (got[0] == null || got[0].length != 1 || got[0][0] != 42) throw new AssertionError("unexpected concurrent read result");
        if (blockingCloses.get() != 1) throw new AssertionError("document not closed after read finished: " + blockingCloses.get());

        // closeAll releases every open handle and refuses subsequent opens.
        int h2 = handles.open("content://tree", "doc-2");
        int h3 = handles.open("content://tree", "doc-3");
        if (h2 == h3) throw new AssertionError("duplicate handle IDs");
        handles.closeAll();
        if (closes.get() != 3) throw new AssertionError("closeAll did not close open handles: " + closes.get());
        try {
            handles.open("content://tree", "doc-4");
            throw new AssertionError("open after closeAll succeeded");
        } catch (IOException expected) {
        }
    }
}
