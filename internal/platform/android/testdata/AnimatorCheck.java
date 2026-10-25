package dev.oneo.nsgo;

import java.io.BufferedInputStream;
import java.io.DataInputStream;
import java.io.FileInputStream;
import java.io.IOException;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

/**
 * Samples Animator on the JVM for animator_test.go: the batches of ops it
 * applies after stepping to their times, then the frame times; prints the
 * reports and each view's values after every step, as the test prints
 * core.Animator's.
 */
final class AnimatorCheck {
    public static void main(String[] args) throws IOException {
        List<String> reports = new ArrayList<>();
        Animator a = new Animator((id, value, finished) -> reports.add("report " + id + " " + value + " " + finished));
        try (DataInputStream in = new DataInputStream(new BufferedInputStream(new FileInputStream(args[0])))) {
            int count = in.readInt();
            long[] at = new long[count];
            List<List<Animator.Op>> batches = new ArrayList<>();
            for (int i = 0; i < count; i++) {
                at[i] = in.readLong();
                List<Animator.Op> ops = new ArrayList<>();
                for (int j = 0, n = in.readInt(); j < n; j++) ops.add(Animator.readOp(in));
                batches.add(ops);
            }
            int next = 0;
            StringBuilder out = new StringBuilder();
            for (int i = 0, n = in.readInt(); i < n; i++) {
                long t = in.readLong();
                a.step(t);
                while (next < count && at[next] <= t) {
                    for (Animator.Op o : batches.get(next)) a.apply(o);
                    next++;
                }
                Collections.sort(reports);
                out.append("step ").append(t).append('\n');
                for (String r : reports) out.append(r).append('\n');
                reports.clear();
                for (int view = 1; view <= 4; view++) {
                    Animator.Values v = a.values(view);
                    if (v == null) v = new Animator.Values();
                    out.append("view ").append(view).append(' ').append(v.set).append(' ').append(v.opacity)
                            .append(' ').append(v.translateX).append(' ').append(v.translateY).append(' ')
                            .append(v.scale).append(' ').append(v.scaleX).append(' ').append(v.scaleY).append(' ')
                            .append(v.rotate).append(' ').append(v.shadowRadius).append(' ')
                            .append(channels(v.background)).append(' ').append(channels(v.borderColor)).append(' ')
                            .append(channels(v.shadowColor)).append(' ').append(v.shadowOpacity).append(' ')
                            .append(v.rotateX).append(' ').append(v.rotateY).append(' ')
                            .append(channels(v.textColor)).append('\n');
                }
            }
            System.out.print(out);
        }
    }

    private static String channels(int c) {
        return (c >>> 24) + " " + ((c >>> 16) & 0xff) + " " + ((c >>> 8) & 0xff) + " " + (c & 0xff);
    }
}
