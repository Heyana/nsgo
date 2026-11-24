package dev.oneo.nsgo;

import java.io.BufferedInputStream;
import java.io.BufferedOutputStream;
import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.util.Arrays;

/** Runs the Android scroll and layout motion calculations on the Go test's inputs. */
public final class MotionCheck {
    public static void main(String[] args) throws IOException {
        DataInputStream in = new DataInputStream(new BufferedInputStream(System.in));
        try (DataOutputStream out = new DataOutputStream(new BufferedOutputStream(System.out))) {
            snaps(in, out);
            sticky(in, out);
            transitions(in, out);
            retargets(in, out);
        }
    }

    private static void snaps(DataInputStream in, DataOutputStream out) throws IOException {
        int cases = in.readInt();
        for (int i = 0; i < cases; i++) {
            float interval = in.readFloat();
            int alignment = in.readUnsignedByte();
            boolean paging = in.readBoolean(), oneStep = in.readBoolean();
            float[] offsets = new float[in.readInt()];
            for (int j = 0; j < offsets.length; j++) offsets[j] = in.readFloat();
            Arrays.sort(offsets); // as Scrolling.Config.read does
            float offset = in.readFloat(), velocity = in.readFloat(), rate = in.readFloat();
            float viewport = in.readFloat(), content = in.readFloat();
            float projected = ScrollPhysics.projected(offset, velocity, rate);
            out.writeDouble(projected);
            out.writeDouble(ScrollPhysics.snapTarget(interval, alignment, offsets, paging, oneStep,
                    offset, projected, velocity, viewport, content));
        }
    }

    private static void sticky(DataInputStream in, DataOutputStream out) throws IOException {
        int cases = in.readInt();
        for (int i = 0; i < cases; i++) {
            float start = in.readFloat(), end = in.readFloat(), offset = in.readFloat();
            double animatedInset = in.readDouble();
            float inset = in.readFloat();
            out.writeDouble(ScrollPhysics.stickyShift(start, end, offset, animatedInset, inset));
        }
    }

    private static void transitions(DataInputStream in, DataOutputStream out) throws IOException {
        int cases = in.readInt();
        for (int i = 0; i < cases; i++) {
            LayoutTransition transition = LayoutTransition.read(in);
            long elapsed = in.readLong();
            out.writeDouble(transition.at(elapsed));
            out.writeDouble(transition.done(elapsed) ? 1 : 0);
        }
    }

    private static float[] frame(DataInputStream in) throws IOException {
        return new float[] {in.readFloat(), in.readFloat(), in.readFloat(), in.readFloat()};
    }

    private static void retargets(DataInputStream in, DataOutputStream out) throws IOException {
        int cases = in.readInt(), steps = in.readInt();
        for (int i = 0; i < cases; i++) {
            float[] from = frame(in), to = frame(in);
            LayoutTransition transition = LayoutTransition.read(in);
            long at = in.readLong();
            float[] next = frame(in);
            LayoutTransition nextTransition = LayoutTransition.read(in);
            LayoutTransition.Motion motion = LayoutTransition.Motion.start(from, to, transition, 0);
            motion.retarget(next, nextTransition, at);
            for (int j = 0; j < steps; j++) {
                boolean done = motion.step(in.readLong());
                for (int k = 0; k < 4; k++) out.writeDouble(motion.shown[k]);
                for (int k = 0; k < 4; k++) out.writeDouble(motion.vel[k]);
                out.writeDouble(done ? 1 : 0);
            }
        }
    }
}
