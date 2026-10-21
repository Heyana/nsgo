package dev.oneo.nsgo;

import java.io.DataInputStream;
import java.io.DataOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;

/**
 * The pipe protocol between the runtime and the app's Go process: messages
 * of a type byte, a big-endian length and a payload. Kept in sync with
 * platform/android/android.go.
 */
final class Protocol {
    // Go to Java.
    static final int BATCH = 1;
    static final int MEASURE = 2;

    // Java to Go.
    static final int START = 1;
    static final int RESIZE = 2;
    static final int PRESS = 3;
    static final int CHANGE = 4;
    static final int SUBMIT = 5;
    static final int MEASURED = 6;

    private Protocol() {}

    static String readString(DataInputStream in) throws IOException {
        byte[] bytes = new byte[in.readInt()];
        in.readFully(bytes);
        return new String(bytes, StandardCharsets.UTF_8);
    }

    static void writeString(DataOutputStream out, String s) throws IOException {
        byte[] bytes = s.getBytes(StandardCharsets.UTF_8);
        out.writeInt(bytes.length);
        out.write(bytes);
    }
}
