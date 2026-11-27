package dev.oneo.nsgo;

import android.content.Context;
import android.os.Handler;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.nio.charset.StandardCharsets;

public final class VideoAlphaCheck {
    private static void require(boolean ok, String why) {
        if (!ok) throw new AssertionError(why);
    }

    private static byte[] emptyConfig(boolean controls) throws Exception {
        ByteArrayOutputStream raw = new ByteArrayOutputStream();
        DataOutputStream out = new DataOutputStream(raw);
        // Source: empty string
        byte[] src = "".getBytes(StandardCharsets.UTF_8);
        out.writeInt(src.length);
        out.write(src);
        // Flags: paused=0, muted=1, repeat=0, controls=bit3
        out.writeByte((1 << 1) | (controls ? (1 << 3) : 0));
        out.writeFloat(1.0f); // volume
        out.writeFloat(1.0f); // rate
        out.writeByte(Protocol.RESIZE_CONTAIN);
        out.writeInt(250); // progressInterval ms
        out.writeByte(0); // audioSession
        out.writeInt(0); // headerCount
        out.flush();
        return raw.toByteArray();
    }

    public static void main(String[] args) throws Exception {
        Context context = new Context();
        Handler ui = new Handler();
        VideoNode.Reporter reporter = new VideoNode.Reporter() {
            @Override public boolean wants(int kind) { return true; }
            @Override public void report(int kind, String text, long time, long duration,
                    int width, int height, int index, boolean buffering) {}
        };
        VideoNode node = new VideoNode(context, ui, reporter);
        require(!node.isOpaque(), "VideoNode TextureView stayed opaque by default");
        require(node.setOpaqueCalls() > 0, "VideoNode did not explicitly call setOpaque(false)");

        node.configure(new DataInputStream(new ByteArrayInputStream(emptyConfig(false))));
        require(!node.isOpaque(), "VideoNode became opaque after plain configure");

        node.configure(new DataInputStream(new ByteArrayInputStream(emptyConfig(true))));
        require(!node.isOpaque(), "VideoNode became opaque after controls configure");

        node.onSizeChanged(120, 120, 0, 0);
        require(!node.isOpaque(), "VideoNode became opaque after resize");

        node.destroy();
        require(!node.isOpaque(), "VideoNode became opaque after destroy");
    }
}
