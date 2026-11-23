package dev.oneo.nsgo;

import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;

/** Exercises the real cache and request decoder with two display densities. */
public final class TextCacheCheck {
    private static final int OVERLAY = 7;

    public static void main(String[] args) throws Exception {
        TextCache cache = new TextCache(1);
        cache.registerSurface(OVERLAY, 2.5f);
        DataInputStream packets = new DataInputStream(System.in);
        for (long id = 1; id <= 3; id++) {
            byte[] payload = new byte[packets.readInt()];
            packets.readFully(payload);
            check(reply(cache.request(payload), id, Protocol.TEXT_PREPARE).read() == -1, "prepare reply");
        }

        layout(cache, 1, 31, 0);
        layout(cache, 1, 31, OVERLAY);
        TextCache.Layout first = cache.show(0, 31);
        TextCache.Layout overlay = cache.show(OVERLAY, 31);
        check(first != null && first.value.fontSize == 16 && first.value.text.equals("main"), "first surface");
        check(overlay != null && overlay.value.fontSize == 40 && overlay.value.text.equals("甲😀"), "overlay density");
        check(overlay.clip == 2 && overlay.shadow != null && overlay.shadow.x == 5, "layout decoration handoff");
        check(cache.show(OVERLAY, 31) == overlay, "repeated mount lost the shown layout");

        cache.setDensity(OVERLAY, 3);
        DataInputStream bounds = reply(request(cache, Protocol.TEXT_MEASURE, 1, OVERLAY, 20), 9, Protocol.TEXT_MEASURE);
        check(bounds.readFloat() == 20 && bounds.readFloat() == 16, "prepared density changed during measurement");
        check(lineWidth(cache, 31, OVERLAY) == 20, "shown layout density changed");
        cache.release(release(31, OVERLAY));
        check(lineWidth(cache, 31, OVERLAY) == 20, "release removed a shown layout");
        cache.hide(OVERLAY, 31);
        error(cache, Protocol.TEXT_LAYOUT_LINES, 31, OVERLAY, 0, "unknown text layout");
        check(lineWidth(cache, 31, 0) == 20, "overlay replaced the first surface's layout");

        layout(cache, 1, 33, OVERLAY);
        cache.release(release(33, OVERLAY));
        check(cache.show(OVERLAY, 33) == null, "release kept a pending layout");
        layout(cache, 1, 34, 0);
        check(cache.show(OVERLAY, 34) == null, "overlay took a scoped first-surface layout");

        layout(cache, 2, 32, null);
        TextCache.Layout legacy = cache.show(OVERLAY, 32);
        check(legacy != null && legacy.value.text.equals("legacy") && legacy.value.fontSize == 16, "legacy overlay layout");
        cache.hide(OVERLAY, 32);
        error(cache, Protocol.TEXT_CARETS, 32, null, 0, "unknown text layout");

        cache.removeSurface(OVERLAY);
        error(cache, Protocol.TEXT_MEASURE, 1, OVERLAY, 20, "unknown text surface");
        check(lineWidth(cache, 31, 0) == 20, "closing the overlay disposed first-surface text");
    }

    private static void layout(TextCache cache, int prepared, int layout, Integer owner) throws IOException {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream out = header(bytes, Protocol.TEXT_LAYOUT, prepared);
        out.writeInt(layout);
        out.writeFloat(20);
        if (owner != null) out.writeInt(owner);
        check(reply(cache.request(bytes.toByteArray()), 9, Protocol.TEXT_LAYOUT).readInt() == 0, "layout reply");
    }

    private static float lineWidth(TextCache cache, int id, Integer owner) throws IOException {
        DataInputStream in = reply(request(cache, Protocol.TEXT_LAYOUT_LINES, id, owner, 0), 9, Protocol.TEXT_LAYOUT_LINES);
        check(in.readInt() == 1 && in.readInt() == 0 && in.readInt() > 0, "line range");
        return in.readFloat();
    }

    private static byte[] request(TextCache cache, int kind, int id, Integer owner, float width) throws IOException {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream out = header(bytes, kind, id);
        if (kind == Protocol.TEXT_MEASURE) out.writeFloat(width);
        if (owner != null) out.writeInt(owner);
        return cache.request(bytes.toByteArray());
    }

    private static DataOutputStream header(ByteArrayOutputStream bytes, int kind, int id) throws IOException {
        DataOutputStream out = new DataOutputStream(bytes);
        out.writeLong(9);
        out.writeByte(kind);
        out.writeInt(id);
        return out;
    }

    private static byte[] release(int id, int owner) throws IOException {
        ByteArrayOutputStream bytes = new ByteArrayOutputStream();
        DataOutputStream out = new DataOutputStream(bytes);
        out.writeInt(id);
        out.writeInt(owner);
        return bytes.toByteArray();
    }

    private static DataInputStream reply(byte[] bytes, long id, int kind) throws IOException {
        DataInputStream in = new DataInputStream(new ByteArrayInputStream(bytes));
        check(in.readLong() == id && in.readUnsignedByte() == kind, "reply ID and kind");
        if (in.readBoolean()) throw new AssertionError(Protocol.readString(in));
        return in;
    }

    private static void error(TextCache cache, int kind, int id, Integer owner, float width, String want) throws IOException {
        DataInputStream in = new DataInputStream(new ByteArrayInputStream(request(cache, kind, id, owner, width)));
        check(in.readLong() == 9 && in.readUnsignedByte() == kind && in.readBoolean()
                && Protocol.readString(in).contains(want), want);
    }

    private static void check(boolean valid, String reason) {
        if (!valid) throw new AssertionError(reason);
    }
}
