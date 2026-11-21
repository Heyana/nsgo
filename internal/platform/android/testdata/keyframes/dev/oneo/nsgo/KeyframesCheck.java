package dev.oneo.nsgo;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;

/** Reads the Go runtime's wire format and samples its cycles through Keyframes. */
public final class KeyframesCheck {
    public static void main(String[] args) throws IOException {
        DataInputStream in = new DataInputStream(new BufferedInputStream(System.in));
        try (DataOutputStream out = new DataOutputStream(new BufferedOutputStream(System.out))) {
            int cases = in.readInt();
            for (int i = 0; i < cases; i++) {
                Keyframes frames = Keyframes.read(in);
                out.writeLong(frames.duration);
                out.writeLong(frames.delay);
                out.writeInt(frames.iterations);
                out.writeInt(frames.steps.length);
                for (float[] step : frames.steps) {
                    for (float field : step) out.writeFloat(field);
                }
                int height = in.readInt();
                Look base = new Look();
                base.opacity = in.readFloat();
                base.scaleX = in.readFloat();
                base.scaleY = in.readFloat();
                base.rotation = in.readFloat();
                base.translateY = in.readFloat();
                int samples = in.readInt();
                for (int j = 0; j < samples; j++) {
                    Look sampled = frames.apply(base, in.readFloat(), height);
                    out.writeFloat(sampled.opacity);
                    out.writeFloat(sampled.scaleX);
                    out.writeFloat(sampled.scaleY);
                    out.writeFloat(sampled.rotation);
                    out.writeFloat(sampled.translateY);
                }
            }
            if (in.read() != -1) throw new IOException("unread keyframe fixture data");
        }
    }

    public static float interpolate(float x1, float y1, float x2, float y2, float x) {
        return (float) Animator.bezier(x1, y1, x2, y2, x);
    }
}
