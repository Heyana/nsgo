package dev.oneo.nsgo;

import android.text.StaticLayout;
import android.util.SparseArray;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.EOFException;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.concurrent.ConcurrentHashMap;

/** The engine's text, split by surface. No entry retains a view or its context. */
final class TextCache {
    private final ConcurrentHashMap<Integer, Surface> surfaces = new ConcurrentHashMap<>();
    private ByteArrayOutputStream replyBytes = new ByteArrayOutputStream(256);
    private DataOutputStream replyOut = new DataOutputStream(replyBytes);
    private boolean closed;

    TextCache(float density) {
        registerSurface(0, density);
    }

    synchronized void registerSurface(int id, float density) {
        if (closed) return;
        surfaces.computeIfAbsent(id, k -> new Surface(density)).density = density;
    }

    void setDensity(int id, float density) {
        Surface surface = surfaces.get(id);
        if (surface != null) surface.density = density;
    }

    synchronized void removeSurface(int id) {
        surfaces.remove(id);
    }

    synchronized void close() {
        closed = true;
        surfaces.clear();
    }

    /** Answers numbered requests on the engine reader thread, even with no view on screen. */
    byte[] request(byte[] payload) {
        long request = 0;
        int kind = 0;
        ByteArrayOutputStream bytes = replyBytes;
        bytes.reset();
        DataOutputStream out = replyOut;
        try (DataInputStream in = input(payload)) {
            request = in.readLong();
            kind = in.readUnsignedByte();
            out.writeLong(request);
            out.writeByte(kind);
            out.writeBoolean(false);
            operation(kind, in, out, payload);
        } catch (Throwable error) {
            bytes.reset();
            try {
                out.writeLong(request);
                out.writeByte(kind);
                out.writeBoolean(true);
                Protocol.writeString(out, error.getClass().getSimpleName() + ": " + error.getMessage());
            } catch (IOException impossible) {
                throw new AssertionError(impossible);
            }
        }
        byte[] result = bytes.toByteArray();
        if (bytes.size() > 4096) {
            replyBytes = new ByteArrayOutputStream(256);
            replyOut = new DataOutputStream(replyBytes);
        }
        return result;
    }

    void release(byte[] payload) throws IOException {
        try (DataInputStream in = input(payload)) {
            int id = in.readInt();
            Surface surface = surfaces.get(trailingSurface(in, 0));
            if (surface != null) {
                surface.prepared.remove(id);
                surface.release(id);
            }
        }
    }

    /** Hands the pending layout to its view without making it unavailable to hit testing. */
    Layout show(int owner, int id) {
        Surface surface = surfaces.get(owner);
        Layout layout = surface == null ? null : surface.show(id, false);
        if (layout == null && owner != 0) {
            // Old Go apps prepared all their text on the first surface.
            Surface first = surfaces.get(0);
            if (first != null) layout = first.show(id, true);
        }
        return layout;
    }

    void hide(int owner, int id) {
        if (id == 0) return;
        Surface surface = surfaces.get(owner);
        if (surface != null && surface.hide(id, false)) return;
        if (owner != 0) {
            Surface first = surfaces.get(0);
            if (first != null) first.hide(id, true);
        }
    }

    private void operation(int kind, DataInputStream in, DataOutputStream out, byte[] payload) throws IOException {
        int id = in.readInt();
        switch (kind) {
            case Protocol.TEXT_PREPARE:
                prepare(id, in, payload);
                break;
            case Protocol.TEXT_MEASURE: {
                Prepared text = surface(trailingSurface(in, Float.BYTES)).text(id);
                float[] size = text.value.measure(in.readFloat() * text.density); // NaN does not wrap
                out.writeFloat(size[0] / text.density);
                out.writeFloat(size[1] / text.density);
                break;
            }
            case Protocol.TEXT_LAYOUT: {
                int layout = in.readInt();
                Surface owner = surface(trailingSurface(in, Float.BYTES));
                Prepared text = owner.text(id);
                StaticLayout value = text.value.layout(in.readFloat() * text.density);
                if (text.value.hasAttachments()) {
                    float[] places = Text.attachments(value);
                    out.writeInt(places.length / 2);
                    for (float p : places) out.writeFloat(p / text.density);
                } else {
                    out.writeInt(0);
                }
                owner.addLayout(layout, new Layout(value, text));
                break;
            }
            case Protocol.TEXT_LAYOUT_LINES: {
                Layout layout = surface(trailingSurface(in, 0)).layout(id);
                float[][] lines = Text.lines(layout.value);
                out.writeInt(lines.length);
                for (float[] line : lines) {
                    out.writeInt((int) line[0]);
                    out.writeInt((int) line[1]);
                    for (int i = 2; i < line.length; i++) out.writeFloat(line[i] / layout.density);
                }
                break;
            }
            case Protocol.TEXT_CARETS: {
                Layout layout = surface(trailingSurface(in, 0)).layout(id);
                out.write(Text.carets(layout.value, layout.density));
                break;
            }
            default:
                throw new IOException("unknown text operation " + kind);
        }
    }

