package dev.oneo.nsgo;

import java.io.ByteArrayOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.util.ArrayList;
import java.util.List;

/**
 * JVM harness for WheelScroll.java, invoked by wheel_test.go.
 * Reads a script of operations from stdin and writes emitted events to stdout.
 */
public final class WheelCheck {
    private static final int OP_SCROLL = 1;
    private static final int OP_END = 2;
    private static final int OP_CANCEL = 3;

    public static void main(String[] args) throws IOException {
        DataInputStream in = new DataInputStream(System.in);
        List<byte[]> events = new ArrayList<>();
        WheelScroll tracker = new WheelScroll((nodeId, phase, x, y, pageX, pageY, dx, dy, vx, vy, touchpad, mods) -> {
            ByteArrayOutputStream bytes = new ByteArrayOutputStream();
            DataOutputStream d = new DataOutputStream(bytes);
            try {
                d.writeInt(nodeId);
                d.writeByte(phase);
                d.writeFloat(x);
                d.writeFloat(y);
                d.writeFloat(pageX);
                d.writeFloat(pageY);
                d.writeFloat(dx);
                d.writeFloat(dy);
                d.writeFloat(vx);
                d.writeFloat(vy);
                d.writeByte(touchpad ? 1 : 0);
                d.writeByte(mods);
            } catch (IOException impossible) {
                throw new AssertionError(impossible);
            }
            events.add(bytes.toByteArray());
        });

        int ops = in.readInt();
        for (int i = 0; i < ops; i++) {
            int op = in.readUnsignedByte();
            switch (op) {
                case OP_SCROLL: {
                    int nodeId = in.readInt();
                    boolean touchpad = in.readUnsignedByte() != 0;
                    float h = in.readFloat();
                    float v = in.readFloat();
                    float x = in.readFloat();
                    float y = in.readFloat();
                    float pageX = in.readFloat();
                    float pageY = in.readFloat();
                    int mods = in.readUnsignedByte();
                    long timeMs = in.readLong();
                    tracker.scroll(nodeId, touchpad, h, v, x, y, pageX, pageY, mods, timeMs);
                    break;
                }
                case OP_END:
                    tracker.end();
                    break;
                case OP_CANCEL:
                    tracker.cancel(in.readInt());
                    break;
                default:
                    throw new IOException("unknown op " + op);
            }
        }

        DataOutputStream out = new DataOutputStream(System.out);
        out.writeInt(events.size());
        for (byte[] e : events) {
            out.write(e);
        }
        out.flush();
    }
}
