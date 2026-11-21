package dev.oneo.nsgo;

import android.content.Context;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.lang.reflect.Field;
import java.nio.ByteBuffer;
import java.util.Arrays;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.locks.ReentrantReadWriteLock;

/** Exercises the real runtime's bulk reader and writer without Android. */
public final class BulkPipeCheck {
    public static void main(String[] args) throws Exception {
        byte[] picture = new byte[2 * Protocol.BULK_CHUNK + 5];
        for (int i = 0; i < picture.length; i++) picture[i] = (byte) i;
        readWhileImageArrives(picture);
        writeWhileFileLeaves(picture);
    }

    private static void readWhileImageArrives(byte[] picture) throws Exception {
        ByteArrayOutputStream incoming = new ByteArrayOutputStream();
        DataOutputStream wire = new DataOutputStream(incoming);
        bulk(wire, new byte[0]);
        bulk(wire, fragment(Protocol.IMAGE, picture, 0));
        frame(wire, Protocol.TEXT_REQUEST, new byte[]{42});
        for (int offset = Protocol.BULK_CHUNK; offset < picture.length; offset += Protocol.BULK_CHUNK) {
            bulk(wire, fragment(Protocol.IMAGE, picture, offset));
        }
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        NsgoEngine engine = new NsgoEngine(new Context());
        set(engine, "out", new DataOutputStream(output));
        set(engine, "process", new Process() {
            public InputStream getInputStream() { return new ByteArrayInputStream(incoming.toByteArray()); }
            public InputStream getErrorStream() { return new ByteArrayInputStream(new byte[0]); }
            public OutputStream getOutputStream() { return output; }
            public int waitFor() { return 0; }
            public int exitValue() { return 0; }
            public void destroy() {}
        });
        CountDownLatch end = new CountDownLatch(1);
        final class Listener implements NsgoEngine.Listener {
            int messages;
            IOException failure;
            public void onMessage(int kind, byte[] payload) {
                if (messages++ == 0) {
                    check(kind == Protocol.TEXT_REQUEST && Arrays.equals(payload, new byte[]{42}), "text was held behind the image");
                } else {
                    check(kind == Protocol.IMAGE && Arrays.equals(payload, picture), "incomplete or corrupted image");
                }
            }
            public void onProcessEnded(IOException e) { failure = e; end.countDown(); }
            public void onLog(String line) {}
        }
        Listener listener = new Listener();
        engine.attach(listener);
        check(end.await(5, TimeUnit.SECONDS), "bulk reader hung");
        check(listener.failure == null && listener.messages == 2, "bulk reader lost a message");
        check(Arrays.equals(output.toByteArray(), new byte[]{(byte) Protocol.BULK_REPLY, 0, 0, 0, 0}), "missing bulk acknowledgement");
    }

    private static void writeWhileFileLeaves(byte[] file) throws Exception {
        CountDownLatch first = new CountDownLatch(1), resume = new CountDownLatch(1);
        ByteArrayOutputStream output = new ByteArrayOutputStream() {
            public void flush() throws IOException {
                if (size() == 5 + Protocol.BULK_HEADER + Protocol.BULK_CHUNK) {
                    first.countDown();
                    try {
                        if (!resume.await(5, TimeUnit.SECONDS)) throw new IOException("test did not resume the pipe");
                    } catch (InterruptedException e) {
                        Thread.currentThread().interrupt();
                        throw new IOException(e);
                    }
                }
            }
        };
        NsgoEngine engine = new NsgoEngine(new Context());
        set(engine, "out", new DataOutputStream(output));
        set(engine, "bulkReady", true);
        ReentrantReadWriteLock priority = (ReentrantReadWriteLock) field("bulkPriority").get(engine);
        Thread bulk = new Thread(() -> engine.write(Protocol.FOLDER_REPLY, file));
        Thread text = new Thread(() -> engine.write(Protocol.TEXT_REPLY, new byte[]{17}));
        bulk.start();
        check(first.await(5, TimeUnit.SECONDS), "bulk writer did not start");
        text.start();
        try {
            long end = System.nanoTime() + TimeUnit.SECONDS.toNanos(5);
            while (!priority.hasQueuedThread(text) && System.nanoTime() < end) Thread.yield();
            check(priority.hasQueuedThread(text), "text writer did not queue");
        } finally {
            resume.countDown();
        }
        bulk.join(5000);
        text.join(5000);
        check(!bulk.isAlive() && !text.isAlive(), "wire writer hung");
        DataInputStream in = new DataInputStream(new ByteArrayInputStream(output.toByteArray()));
        for (int offset = 0; offset < file.length; offset += Protocol.BULK_CHUNK) {
            if (offset == Protocol.BULK_CHUNK) {
                check(in.readUnsignedByte() == Protocol.TEXT_REPLY && in.readInt() == 1 && in.readByte() == 17,
                        "text reply was held behind the file");
            }
            byte[] want = fragment(Protocol.FOLDER_REPLY, file, offset);
            check(in.readUnsignedByte() == Protocol.BULK_REPLY && in.readInt() == want.length &&
                    Arrays.equals(in.readNBytes(want.length), want), "changed file fragment");
        }
        check(in.read() == -1, "unexpected wire data");
    }

    private static byte[] fragment(int kind, byte[] data, int offset) {
        int length = Math.min(Protocol.BULK_CHUNK, data.length - offset);
        return ByteBuffer.allocate(Protocol.BULK_HEADER + length)
                .put((byte) kind).putInt(data.length).putInt(offset).put(data, offset, length).array();
    }

    /** Frames body as an EXTENDED message of kind EXT_BULK, as Go's sendExtended does. */
    private static void bulk(DataOutputStream wire, byte[] body) throws IOException {
        byte[] extended = new byte[body.length + 1];
        extended[0] = (byte) Protocol.EXT_BULK;
        System.arraycopy(body, 0, extended, 1, body.length);
        frame(wire, Protocol.EXTENDED, extended);
    }

    private static void frame(DataOutputStream out, int kind, byte[] payload) throws IOException {
        out.writeByte(kind);
        out.writeInt(payload.length);
        out.write(payload);
    }

    private static Field field(String name) throws ReflectiveOperationException {
        Field field = NsgoEngine.class.getDeclaredField(name);
        field.setAccessible(true);
        return field;
    }

    private static void set(NsgoEngine engine, String name, Object value) throws ReflectiveOperationException {
        field(name).set(engine, value);
    }

    private static void check(boolean valid, String message) {
        if (!valid) throw new AssertionError(message);
    }
}
