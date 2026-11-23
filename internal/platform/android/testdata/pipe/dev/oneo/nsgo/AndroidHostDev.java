package dev.oneo.nsgo;

import android.net.LocalSocket;
import java.io.IOException;

/** There is no development socket in the JVM reader test. */
final class AndroidHostDev {
    static void closeSocket(LocalSocket socket) {
        try { socket.close(); } catch (IOException ignored) {}
    }
}
