package dev.oneo.nsgo;

import java.io.DataInputStream;
import java.io.IOException;

/** A text's copy metadata, independent of its cached layout, in UTF-16 units. */
final class TextCopy {
    static final TextCopy EMPTY = new TextCopy(new int[0], new boolean[0], new String[0]);
    private final int[] ranges;
    private final boolean[] hidden;
    private final String[] copies;

    private TextCopy(int[] ranges, boolean[] hidden, String[] copies) {
        this.ranges = ranges;
        this.hidden = hidden;
        this.copies = copies;
    }

    static TextCopy read(DataInputStream in) throws IOException {
        int n = in.readInt();
        if (n < 0 || n > in.available() / 13) throw new IOException("invalid text copy span count");
        if (n == 0) return EMPTY;
        int[] ranges = new int[n * 2];
        boolean[] hidden = new boolean[n];
        String[] copies = new String[n];
        for (int i = 0; i < n; i++) {
            ranges[i * 2] = in.readInt();
            ranges[i * 2 + 1] = in.readInt();
            hidden[i] = in.readUnsignedByte() != 0;
            copies[i] = Protocol.readString(in);
        }
        return new TextCopy(ranges, hidden, copies);
    }

    /** As driver.CopiedText: omit hidden runs and replace whole attachments. */
    String text(CharSequence text, int from, int to) {
        from = Math.max(0, Math.min(from, text.length()));
        to = Math.max(from, Math.min(to, text.length()));
        if (from > 0 && from < text.length() && Character.isLowSurrogate(text.charAt(from))) from--;
        if (to > 0 && to < text.length() && Character.isLowSurrogate(text.charAt(to))) to--;
        StringBuilder out = new StringBuilder();
        int at = from;
        for (int i = 0; i < hidden.length; i++) {
            if (!hidden[i] || ranges[i * 2 + 1] <= at || ranges[i * 2] >= to) continue;
            if (ranges[i * 2] > at) append(out, text, at, ranges[i * 2]);
            at = Math.max(at, ranges[i * 2 + 1]);
        }
        if (at < to) append(out, text, at, to);
        return out.toString();
    }

    private void append(StringBuilder out, CharSequence text, int from, int to) {
        for (int at = from; at < to; at++) {
            char c = text.charAt(at);
            if (c != '\uFFFC') {
                out.append(c);
                continue;
            }
            for (int i = 0; i < copies.length; i++) {
                if (ranges[i * 2] == at && ranges[i * 2 + 1] == at + 1) {
                    out.append(copies[i]);
                    break;
                }
            }
        }
    }
}