    private void prepare(int id, DataInputStream in, byte[] payload) throws IOException {
        boolean scoped = hasPrepareSurface(in);
        int surface = 0;
        if (scoped) {
            int off = payload.length - Integer.BYTES;
            surface = ((payload[off] & 0xff) << 24)
                    | ((payload[off + 1] & 0xff) << 16)
                    | ((payload[off + 2] & 0xff) << 8)
                    | (payload[off + 3] & 0xff);
        }
        Surface owner = surface(surface);
        float density = owner.density;
        String text = Protocol.readString(in);
        Text.Style style = new Text.Style(in, density);
        int maxLines = in.readInt();
        int spanCount = in.readInt();
        List<Text.Span> spans = spanCount == 0 ? Collections.emptyList() : new ArrayList<>(spanCount);
        for (int i = 0; i < spanCount; i++) spans.add(new Text.Span(in, density));
        int attachCount = in.readInt();
        List<Text.Attachment> attachments = attachCount == 0 ? Collections.emptyList() : new ArrayList<>(attachCount);
        for (int i = 0; i < attachCount; i++) attachments.add(new Text.Attachment(in, density));
        int ellipsize = in.readUnsignedByte();
        Text.Shadow shadow = new Text.Shadow(in, density);
        style.readDecoration(in, density);
        for (int i = 0; i < spanCount; i++) spans.get(i).style.readDecoration(in, density);
        int suffix = scoped ? Integer.BYTES : 0;
        // Each span's hidden flag follows the decorations, so a runtime that predates it reads the rest unchanged; with none sent, every span is visible.
        for (int i = 0; i < spanCount; i++) {
            if (in.available() > suffix) spans.get(i).hidden = Protocol.flag(in.readUnsignedByte(), 0);
        }
        if (in.available() != suffix) throw new IOException("invalid text prepare surface");
        owner.prepared.put(id, new Prepared(
                new Text.Prepared(text, style, maxLines, ellipsize, shadow, spans, attachments), density, !scoped));
    }

    /** PREPARE has a variable body. The base style's spare flag identifies its optional suffix. */
    private static boolean hasPrepareSurface(DataInputStream in) throws IOException {
        in.mark(in.available());
        skip(in, in.readInt()); // text
        skip(in, in.readInt()); // font family
        skip(in, in.readInt()); // language
        skip(in, Float.BYTES * 3 + Short.BYTES + Integer.BYTES + 1); // size, weight, color, height, spacing, align
        boolean scoped = (in.readUnsignedByte() & Protocol.TEXT_PREPARE_SURFACE) != 0;
        in.reset();
        return scoped;
    }

    /** The fixed-size operations have either their legacy body or that body followed by a surface ID. */
    private static int trailingSurface(DataInputStream in, int bodySize) throws IOException {
        int length = in.available();
        if (length <= bodySize) return 0;
        if (length != bodySize + Integer.BYTES) throw new IOException("invalid text request surface");
        in.mark(length);
        skip(in, bodySize);
        int surface = in.readInt();
        in.reset();
        return surface;
    }

    private static DataInputStream input(byte[] payload) {
        return new DataInputStream(new ByteArrayInputStream(payload));
    }

    private static void skip(DataInputStream in, int count) throws IOException {
        if (count < 0 || in.skipBytes(count) != count) throw new EOFException();
    }

    private Surface surface(int id) throws IOException {
        Surface owner = surfaces.get(id);
        if (owner == null) throw new IOException("unknown text surface " + Integer.toUnsignedString(id));
        return owner;
    }

    private static final class Prepared {
        final Text.Prepared value;
        final float density;
        final boolean legacy;

        Prepared(Text.Prepared value, float density, boolean legacy) {
            this.value = value;
            this.density = density;
            this.legacy = legacy;
        }
    }

    static final class Layout {
        final StaticLayout value;
        final float density;
        final int clip;
        final Text.Shadow shadow;
        final boolean legacy;
        boolean shown; // under its Surface's lock

        Layout(StaticLayout value, Prepared text) {
            this.value = value;
            density = text.density;
            clip = text.value.clipLines();
            shadow = text.value.shadow();
            legacy = text.legacy;
        }
    }

    private static final class Surface {
        volatile float density;
        final SparseArray<Prepared> prepared = new SparseArray<>(); // reader thread
        private final HashMap<Integer, Layout> layouts = new HashMap<>();

        Surface(float density) {
            this.density = density;
        }

        Prepared text(int id) throws IOException {
            Prepared text = prepared.get(id);
            if (text == null) throw new IOException("unknown prepared text " + id);
            return text;
        }

        synchronized void addLayout(int id, Layout layout) {
            layouts.put(id, layout);
        }

        synchronized Layout layout(int id) throws IOException {
            Layout layout = layouts.get(id);
            if (layout == null) throw new IOException("unknown text layout " + id);
            return layout;
        }

        synchronized Layout show(int id, boolean legacyOnly) {
            Layout layout = layouts.get(id);
            if (layout == null || legacyOnly && !layout.legacy) return null;
            layout.shown = true;
            return layout;
        }

        synchronized boolean hide(int id, boolean legacyOnly) {
            Layout layout = layouts.get(id);
            if (layout == null || !layout.shown || legacyOnly && !layout.legacy) return false;
            layouts.remove(id);
            return true;
        }

        synchronized void release(int id) {
            Layout layout = layouts.get(id);
            if (layout != null && !layout.shown) layouts.remove(id);
        }
    }
}
