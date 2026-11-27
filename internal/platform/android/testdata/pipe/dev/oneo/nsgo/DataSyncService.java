package dev.oneo.nsgo;

import android.content.Context;

/** The JVM pipe tests exercise framing without Android services. */
final class DataSyncService {
    static void message(Context context, NsgoEngine owner, byte[] data) {}
    static void ownerEnded(NsgoEngine owner) {}
}
